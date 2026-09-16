package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHistoryRingRetainsNewest(t *testing.T) {
	t.Parallel()

	ring := newHistory()
	require.Equal(t, HistorySnapshot{Messages: []Transmission{}, Limit: HistoryLimit}, ring.snapshot())

	for i := 1; i <= HistoryLimit+250; i++ {
		ring.append(Transmission{Sequence: uint64(i)})
	}

	got := ring.snapshot()
	require.Len(t, got.Messages, HistoryLimit)
	require.Equal(t, uint64(251), got.OldestSequence)
	require.Equal(t, uint64(HistoryLimit+250), got.LatestSequence)
	require.Equal(t, HistoryLimit, got.Limit)

	for i, message := range got.Messages {
		require.Equal(t, uint64(251+i), message.Sequence)
	}
}

func TestHistoryRingBelowCapacity(t *testing.T) {
	t.Parallel()

	ring := newHistory()
	for i := 1; i <= 3; i++ {
		ring.append(Transmission{Sequence: uint64(i)})
	}

	got := ring.snapshot()
	require.Len(t, got.Messages, 3)
	require.Equal(t, uint64(1), got.OldestSequence)
	require.Equal(t, uint64(3), got.LatestSequence)
}

// A cloned ring shares no storage with the original.
func TestHistoryCloneIsDetached(t *testing.T) {
	t.Parallel()

	ring := newHistory()
	ring.append(Transmission{Sequence: 1})

	copied := ring.clone()
	copied.append(Transmission{Sequence: 2})

	require.Len(t, ring.snapshot().Messages, 1)
	require.Len(t, copied.snapshot().Messages, 2)
}

// A returned snapshot owns its slice.
func TestHistorySnapshotIsOwnedByCaller(t *testing.T) {
	t.Parallel()

	ring := newHistory()
	ring.append(Transmission{Sequence: 1, ICAO: 7})

	got := ring.snapshot()
	got.Messages[0].ICAO = 99
	require.Equal(t, uint32(7), ring.snapshot().Messages[0].ICAO)
}

// A batch larger than retention stays complete, while history keeps exactly
// its tail.
func TestHistoryCompleteBatch(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 10
	engine := newEngine(t, cfg)

	got, err := engine.Advance(t.Context(), MaxAdvance)
	require.NoError(t, err)
	batch := got.Transmissions
	require.Greater(t, len(batch), HistoryLimit)
	require.LessOrEqual(t, len(batch), MaxBatchFrames)

	// The batch is a gapless run of sequences.
	for i, report := range batch {
		require.Equal(t, batch[0].Sequence+uint64(i), report.Sequence)
	}

	snapshot := engine.Snapshot()
	require.Len(t, snapshot.History.Messages, HistoryLimit)
	require.Equal(t, batch[len(batch)-HistoryLimit:], snapshot.History.Messages)
	require.Equal(t, batch[len(batch)-1].Sequence, snapshot.History.LatestSequence)
	require.Equal(t, batch[len(batch)-HistoryLimit].Sequence, snapshot.History.OldestSequence)
}

// A consumer detects lost retention by comparing its own progress with the
// reported bounds within the same run.
func TestHistoryGapDetection(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 10
	engine := newEngine(t, cfg)

	processed := engine.Snapshot().History.LatestSequence
	_, err := engine.Advance(t.Context(), 30*time.Second)
	require.NoError(t, err)

	history := engine.Snapshot().History
	require.Greater(t, history.OldestSequence, processed+1, "retention was exceeded")
}
