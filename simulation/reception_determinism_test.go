package simulation

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Station operations, so station commands can appear inside the same ordered
// scripts already used for replay and partition tests.

func addStationOp(cfg StationConfig) operation {
	return operation{"add station " + cfg.ID, func(e *Engine) (Batch, error) {
		_, err := e.AddStation(context.Background(), cfg)
		return newBatch(), err
	}}
}

func updateStationOp(expectedRevision uint64, cfg StationConfig) operation {
	return operation{"update station " + cfg.ID, func(e *Engine) (Batch, error) {
		_, err := e.UpdateStation(context.Background(), expectedRevision, cfg)
		return newBatch(), err
	}}
}

func removeStationOp(id string, expectedRevision uint64) operation {
	return operation{"remove station " + id, func(e *Engine) (Batch, error) {
		return newBatch(), e.RemoveStation(context.Background(), id, expectedRevision)
	}}
}

// receivingScript is one fixed ordered script that mixes time, aircraft, and
// station commands. Revisions are fixed because the script is fixed.
func receivingScript() []operation {
	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.35

	disabled := lossy
	disabled.Enabled = false

	return []operation{
		addStationOp(lossy),
		addStationOp(insensitiveStationConfig()),
		advanceOp(3 * time.Second),
		countOp(5),
		elapseOp(2 * time.Second),
		updateStationOp(1, disabled),
		advanceOp(4 * time.Second),
		updateStationOp(2, lossy),
		speedOp(250),
		elapseOp(1500 * time.Millisecond),
		addStationOp(namedStation("late")),
		removeStationOp("insensitive", 1),
		countOp(3),
		advanceOp(4500 * time.Millisecond),
	}
}

// TestReceptionDeterminismReplay covers identical configuration and ordered
// calls producing identical receptions and identical final state.
func TestReceptionDeterminismReplay(t *testing.T) {
	t.Parallel()

	script := receivingScript()

	first := newEngine(t, validConfig())
	second := newEngine(t, validConfig())

	firstBatch := runScript(t, first, script)
	secondBatch := runScript(t, second, script)

	require.NotEmpty(t, firstBatch.Receptions)
	require.Equal(t, secondBatch, firstBatch)
	require.Equal(t, second.Snapshot(), first.Snapshot())

	// A different seed changes the reception stream as well as the frames.
	other := validConfig()
	other.Seed++
	different := runScript(t, newEngine(t, other), script)
	require.NotEqual(t, firstBatch.Transmissions, different.Transmissions)
	require.NotEqual(t, frameSet(firstBatch.Receptions), frameSet(different.Receptions))
}

// TestReceptionDeterminismPartitions covers split and combined time calls
// producing identical receptions.
func TestReceptionDeterminismPartitions(t *testing.T) {
	t.Parallel()

	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.3

	withStations := func(t testing.TB, cfg Config) *Engine {
		t.Helper()

		engine := newEngine(t, cfg)
		addStation(t, engine, lossy)
		addStation(t, engine, insensitiveStationConfig())
		addStation(t, engine, disabledStationConfig())
		return engine
	}

	t.Run("virtual", func(t *testing.T) {
		t.Parallel()

		const total = 30 * time.Second

		control := withStations(t, validConfig())
		combined, err := control.Advance(context.Background(), total)
		require.NoError(t, err)
		require.NotEmpty(t, combined.Receptions)
		want := control.Snapshot()

		probe := newEngine(t, validConfig())
		exact := probe.state.fleet[0].positionDeadline

		partitions := [][]time.Duration{
			{total},
			{10 * time.Second, 10 * time.Second, 10 * time.Second},
			{0, total, 0},
			{time.Nanosecond, 7*time.Second - time.Nanosecond, 23 * time.Second},
			{exact, total - exact},
			{exact - time.Nanosecond, time.Nanosecond, total - exact},
		}
		for i := range 10 {
			step := time.Second + time.Duration(i)*137*time.Millisecond
			var plan []time.Duration
			remaining := total
			for remaining > 0 {
				part := min(step, remaining)
				plan = append(plan, part)
				remaining -= part
			}
			partitions = append(partitions, plan)
		}

		for _, plan := range partitions {
			split := withStations(t, validConfig())
			require.Equal(t, combined, advanceAll(t, split, plan), "plan %v", plan)
			require.Equal(t, want, split.Snapshot(), "plan %v", plan)
		}
	})

	t.Run("scaled", func(t *testing.T) {
		t.Parallel()

		// 500 ms keeps 100x scaling inside MaxAdvance.
		const total = 500 * time.Millisecond

		for _, speed := range []uint16{1, 100, 10000} {
			cfg := validConfig()
			cfg.SpeedHundredths = speed

			control := withStations(t, cfg)
			combined, err := control.Elapse(context.Background(), total)
			require.NoError(t, err)
			want := control.Snapshot()

			source := rand.New(rand.NewPCG(uint64(speed), 19))
			partitions := [][]time.Duration{
				{total},
				{250 * time.Millisecond, 250 * time.Millisecond},
				{1, 1, 1, total - 3},
				{0, total, 0},
			}
			for range 5 {
				var plan []time.Duration
				remaining := total
				for remaining > 0 {
					part := time.Duration(source.Int64N(int64(remaining)) + 1)
					plan = append(plan, part)
					remaining -= part
				}
				partitions = append(partitions, plan)
			}

			for _, plan := range partitions {
				split := withStations(t, cfg)
				require.Equal(t, combined, elapseAll(t, split, plan), "speed %d", speed)
				require.Equal(t, want, split.Snapshot(), "speed %d", speed)
			}
		}
	})
}

// TestReceptionFrameLoss covers the configured impairment: both extremes, and
// an intermediate value reproducing exactly across engines and split calls.
func TestReceptionFrameLoss(t *testing.T) {
	t.Parallel()

	lossy := validStationConfig()
	lossy.FrameLossProbability = 0.5

	control := newEngine(t, validConfig())
	addStation(t, control, lossy)
	wanted, err := control.Advance(t.Context(), 10*time.Second)
	require.NoError(t, err)

	require.NotEmpty(t, wanted.Receptions)
	require.Less(t, len(wanted.Receptions), len(wanted.Transmissions),
		"an intermediate probability drops some frames")

	// A second engine reproduces the same drops exactly.
	twin := newEngine(t, validConfig())
	addStation(t, twin, lossy)
	twinBatch, err := twin.Advance(t.Context(), 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, wanted.Receptions, twinBatch.Receptions)

	// Splitting the call reproduces them too.
	split := newEngine(t, validConfig())
	addStation(t, split, lossy)
	splitBatch := advanceAll(t, split, []time.Duration{
		time.Second, 2500 * time.Millisecond, time.Nanosecond,
		6500*time.Millisecond - time.Nanosecond,
	})
	require.Equal(t, wanted, splitBatch)

	// Neither extreme changes a single transmission.
	bare := newEngine(t, validConfig())
	bareBatch, err := bare.Advance(t.Context(), 10*time.Second)
	require.NoError(t, err)
	require.Equal(t, bareBatch.Transmissions, wanted.Transmissions)
}
