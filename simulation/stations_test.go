package simulation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// advanceFixed runs one advance against a fixed-geometry engine that already
// has the given stations, and returns the batch.
func advanceFixed(t testing.TB, altitudeFeet float64, stations ...StationConfig) Batch {
	t.Helper()

	engine := newEngine(t, fixedGeometryConfig(altitudeFeet))
	for _, cfg := range stations {
		addStation(t, engine, cfg)
	}
	batch, err := engine.Advance(context.Background(), 5*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch.Transmissions)
	return batch
}

// TestStationValidationRejects covers every rejection category of the station
// commands, each leaving the engine untouched.
func TestStationValidationRejects(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())
	require.NoError(t, engine.RemoveStation(t.Context(), created.Config.ID, created.Revision))
	kept := addStation(t, engine, namedStation("kept"))
	before := engine.Snapshot()

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"out of domain field", func() error {
			cfg := namedStation("new")
			cfg.SensitivityDBm = 1
			_, err := engine.AddStation(t.Context(), cfg)
			return err
		}, ErrInvalid},
		{"malformed identifier", func() error {
			_, err := engine.AddStation(t.Context(), namedStation("bad id"))
			return err
		}, ErrInvalid},
		{"duplicate identifier", func() error {
			_, err := engine.AddStation(t.Context(), namedStation("kept"))
			return err
		}, ErrInvalid},
		{"removed identifier", func() error {
			_, err := engine.AddStation(t.Context(), validStationConfig())
			return err
		}, ErrInvalid},
		{"unknown identifier on update", func() error {
			_, err := engine.UpdateStation(t.Context(), 1, namedStation("absent"))
			return err
		}, ErrNotFound},
		{"unknown identifier on remove", func() error {
			return engine.RemoveStation(t.Context(), "absent", 1)
		}, ErrNotFound},
		{"stale revision on update", func() error {
			_, err := engine.UpdateStation(t.Context(), kept.Revision+1, namedStation("kept"))
			return err
		}, ErrConflict},
		{"stale revision on remove", func() error {
			return engine.RemoveStation(t.Context(), "kept", kept.Revision+1)
		}, ErrConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.call(), test.want)
			require.Equal(t, before, engine.Snapshot())
		})
	}

	// Filling the remaining slots makes one more addition exceed the limit.
	for i := range MaxStations - 1 {
		addStation(t, engine, namedStation(fmt.Sprintf("filler-%d", i)))
	}
	full := engine.Snapshot()
	_, err := engine.AddStation(t.Context(), namedStation("overflow"))
	require.ErrorIs(t, err, ErrLimit)
	require.Equal(t, full, engine.Snapshot())
}

// TestStationDisabled covers disablement: an identical station that is
// switched off hears nothing, while the enabled one hears everything.
func TestStationDisabled(t *testing.T) {
	t.Parallel()

	enabled := validStationConfig()
	disabled := disabledStationConfig()

	engine := newEngine(t, validConfig())
	addStation(t, engine, enabled)
	addStation(t, engine, disabled)

	batch, err := engine.Advance(t.Context(), 6*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch.Transmissions)

	require.Len(t, receptionsFor(batch, enabled.ID), len(batch.Transmissions))
	require.Empty(t, receptionsFor(batch, disabled.ID))

	// Enabling it makes it hear from then on, and only from then on.
	stations := engine.Snapshot().Stations
	turnedOn := disabled
	turnedOn.Enabled = true
	_, err = engine.UpdateStation(t.Context(), stations[1].Revision, turnedOn)
	require.NoError(t, err)

	later, err := engine.Advance(t.Context(), 6*time.Second)
	require.NoError(t, err)
	require.Len(t, receptionsFor(later, disabled.ID), len(later.Transmissions))
	require.Equal(t, uint64(2), later.Receptions[1].StationRevision)
}

// TestStationsPreserveTransmissions covers the isolation guarantee: station
// churn must not change a single generated frame or any aircraft truth.
func TestStationsPreserveTransmissions(t *testing.T) {
	t.Parallel()

	script := []operation{
		advanceOp(3 * time.Second),
		countOp(5),
		elapseOp(2 * time.Second),
		speedOp(250),
		elapseOp(1500 * time.Millisecond),
		countOp(3),
		advanceOp(4500 * time.Millisecond),
	}

	control := newEngine(t, validConfig())
	subject := newEngine(t, validConfig())

	// Interleave heavy station churn with the same script.
	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.4
	first := addStation(t, subject, lossy)
	addStation(t, subject, insensitiveStationConfig())
	addStation(t, subject, disabledStationConfig())

	wanted := runScript(t, control, script[:3])
	got := runScript(t, subject, script[:3])
	require.Equal(t, wanted.Transmissions, got.Transmissions)

	off := lossy
	off.Enabled = false
	updated, err := subject.UpdateStation(t.Context(), first.Revision, off)
	require.NoError(t, err)
	require.NoError(t, subject.RemoveStation(t.Context(), "insensitive", 1))
	addStation(t, subject, namedStation("late"))
	_, err = subject.UpdateStation(t.Context(), updated.Revision, lossy)
	require.NoError(t, err)

	wanted = runScript(t, control, script[3:])
	got = runScript(t, subject, script[3:])
	require.Equal(t, wanted.Transmissions, got.Transmissions)
	require.NotEmpty(t, got.Receptions)

	controlSnapshot := control.Snapshot()
	subjectSnapshot := subject.Snapshot()
	require.Equal(t, controlSnapshot.Aircraft, subjectSnapshot.Aircraft)
	require.Equal(t, controlSnapshot.History, subjectSnapshot.History)
	require.Equal(t, controlSnapshot.Elapsed, subjectSnapshot.Elapsed)
	require.Equal(t, control.state.fleet, subject.state.fleet)
	require.Equal(t, control.state.clock, subject.state.clock)
	require.Equal(t, control.state.identity, subject.state.identity)
	require.Equal(t, control.state.lastSequence, subject.state.lastSequence)
}

// TestReceptionAltitudeEffect covers the altitude dependence of the horizon
// rule at a fixed range.
func TestReceptionAltitudeEffect(t *testing.T) {
	t.Parallel()

	// 300 km is inside the horizon at 35000 feet and beyond it at 5000 feet.
	const rangeMetres = 300000
	cfg := stationAtDistance("horizon", rangeMetres)

	high := advanceFixed(t, 35000, cfg)
	low := advanceFixed(t, 5000, cfg)

	require.Len(t, high.Receptions, len(high.Transmissions), "the high aircraft is heard")
	require.Empty(t, low.Receptions, "the low aircraft is below the horizon")

	// Coverage grows with altitude, and the crossover matches the published
	// horizon radius while the horizon is the binding limit.
	previous := 0.0
	for _, altitude := range []float64{5000, 20000, 35000} {
		coverage, err := EstimateCoverage(cfg, altitude)
		require.NoError(t, err)
		require.Equal(t, coverage.HorizonRadiusNauticalMiles, coverage.EffectiveRadiusNauticalMiles,
			"this station is horizon limited at %g feet", altitude)
		require.Greater(t, coverage.EffectiveRadiusNauticalMiles, previous, "coverage grows with altitude")
		previous = coverage.EffectiveRadiusNauticalMiles

		radius := coverage.HorizonRadiusNauticalMiles * metresPerNauticalMile
		antenna := cfg.SiteElevationMetres + cfg.AntennaHeightMetres
		want := oracleHorizonMetres(antenna, altitude*metresPerFoot)
		require.InDelta(t, want, radius, want*0.001)

		require.NotEmpty(t, advanceFixed(t, altitude, stationAtDistance("in", radius*0.999)).Receptions)
		require.Empty(t, advanceFixed(t, altitude, stationAtDistance("out", radius*1.001)).Receptions)
	}

	// High enough, the horizon stops binding and the link budget takes over.
	ceiling, err := EstimateCoverage(cfg, maxAltitudeFeet)
	require.NoError(t, err)
	require.Greater(t, ceiling.HorizonRadiusNauticalMiles, ceiling.LinkBudgetRadiusNauticalMiles)
	require.Equal(t, ceiling.LinkBudgetRadiusNauticalMiles, ceiling.EffectiveRadiusNauticalMiles)

	radius := ceiling.EffectiveRadiusNauticalMiles * metresPerNauticalMile
	require.NotEmpty(t, advanceFixed(t, maxAltitudeFeet, stationAtDistance("in", radius*0.999)).Receptions)
	require.Empty(t, advanceFixed(t, maxAltitudeFeet, stationAtDistance("out", radius*1.001)).Receptions)
}

// TestReceptionSensitivityEffect covers the sensitivity dependence of the link
// budget at a fixed range well inside the horizon.
func TestReceptionSensitivityEffect(t *testing.T) {
	t.Parallel()

	const rangeMetres = 200000
	sensitive := stationAtDistance("sensitive", rangeMetres)
	insensitive := stationAtDistance("insensitive", rangeMetres)
	insensitive.SensitivityDBm = -85

	batch := advanceFixed(t, 35000, sensitive, insensitive)
	require.Len(t, receptionsFor(batch, "sensitive"), len(batch.Transmissions))
	require.Empty(t, receptionsFor(batch, "insensitive"))

	// The crossover matches the published link budget radius.
	for _, sensitivity := range []float64{-85, -90, -80} {
		cfg := stationAtDistance("probe", rangeMetres)
		cfg.SensitivityDBm = sensitivity

		coverage, err := EstimateCoverage(cfg, 35000)
		require.NoError(t, err)
		require.Equal(t, coverage.LinkBudgetRadiusNauticalMiles, coverage.EffectiveRadiusNauticalMiles,
			"this station is link budget limited at %g dBm", sensitivity)

		radius := coverage.LinkBudgetRadiusNauticalMiles * metresPerNauticalMile
		inside := stationAtDistance("in", radius*0.999)
		inside.SensitivityDBm = sensitivity
		outside := stationAtDistance("out", radius*1.001)
		outside.SensitivityDBm = sensitivity

		require.NotEmpty(t, advanceFixed(t, 35000, inside).Receptions)
		require.Empty(t, advanceFixed(t, 35000, outside).Receptions)
	}
}

// TestStationAtomicity covers rejected and canceled station commands leaving
// no trace, including an identical valid suffix against a control.
func TestStationAtomicity(t *testing.T) {
	t.Parallel()

	prelude := []operation{advanceOp(2 * time.Second)}

	rejections := []struct {
		name string
		call func(*Engine) error
	}{
		{"invalid settings", func(e *Engine) error {
			cfg := validStationConfig()
			cfg.AntennaGainDBi = 99
			_, err := e.AddStation(context.Background(), cfg)
			return err
		}},
		{"unknown station", func(e *Engine) error {
			return e.RemoveStation(context.Background(), "absent", 1)
		}},
		{"stale revision", func(e *Engine) error {
			_, err := e.UpdateStation(context.Background(), 7, validStationConfig())
			return err
		}},
		{"canceled add", func(e *Engine) error {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := e.AddStation(ctx, namedStation("canceled"))
			return err
		}},
		{"deadline exceeded remove", func(e *Engine) error {
			ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
			defer cancel()
			return e.RemoveStation(ctx, validStationConfig().ID, 1)
		}},
	}

	for _, rejection := range rejections {
		t.Run(rejection.name, func(t *testing.T) {
			t.Parallel()

			subject, control := controlPair(t, validConfig(), prelude)
			addStation(t, subject, validStationConfig())
			addStation(t, control, validStationConfig())

			require.Error(t, rejection.call(subject))
			require.Equal(t, control.Snapshot(), subject.Snapshot())
			requireIdenticalFuture(t, subject, control)
		})
	}
}

// TestStationConcurrentAccess asserts safety, not a concurrent command order.
func TestStationConcurrentAccess(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addStation(t, engine, validStationConfig())

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				got := engine.Snapshot()
				require.True(t, got.Now.Equal(got.Config.StartTime.Add(got.Elapsed)))
				require.LessOrEqual(t, len(got.Stations), MaxStations)
				for _, station := range got.Stations {
					require.GreaterOrEqual(t, station.Revision, uint64(1))
					require.False(t, station.CreatedAt.After(got.Now))
					require.NoError(t, validateStation(station.Config))
				}
			}
		}()
	}
	for worker := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 10 {
				id := fmt.Sprintf("worker-%d-%d", worker, i)
				if _, err := engine.AddStation(context.Background(), namedStation(id)); err != nil {
					require.ErrorIs(t, err, ErrLimit)
					continue
				}
				require.NoError(t, engine.RemoveStation(context.Background(), id, 1))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 20 {
			_, err := engine.Advance(context.Background(), 100*time.Millisecond)
			require.NoError(t, err)
		}
	}()
	wg.Wait()

	final := engine.Snapshot()
	require.Equal(t, 2*time.Second, final.Elapsed)
	require.Len(t, final.Stations, 1)
}
