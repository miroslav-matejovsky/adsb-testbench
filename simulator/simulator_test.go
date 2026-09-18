package simulator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/stretchr/testify/require"
)

type runtimeClock struct {
	mu      sync.Mutex
	now     time.Time
	started chan struct{}
	ticks   chan time.Time
	stopped chan struct{}
}

func (c *runtimeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *runtimeClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	close(c.started)
	return c.ticks, func() { close(c.stopped) }
}

func (c *runtimeClock) Add(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

func runtimeTestConfig() Config {
	return Config{Simulation: simulation.Config{
		ID: "runtime-test", StartTime: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC), Seed: 1,
		InitialAircraftCount: 0, SpeedHundredths: 100,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: simulation.Range{Min: 50, Max: 50}, LongitudeDegrees: simulation.Range{Min: 14, Max: 14},
			AltitudeFeet: simulation.Range{Min: 35000, Max: 35000}, GroundSpeedKnots: simulation.Range{Min: 0, Max: 0},
			TrackDegrees: simulation.Range{Min: 0, Max: 0}, VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
		},
	}}
}

func TestSimulatorConstructionAndLifecycle(t *testing.T) {
	t.Parallel()

	_, err := New(Config{})
	require.ErrorIs(t, err, simulation.ErrInvalid)
	_, err = newSimulator(runtimeTestConfig(), nil)
	require.ErrorIs(t, err, simulation.ErrInvalid)

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)
	select {
	case <-clock.started:
		t.Fatal("construction started pacing")
	default:
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	<-clock.started
	require.ErrorIs(t, runtime.Run(t.Context()), simdriver.ErrAlreadyStarted)
	require.NoError(t, runtime.SetCount(t.Context(), 1))
	cancel()
	require.NoError(t, <-done)
	<-clock.stopped

	before := runtime.Snapshot()
	require.ErrorIs(t, runtime.SetCount(t.Context(), 0), simdriver.ErrStopped)
	require.ErrorIs(t, runtime.SetSpeed(t.Context(), 0), simdriver.ErrStopped)
	require.Equal(t, before, runtime.Snapshot())
}

func TestSimulatorReadMethodsDoNotSettle(t *testing.T) {
	t.Parallel()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)
	station, err := runtime.AddStation(t.Context(), runtimeStation())
	require.NoError(t, err)
	before := runtime.Snapshot()

	_, err = runtime.ReceptionHistory(t.Context(), simulation.HistoryRequest{StationID: station.Config.ID, Limit: 1})
	require.NoError(t, err)
	_, err = runtime.Observations(t.Context(), simulation.ObservationRequest{
		StationIDs: []string{station.Config.ID},
		Expiry:     simulation.ObservationExpiry{Identity: time.Second, Position: time.Second, Altitude: time.Second, Velocity: time.Second},
	})
	require.NoError(t, err)
	snapshot, err := runtime.ReceptionSnapshot(t.Context(), simulation.ReceptionSnapshotRequest{
		StationIDs: []string{station.Config.ID},
	})
	require.NoError(t, err)
	require.Equal(t, []string{station.Config.ID}, snapshot.StationIDs)
	require.Empty(t, snapshot.Records)
	require.Equal(t, before, runtime.Snapshot())

	clock.Add(time.Hour)
	require.Equal(t, before, runtime.Snapshot())
}

func TestSimulatorPropagatesPacingFailureAndStops(t *testing.T) {
	t.Parallel()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)
	clock.Add(simdriver.MaxCatchUp + time.Nanosecond)
	done := make(chan error, 1)
	go func() { done <- runtime.Run(t.Context()) }()
	<-clock.started
	clock.ticks <- time.Time{}
	err = <-done
	require.ErrorContains(t, err, "catch-up limit")
	require.ErrorIs(t, runtime.SetCount(t.Context(), 1), simdriver.ErrStopped)
	<-clock.stopped
}

func runtimeStation() simulation.StationConfig {
	return simulation.StationConfig{
		ID: "primary", Enabled: true, LatitudeDegrees: 50, LongitudeDegrees: 14,
		SiteElevationMetres: 100, AntennaHeightMetres: 20, AntennaGainDBi: 3,
		SensitivityDBm: -95, SystemLossDB: 2, FrameLossProbability: 0,
	}
}
