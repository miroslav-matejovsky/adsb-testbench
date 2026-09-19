package simdriver_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/stretchr/testify/require"
)

var (
	realStart    = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	virtualStart = time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
)

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	ticks   chan time.Time
	stopped atomic.Bool
}

type cancelAfter struct {
	context.Context
	checks int
}

func (c *cancelAfter) Err() error {
	if c.checks == 0 {
		return context.Canceled
	}
	c.checks--
	return nil
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: realStart, ticks: make(chan time.Time)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Add(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

func (c *fakeClock) NewTicker(time.Duration) (<-chan time.Time, func()) {
	return c.ticks, func() { c.stopped.Store(true) }
}

func testConfig(count int, speed uint16) simulation.Config {
	return simulation.Config{
		ID: "driver-test", StartTime: virtualStart, Seed: 7,
		InitialAircraftCount: count, SpeedHundredths: speed,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: simulation.Range{Min: 50, Max: 50}, LongitudeDegrees: simulation.Range{Min: 14, Max: 14},
			AltitudeFeet: simulation.Range{Min: 35000, Max: 35000}, GroundSpeedKnots: simulation.Range{Min: 0, Max: 0},
			TrackDegrees: simulation.Range{Min: 0, Max: 0}, VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
		},
	}
}

func newDriver(t testing.TB, count int, speed uint16) (*simulation.Engine, *fakeClock, *simdriver.Driver) {
	t.Helper()
	engine, err := simulation.New(testConfig(count, speed))
	require.NoError(t, err)
	clock := newFakeClock()
	return engine, clock, simdriver.New(engine, clock)
}

func runDriver(t testing.TB, clock *fakeClock, driver *simdriver.Driver) (func(), func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx) }()
	wake := func() {
		clock.ticks <- time.Time{}
		clock.ticks <- time.Time{}
	}
	stop := func() error {
		cancel()
		return <-done
	}
	return wake, stop
}

func TestDriverUsesMeasuredElapsedTime(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 1, 100)
	wake, stop := runDriver(t, clock, driver)
	clock.Add(simdriver.Heartbeat)
	wake()
	require.Equal(t, simdriver.Heartbeat, engine.Snapshot().Elapsed)

	clock.Add(2350 * time.Millisecond)
	wake()
	require.Equal(t, 2450*time.Millisecond, engine.Snapshot().Elapsed)
	require.NoError(t, stop())
	require.True(t, clock.stopped.Load())
}

func TestDriverCommandsSettleAtPreviousSpeed(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 1, 100)
	clock.Add(1500 * time.Millisecond)
	require.NoError(t, driver.SetSpeed(t.Context(), 200))
	require.Equal(t, 1500*time.Millisecond, engine.Snapshot().Elapsed)
	require.Equal(t, uint16(200), engine.Snapshot().Config.SpeedHundredths)

	clock.Add(250 * time.Millisecond)
	require.NoError(t, driver.SetCount(t.Context(), 2))
	snapshot := engine.Snapshot()
	require.Equal(t, 2*time.Second, snapshot.Elapsed)
	require.Len(t, snapshot.Aircraft, 2)
	require.Equal(t, virtualStart.Add(2*time.Second), snapshot.Aircraft[1].CreatedAt)

	stationConfig := testStation("primary")
	clock.Add(500 * time.Millisecond)
	station, err := driver.AddStation(t.Context(), stationConfig)
	require.NoError(t, err)
	require.Equal(t, virtualStart.Add(3*time.Second), station.CreatedAt)
}

func TestDriverPauseDropsRealTime(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 0, 100)
	wake, stop := runDriver(t, clock, driver)
	clock.Add(time.Second)
	require.NoError(t, driver.SetSpeed(t.Context(), 0))
	require.Equal(t, time.Second, engine.Snapshot().Elapsed)

	clock.Add(3 * simdriver.MaxCatchUp)
	wake()
	require.NoError(t, driver.SetSpeed(t.Context(), 100))
	require.Equal(t, time.Second, engine.Snapshot().Elapsed)

	clock.Add(500 * time.Millisecond)
	wake()
	require.Equal(t, 1500*time.Millisecond, engine.Snapshot().Elapsed)
	require.NoError(t, stop())
}

func TestDriverCatchUpLimitAndClockRegression(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 0, simulation.MaxSpeedHundredths)
	clock.Add(-time.Second)
	require.NoError(t, driver.SetCount(t.Context(), 0))
	require.Zero(t, engine.Snapshot().Elapsed)
	clock.Add(1500 * time.Millisecond)
	require.NoError(t, driver.SetCount(t.Context(), 0))
	require.Equal(t, 50*time.Second, engine.Snapshot().Elapsed)

	before := engine.Snapshot()
	clock.Add(simdriver.MaxCatchUp*100/time.Duration(simulation.MaxSpeedHundredths) + time.Nanosecond)
	require.ErrorContains(t, driver.SetCount(t.Context(), 1), "catch-up limit")
	require.Equal(t, before, engine.Snapshot())
}

func TestDriverAcceptsCatchUpLimitAndCancelsBetweenChunks(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 0, 100)
	clock.Add(simdriver.MaxCatchUp)
	require.NoError(t, driver.SetCount(t.Context(), 0))
	require.Equal(t, simdriver.MaxCatchUp, engine.Snapshot().Elapsed)

	engine, clock, driver = newDriver(t, 0, 100)
	clock.Add(2 * simulation.MaxAdvance)
	err := driver.SetCount(&cancelAfter{Context: t.Context(), checks: 4}, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, simulation.MaxAdvance, engine.Snapshot().Elapsed)
	require.Empty(t, engine.Snapshot().Aircraft)
	require.NoError(t, driver.SetCount(t.Context(), 1))
	require.Equal(t, 2*simulation.MaxAdvance, engine.Snapshot().Elapsed)
	require.Len(t, engine.Snapshot().Aircraft, 1)
}

func TestDriverInvalidAndCancelledCommandsDoNotSettle(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 0, 100)
	clock.Add(5 * time.Second)
	before := engine.Snapshot()
	require.ErrorIs(t, driver.SetCount(t.Context(), -1), simulation.ErrInvalid)
	require.ErrorIs(t, driver.SetSpeed(t.Context(), simulation.MaxSpeedHundredths+1), simulation.ErrInvalid)
	invalidStation := testStation("")
	_, err := driver.AddStation(t.Context(), invalidStation)
	require.ErrorIs(t, err, simulation.ErrInvalid)
	require.Equal(t, before, engine.Snapshot())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, driver.SetCount(ctx, 1), context.Canceled)
	require.Equal(t, before, engine.Snapshot())
}

func TestDriverRunCancellationAndTerminalCommands(t *testing.T) {
	t.Parallel()

	engine, clock, driver := newDriver(t, 0, 100)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, driver.Run(ctx))
	require.True(t, clock.stopped.Load())
	require.ErrorIs(t, driver.Run(t.Context()), simdriver.ErrAlreadyStarted)
	require.ErrorIs(t, driver.SetCount(t.Context(), 1), simdriver.ErrStopped)
	require.ErrorIs(t, driver.SetSpeed(t.Context(), 0), simdriver.ErrStopped)
	_, err := driver.AddStation(t.Context(), testStation("primary"))
	require.ErrorIs(t, err, simdriver.ErrStopped)
	_, err = driver.UpdateStation(t.Context(), 1, testStation("primary"))
	require.ErrorIs(t, err, simdriver.ErrStopped)
	require.ErrorIs(t, driver.RemoveStation(t.Context(), "primary", 1), simdriver.ErrStopped)
	require.Empty(t, engine.Snapshot().Aircraft)
}

func testStation(id string) simulation.StationConfig {
	return simulation.StationConfig{
		ID: id, Enabled: true, LatitudeDegrees: 50, LongitudeDegrees: 14,
		SiteElevationMetres: 100, AntennaHeightMetres: 20, AntennaGainDBi: 3,
		SensitivityDBm: -95, SystemLossDB: 2, FrameLossProbability: 0,
	}
}
