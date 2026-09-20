package simulation_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/capacitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Capacity benchmarks of the engine.
//
// Every input comes from a named workload in testdata/quality/capacity.json;
// no benchmark chooses a fleet, a station or a duration of its own. Engines
// are constructed and prewarmed outside the timed section, and the intended
// retention state is asserted before timing starts. Encoding, scheduling,
// reception decisions, mutation cloning and batch creation stay inside the
// measured operation.

const capacityFile = "../testdata/quality/capacity.json"

func benchmarkCapacity(b *testing.B) qualitytest.Capacity {
	b.Helper()

	return capacitytest.Load(b, capacityFile)
}

func benchmarkWorkload(b *testing.B, name string) qualitytest.CapacityWorkload {
	b.Helper()

	return capacitytest.Workload(b, capacityFile, name)
}

// measured accumulates the work counts of a timed loop.
type measured struct {
	frames     int
	receptions int
	virtual    time.Duration
}

// report publishes work counts per operation and, when frames were emitted,
// the cost of one frame.
func (m measured) report(b *testing.B, elapsed time.Duration) {
	b.ReportMetric(float64(m.frames)/float64(b.N), "frames/op")
	b.ReportMetric(float64(m.receptions)/float64(b.N), "receptions/op")
	b.ReportMetric(float64(m.virtual.Nanoseconds())/float64(b.N), "virtual-ns/op")
	if m.frames > 0 {
		b.ReportMetric(float64(elapsed.Nanoseconds())/float64(m.frames), "ns/frame")
	}
}

// advanceCases enumerates the sub-benchmarks of one workload.
type advanceCase struct {
	name     string
	aircraft int
	stations int
	speed    uint16
	advance  time.Duration
	station  qualitytest.Station
}

func advanceCases(workload qualitytest.CapacityWorkload) []advanceCase {
	var cases []advanceCase
	variants := workload.StationVariants
	if len(variants) == 0 {
		variants = []qualitytest.CapacityStationVariant{{Name: "", Station: workload.Station}}
	}
	for _, aircraft := range workload.AircraftCounts {
		for _, stations := range workload.StationCounts {
			for _, speed := range workload.SpeedHundredths {
				for _, advance := range workload.Advances {
					for _, variant := range variants {
						name := fmt.Sprintf("%s/aircraft-%d/stations-%d/speed-%d/advance-%s",
							workload.Name, aircraft, stations, speed, advance)
						if variant.Name != "" {
							name += "/" + variant.Name
						}
						cases = append(cases, advanceCase{
							name: name, aircraft: aircraft, stations: stations,
							speed: speed, advance: advance, station: variant.Station,
						})
					}
				}
			}
		}
	}
	return cases
}

// BenchmarkCapacityEngineAdvance measures steady advances of prewarmed
// engines. Aircraft are stationary, so repeated advances of a fixed virtual
// duration keep the retained state stable.
func BenchmarkCapacityEngineAdvance(b *testing.B) {
	capacity := benchmarkCapacity(b)
	for _, name := range []string{"empty-overhead", "scheduler-scaling", "reception-scaling", "radio-outcomes"} {
		workload, err := capacity.Workload(name)
		require.NoError(b, err)
		for _, tc := range advanceCases(workload) {
			b.Run(tc.name, func(b *testing.B) {
				engine, _ := capacitytest.NewEngine(b, workload, tc.aircraft, tc.stations, tc.speed, tc.station)
				var work measured
				b.ReportAllocs()
				b.ResetTimer()
				start := time.Now()
				for b.Loop() {
					batch, err := engine.Advance(context.Background(), tc.advance)
					if err != nil {
						b.Fatal(err)
					}
					work.frames += len(batch.Transmissions)
					work.receptions += len(batch.Receptions)
					work.virtual += tc.advance
				}
				elapsed := time.Since(start)
				b.StopTimer()
				work.report(b, elapsed)
			})
		}
	}
}

// BenchmarkCapacityEngineConstruction measures cold construction only: the
// configuration, the fleet and its creation reports, with no advance.
func BenchmarkCapacityEngineConstruction(b *testing.B) {
	workload := benchmarkWorkload(b, "scheduler-scaling")
	for _, aircraft := range workload.AircraftCounts {
		b.Run(fmt.Sprintf("aircraft-%d", aircraft), func(b *testing.B) {
			cfg := capacitytest.EngineConfig(workload, "capacity-construction", aircraft, 100)
			b.ReportAllocs()
			for b.Loop() {
				engine, err := simulation.New(cfg)
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(engine)
			}
		})
	}
}

// BenchmarkCapacityEngineZeroAdvance measures the cost of a mutation that
// delivers no event: state cloning and commit without work.
func BenchmarkCapacityEngineZeroAdvance(b *testing.B) {
	workload := benchmarkWorkload(b, "reception-scaling")
	engine, _ := capacitytest.NewEngine(b, workload, 100, 8, 100, workload.Station)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.Advance(context.Background(), 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCapacityEngineSameCount measures a no-op fleet command with full
// histories: it stages and commits a clone without changing anything.
func BenchmarkCapacityEngineSameCount(b *testing.B) {
	workload := benchmarkWorkload(b, "reception-scaling")
	engine, _ := capacitytest.NewEngine(b, workload, 100, 8, 100, workload.Station)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.SetCount(context.Background(), 100); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCapacityEngineStationUpdate measures one station edit with full
// histories. Each iteration supplies the revision the previous one produced.
func BenchmarkCapacityEngineStationUpdate(b *testing.B) {
	workload := benchmarkWorkload(b, "reception-scaling")
	engine, ids := capacitytest.NewEngine(b, workload, 100, 8, 100, workload.Station)
	cfg := capacitytest.StationConfig(workload.Station, ids[0])
	revision := engine.Snapshot().Stations[0].Revision
	b.ReportAllocs()
	for b.Loop() {
		station, err := engine.UpdateStation(context.Background(), revision, cfg)
		if err != nil {
			b.Fatal(err)
		}
		revision = station.Revision
	}
}

// BenchmarkCapacityReceptionSnapshot measures raw evidence capture: copying
// and ordering the retained records of a selection.
func BenchmarkCapacityReceptionSnapshot(b *testing.B) {
	workload := benchmarkWorkload(b, "read-refresh")
	for _, aircraft := range workload.AircraftCounts {
		for _, stations := range workload.StationCounts {
			b.Run(fmt.Sprintf("aircraft-%d/stations-%d", aircraft, stations), func(b *testing.B) {
				engine, ids := capacitytest.NewEngine(b, workload, aircraft, stations, 100, workload.Station)
				request := simulation.ReceptionSnapshotRequest{StationIDs: ids}
				records := 0
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					snapshot, err := engine.ReceptionSnapshot(context.Background(), request)
					if err != nil {
						b.Fatal(err)
					}
					records += len(snapshot.Records)
				}
				b.StopTimer()
				b.ReportMetric(float64(records)/float64(b.N), "records/op")
			})
		}
	}
}
