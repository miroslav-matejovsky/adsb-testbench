package simdriver_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/capacitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Capacity benchmarks of the real-time driver.
//
// Time comes from the fake clock only: no benchmark starts a system ticker
// and none sleeps. The measured operation is one command that settles the
// elapsed time the clock was moved by, which is exactly the work a heartbeat
// wakeup performs plus one no-op fleet command.

const capacityFile = "../../testdata/quality/capacity.json"

// benchmarkRig is one driver over a prewarmed engine and its fake clock.
type benchmarkRig struct {
	engine *simulation.Engine
	clock  *fakeClock
	driver *simdriver.Driver
	count  int
}

func newBenchmarkRig(b *testing.B, workloadName string, aircraft, stations int, speed uint16) benchmarkRig {
	b.Helper()

	workload := capacitytest.Workload(b, capacityFile, workloadName)
	engine, _ := capacitytest.NewEngine(b, workload, aircraft, stations, speed, workload.Station)
	clock := newFakeClock()
	return benchmarkRig{engine: engine, clock: clock, driver: simdriver.New(engine, clock), count: aircraft}
}

// settle moves the fake clock and applies one command, which settles the
// elapsed time at the current speed before its no-op mutation.
func (r benchmarkRig) settle(b *testing.B, real time.Duration) {
	b.Helper()

	r.clock.Add(real)
	if err := r.driver.SetCount(context.Background(), r.count); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkCapacityDriverTick measures one real heartbeat of settlement at
// the declared speeds, with the maximum fleet and station count.
func BenchmarkCapacityDriverTick(b *testing.B) {
	workload := capacitytest.Workload(b, capacityFile, "driver-acceleration")
	speeds := append([]uint16{0}, workload.SpeedHundredths...)
	for _, speed := range speeds {
		for _, heartbeat := range workload.Advances {
			b.Run(fmt.Sprintf("speed-%d/real-%s", speed, heartbeat), func(b *testing.B) {
				rig := newBenchmarkRig(b, workload.Name, workload.AircraftCounts[0], workload.StationCounts[0], speed)
				rig.settle(b, heartbeat)
				before := rig.engine.Snapshot().Elapsed
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					rig.settle(b, heartbeat)
				}
				b.StopTimer()
				virtual := rig.engine.Snapshot().Elapsed - before
				b.ReportMetric(float64(virtual.Nanoseconds())/float64(b.N), "virtual-ns/op")
			})
		}
	}
}

// BenchmarkCapacityDriverCatchUp measures one suspended-driver catch-up.
// Each operation runs on a fresh engine and clock with the timer stopped, so
// the measurement covers exactly one catch-up.
func BenchmarkCapacityDriverCatchUp(b *testing.B) {
	workload := capacitytest.Workload(b, capacityFile, "catch-up")
	speed := workload.SpeedHundredths[0]
	for _, virtual := range workload.Advances {
		real := time.Duration(int64(virtual) * 100 / int64(speed))
		b.Run(fmt.Sprintf("virtual-%s/speed-%d", virtual, speed), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				rig := newBenchmarkRig(b, workload.Name, workload.AircraftCounts[0], workload.StationCounts[0], speed)
				rig.clock.Add(real)
				b.StartTimer()

				if err := rig.driver.SetCount(context.Background(), rig.count); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				if got := rig.engine.Snapshot().Elapsed; got != virtual {
					b.Fatalf("settled %s of virtual time, want %s", got, virtual)
				}
				b.StartTimer()
			}
			b.ReportMetric(float64(virtual.Nanoseconds()), "virtual-ns/op")
		})
	}
}
