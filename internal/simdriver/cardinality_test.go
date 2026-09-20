package simdriver_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Pacing at the maximum configured cardinality.
//
// The existing pacing tests use small fleets. These cover the same rules with
// the largest fleet and station count the engine accepts, in durations small
// enough for the ordinary suite. The expensive sustained runs belong to the
// measurement task.

// newFullRig returns a driver over an engine with the maximum fleet and the
// maximum number of co-located all-receiving stations.
func newFullRig(t *testing.T, speed uint16) rig {
	t.Helper()

	r := newRig(t, simulation.MaxAircraft, speed)
	for i := range simulation.MaxStations {
		_, err := r.driver.AddStation(t.Context(), testStation(string(rune('a'+i))))
		require.NoError(t, err)
	}
	r.count = simulation.MaxAircraft
	return r
}

// requireReceiving asserts that every station heard something, so a timing
// assertion cannot pass on an engine that quietly produced no work.
func requireReceiving(t *testing.T, r rig) {
	t.Helper()

	snapshot := r.receptions(t)
	require.Len(t, snapshot.Retention, simulation.MaxStations)
	for _, entry := range snapshot.Retention {
		require.Positive(t, entry.LatestSequence, "station %s heard nothing", entry.StationID)
	}
}

func TestDriverFullCardinalityPacing(t *testing.T) {
	t.Parallel()

	t.Run("pause and resume", func(t *testing.T) {
		t.Parallel()

		r := newFullRig(t, 100)
		r.clock.Add(time.Second)
		require.NoError(t, r.driver.SetSpeed(t.Context(), 0))
		require.Equal(t, time.Second, r.engine.Snapshot().Elapsed)

		r.clock.Add(time.Hour)
		require.NoError(t, r.driver.SetSpeed(t.Context(), 100), "paused real time is discarded")
		require.Equal(t, time.Second, r.engine.Snapshot().Elapsed)

		r.clock.Add(500 * time.Millisecond)
		require.NoError(t, r.driver.SetCount(t.Context(), r.count))
		require.Equal(t, 1500*time.Millisecond, r.engine.Snapshot().Elapsed)
		requireReceiving(t, r)
	})

	t.Run("fractional speed carries", func(t *testing.T) {
		t.Parallel()

		// 33 hundredths of 10 ms is 3.3 ms, so every tick leaves a remainder
		// that the next one must carry.
		r := newFullRig(t, 33)
		for range 10 {
			r.clock.Add(10 * time.Millisecond)
			require.NoError(t, r.driver.SetCount(t.Context(), r.count))
		}
		require.Equal(t, 33*time.Millisecond, r.engine.Snapshot().Elapsed)
	})

	t.Run("delayed wakeup settles one interval", func(t *testing.T) {
		t.Parallel()

		r := newFullRig(t, 100)
		r.clock.Add(37 * simdriver.Heartbeat)
		require.NoError(t, r.driver.SetCount(t.Context(), r.count))
		require.Equal(t, 37*simdriver.Heartbeat, r.engine.Snapshot().Elapsed,
			"a late wakeup delivers the whole measured interval once")
		requireReceiving(t, r)
	})

	t.Run("exact catch-up limit", func(t *testing.T) {
		t.Parallel()

		r := newFullRig(t, 100)
		r.clock.Add(simulation.MaxAdvance)
		require.NoError(t, r.driver.SetCount(t.Context(), r.count))
		require.Equal(t, simulation.MaxAdvance, r.engine.Snapshot().Elapsed)
	})

	t.Run("over the catch-up limit", func(t *testing.T) {
		t.Parallel()

		r := newFullRig(t, simulation.MaxSpeedHundredths)
		r.clock.Add(simdriver.MaxCatchUp*100/time.Duration(simulation.MaxSpeedHundredths) + time.Nanosecond)
		before := r.engine.Snapshot()
		require.ErrorContains(t, r.driver.SetCount(t.Context(), r.count), "catch-up limit")
		require.Equal(t, before, r.engine.Snapshot(), "a refused backlog changes nothing")
	})

	t.Run("cancellation between committed chunks", func(t *testing.T) {
		t.Parallel()

		r := newFullRig(t, 100)
		r.clock.Add(2 * simulation.MaxAdvance)
		err := r.driver.SetCount(&cancelAfter{Context: context.Background(), checks: 4}, r.count)
		require.ErrorIs(t, err, context.Canceled)

		// A canceled chunk is discarded whole, so whatever was committed is a
		// multiple of the chunk size and the rest is still pending. The exact
		// depth depends on how often the engine checks the context, which is
		// not a promise of this package.
		settled := r.engine.Snapshot().Elapsed
		require.Zero(t, settled%simulation.MaxAdvance, "only complete chunks are committed")
		require.Less(t, settled, 2*simulation.MaxAdvance)

		require.NoError(t, r.driver.SetCount(t.Context(), r.count))
		require.Equal(t, 2*simulation.MaxAdvance, r.engine.Snapshot().Elapsed)
		requireReceiving(t, r)
	})
}
