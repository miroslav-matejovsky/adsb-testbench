package display

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Capacity benchmarks of the display backend.
//
// Sources are immutable fixtures, so a measured refresh always decodes the
// same evidence and the consumer cannot change what the next iteration
// reads. Validation and decoding, browser-facing encoding, transport parsing
// and the refresh gate are separate benchmarks.

func benchmarkCapacity(b *testing.B) qualitytest.Capacity {
	b.Helper()

	capacity, err := qualitytest.LoadCapacity(qualityDir + "/capacity.json")
	if err != nil {
		b.Fatal(err)
	}
	return capacity
}

// benchmarkSource returns a fixture snapshot with distinct addresses spread
// over stations. The 8000-record case is the largest conforming source.
func benchmarkSource(b *testing.B, records, stations int) simulatorapi.ReceptionSnapshot {
	b.Helper()

	fixtures := make([]simulatorapi.Station, 0, stations)
	ids := make([]string, 0, stations)
	for i := range stations {
		id := fmt.Sprintf("station-%d", i+1)
		ids = append(ids, id)
		fixtures = append(fixtures, fixtureStation(id, 1, fixtureStart))
	}
	all := make([]simulatorapi.Reception, 0, records)
	for k := range records {
		station := fixtures[k%stations]
		icao := uint32(0x100000 + k)
		all = append(all, fixtureReception(uint64(k/stations+1), uint64(k+1), station, icao,
			identificationKind, fixtureStart, identificationFrame(b, icao, fmt.Sprintf("TB%06X", icao))))
	}
	retention := make([]simulatorapi.StationRetention, 0, stations)
	for _, id := range ids {
		retention = append(retention, simulatorapi.StationRetention{
			StationID: id, OldestSequence: "1", LatestSequence: simulatorapi.FormatUint64(uint64(records / stations)),
			Truncated: false, Limit: maxStationRecords,
		})
	}
	return simulatorapi.ReceptionSnapshot{
		RunID: fixtureRunID, Now: simulatorapi.FormatTime(fixtureStart.Add(time.Second)),
		StationIDs: ids, Retention: retention, Records: all,
	}
}

// benchmarkDisplay returns a display over an immutable fixture snapshot.
func benchmarkDisplay(b *testing.B, raw simulatorapi.ReceptionSnapshot, budget int) *Display {
	b.Helper()

	config := displayFixtureConfig()
	config.MaxResponseBytes = budget
	config.ReportError = func(err error) { b.Error(err) }
	return newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	}, funcSource{}, time.Now)
}

// benchmarkShapes are the fixture sizes measured by the display benchmarks.
func benchmarkShapes(b *testing.B) []struct {
	name     string
	records  int
	stations int
} {
	b.Helper()

	capacity := benchmarkCapacity(b)
	read, err := capacity.Workload("read-refresh")
	if err != nil {
		b.Fatal(err)
	}
	cardinality, err := capacity.Workload("source-cardinality")
	if err != nil {
		b.Fatal(err)
	}
	var shapes []struct {
		name     string
		records  int
		stations int
	}
	for _, stations := range read.StationCounts {
		shapes = append(shapes, struct {
			name     string
			records  int
			stations int
		}{fmt.Sprintf("stations-%d/records-%d", stations, stations*read.RetentionFillTarget),
			stations * read.RetentionFillTarget, stations})
	}
	shapes = append(shapes, struct {
		name     string
		records  int
		stations int
	}{"source-cardinality/records-8000",
		cardinality.StationCounts[0] * cardinality.RetentionFillTarget, cardinality.StationCounts[0]})
	return shapes
}

// BenchmarkCapacityDisplayRefresh measures validation, decoding and
// publication of one refresh over an in-process source.
func BenchmarkCapacityDisplayRefresh(b *testing.B) {
	capacity := benchmarkCapacity(b)
	for _, shape := range benchmarkShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			raw := benchmarkSource(b, shape.records, shape.stations)
			backend := benchmarkDisplay(b, raw, capacity.Budgets.DisplayMaxResponseBytes)
			tracks := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				snapshot, err := backend.Refresh(context.Background(), raw.StationIDs)
				if err != nil {
					b.Fatal(err)
				}
				tracks += len(snapshot.Observations.Aircraft)
			}
			b.StopTimer()
			b.ReportMetric(float64(shape.records), "records/op")
			b.ReportMetric(float64(tracks)/float64(b.N), "tracks/op")
		})
	}
}

// BenchmarkCapacityDisplayHTTP measures one complete browser-facing refresh
// request, including request parsing and response encoding.
func BenchmarkCapacityDisplayHTTP(b *testing.B) {
	capacity := benchmarkCapacity(b)
	for _, shape := range benchmarkShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			raw := benchmarkSource(b, shape.records, shape.stations)
			backend := benchmarkDisplay(b, raw, capacity.Budgets.DisplayMaxResponseBytes)
			handler := http.StripPrefix(displayMount, backend.Handler())
			body, err := json.Marshal(map[string][]string{"stationIds": raw.StationIDs})
			if err != nil {
				b.Fatal(err)
			}
			bytes := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				recorder := serveDisplayRequest(handler, string(body))
				if recorder.Code != http.StatusOK {
					b.Fatalf("status %d", recorder.Code)
				}
				bytes += recorder.Body.Len()
			}
			b.StopTimer()
			b.ReportMetric(float64(bytes)/float64(b.N), "bytes/op")
		})
	}
}

// BenchmarkCapacityDisplaySourceParse measures reading, bounding and
// parsing one raw snapshot response through the HTTP source, without a
// socket.
func BenchmarkCapacityDisplaySourceParse(b *testing.B) {
	capacity := benchmarkCapacity(b)
	for _, shape := range benchmarkShapes(b) {
		b.Run(shape.name, func(b *testing.B) {
			raw := benchmarkSource(b, shape.records, shape.stations)
			encoded, err := json.Marshal(raw)
			if err != nil {
				b.Fatal(err)
			}
			transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, string(encoded)), nil
			}}
			source, err := NewHTTPSource(HTTPSourceConfig{
				BaseURL: sourceBaseURL, Timeout: time.Minute, MaxRequestBytes: 65536,
				MaxResponseBytes: capacity.Budgets.SourceMaxResponseBytes,
				Client:           &http.Client{Transport: transport},
			})
			if err != nil {
				b.Fatal(err)
			}
			request := simulatorapi.ReceptionSnapshotRequest{StationIDs: raw.StationIDs}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := source.ReceptionSnapshot(context.Background(), request); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(len(encoded)), "bytes/op")
		})
	}
}

// BenchmarkCapacityDisplayFailures measures the two failure paths a browser
// sees at maximum cardinality: an undersized response budget, and a stale
// publication after the source fails.
func BenchmarkCapacityDisplayFailures(b *testing.B) {
	raw := benchmarkSource(b, 8000, 8)
	body, err := json.Marshal(map[string][]string{"stationIds": raw.StationIDs})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("response-limit", func(b *testing.B) {
		config := displayFixtureConfig()
		config.MaxResponseBytes = simulatorapi.MinResponseBytes
		backend := newDisplay(config, funcSource{
			snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
				return raw, nil
			},
		}, funcSource{}, time.Now)
		handler := http.StripPrefix(displayMount, backend.Handler())
		b.ReportAllocs()
		for b.Loop() {
			recorder := serveDisplayRequest(handler, string(body))
			if recorder.Code != http.StatusServiceUnavailable {
				b.Fatalf("status %d", recorder.Code)
			}
		}
	})

	b.Run("stale-fallback", func(b *testing.B) {
		failing := false
		config := displayFixtureConfig()
		config.ReportError = func(error) {}
		backend := newDisplay(config, funcSource{
			snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
				if failing {
					return simulatorapi.ReceptionSnapshot{}, simulatorapi.NewError(
						simulatorapi.CategoryUnavailable, "the source is unreachable")
				}
				return raw, nil
			},
		}, funcSource{}, time.Now)
		if _, err := backend.Refresh(context.Background(), raw.StationIDs); err != nil {
			b.Fatal(err)
		}
		failing = true
		b.ReportAllocs()
		for b.Loop() {
			snapshot, err := backend.Refresh(context.Background(), raw.StationIDs)
			if err == nil {
				b.Fatal("a failing source must report an error")
			}
			if snapshot.Status != StatusStale {
				b.Fatalf("status %q", snapshot.Status)
			}
		}
	})
}

// BenchmarkCapacityDisplayContention measures the declared caller counts
// against one display. A display serializes refreshes through its gate, so
// this measures contention rather than parallel throughput.
func BenchmarkCapacityDisplayContention(b *testing.B) {
	capacity := benchmarkCapacity(b)
	workload, err := capacity.Workload("contention")
	if err != nil {
		b.Fatal(err)
	}
	raw := benchmarkSource(b, workload.StationCounts[0]*workload.RetentionFillTarget, workload.StationCounts[0])
	backend := benchmarkDisplay(b, raw, capacity.Budgets.DisplayMaxResponseBytes)

	for _, callers := range workload.Callers {
		b.Run(fmt.Sprintf("callers-%d", callers), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				work := make(chan struct{}, callers)
				var group sync.WaitGroup
				for range callers {
					group.Add(1)
					go func() {
						defer group.Done()
						for range work {
							if _, err := backend.Refresh(context.Background(), raw.StationIDs); err != nil {
								b.Error(err)
								return
							}
						}
					}()
				}
				for range callers {
					work <- struct{}{}
				}
				close(work)
				group.Wait()
			}
			b.ReportMetric(float64(callers), "refreshes/op")
		})
	}
}

// serveDisplayRequest posts one observation request to a mounted handler.
func serveDisplayRequest(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, displayMount+"/observations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
