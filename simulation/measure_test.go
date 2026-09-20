package simulation_test

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/capacitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Opt-in capacity measurements.
//
// This report samples process memory, which no ordinary test may assert on:
// the numbers depend on the garbage collector, on runtime-owned caches and
// on whatever else the machine is doing. It runs only when
// ADSB_CAPACITY_MEASURE=1, which the capacity task sets, and it asserts
// structural counts only.

// measureEnabled reports whether the opt-in measurement path is on.
func measureEnabled() bool { return os.Getenv("ADSB_CAPACITY_MEASURE") == "1" }

// epochs is the fixed number of measurement epochs after warmup.
const epochs = 10

// TestCapacityMemoryReport reports retained engine state separately from
// cumulative allocation over a fixed number of fixed-work epochs.
func TestCapacityMemoryReport(t *testing.T) {
	if !measureEnabled() {
		t.Skip("set ADSB_CAPACITY_MEASURE=1 to run the memory report")
	}

	const path = "../testdata/quality/capacity.json"
	workload := capacitytest.Workload(t, path, "read-refresh")
	aircraft := workload.AircraftCounts[len(workload.AircraftCounts)-1]
	stations := workload.StationCounts[len(workload.StationCounts)-1]
	engine, ids := capacitytest.NewEngine(t, workload, aircraft, stations, 100, workload.Station)

	// Warm the fixture, then release every response before sampling.
	snapshot, err := engine.ReceptionSnapshot(context.Background(),
		simulation.ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	require.Len(t, snapshot.Records, stations*workload.RetentionFillTarget)
	records := len(snapshot.Records)
	snapshot = simulation.ReceptionSnapshot{}

	sample := func() runtime.MemStats {
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats
	}
	before := sample()
	t.Logf("retained after warmup: HeapAlloc %d bytes, HeapObjects %d, NumGC %d, aircraft %d, stations %d, records %d",
		before.HeapAlloc, before.HeapObjects, before.NumGC, aircraft, stations, records)

	// Each epoch performs a fixed amount of work and retains nothing.
	const heartbeats = 100
	for epoch := range epochs {
		for range heartbeats {
			batch, err := engine.Advance(context.Background(), 100*time.Millisecond)
			require.NoError(t, err)
			_ = batch
		}
		for range 10 {
			published, err := engine.Observations(context.Background(), simulation.ObservationRequest{
				StationIDs: ids,
				Expiry: simulation.ObservationExpiry{
					Identity: workload.Expiry.Identity, Position: workload.Expiry.Position,
					Altitude: workload.Expiry.Altitude, Velocity: workload.Expiry.Velocity,
				},
			})
			require.NoError(t, err)
			_ = published
		}
		after := sample()
		t.Logf("epoch %d: HeapAlloc %d bytes, HeapObjects %d, TotalAlloc delta %d bytes, NumGC %d",
			epoch+1, after.HeapAlloc, after.HeapObjects, after.TotalAlloc-before.TotalAlloc, after.NumGC)
	}

	// The engine must still be alive while its retained state is measured.
	runtime.KeepAlive(engine)

	// Only structural counts are asserted; heap numbers are reported, not gated.
	final, err := engine.ReceptionSnapshot(context.Background(),
		simulation.ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	require.Len(t, final.Records, stations*workload.RetentionFillTarget)
	require.LessOrEqual(t, len(engine.Snapshot().History.Messages), simulation.HistoryLimit)
	require.Len(t, engine.Snapshot().Stations, stations)
	require.Len(t, engine.Snapshot().Aircraft, aircraft)
}
