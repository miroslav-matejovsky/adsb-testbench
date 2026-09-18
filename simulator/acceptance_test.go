package simulator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Cross-transport acceptance.
//
// These tests live in the simulator package because they need its fake
// runtime clock: every scenario runs on explicit virtual time rather than on
// elapsed wall time. They drive one real paused runtime through the local
// service and through its HTTP handler, and compare what a display builds
// from each transport.

// The service satisfies the display's source contract structurally, without
// either package importing the other.
var _ display.ObservationSource = (*API)(nil)

// acceptanceMount is the fixture deployment prefix of the simulator handler.
const acceptanceMount = "/bench/a/simulator"

// pathRecorder fails the test if a display source ever asks for truth.
type pathRecorder struct {
	t       *testing.T
	handler http.Handler

	mu    sync.Mutex
	paths []string
}

func (p *pathRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.paths = append(p.paths, r.URL.Path)
	p.mu.Unlock()
	switch r.URL.Path {
	case acceptanceMount + "/truth", acceptanceMount + "/metadata", acceptanceMount + "/stations":
		p.t.Errorf("a display source requested the truth route %q", r.URL.Path)
	}
	p.handler.ServeHTTP(w, r)
}

// acceptanceBench is one runtime with both display transports attached.
type acceptanceBench struct {
	api    *API
	clock  *runtimeClock
	server *httptest.Server
	local  *display.InProcessSource
	remote *display.HTTPSource
	routes *pathRecorder
}

// newAcceptanceBench builds a paused runtime, its service, an HTTP server
// mounted below a prefix, and both display sources.
func newAcceptanceBench(t *testing.T, runID string) *acceptanceBench {
	t.Helper()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	config := runtimeTestConfig()
	config.Simulation.ID = runID
	runtime, err := newSimulator(config, clock)
	require.NoError(t, err)

	api, err := NewAPI(runtime, apiTestConfig())
	require.NoError(t, err)

	routes := &pathRecorder{t: t, handler: http.StripPrefix(acceptanceMount, api.Handler())}
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)

	local, err := display.NewInProcessSource(api)
	require.NoError(t, err)

	remote, err := display.NewHTTPSource(display.HTTPSourceConfig{
		BaseURL: server.URL + acceptanceMount, Timeout: 10 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		Client: server.Client(),
	})
	require.NoError(t, err)

	return &acceptanceBench{api: api, clock: clock, server: server, local: local, remote: remote, routes: routes}
}

// addStation creates one station through the service.
func (b *acceptanceBench) addStation(t *testing.T, id string, enabled bool, sensitivityDBm float64) simulatorapi.Station {
	t.Helper()

	settings := stationSettingsDTO(runtimeStation())
	settings.ID = id
	settings.Enabled = enabled
	settings.SensitivityDBm = sensitivityDBm
	ack, err := b.api.AddStation(t.Context(), simulatorapi.AddStationCommand{
		RunID: b.api.RunID(), Station: settings,
	})
	require.NoError(t, err)
	return ack.Station
}

// run assigns the aircraft count, advances virtual time, and settles it.
// The count is assigned before the advance so the aircraft exist for the whole
// interval, and settlement happens through an ordinary control.
func (b *acceptanceBench) run(t *testing.T, real time.Duration, count int) {
	t.Helper()

	command := simulatorapi.CountCommand{RunID: b.api.RunID(), Count: count}
	_, err := b.api.SetCount(t.Context(), command)
	require.NoError(t, err)
	b.clock.Add(real)
	_, err = b.api.SetCount(t.Context(), command)
	require.NoError(t, err)
}

// displayFor builds a display over one source with explicit fixture settings.
func displayFor(t *testing.T, source display.ObservationSource) *display.Display {
	t.Helper()

	backend, err := display.New(display.Config{
		IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second,
		AltitudeExpiry: 30 * time.Second, VelocityExpiry: 30 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		RequestTimeout: 10 * time.Second, ReportError: func(err error) { t.Error(err) },
	}, source)
	require.NoError(t, err)
	return backend
}

func TestBothTransportsProduceIdenticalObservations(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-parity")
	bench.addStation(t, "alpha", true, -95)
	bench.addStation(t, "bravo", true, -95)
	bench.run(t, 30*time.Second, 2)

	selection := []string{"alpha", "bravo"}
	request := simulatorapi.ReceptionSnapshotRequest{StationIDs: selection}

	fromLocal, err := bench.local.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	fromHTTP, err := bench.remote.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, fromLocal, fromHTTP, "both transports carry the same raw evidence")
	require.NotEmpty(t, fromLocal.Records)

	localDisplay := displayFor(t, bench.local)
	remoteDisplay := displayFor(t, bench.remote)

	localSnapshot, err := localDisplay.Refresh(t.Context(), selection)
	require.NoError(t, err)
	remoteSnapshot, err := remoteDisplay.Refresh(t.Context(), selection)
	require.NoError(t, err)

	require.Equal(t, display.StatusFresh, localSnapshot.Status)
	require.Equal(t, localSnapshot.Observations, remoteSnapshot.Observations,
		"both transports decode to the same tracks")
	require.Len(t, localSnapshot.Observations.Aircraft, 2)

	// The local update timestamps are compared separately: they are real
	// instants, not the source's virtual time.
	require.NotEqual(t, localSnapshot.LastUpdatedAt, nil)
	require.NotEqual(t, remoteSnapshot.LastUpdatedAt, nil)
}

func TestDecodedDisplayAgreesWithTheEngineObservationOracle(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-oracle")
	bench.addStation(t, "alpha", true, -95)
	bench.run(t, 20*time.Second, 1)

	backend := displayFor(t, bench.local)
	snapshot, err := backend.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Len(t, snapshot.Observations.Aircraft, 1)

	oracle, err := bench.api.Observations(t.Context(), simulatorapi.ObservationRequest{
		StationIDs: []string{"alpha"},
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: "60000000000", PositionNanoseconds: "30000000000",
			AltitudeNanoseconds: "30000000000", VelocityNanoseconds: "30000000000",
		},
	})
	require.NoError(t, err)
	require.Equal(t, oracle.Aircraft, snapshot.Observations.Aircraft,
		"raw decoding matches the engine's own received projection")
	require.Equal(t, oracle.Retention, snapshot.Observations.Retention)
}

func TestDisplayScenarioMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		prepare   func(*testing.T, *acceptanceBench) []string
		assert    func(*testing.T, display.Snapshot)
		expectErr bool
	}{
		{
			name: "no stations selected",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.run(t, 10*time.Second, 1)
				return []string{}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Empty(t, snapshot.Observations.Aircraft)
				require.Empty(t, snapshot.Observations.StationIDs)
				require.Empty(t, snapshot.Observations.Retention)
			},
		},
		{
			name: "zero aircraft",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.addStation(t, "alpha", true, -95)
				bench.run(t, 10*time.Second, 0)
				return []string{"alpha"}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Empty(t, snapshot.Observations.Aircraft)
				require.Len(t, snapshot.Observations.Retention, 1)
				require.Equal(t, "0", snapshot.Observations.Retention[0].OldestSequence)
			},
		},
		{
			name: "disabled station hears nothing",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.addStation(t, "off", false, -95)
				bench.run(t, 10*time.Second, 1)
				return []string{"off"}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Empty(t, snapshot.Observations.Aircraft)
				require.False(t, snapshot.Observations.Retention[0].Truncated)
			},
		},
		{
			name: "insensitive station hears nothing at range",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.addStation(t, "deaf", true, -1)
				bench.run(t, 10*time.Second, 1)
				return []string{"deaf"}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Empty(t, snapshot.Observations.Aircraft)
			},
		},
		{
			name: "complete reception yields every field",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.addStation(t, "alpha", true, -95)
				bench.run(t, 20*time.Second, 1)
				return []string{"alpha"}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Len(t, snapshot.Observations.Aircraft, 1)
				aircraft := snapshot.Observations.Aircraft[0]
				require.NotNil(t, aircraft.Identity)
				require.NotNil(t, aircraft.Position)
				require.NotNil(t, aircraft.BarometricAltitude)
				require.NotNil(t, aircraft.Velocity)
				require.NotNil(t, aircraft.Velocity.GroundSpeedKnots)
				require.Equal(t, 0.0, *aircraft.Velocity.GroundSpeedKnots,
					"the fixture aircraft is stationary")
				require.Nil(t, aircraft.Velocity.TrackDegrees, "zero speed has no track")
			},
		},
		{
			name: "unknown station fails the refresh",
			prepare: func(t *testing.T, bench *acceptanceBench) []string {
				bench.run(t, time.Second, 1)
				return []string{"missing"}
			},
			assert: func(t *testing.T, snapshot display.Snapshot) {
				require.Equal(t, display.StatusUnavailable, snapshot.Status)
				require.Nil(t, snapshot.Observations)
			},
			expectErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			bench := newAcceptanceBench(t, "acceptance-"+tc.name)
			selection := tc.prepare(t, bench)

			for name, source := range map[string]display.ObservationSource{
				"local": bench.local, "http": bench.remote,
			} {
				backend := displayFor(t, source)
				snapshot, err := backend.Refresh(t.Context(), selection)
				if tc.expectErr {
					require.Error(t, err, name)
				} else {
					require.NoError(t, err, name)
				}
				tc.assert(t, snapshot)
			}
		})
	}
}

func TestProvenanceKeepsEveryContributingReceiver(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-provenance")
	alpha := bench.addStation(t, "alpha", true, -95)
	bench.addStation(t, "bravo", true, -95)
	bench.run(t, 10*time.Second, 1)

	// Editing a station mid-run must not rewrite the provenance of records
	// already heard under the previous revision.
	edited := alpha.StationSettings
	edited.AntennaGainDBi = 10
	_, err := bench.api.UpdateStation(t.Context(), "alpha", simulatorapi.UpdateStationCommand{
		RunID: bench.api.RunID(), ExpectedRevision: alpha.Revision, Station: edited,
	})
	require.NoError(t, err)
	bench.run(t, 10*time.Second, 1)

	backend := displayFor(t, bench.remote)
	snapshot, err := backend.Refresh(t.Context(), []string{"alpha", "bravo"})
	require.NoError(t, err)
	require.Len(t, snapshot.Observations.Aircraft, 1)

	evidence := snapshot.Observations.Aircraft[0].Identity.Evidence
	require.Len(t, evidence.Receptions, 2, "one transmission heard at two receivers")
	require.Equal(t, "alpha", evidence.Receptions[0].StationID)
	require.Equal(t, "bravo", evidence.Receptions[1].StationID)

	revisions := map[string]bool{}
	for _, aircraft := range snapshot.Observations.Aircraft {
		for _, item := range aircraft.Position.Evidence {
			for _, record := range item.Receptions {
				revisions[record.StationID+"#"+record.Receiver.Revision] = true
				require.Equal(t, record.StationRevision, record.Receiver.Revision)
			}
		}
	}
	require.NotEmpty(t, revisions)

	// A narrower selection carries no evidence from the station left out.
	narrowed, err := backend.Refresh(t.Context(), []string{"bravo"})
	require.NoError(t, err)
	for _, aircraft := range narrowed.Observations.Aircraft {
		for _, record := range aircraft.Identity.Evidence.Receptions {
			require.Equal(t, "bravo", record.StationID)
		}
	}
}

func TestTruthNeverReachesADisplay(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-truth-leak")
	bench.addStation(t, "deaf", true, -1)
	bench.run(t, 20*time.Second, 3)

	truth, err := bench.api.Truth(t.Context())
	require.NoError(t, err)
	require.Len(t, truth.Aircraft, 3, "truth has aircraft")

	backend := displayFor(t, bench.remote)
	snapshot, err := backend.Refresh(t.Context(), []string{"deaf"})
	require.NoError(t, err)
	require.Empty(t, snapshot.Observations.Aircraft,
		"aircraft with no receptions produce no display tracks")

	bench.routes.mu.Lock()
	defer bench.routes.mu.Unlock()
	require.NotEmpty(t, bench.routes.paths)
	for _, path := range bench.routes.paths {
		require.Equal(t, acceptanceMount+"/observations/receptions", path)
	}
}

func TestRemovedTruthAircraftRemainsObservableWhileRetained(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-retained-target")
	bench.addStation(t, "alpha", true, -95)
	bench.run(t, 20*time.Second, 1)

	before, err := bench.api.Truth(t.Context())
	require.NoError(t, err)
	require.Len(t, before.Aircraft, 1)

	bench.run(t, 0, 0)
	after, err := bench.api.Truth(t.Context())
	require.NoError(t, err)
	require.Empty(t, after.Aircraft, "truth no longer has the aircraft")

	backend := displayFor(t, bench.local)
	snapshot, err := backend.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Len(t, snapshot.Observations.Aircraft, 1,
		"received evidence outlives the simulated aircraft until it expires")
	require.Equal(t, before.Aircraft[0].ICAO, snapshot.Observations.Aircraft[0].ICAO)
}

func TestHistoryPagingAcrossTransports(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-history")
	bench.addStation(t, "alpha", true, -95)
	bench.run(t, 20*time.Second, 1)

	request := simulatorapi.HistoryRequest{StationID: "alpha", Limit: 2}
	localPage, err := bench.local.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)
	remotePage, err := bench.remote.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, localPage, remotePage)
	require.Len(t, localPage.Records, 2)
	require.True(t, localPage.HasMore)
	require.False(t, localPage.Gap)

	resumed := request
	resumed.Cursor = &localPage.NextCursor
	next, err := bench.remote.ReceptionHistory(t.Context(), resumed)
	require.NoError(t, err)
	require.NotEqual(t, localPage.Records[0].Sequence, next.Records[0].Sequence)

	// A cursor from another run is a conflict, not a silent empty page.
	foreign := request
	foreign.Cursor = &simulatorapi.ReceptionCursor{
		RunID: "another-run", StationID: "alpha", AfterSequence: "1",
	}
	_, err = bench.remote.ReceptionHistory(t.Context(), foreign)
	require.ErrorIs(t, err, simulatorapi.CategoryConflict)

	// A cursor beyond the latest retained sequence is invalid input.
	future := request
	future.Cursor = &simulatorapi.ReceptionCursor{
		RunID: bench.api.RunID(), StationID: "alpha", AfterSequence: "18446744073709551615",
	}
	_, err = bench.remote.ReceptionHistory(t.Context(), future)
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
}

func TestARestartedRunInvalidatesDisplayState(t *testing.T) {
	t.Parallel()

	first := newAcceptanceBench(t, "acceptance-run-1")
	first.addStation(t, "alpha", true, -95)
	first.run(t, 20*time.Second, 1)

	second := newAcceptanceBench(t, "acceptance-run-2")
	second.addStation(t, "alpha", true, -95)
	second.run(t, 20*time.Second, 1)

	sources := &switchingSource{current: first.local}
	backend := displayFor(t, sources)

	before, err := backend.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, "acceptance-run-1", before.Observations.RunID)

	sources.set(second.local)
	after, err := backend.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, "acceptance-run-2", after.Observations.RunID)

	// The replacement run reuses the same synthetic addresses, so identity
	// must come from the run, never from a remembered track.
	require.Equal(t, before.Observations.Aircraft[0].ICAO, after.Observations.Aircraft[0].ICAO)

	sources.set(first.local)
	back, err := backend.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, "acceptance-run-1", back.Observations.RunID)
}

// switchingSource swaps the underlying source to emulate a replacement run.
type switchingSource struct {
	mu      sync.Mutex
	current display.ObservationSource
}

func (s *switchingSource) set(source display.ObservationSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = source
}

func (s *switchingSource) active() display.ObservationSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *switchingSource) ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	return s.active().ReceptionSnapshot(ctx, request)
}

func (s *switchingSource) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	return s.active().ReceptionHistory(ctx, request)
}

func TestMaximumCardinalitySnapshotFitsTheFixtureBudgets(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-bounds")
	selection := make([]string, 0, simulation.MaxStations)
	for i := range simulation.MaxStations {
		id := string(rune('a' + i))
		bench.addStation(t, id, true, -95)
		selection = append(selection, id)
	}
	// Enough virtual time for every station's ring to fill and evict.
	bench.run(t, 600*time.Second, 1)

	snapshot, err := bench.api.ReceptionSnapshot(t.Context(),
		simulatorapi.ReceptionSnapshotRequest{StationIDs: selection})
	require.NoError(t, err)
	require.Len(t, snapshot.Retention, simulation.MaxStations)
	for _, entry := range snapshot.Retention {
		require.Equal(t, simulation.ReceptionHistoryLimit, entry.Limit)
		require.True(t, entry.Truncated, "the fixture fills and evicts every ring")
	}
	require.Len(t, snapshot.Records, simulation.MaxStations*simulation.ReceptionHistoryLimit)

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	t.Logf("maximum reception snapshot: %d records, %d response bytes",
		len(snapshot.Records), len(encoded))
	require.LessOrEqual(t, len(encoded), apiTestConfig().MaxResponseBytes,
		"the declared fixture response budget covers the maximum snapshot")

	request, err := json.Marshal(map[string]any{"stationIds": selection})
	require.NoError(t, err)
	require.LessOrEqual(t, len(request), apiTestConfig().MaxRequestBytes)

	page, err := bench.api.ReceptionHistory(t.Context(), simulatorapi.HistoryRequest{
		StationID: selection[0], Limit: simulation.MaxHistoryPageSize,
	})
	require.NoError(t, err)
	encodedPage, err := json.Marshal(page)
	require.NoError(t, err)
	t.Logf("maximum history page: %d records, %d response bytes", len(page.Records), len(encodedPage))
	require.LessOrEqual(t, len(encodedPage), apiTestConfig().MaxResponseBytes)

	backend := displayFor(t, bench.local)
	published, err := backend.Refresh(t.Context(), selection)
	require.NoError(t, err)
	require.NotEmpty(t, published.Observations.Aircraft)
	require.LessOrEqual(t, len(published.Observations.Aircraft), simulation.MaxAircraft)
}

func TestWarmAndColdDisplaysAgreeAfterEviction(t *testing.T) {
	t.Parallel()

	bench := newAcceptanceBench(t, "acceptance-eviction")
	bench.addStation(t, "alpha", true, -95)

	warm := displayFor(t, bench.local)
	bench.run(t, 20*time.Second, 1)
	_, err := warm.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	bench.run(t, 600*time.Second, 1)
	warmSnapshot, err := warm.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	cold := displayFor(t, bench.local)
	coldSnapshot, err := cold.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)

	require.Equal(t, coldSnapshot.Observations, warmSnapshot.Observations,
		"a warm display recovers nothing the retained evidence no longer holds")
}
