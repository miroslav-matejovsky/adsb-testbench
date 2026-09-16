package simulation_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// demoConfig assigns every configuration field explicitly. The engine fills in
// nothing, so a caller must always do this.
func demoConfig() simulation.Config {
	return simulation.Config{
		ID:                   "example",
		StartTime:            time.Date(2024, time.March, 5, 12, 0, 0, 0, time.UTC),
		Seed:                 20240305,
		InitialAircraftCount: 2,
		SpeedHundredths:      100,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees:           simulation.Range{Min: 50, Max: 50},
			LongitudeDegrees:          simulation.Range{Min: 14, Max: 14},
			AltitudeFeet:              simulation.Range{Min: 35000, Max: 35000},
			GroundSpeedKnots:          simulation.Range{Min: 450, Max: 450},
			TrackDegrees:              simulation.Range{Min: 90, Max: 90},
			VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
		},
	}
}

// New creates the configured aircraft and retains their creation reports.
func Example() {
	engine, err := simulation.New(demoConfig())
	if err != nil {
		panic(err)
	}

	snapshot := engine.Snapshot()
	fmt.Println("aircraft:", len(snapshot.Aircraft))
	fmt.Println("elapsed:", snapshot.Elapsed)
	fmt.Println("retained:", len(snapshot.History.Messages))

	for _, craft := range snapshot.Aircraft {
		fmt.Printf("%s at %.4fN %.4fE, %.0f ft, %.0f kt, track %.0f\n",
			craft.Callsign, craft.LatitudeDegrees, craft.LongitudeDegrees,
			craft.BarometricAltitudeFeet, craft.GroundSpeedKnots, craft.TrackDegrees)
	}

	// Each aircraft is created with one report per family, even CPR first.
	for _, report := range snapshot.History.Messages[:3] {
		fmt.Println(report.Sequence, report.Kind, hex.EncodeToString(report.Frame[:]))
	}

	// Output:
	// aircraft: 2
	// elapsed: 0s
	// retained: 6
	// TB000001 at 50.0000N 14.0000E, 35000 ft, 450 kt, track 90
	// TB000002 at 50.0000N 14.0000E, 35000 ft, 450 kt, track 90
	// 1 identification 8d00000120502c30c30c31a242b6
	// 2 position 8d00000158b5015556f49f1870d8
	// 3 velocity 8d0000019901c30030040092d822
}

// Advance steps virtual time exactly, whatever the current speed.
func ExampleEngine_Advance() {
	engine, err := simulation.New(demoConfig())
	if err != nil {
		panic(err)
	}

	batch, err := engine.Advance(context.Background(), 2*time.Second)
	if err != nil {
		panic(err)
	}

	fmt.Println("frames:", len(batch))
	fmt.Println("first:", batch[0].Kind, batch[0].Timestamp.Format(time.RFC3339Nano))
	fmt.Println("elapsed:", engine.Snapshot().Elapsed)

	// Splitting the same total produces exactly the same frames.
	split, err := simulation.New(demoConfig())
	if err != nil {
		panic(err)
	}
	var parts []simulation.Transmission
	for range 4 {
		part, err := split.Advance(context.Background(), 500*time.Millisecond)
		if err != nil {
			panic(err)
		}
		parts = append(parts, part...)
	}
	fmt.Println("split matches:", len(parts) == len(batch) && parts[len(parts)-1] == batch[len(batch)-1])

	// Output:
	// frames: 14
	// first: position 2024-03-05T12:00:00.41Z
	// elapsed: 2s
	// split matches: true
}

// Elapse converts supplied real time using the current speed.
func ExampleEngine_Elapse() {
	cfg := demoConfig()
	cfg.SpeedHundredths = 250 // 2.5x real time.
	engine, err := simulation.New(cfg)
	if err != nil {
		panic(err)
	}

	if _, err := engine.Elapse(context.Background(), 2*time.Second); err != nil {
		panic(err)
	}
	fmt.Println("elapsed:", engine.Snapshot().Elapsed)

	// Output:
	// elapsed: 5s
}

// SetSpeed pauses and resumes without settling any time itself.
func ExampleEngine_SetSpeed() {
	engine, err := simulation.New(demoConfig())
	if err != nil {
		panic(err)
	}

	if err := engine.SetSpeed(context.Background(), 0); err != nil {
		panic(err)
	}
	paused, err := engine.Elapse(context.Background(), time.Minute)
	if err != nil {
		panic(err)
	}
	fmt.Println("paused frames:", len(paused), "elapsed:", engine.Snapshot().Elapsed)

	// Direct stepping still works while paused.
	stepped, err := engine.Advance(context.Background(), time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Println("stepped frames:", len(stepped) > 0, "elapsed:", engine.Snapshot().Elapsed)

	if err := engine.SetSpeed(context.Background(), 100); err != nil {
		panic(err)
	}
	if _, err := engine.Elapse(context.Background(), time.Second); err != nil {
		panic(err)
	}
	fmt.Println("resumed elapsed:", engine.Snapshot().Elapsed)

	// Output:
	// paused frames: 0 elapsed: 0s
	// stepped frames: true elapsed: 1s
	// resumed elapsed: 2s
}

// SetCount adds and removes aircraft at the current virtual time.
func ExampleEngine_SetCount() {
	engine, err := simulation.New(demoConfig())
	if err != nil {
		panic(err)
	}
	if _, err := engine.Advance(context.Background(), time.Second); err != nil {
		panic(err)
	}

	added, err := engine.SetCount(context.Background(), 4)
	if err != nil {
		panic(err)
	}
	fmt.Println("creation reports:", len(added))
	for _, craft := range engine.Snapshot().Aircraft {
		fmt.Println(craft.Callsign, "created at", craft.CreatedAt.Format(time.RFC3339))
	}

	// Removal takes the newest aircraft first and returns no frames.
	removed, err := engine.SetCount(context.Background(), 2)
	if err != nil {
		panic(err)
	}
	fmt.Println("removal reports:", len(removed))
	for _, craft := range engine.Snapshot().Aircraft {
		fmt.Println("kept", craft.Callsign)
	}

	// Output:
	// creation reports: 6
	// TB000001 created at 2024-03-05T12:00:00Z
	// TB000002 created at 2024-03-05T12:00:00Z
	// TB000003 created at 2024-03-05T12:00:01Z
	// TB000004 created at 2024-03-05T12:00:01Z
	// removal reports: 0
	// kept TB000001
	// kept TB000002
}

// A consumer detects lost retention by comparing its own progress with the
// history bounds of the same run.
func ExampleEngine_Snapshot() {
	cfg := demoConfig()
	cfg.InitialAircraftCount = 10
	engine, err := simulation.New(cfg)
	if err != nil {
		panic(err)
	}

	processed := engine.Snapshot().History.LatestSequence
	if _, err := engine.Advance(context.Background(), 30*time.Second); err != nil {
		panic(err)
	}

	snapshot := engine.Snapshot()
	fmt.Println("run:", snapshot.Config.ID)
	fmt.Println("retained:", len(snapshot.History.Messages), "of", snapshot.History.Limit)
	fmt.Println("gap:", processed+1 < snapshot.History.OldestSequence)

	// Output:
	// run: example
	// retained: 1000 of 1000
	// gap: true
}
