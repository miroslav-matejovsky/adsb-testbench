package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

func newEngine(t testing.TB, cfg Config) *Engine {
	t.Helper()

	engine, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)
	return engine
}

// kindsOf lists the message families of a batch in order.
func kindsOf(batch []Transmission) []MessageKind {
	kinds := make([]MessageKind, len(batch))
	for i, t := range batch {
		kinds[i] = t.Kind
	}
	return kinds
}

// requireFutureDeadlines asserts the post-commit invariant that no aircraft
// is still due at or before the committed time.
func requireFutureDeadlines(t testing.TB, engine *Engine) {
	t.Helper()

	for i := range engine.state.fleet {
		for _, kind := range families {
			require.Greater(t, engine.state.fleet[i].deadline(kind), engine.state.clock.elapsed)
		}
	}
}

func TestEngineNewCreatesFleet(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	engine := newEngine(t, cfg)
	snapshot := engine.Snapshot()

	require.Len(t, snapshot.Aircraft, cfg.InitialAircraftCount)
	require.Equal(t, cfg.InitialAircraftCount, snapshot.Config.InitialAircraftCount)
	require.Equal(t, time.Duration(0), snapshot.Elapsed)
	require.True(t, snapshot.Now.Equal(fixtureStart))

	require.Len(t, snapshot.History.Messages, 3*cfg.InitialAircraftCount)
	require.Equal(t, uint64(1), snapshot.History.OldestSequence)
	require.Equal(t, uint64(3*cfg.InitialAircraftCount), snapshot.History.LatestSequence)
	require.Equal(t, HistoryLimit, snapshot.History.Limit)

	for i, craft := range snapshot.Aircraft {
		reports := snapshot.History.Messages[3*i : 3*i+3]
		require.Equal(t, []MessageKind{IdentificationMessage, PositionMessage, VelocityMessage}, kindsOf(reports))
		for _, report := range reports {
			require.Equal(t, craft.ICAO, report.ICAO)
			require.True(t, report.Timestamp.Equal(fixtureStart))
		}

		// The creation position report is the even half of the CPR pair.
		message, err := adsb.Decode(reports[1].Frame[:])
		require.NoError(t, err)
		require.NotNil(t, message.Position)
		require.False(t, message.Position.CPR.Odd)
	}

	requireFutureDeadlines(t, engine)
}

func TestEngineNewRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.ID = ""
	engine, err := New(cfg)
	require.ErrorIs(t, err, ErrInvalid)
	require.Nil(t, engine)
}

func TestEngineNewWithoutAircraft(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 0
	snapshot := newEngine(t, cfg).Snapshot()

	require.Empty(t, snapshot.Aircraft)
	require.Empty(t, snapshot.History.Messages)
	require.Zero(t, snapshot.History.OldestSequence)
	require.Zero(t, snapshot.History.LatestSequence)
}

func TestEngineAdvanceEmitsOrderedFrames(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.Snapshot()

	batch, err := engine.Advance(t.Context(), 10*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch)

	previousSequence := before.History.LatestSequence
	previousTime := before.Now
	for _, report := range batch {
		require.Equal(t, previousSequence+1, report.Sequence)
		previousSequence = report.Sequence
		require.False(t, report.Timestamp.Before(previousTime))
		previousTime = report.Timestamp
		require.False(t, report.Timestamp.After(before.Now.Add(10*time.Second)))

		message, err := adsb.Decode(report.Frame[:])
		require.NoError(t, err)
		require.Equal(t, report.ICAO, message.Header.ICAO)
	}

	after := engine.Snapshot()
	require.Equal(t, 10*time.Second, after.Elapsed)
	requireFutureDeadlines(t, engine)
}

// Position reports alternate parity and unrelated families never disturb it.
func TestEngineAlternatesPositionParity(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 1
	engine := newEngine(t, cfg)

	batch, err := engine.Advance(t.Context(), 20*time.Second)
	require.NoError(t, err)

	wantOdd := true
	positions := 0
	for _, report := range batch {
		if report.Kind != PositionMessage {
			continue
		}
		message, err := adsb.Decode(report.Frame[:])
		require.NoError(t, err)
		require.Equal(t, wantOdd, message.Position.CPR.Odd)
		wantOdd = !wantOdd
		positions++
	}
	require.Greater(t, positions, 30)
}

func TestEngineZeroDurationIsANoOp(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.Snapshot()
	beforeState := engine.state.clone()

	batch, err := engine.Advance(t.Context(), 0)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Empty(t, batch)

	batch, err = engine.Elapse(t.Context(), 0)
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Empty(t, batch)

	require.Equal(t, before, engine.Snapshot())
	require.Equal(t, beforeState.fleet, engine.state.fleet)
	require.Equal(t, beforeState.identity, engine.state.identity)
	require.Equal(t, beforeState.lastSequence, engine.state.lastSequence)
	require.Equal(t, beforeState.clock, engine.state.clock)
}

func TestEngineElapseScalesRealTime(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.SpeedHundredths = 200
	engine := newEngine(t, cfg)

	_, err := engine.Elapse(t.Context(), 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, engine.Snapshot().Elapsed)
}

func TestEngineSetCountAddsAndRemoves(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 2
	engine := newEngine(t, cfg)

	_, err := engine.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	now := engine.Snapshot().Now

	added, err := engine.SetCount(t.Context(), 5)
	require.NoError(t, err)
	require.Len(t, added, 9)
	for _, report := range added {
		require.True(t, report.Timestamp.Equal(now))
	}

	snapshot := engine.Snapshot()
	require.Len(t, snapshot.Aircraft, 5)
	for _, craft := range snapshot.Aircraft[2:] {
		require.True(t, craft.CreatedAt.Equal(now))
	}
	requireFutureDeadlines(t, engine)

	survivors := snapshot.Aircraft[:3]
	removed, err := engine.SetCount(t.Context(), 3)
	require.NoError(t, err)
	require.NotNil(t, removed)
	require.Empty(t, removed)

	reduced := engine.Snapshot()
	require.Equal(t, survivors, reduced.Aircraft)
	// Removal never discards retained transmissions.
	require.Equal(t, snapshot.History.Messages, reduced.History.Messages)
}

func TestEngineSetCountNoOpChangesNothing(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.state.clone()

	batch, err := engine.SetCount(t.Context(), len(before.fleet))
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Empty(t, batch)

	require.Equal(t, before.fleet, engine.state.fleet)
	require.Equal(t, before.identity, engine.state.identity)
	require.Equal(t, before.lastSequence, engine.state.lastSequence)
}

func TestEngineSetCountRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.Snapshot()

	for _, count := range []int{-1, MaxAircraft + 1} {
		batch, err := engine.SetCount(t.Context(), count)
		require.ErrorIs(t, err, ErrInvalid)
		require.Nil(t, batch)
	}
	require.Equal(t, before, engine.Snapshot())
}

// SetSpeed changes only later scaling: no reports, no time, no carry change.
func TestEngineSetSpeed(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	_, err := engine.Elapse(t.Context(), 7*time.Nanosecond)
	require.NoError(t, err)
	before := engine.Snapshot()
	carry := engine.state.clock.carry

	require.NoError(t, engine.SetSpeed(t.Context(), 0))
	after := engine.Snapshot()
	require.Equal(t, uint16(0), after.Config.SpeedHundredths)
	require.Equal(t, before.Elapsed, after.Elapsed)
	require.Equal(t, before.Aircraft, after.Aircraft)
	require.Equal(t, before.History.Messages, after.History.Messages)
	require.Equal(t, carry, engine.state.clock.carry)

	require.ErrorIs(t, engine.SetSpeed(t.Context(), maxSpeedHundredths+1), ErrInvalid)
	require.Equal(t, uint16(0), engine.Snapshot().Config.SpeedHundredths)
}

// A paused engine emits nothing for supplied real time but still steps
// directly with Advance.
func TestEnginePauseAndResume(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.SpeedHundredths = 0
	engine := newEngine(t, cfg)

	batch, err := engine.Elapse(t.Context(), time.Minute)
	require.NoError(t, err)
	require.Empty(t, batch)
	require.Equal(t, time.Duration(0), engine.Snapshot().Elapsed)

	batch, err = engine.Advance(t.Context(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch)
	require.Equal(t, 2*time.Second, engine.Snapshot().Elapsed)

	require.NoError(t, engine.SetSpeed(t.Context(), 100))
	_, err = engine.Elapse(t.Context(), time.Second)
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, engine.Snapshot().Elapsed)
}

func TestEngineRejectsInvalidDurations(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.Snapshot()

	batch, err := engine.Advance(t.Context(), -time.Nanosecond)
	require.ErrorIs(t, err, ErrInvalid)
	require.Nil(t, batch)

	batch, err = engine.Advance(t.Context(), MaxAdvance+time.Nanosecond)
	require.ErrorIs(t, err, ErrLimit)
	require.Nil(t, batch)

	batch, err = engine.Elapse(t.Context(), -time.Second)
	require.ErrorIs(t, err, ErrInvalid)
	require.Nil(t, batch)

	batch, err = engine.Elapse(t.Context(), time.Hour)
	require.ErrorIs(t, err, ErrLimit)
	require.Nil(t, batch)

	require.Equal(t, before, engine.Snapshot())
}

func TestEngineCancelledContext(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	before := engine.Snapshot()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	batch, err := engine.Advance(ctx, time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, batch)

	batch, err = engine.Elapse(ctx, time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, batch)

	batch, err = engine.SetCount(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, batch)

	require.ErrorIs(t, engine.SetSpeed(ctx, 50), context.Canceled)
	require.Equal(t, before, engine.Snapshot())
}
