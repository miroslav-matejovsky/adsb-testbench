// Package capacitytest builds engines and stations from the named capacity
// workloads in testdata/quality/capacity.json.
//
// It exists so the engine, driver and simulator benchmarks share one
// conversion from stored workload inputs to engine configuration, and one
// prewarm that asserts the intended retention state instead of assuming a
// warmup duration was enough. It is test support only; production packages
// must not import it.
package capacitytest

import (
	"context"
	"fmt"
	"testing"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Load reads the capacity file at path.
func Load(tb testing.TB, path string) qualitytest.Capacity {
	tb.Helper()

	capacity, err := qualitytest.LoadCapacity(path)
	if err != nil {
		tb.Fatalf("load capacity: %v", err)
	}
	return capacity
}

// Workload reads one named workload from the capacity file at path.
func Workload(tb testing.TB, path, name string) qualitytest.CapacityWorkload {
	tb.Helper()

	workload, err := Load(tb, path).Workload(name)
	if err != nil {
		tb.Fatal(err)
	}
	return workload
}

// EngineConfig converts workload inputs into a complete engine
// configuration. Every field is assigned explicitly.
func EngineConfig(workload qualitytest.CapacityWorkload, runID string, aircraft int, speed uint16) simulation.Config {
	r := func(value qualitytest.Range) simulation.Range {
		return simulation.Range{Min: value.Min, Max: value.Max}
	}
	spawn := workload.Spawn
	return simulation.Config{
		ID: runID, StartTime: workload.StartTime, Seed: workload.Seed,
		InitialAircraftCount: aircraft, SpeedHundredths: speed,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: r(spawn.LatitudeDegrees), LongitudeDegrees: r(spawn.LongitudeDegrees),
			AltitudeFeet: r(spawn.AltitudeFeet), GroundSpeedKnots: r(spawn.GroundSpeedKnots),
			TrackDegrees: r(spawn.TrackDegrees), VerticalRateFeetPerMinute: r(spawn.VerticalRateFeetPerMinute),
		},
	}
}

// StationConfig converts a workload station template into a configuration
// with an explicit identifier.
func StationConfig(station qualitytest.Station, id string) simulation.StationConfig {
	return simulation.StationConfig{
		ID: id, Enabled: station.Enabled,
		LatitudeDegrees: station.LatitudeDegrees, LongitudeDegrees: station.LongitudeDegrees,
		SiteElevationMetres: station.SiteElevationMetres, AntennaHeightMetres: station.AntennaHeightMetres,
		AntennaGainDBi: station.AntennaGainDBi, SensitivityDBm: station.SensitivityDBm,
		SystemLossDB: station.SystemLossDB, FrameLossProbability: station.FrameLossProbability,
	}
}

// StationIDs returns count generated station identifiers.
func StationIDs(count int) []string {
	ids := make([]string, 0, count)
	for i := range count {
		ids = append(ids, fmt.Sprintf("station-%d", i+1))
	}
	return ids
}

// NewEngine builds one prewarmed engine: the stations are created before any
// aircraft, so creation reports are received, and retention is filled to the
// workload target. It returns the engine and its station identifiers.
func NewEngine(tb testing.TB, workload qualitytest.CapacityWorkload,
	aircraft, stations int, speed uint16, station qualitytest.Station,
) (*simulation.Engine, []string) {
	tb.Helper()

	engine, err := simulation.New(EngineConfig(workload, "capacity-"+workload.Name, 0, speed))
	if err != nil {
		tb.Fatalf("new engine: %v", err)
	}
	ids := StationIDs(stations)
	for _, id := range ids {
		if _, err := engine.AddStation(context.Background(), StationConfig(station, id)); err != nil {
			tb.Fatalf("add station %s: %v", id, err)
		}
	}
	if aircraft > 0 {
		if _, err := engine.SetCount(context.Background(), aircraft); err != nil {
			tb.Fatalf("set count: %v", err)
		}
	}
	if workload.RetentionFillTarget > 0 && stations > 0 {
		FillRetention(tb, engine, ids, workload.RetentionFillTarget)
	}
	return engine, ids
}

// FillRetention advances in chunks until every station holds target records
// and fails when the target is unreachable. It never assumes a fixed warmup
// duration filled a ring.
func FillRetention(tb testing.TB, engine *simulation.Engine, ids []string, target int) {
	tb.Helper()

	for range 100 {
		snapshot, err := engine.ReceptionSnapshot(context.Background(),
			simulation.ReceptionSnapshotRequest{StationIDs: ids})
		if err != nil {
			tb.Fatalf("capture retention: %v", err)
		}
		filled := len(snapshot.Retention) == len(ids)
		for _, entry := range snapshot.Retention {
			filled = filled && entry.LatestSequence-entry.OldestSequence+1 >= uint64(target)
		}
		if filled {
			return
		}
		if _, err := engine.Advance(context.Background(), simulation.MaxAdvance); err != nil {
			tb.Fatalf("advance: %v", err)
		}
	}
	tb.Fatalf("retention target %d was not reached", target)
}
