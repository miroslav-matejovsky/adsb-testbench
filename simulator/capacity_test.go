package simulator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Response capacity through the real runtime and both transports.
//
// These tests run maximum-shape engine states on the fake-clock runtime and
// serve them with the response budgets of the shipped example
// configurations, read from the configuration files themselves. The full CLI
// parse of those files is tested in internal/cli.

// shippedBudgets are the response budgets of the example configurations.
type shippedBudgets struct {
	simulatorAPI int
	source       int
	display      int
}

func readShippedBudgets(t *testing.T) shippedBudgets {
	t.Helper()

	type bytesSetting struct {
		MaxResponseBytes int `json:"maxResponseBytes"`
	}
	type file struct {
		SimulatorAPI *bytesSetting `json:"simulatorApi"`
		Display      *bytesSetting `json:"display"`
		Source       *bytesSetting `json:"source"`
	}
	read := func(name string) file {
		data, err := os.ReadFile("../configs/" + name)
		require.NoError(t, err)
		var parsed file
		require.NoError(t, json.Unmarshal(data, &parsed))
		return parsed
	}
	combined, simulatorFile, displayFile := read("combined.json"), read("simulator.json"), read("display.json")
	require.Equal(t, combined.SimulatorAPI.MaxResponseBytes, simulatorFile.SimulatorAPI.MaxResponseBytes)
	require.Equal(t, combined.Display.MaxResponseBytes, displayFile.Display.MaxResponseBytes)
	return shippedBudgets{
		simulatorAPI: combined.SimulatorAPI.MaxResponseBytes,
		source:       displayFile.Source.MaxResponseBytes,
		display:      displayFile.Display.MaxResponseBytes,
	}
}

// capacityBench is a paused runtime served with explicit response budgets.
type capacityBench struct {
	api     *API
	clock   *runtimeClock
	handler http.Handler
	local   *display.InProcessSource
	remote  *display.HTTPSource
}

func newCapacityBench(t *testing.T, runID string, budgets shippedBudgets) *capacityBench {
	t.Helper()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	config := runtimeTestConfig()
	config.Simulation.ID = runID
	runtime, err := newSimulator(config, clock)
	require.NoError(t, err)
	apiConfig := apiTestConfig()
	apiConfig.MaxResponseBytes = budgets.simulatorAPI
	api, err := NewAPI(runtime, apiConfig)
	require.NoError(t, err)

	handler := http.StripPrefix(acceptanceMount, api.Handler())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	local, err := display.NewInProcessSource(api)
	require.NoError(t, err)
	remote, err := display.NewHTTPSource(display.HTTPSourceConfig{
		BaseURL: server.URL + acceptanceMount, Timeout: 10 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: budgets.source, Client: server.Client(),
	})
	require.NoError(t, err)
	return &capacityBench{api: api, clock: clock, handler: handler, local: local, remote: remote}
}

// serve serves one relative route through the mounted handler and returns
// its complete body.
func (b *capacityBench) serve(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(data))
	}
	request := httptest.NewRequest(method, acceptanceMount+path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	b.handler.ServeHTTP(recorder, request)
	return recorder
}

// capacityStationIDs returns eight distinct 64-byte station identifiers.
func capacityStationIDs() []string {
	ids := make([]string, 0, simulation.MaxStations)
	for i := range simulation.MaxStations {
		ids = append(ids, string(rune('a'+i))+strings.Repeat("Z", 63))
	}
	return ids
}

// fillMaximum adds eight co-located all-receiving stations before any
// aircraft exists, then runs the fleet until every ring has wrapped.
func (b *capacityBench) fillMaximum(t *testing.T, count int, run time.Duration) {
	t.Helper()

	for _, id := range capacityStationIDs() {
		settings := stationSettingsDTO(runtimeStation())
		settings.ID = id
		_, err := b.api.AddStation(t.Context(), simulatorapi.AddStationCommand{RunID: b.api.RunID(), Station: settings})
		require.NoError(t, err)
	}
	command := simulatorapi.CountCommand{RunID: b.api.RunID(), Count: count}
	_, err := b.api.SetCount(t.Context(), command)
	require.NoError(t, err)
	b.clock.Add(run)
	_, err = b.api.SetCount(t.Context(), command)
	require.NoError(t, err)
}

// capacityRunID is an effective run ID of exactly 128 UTF-8 bytes whose
// characters need JSON escaping or multibyte encoding.
func capacityRunID() string {
	return strings.Repeat("<", 64) + strings.Repeat("é", 32)
}

// provenanceCopies counts the reception copies carried as evidence.
func provenanceCopies(observations simulatorapi.ObservationSnapshot) int {
	copies := 0
	for _, aircraft := range observations.Aircraft {
		if aircraft.Identity != nil {
			copies += len(aircraft.Identity.Evidence.Receptions)
		}
		if aircraft.Position != nil {
			for _, item := range aircraft.Position.Evidence {
				copies += len(item.Receptions)
			}
		}
		if aircraft.BarometricAltitude != nil {
			copies += len(aircraft.BarometricAltitude.Evidence.Receptions)
		}
		if aircraft.Velocity != nil {
			copies += len(aircraft.Velocity.Evidence.Receptions)
		}
	}
	return copies
}

func TestCapacityShippedBudgetsCarryMaximumEngineShapes(t *testing.T) {
	t.Parallel()

	capacity, err := qualitytest.LoadCapacity("../testdata/quality/capacity.json")
	require.NoError(t, err)
	budgets := readShippedBudgets(t)
	require.Len(t, capacityRunID(), capacity.Profile.MaxRunIDBytes)

	for _, fleet := range []struct {
		count int
		run   time.Duration
	}{{1, 300 * time.Second}, {simulation.MaxAircraft, 20 * time.Second}} {
		t.Run(fmt.Sprintf("%d aircraft", fleet.count), func(t *testing.T) {
			t.Parallel()

			bench := newCapacityBench(t, capacityRunID(), budgets)
			bench.fillMaximum(t, fleet.count, fleet.run)
			ids := capacityStationIDs()

			full, err := bench.api.ReceptionSnapshot(t.Context(), simulatorapi.ReceptionSnapshotRequest{StationIDs: ids})
			require.NoError(t, err)
			for _, entry := range full.Retention {
				oldest, err := simulatorapi.ParseUint64(entry.OldestSequence)
				require.NoError(t, err)
				latest, err := simulatorapi.ParseUint64(entry.LatestSequence)
				require.NoError(t, err)
				require.Equal(t, uint64(simulation.ReceptionHistoryLimit), latest-oldest+1, "every ring is full")
				require.True(t, entry.Truncated, "every ring wrapped and evicted")
			}
			require.Len(t, full.Records, simulation.MaxStations*simulation.ReceptionHistoryLimit)

			for _, selected := range []int{0, 1, 2, simulation.MaxStations} {
				selection := ids[:selected]
				request := simulatorapi.ReceptionSnapshotRequest{StationIDs: selection}
				raw := bench.serve(t, http.MethodPost, "/observations/receptions", request)
				require.Equal(t, http.StatusOK, raw.Code, "raw snapshot of %d stations", selected)
				require.LessOrEqual(t, raw.Body.Len(), capacity.Bounds["rawSnapshotBytes"])

				fromLocal, err := bench.local.ReceptionSnapshot(t.Context(), request)
				require.NoError(t, err)
				fromHTTP, err := bench.remote.ReceptionSnapshot(t.Context(), request)
				require.NoError(t, err, "the shipped source budget reads %d stations", selected)
				require.Equal(t, fromLocal, fromHTTP)

				observed := bench.serve(t, http.MethodPost, "/observations", simulatorapi.ObservationRequest{
					StationIDs: selection, Expiry: simulatorapi.ObservationExpiry{
						IdentityNanoseconds: "60000000000", PositionNanoseconds: "30000000000",
						AltitudeNanoseconds: "30000000000", VelocityNanoseconds: "30000000000",
					},
				})
				require.Equal(t, http.StatusOK, observed.Code)
				require.LessOrEqual(t, observed.Body.Len(), capacity.Bounds["observationSnapshotBytes"])

				// Displays over both transports agree and fit the display budget.
				published := map[string]display.Snapshot{}
				for name, source := range map[string]display.ObservationSource{"local": bench.local, "http": bench.remote} {
					backend, err := display.New(display.Config{
						IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second,
						AltitudeExpiry: 30 * time.Second, VelocityExpiry: 30 * time.Second,
						MaxRequestBytes: 65536, MaxResponseBytes: budgets.display,
						RequestTimeout: 10 * time.Second, ReportError: func(err error) { t.Error(err) },
					}, source, noStations{})
					require.NoError(t, err)
					recorder := httptest.NewRecorder()
					data, err := json.Marshal(map[string][]string{"stationIds": selection})
					require.NoError(t, err)
					post := httptest.NewRequest(http.MethodPost, "/observations", strings.NewReader(string(data)))
					post.Header.Set("Content-Type", "application/json")
					backend.Handler().ServeHTTP(recorder, post)
					require.Equal(t, http.StatusOK, recorder.Code, name)
					require.LessOrEqual(t, recorder.Body.Len(), budgets.display)
					var response display.RefreshResponse
					require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
					published[name] = response.Snapshot
					if name == "http" {
						t.Logf("%d aircraft, %d selected stations: %d records, %d distinct aircraft, %d provenance copies; "+
							"raw %d bytes, simulator observations %d bytes, display refresh %d bytes",
							fleet.count, selected, len(fromHTTP.Records), len(response.Snapshot.Observations.Aircraft),
							provenanceCopies(*response.Snapshot.Observations), raw.Body.Len(), observed.Body.Len(),
							recorder.Body.Len())
					}
				}
				// Separate displays have separate local update clocks; only the
				// diagnostic real instant may differ.
				require.Equal(t, published["local"].Observations, published["http"].Observations)
				if selected == simulation.MaxStations && fleet.count == 1 {
					require.Greater(t, raw.Body.Len(), 4194304,
						"Q1: the former 4 MiB example budget cannot carry an eight-station snapshot")
				}
			}

			page := bench.serve(t, http.MethodPost, "/receptions/history",
				simulatorapi.HistoryRequest{StationID: ids[0], Limit: simulation.MaxHistoryPageSize})
			require.Equal(t, http.StatusOK, page.Code)
			require.LessOrEqual(t, page.Body.Len(), capacity.Bounds["historyPageBytes"])
			for _, route := range []struct {
				path  string
				bound string
			}{{"/truth", "truthBytes"}, {"/metadata", "metadataBytes"}, {"/stations", "stationsBytes"}} {
				recorder := bench.serve(t, http.MethodGet, route.path, nil)
				require.Equal(t, http.StatusOK, recorder.Code, route.path)
				require.LessOrEqual(t, recorder.Body.Len(), capacity.Bounds[route.bound], route.path)
				t.Logf("%d aircraft: %s %d bytes", fleet.count, route.path, recorder.Body.Len())
			}
			t.Logf("%d aircraft: history page %d bytes", fleet.count, page.Body.Len())
		})
	}
}

// TestCapacityRetainedAddressesExceedTheFleet replaces the whole fleet
// without advancing time, so retained evidence names more distinct aircraft
// than the engine may ever have active at once.
func TestCapacityRetainedAddressesExceedTheFleet(t *testing.T) {
	t.Parallel()

	budgets := readShippedBudgets(t)
	bench := newCapacityBench(t, "capacity-churn", budgets)
	settings := stationSettingsDTO(runtimeStation())
	_, err := bench.api.AddStation(t.Context(), simulatorapi.AddStationCommand{RunID: bench.api.RunID(), Station: settings})
	require.NoError(t, err)
	for _, count := range []int{simulation.MaxAircraft, 0, simulation.MaxAircraft, 0, simulation.MaxAircraft, 0, simulation.MaxAircraft} {
		_, err := bench.api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: bench.api.RunID(), Count: count})
		require.NoError(t, err)
	}

	selection := []string{settings.ID}
	for name, source := range map[string]display.ObservationSource{"local": bench.local, "http": bench.remote} {
		backend, err := display.New(display.Config{
			IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second,
			AltitudeExpiry: 30 * time.Second, VelocityExpiry: 30 * time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: budgets.display,
			RequestTimeout: 10 * time.Second, ReportError: func(err error) { t.Error(err) },
		}, source, noStations{})
		require.NoError(t, err)
		snapshot, err := backend.Refresh(t.Context(), selection)
		require.NoError(t, err, name)
		require.Greater(t, len(snapshot.Observations.Aircraft), simulation.MaxAircraft,
			"%s: received tracks are not bounded by the active fleet size", name)
		t.Logf("%s: %d retained tracks from churned fleets", name, len(snapshot.Observations.Aircraft))
	}
}

// TestCapacitySimulatorResponseLimitIsExact serves one valid truth snapshot
// at exactly its own size and one byte below it.
func TestCapacitySimulatorResponseLimitIsExact(t *testing.T) {
	t.Parallel()

	budgets := readShippedBudgets(t)
	measure := newCapacityBench(t, "capacity-exact", budgets)
	measure.fillMaximum(t, 3, 5*time.Second)
	body := measure.serve(t, http.MethodGet, "/truth", nil)
	require.Equal(t, http.StatusOK, body.Code)
	size := body.Body.Len()
	require.Greater(t, size, simulatorapi.MinResponseBytes)

	for _, tc := range []struct {
		limit int
		ok    bool
	}{{size, true}, {size - 1, false}} {
		bench := newCapacityBench(t, "capacity-exact", shippedBudgets{
			simulatorAPI: tc.limit, source: budgets.source, display: budgets.display,
		})
		bench.fillMaximum(t, 3, 5*time.Second)
		recorder := bench.serve(t, http.MethodGet, "/truth", nil)
		if tc.ok {
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, size, recorder.Body.Len())
			require.Equal(t, body.Body.String(), recorder.Body.String())
			continue
		}
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		var failure simulatorapi.ErrorResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &failure), "a complete error envelope")
		require.Equal(t, simulatorapi.CategoryResponseLimit, failure.Error.Code)
		require.NotContains(t, recorder.Body.String(), `"aircraft"`, "no partial success body")
	}
}

// TestCapacityLatencyReport measures complete request latency over loopback
// for the declared caller counts. It runs only when ADSB_CAPACITY_MEASURE=1,
// which the capacity task sets: the measurement uses real time, so it is a
// report, never a correctness gate.
func TestCapacityLatencyReport(t *testing.T) {
	if os.Getenv("ADSB_CAPACITY_MEASURE") != "1" {
		t.Skip("set ADSB_CAPACITY_MEASURE=1 to run the latency report")
	}

	const warmup, measured = 10, 100
	capacity, err := qualitytest.LoadCapacity("../testdata/quality/capacity.json")
	require.NoError(t, err)
	workload, err := capacity.Workload("contention")
	require.NoError(t, err)
	budgets := readShippedBudgets(t)

	bench := newCapacityBench(t, capacityRunID(), budgets)
	bench.fillMaximum(t, workload.AircraftCounts[0], 60*time.Second)
	server := httptest.NewServer(bench.handler)
	t.Cleanup(server.Close)
	body, err := json.Marshal(simulatorapi.ReceptionSnapshotRequest{StationIDs: capacityStationIDs()})
	require.NoError(t, err)

	request := func() (time.Duration, error) {
		start := time.Now()
		response, err := server.Client().Post(server.URL+acceptanceMount+"/observations/receptions",
			"application/json", strings.NewReader(string(body)))
		if err != nil {
			return time.Since(start), err
		}
		defer func() { _ = response.Body.Close() }()
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			return time.Since(start), err
		}
		if response.StatusCode != http.StatusOK {
			return time.Since(start), fmt.Errorf("status %d", response.StatusCode)
		}
		return time.Since(start), nil
	}
	for range warmup {
		_, err := request()
		require.NoError(t, err)
	}

	for _, callers := range workload.Callers {
		work := make(chan int, measured)
		for i := range measured {
			work <- i
		}
		close(work)
		samples := make([]time.Duration, measured)
		failures := make([]error, measured)
		var group sync.WaitGroup
		for range callers {
			group.Add(1)
			go func() {
				defer group.Done()
				for index := range work {
					samples[index], failures[index] = request()
				}
			}()
		}
		group.Wait()

		errorCount := 0
		for _, err := range failures {
			if err != nil {
				errorCount++
				t.Logf("callers %d: request error: %v", callers, err)
			}
		}
		sorted := slices.Clone(samples)
		slices.Sort(sorted)
		median := sorted[len(sorted)/2]
		p95 := sorted[(len(sorted)*95+99)/100-1]
		t.Logf("callers %d: %d requests, median %s, p95 %s, max %s, errors %d",
			callers, measured, median, p95, sorted[len(sorted)-1], errorCount)
		require.Zero(t, errorCount, "every measured request must complete")
	}
}
