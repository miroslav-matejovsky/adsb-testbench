package simulation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReceptionSnapshotIsEmptyForAnEmptySelection(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	got, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{}})
	require.NoError(t, err)
	require.Equal(t, "fixture", got.RunID)
	require.Equal(t, fixtureStart.Add(time.Second), got.Now)
	require.Empty(t, got.StationIDs)
	require.Empty(t, got.Retention)
	require.Empty(t, got.Records)
	require.NotNil(t, got.StationIDs)
	require.NotNil(t, got.Retention)
	require.NotNil(t, got.Records)
}

func TestReceptionSnapshotOrdersRecordsAndDeduplicatesStations(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "bravo")
	addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), 2*time.Second)
	require.NoError(t, err)

	got, err := engine.ReceptionSnapshot(t.Context(),
		ReceptionSnapshotRequest{StationIDs: []string{"bravo", "alpha", "bravo"}})
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "bravo"}, got.StationIDs)
	require.NotEmpty(t, got.Records)

	for i := 1; i < len(got.Records); i++ {
		previous, current := got.Records[i-1], got.Records[i]
		if previous.TransmissionSequence == current.TransmissionSequence {
			require.Less(t, previous.StationID, current.StationID)
			continue
		}
		require.Less(t, previous.TransmissionSequence, current.TransmissionSequence)
	}
	for _, record := range got.Records {
		require.False(t, record.Timestamp.After(got.Now))
		require.Equal(t, record.StationID, record.Receiver.Config.ID)
		require.Equal(t, record.StationRevision, record.Receiver.Revision)
	}
}

func TestReceptionSnapshotRetentionMatchesCapturedRecords(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	addNamedStation(t, engine, "disabled-station")
	require.NoError(t, disableStation(t, engine, "disabled-station"))
	_, err := engine.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)

	got, err := engine.ReceptionSnapshot(t.Context(),
		ReceptionSnapshotRequest{StationIDs: []string{"alpha", "disabled-station"}})
	require.NoError(t, err)
	require.Len(t, got.Retention, 2)

	for _, retention := range got.Retention {
		var sequences []uint64
		for _, record := range got.Records {
			if record.StationID == retention.StationID {
				sequences = append(sequences, record.Sequence)
			}
		}
		require.Equal(t, ReceptionHistoryLimit, retention.Limit)
		if len(sequences) == 0 {
			require.Zero(t, retention.OldestSequence)
			require.Zero(t, retention.LatestSequence)
			require.False(t, retention.Truncated)
			continue
		}
		require.Equal(t, retention.OldestSequence, sequences[0])
		require.Equal(t, retention.LatestSequence, sequences[len(sequences)-1])
		for i := 1; i < len(sequences); i++ {
			require.Equal(t, sequences[i-1]+1, sequences[i])
		}
	}
	require.Empty(t, filterStation(got.Records, "disabled-station"))
}

func TestReceptionSnapshotRejectsUnknownAndRemovedStations(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	station := addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)
	require.NoError(t, engine.RemoveStation(t.Context(), "alpha", station.Revision))

	for _, selection := range [][]string{{"alpha"}, {"missing"}} {
		got, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: selection})
		require.ErrorIs(t, err, ErrNotFound)
		require.Equal(t, ReceptionSnapshot{}, got)
	}

	_, err = engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{"bad id"}})
	require.ErrorIs(t, err, ErrInvalid)

	tooMany := make([]string, 0, MaxStations+1)
	for i := range MaxStations + 1 {
		tooMany = append(tooMany, string(rune('a'+i)))
	}
	_, err = engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: tooMany})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestReceptionSnapshotKeepsHistoricalReceiverProvenance(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	station := addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	edited := station.Config
	edited.AntennaGainDBi = 12
	_, err = engine.UpdateStation(t.Context(), station.Revision, edited)
	require.NoError(t, err)
	_, err = engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	got, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.NoError(t, err)

	var revisions []uint64
	for _, record := range got.Records {
		revisions = append(revisions, record.StationRevision)
		switch record.StationRevision {
		case 1:
			require.Equal(t, station.Config.AntennaGainDBi, record.Receiver.Config.AntennaGainDBi)
		case 2:
			require.Equal(t, 12.0, record.Receiver.Config.AntennaGainDBi)
		default:
			t.Fatalf("unexpected station revision %d", record.StationRevision)
		}
	}
	require.Contains(t, revisions, uint64(1))
	require.Contains(t, revisions, uint64(2))
}

func TestReceptionSnapshotIsDetachedFromEngineState(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	request := ReceptionSnapshotRequest{StationIDs: []string{"alpha"}}
	want, err := engine.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)

	mutated, err := engine.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	mutated.StationIDs[0] = "changed"
	mutated.Retention[0].Limit = -1
	mutated.Records[0].Frame[0] ^= 1
	mutated.Records[0].Receiver.Config.ID = "changed"

	again, err := engine.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, want, again)
}

func TestReceptionSnapshotLeavesEngineOutputUnchanged(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	before := engine.Snapshot()
	_, err = engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.NoError(t, err)
	require.Equal(t, before, engine.Snapshot())

	reference := newEngine(t, validConfig())
	addNamedStation(t, reference, "alpha")
	_, err = reference.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	got, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)
	want, err := reference.Advance(t.Context(), time.Second)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestReceptionSnapshotCancellationYieldsNoPartialResult(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	_, err := engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := engine.ReceptionSnapshot(ctx, ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, ReceptionSnapshot{}, got)
}

func TestReceptionSnapshotStaysConsistentDuringMutation(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")

	var group sync.WaitGroup
	start := make(chan struct{})
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		for range 20 {
			_, err := engine.Advance(context.Background(), 100*time.Millisecond)
			require.NoError(t, err)
		}
	}()
	go func() {
		defer group.Done()
		<-start
		for range 20 {
			got, err := engine.ReceptionSnapshot(context.Background(),
				ReceptionSnapshotRequest{StationIDs: []string{"alpha"}})
			require.NoError(t, err)
			require.Len(t, got.Retention, 1)
			for _, record := range got.Records {
				require.False(t, record.Timestamp.After(got.Now))
			}
			if len(got.Records) > 0 {
				require.Equal(t, got.Retention[0].OldestSequence, got.Records[0].Sequence)
				require.Equal(t, got.Retention[0].LatestSequence, got.Records[len(got.Records)-1].Sequence)
			}
		}
	}()
	close(start)
	group.Wait()
}

// disableStation switches an existing station off through an ordinary update.
func disableStation(t testing.TB, engine *Engine, id string) error {
	t.Helper()

	for _, station := range engine.Snapshot().Stations {
		if station.Config.ID != id {
			continue
		}
		cfg := station.Config
		cfg.Enabled = false
		_, err := engine.UpdateStation(context.Background(), station.Revision, cfg)
		return err
	}
	t.Fatalf("station %q is not active", id)
	return nil
}

// filterStation returns the records of one station.
func filterStation(records []Reception, id string) []Reception {
	var filtered []Reception
	for _, record := range records {
		if record.StationID == id {
			filtered = append(filtered, record)
		}
	}
	return filtered
}
