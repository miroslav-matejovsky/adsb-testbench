package simulation

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// countingContext reports success for a fixed number of Err calls and fails
// from then on. It lets a test place a cancellation at a chosen depth inside
// one mutation without sleeps or wall-clock reads. Err is monotonic: once it
// fails it never succeeds again.
//
// This is an internal algorithm test seam, not a recommended way to implement
// a public context.
type countingContext struct {
	context.Context

	remaining *int
	err       error
}

func newCountingContext(successes int, err error) countingContext {
	allowed := successes
	return countingContext{Context: context.Background(), remaining: &allowed, err: err}
}

func (c countingContext) Err() error {
	if *c.remaining > 0 {
		*c.remaining--
		return nil
	}
	return c.err
}

// controlPair returns two identically configured engines that have already run
// the same prelude, so a failure injected into one can be compared with the
// other and with an identical valid suffix.
func controlPair(t *testing.T, cfg Config, prelude []operation) (subject, control *Engine) {
	t.Helper()

	subject = newEngine(t, cfg)
	control = newEngine(t, cfg)
	require.Equal(t, runScript(t, control, prelude), runScript(t, subject, prelude))
	return subject, control
}

// requireIdenticalFuture asserts that both engines still have the same visible
// state and produce identical output for an identical suffix. Comparing
// snapshots alone would miss hidden random, parity, carry, and identity state.
func requireIdenticalFuture(t *testing.T, subject, control *Engine) {
	t.Helper()

	require.Equal(t, control.Snapshot(), subject.Snapshot())

	suffix := []operation{
		advanceOp(2 * time.Second),
		countOp(4),
		advanceOp(3500 * time.Millisecond),
		elapseOp(time.Second),
	}
	require.Equal(t, runScript(t, control, suffix), runScript(t, subject, suffix))
}

// TestAtomicityRejectedInput covers invalid commands leaving no trace.
func TestAtomicityRejectedInput(t *testing.T) {
	t.Parallel()

	prelude := []operation{advanceOp(4 * time.Second), countOp(3)}

	rejections := []struct {
		name string
		call func(*Engine) error
	}{
		{"negative advance", func(e *Engine) error {
			_, err := e.Advance(context.Background(), -time.Second)
			return err
		}},
		{"advance above limit", func(e *Engine) error {
			_, err := e.Advance(context.Background(), MaxAdvance+time.Nanosecond)
			return err
		}},
		{"negative elapse", func(e *Engine) error {
			_, err := e.Elapse(context.Background(), -time.Nanosecond)
			return err
		}},
		{"scaled elapse above limit", func(e *Engine) error {
			_, err := e.Elapse(context.Background(), time.Hour)
			return err
		}},
		{"negative count", func(e *Engine) error {
			_, err := e.SetCount(context.Background(), -1)
			return err
		}},
		{"count above limit", func(e *Engine) error {
			_, err := e.SetCount(context.Background(), MaxAircraft+1)
			return err
		}},
		{"speed above limit", func(e *Engine) error {
			return e.SetSpeed(context.Background(), maxSpeedHundredths+1)
		}},
	}

	for _, rejection := range rejections {
		t.Run(rejection.name, func(t *testing.T) {
			t.Parallel()

			subject, control := controlPair(t, validConfig(), prelude)
			err := rejection.call(subject)
			require.Error(t, err)
			requireIdenticalFuture(t, subject, control)
		})
	}
}

// TestAtomicityCancellation covers pre-canceled, deadline-exceeded, and
// late-observed cancellation.
func TestAtomicityCancellation(t *testing.T) {
	t.Parallel()

	prelude := []operation{advanceOp(2 * time.Second)}

	t.Run("pre-canceled", func(t *testing.T) {
		t.Parallel()

		subject, control := controlPair(t, validConfig(), prelude)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		batch, err := subject.Advance(ctx, 5*time.Second)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, batch)
		requireIdenticalFuture(t, subject, control)
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		t.Parallel()

		subject, control := controlPair(t, validConfig(), prelude)
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
		defer cancel()

		batch, err := subject.SetCount(ctx, 6)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, batch)
		requireIdenticalFuture(t, subject, control)
	})

	// Cancellation observed after real staged work must still discard it all.
	for _, depth := range []int{1, 3, 10, 50} {
		t.Run(fmt.Sprintf("late during advance at %d", depth), func(t *testing.T) {
			t.Parallel()

			subject, control := controlPair(t, validConfig(), prelude)
			batch, err := subject.Advance(newCountingContext(depth, context.Canceled), 20*time.Second)
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, batch)
			requireIdenticalFuture(t, subject, control)
		})

		t.Run(fmt.Sprintf("late during creation at %d", depth), func(t *testing.T) {
			t.Parallel()

			subject, control := controlPair(t, validConfig(), prelude)
			batch, err := subject.SetCount(newCountingContext(depth, context.DeadlineExceeded), MaxAircraft)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Nil(t, batch)
			requireIdenticalFuture(t, subject, control)
		})
	}
}

// TestAtomicityExhaustion covers sequence, address, elapsed, and date limits.
func TestAtomicityExhaustion(t *testing.T) {
	t.Parallel()

	t.Run("sequence", func(t *testing.T) {
		t.Parallel()

		engine := newEngine(t, validConfig())
		engine.state.lastSequence = math.MaxUint64 - 2
		before := engine.Snapshot()
		beforeState := engine.state.clone()

		batch, err := engine.Advance(context.Background(), 10*time.Second)
		require.ErrorIs(t, err, ErrLimit)
		require.Nil(t, batch)
		require.Equal(t, before, engine.Snapshot())
		require.Equal(t, beforeState.fleet, engine.state.fleet)
		require.Equal(t, beforeState.lastSequence, engine.state.lastSequence)
	})

	t.Run("address", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.InitialAircraftCount = 1
		engine := newEngine(t, cfg)
		engine.state.identity.nextAddress = maxAddress
		before := engine.Snapshot()
		beforeIdentity := engine.state.identity

		batch, err := engine.SetCount(context.Background(), 3)
		require.ErrorIs(t, err, ErrLimit)
		require.Nil(t, batch)
		require.Equal(t, before, engine.Snapshot())
		require.Equal(t, beforeIdentity, engine.state.identity)
		require.Len(t, engine.state.fleet, 1)
	})

	t.Run("elapsed duration", func(t *testing.T) {
		t.Parallel()

		engine := newEngine(t, validConfig())
		engine.state.clock.elapsed = time.Duration(math.MaxInt64) - time.Second
		before := engine.Snapshot()

		batch, err := engine.Advance(context.Background(), 2*time.Second)
		require.ErrorIs(t, err, ErrLimit)
		require.Nil(t, batch)
		require.Equal(t, before, engine.Snapshot())
	})

	t.Run("representable date", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.StartTime = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)
		engine := newEngine(t, cfg)
		before := engine.Snapshot()

		batch, err := engine.Advance(context.Background(), 2*time.Second)
		require.ErrorIs(t, err, ErrLimit)
		require.Nil(t, batch)
		require.Equal(t, before, engine.Snapshot())

		// The remaining representable time is still usable.
		_, err = engine.Advance(context.Background(), 500*time.Millisecond)
		require.NoError(t, err)
	})

	t.Run("batch bound", func(t *testing.T) {
		t.Parallel()

		engine := newEngine(t, validConfig())
		var batch []Transmission
		for range MaxBatchFrames {
			batch = append(batch, Transmission{})
		}
		err := engine.state.record(&engine.state.fleet[0], PositionMessage,
			engine.state.fleet[0].navAt(0), false, 0, &batch)
		require.ErrorIs(t, err, ErrLimit)
	})
}

// TestAtomicityCodecFailure covers a wrapped codec error raised after staged
// work, leaving committed state untouched.
func TestAtomicityCodecFailure(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 1
	engine := newEngine(t, cfg)

	// A callsign that cannot be encoded can only be reached by corrupting
	// internal state; no valid configuration produces one.
	engine.state.fleet[0].callsign = "not a valid callsign"
	before := engine.Snapshot()
	beforeState := engine.state.clone()

	batch, err := engine.Advance(context.Background(), 10*time.Second)
	require.Error(t, err)
	require.Nil(t, batch)
	require.ErrorContains(t, err, "identification report")
	require.ErrorContains(t, err, "not a valid callsign")

	require.Equal(t, before, engine.Snapshot())
	require.Equal(t, beforeState.fleet, engine.state.fleet)
	require.Equal(t, beforeState.lastSequence, engine.state.lastSequence)
	require.Equal(t, beforeState.clock, engine.state.clock)
}
