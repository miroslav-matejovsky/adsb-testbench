package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/capacitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Capacity benchmarks of the simulator service and its transports.
//
// Raw capture, DTO conversion, serialization, in-memory transport and
// loopback transport are separate benchmarks, so a report can tell them
// apart. The in-memory cases measure request and response processing without
// a socket; only the loopback cases include network overhead.

const benchmarkCapacityFile = "../testdata/quality/capacity.json"

// benchmarkService is a paused runtime with full reception rings.
type benchmarkService struct {
	api     *API
	handler http.Handler
	ids     []string
}

// newBenchmarkService builds a runtime from a workload, fills every station
// ring to the workload target, and returns the service and its handler.
func newBenchmarkService(b *testing.B, workload qualitytest.CapacityWorkload, aircraft, stations int) *benchmarkService {
	b.Helper()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	config := Config{Simulation: capacitytest.EngineConfig(workload, "capacity-"+workload.Name, 0, 100)}
	runtime, err := newSimulator(config, clock)
	if err != nil {
		b.Fatal(err)
	}
	capacity := capacitytest.Load(b, benchmarkCapacityFile)
	apiConfig := apiTestConfig()
	apiConfig.MaxResponseBytes = capacity.Budgets.SimulatorAPIMaxResponseBytes
	api, err := NewAPI(runtime, apiConfig)
	if err != nil {
		b.Fatal(err)
	}
	ids := capacitytest.StationIDs(stations)
	for _, id := range ids {
		settings := stationSettingsDTO(capacitytest.StationConfig(workload.Station, id))
		if _, err := api.AddStation(context.Background(), simulatorapi.AddStationCommand{
			RunID: api.RunID(), Station: settings,
		}); err != nil {
			b.Fatal(err)
		}
	}
	command := simulatorapi.CountCommand{RunID: api.RunID(), Count: aircraft}
	if _, err := api.SetCount(context.Background(), command); err != nil {
		b.Fatal(err)
	}
	service := &benchmarkService{api: api, handler: http.StripPrefix(acceptanceMount, api.Handler()), ids: ids}
	service.fill(b, clock, command, workload.RetentionFillTarget)
	return service
}

// fill advances the runtime until every station holds the target number of
// records, and fails if the target is unreachable.
func (s *benchmarkService) fill(b *testing.B, clock *runtimeClock, command simulatorapi.CountCommand, target int) {
	b.Helper()

	if target == 0 || len(s.ids) == 0 {
		return
	}
	for range 100 {
		snapshot, err := s.api.ReceptionSnapshot(context.Background(),
			simulatorapi.ReceptionSnapshotRequest{StationIDs: s.ids})
		if err != nil {
			b.Fatal(err)
		}
		filled := len(snapshot.Retention) == len(s.ids)
		for _, entry := range snapshot.Retention {
			oldest, err := simulatorapi.ParseUint64(entry.OldestSequence)
			if err != nil {
				b.Fatal(err)
			}
			latest, err := simulatorapi.ParseUint64(entry.LatestSequence)
			if err != nil {
				b.Fatal(err)
			}
			filled = filled && latest-oldest+1 >= uint64(target)
		}
		if filled {
			return
		}
		clock.Add(60 * time.Second)
		if _, err := s.api.SetCount(context.Background(), command); err != nil {
			b.Fatal(err)
		}
	}
	b.Fatalf("retention target %d was not reached", target)
}

// readRefreshCases enumerates the fleet and selection dimensions.
func readRefreshCases(b *testing.B) (qualitytest.CapacityWorkload, func(func(name string, aircraft, stations int))) {
	b.Helper()

	workload := capacitytest.Workload(b, benchmarkCapacityFile, "read-refresh")
	return workload, func(run func(string, int, int)) {
		for _, aircraft := range workload.AircraftCounts {
			for _, stations := range workload.StationCounts {
				run(fmt.Sprintf("aircraft-%d/stations-%d", aircraft, stations), aircraft, stations)
			}
		}
	}
}

// BenchmarkCapacityReceptionSnapshot measures raw capture and DTO
// conversion inside the service, without any transport.
func BenchmarkCapacityReceptionSnapshot(b *testing.B) {
	workload, cases := readRefreshCases(b)
	cases(func(name string, aircraft, stations int) {
		b.Run(name, func(b *testing.B) {
			service := newBenchmarkService(b, workload, aircraft, stations)
			request := simulatorapi.ReceptionSnapshotRequest{StationIDs: service.ids}
			records := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				snapshot, err := service.api.ReceptionSnapshot(context.Background(), request)
				if err != nil {
					b.Fatal(err)
				}
				records += len(snapshot.Records)
			}
			b.StopTimer()
			b.ReportMetric(float64(records)/float64(b.N), "records/op")
		})
	})
}

// BenchmarkCapacityReceptionSnapshotJSON measures serialization of an
// already captured snapshot.
func BenchmarkCapacityReceptionSnapshotJSON(b *testing.B) {
	workload, cases := readRefreshCases(b)
	cases(func(name string, aircraft, stations int) {
		b.Run(name, func(b *testing.B) {
			service := newBenchmarkService(b, workload, aircraft, stations)
			snapshot, err := service.api.ReceptionSnapshot(context.Background(),
				simulatorapi.ReceptionSnapshotRequest{StationIDs: service.ids})
			if err != nil {
				b.Fatal(err)
			}
			bytes := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				encoded, err := json.Marshal(snapshot)
				if err != nil {
					b.Fatal(err)
				}
				bytes += len(encoded)
			}
			b.StopTimer()
			b.ReportMetric(float64(bytes)/float64(b.N), "bytes/op")
		})
	})
}

// handlerTransport calls a handler directly, so a source measures request
// and response processing without a socket.
type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

// BenchmarkCapacitySimulatorHTTP measures one complete raw snapshot request
// through the real handler: in memory, and over a loopback socket.
func BenchmarkCapacitySimulatorHTTP(b *testing.B) {
	workload, cases := readRefreshCases(b)
	capacity := capacitytest.Load(b, benchmarkCapacityFile)
	cases(func(name string, aircraft, stations int) {
		service := newBenchmarkService(b, workload, aircraft, stations)
		server := httptest.NewServer(service.handler)
		b.Cleanup(server.Close)

		sources := map[string]func() *display.HTTPSource{
			"in-memory": func() *display.HTTPSource {
				source, err := display.NewHTTPSource(display.HTTPSourceConfig{
					BaseURL: "http://simulator.test" + acceptanceMount, Timeout: time.Minute,
					MaxRequestBytes: 65536, MaxResponseBytes: capacity.Budgets.SourceMaxResponseBytes,
					Client: &http.Client{Transport: handlerTransport{handler: service.handler}},
				})
				if err != nil {
					b.Fatal(err)
				}
				return source
			},
			"loopback": func() *display.HTTPSource {
				source, err := display.NewHTTPSource(display.HTTPSourceConfig{
					BaseURL: server.URL + acceptanceMount, Timeout: time.Minute,
					MaxRequestBytes: 65536, MaxResponseBytes: capacity.Budgets.SourceMaxResponseBytes,
					Client: server.Client(),
				})
				if err != nil {
					b.Fatal(err)
				}
				return source
			},
		}
		for transport, build := range sources {
			b.Run(transport+"/"+name, func(b *testing.B) {
				source := build()
				request := simulatorapi.ReceptionSnapshotRequest{StationIDs: service.ids}
				records := 0
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					snapshot, err := source.ReceptionSnapshot(context.Background(), request)
					if err != nil {
						b.Fatal(err)
					}
					records += len(snapshot.Records)
				}
				b.StopTimer()
				b.ReportMetric(float64(records)/float64(b.N), "records/op")
			})
		}
	})
}

// BenchmarkCapacitySimulatorContention measures the declared caller counts
// reading the same maximum-cardinality service over loopback.
func BenchmarkCapacitySimulatorContention(b *testing.B) {
	workload := capacitytest.Workload(b, benchmarkCapacityFile, "contention")
	service := newBenchmarkService(b, workload, workload.AircraftCounts[0], workload.StationCounts[0])
	server := httptest.NewServer(service.handler)
	b.Cleanup(server.Close)
	body, err := json.Marshal(simulatorapi.ReceptionSnapshotRequest{StationIDs: service.ids})
	if err != nil {
		b.Fatal(err)
	}

	for _, callers := range workload.Callers {
		b.Run(fmt.Sprintf("callers-%d", callers), func(b *testing.B) {
			client := server.Client()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				requests := make(chan struct{}, callers)
				var group sync.WaitGroup
				for range callers {
					group.Add(1)
					go func() {
						defer group.Done()
						for range requests {
							response, err := client.Post(server.URL+acceptanceMount+"/observations/receptions",
								"application/json", strings.NewReader(string(body)))
							if err != nil {
								b.Error(err)
								return
							}
							if _, err := io.Copy(io.Discard, response.Body); err != nil {
								b.Error(err)
							}
							if err := response.Body.Close(); err != nil {
								b.Error(err)
							}
						}
					}()
				}
				for range callers {
					requests <- struct{}{}
				}
				close(requests)
				group.Wait()
			}
			b.ReportMetric(float64(callers), "requests/op")
		})
	}
}
