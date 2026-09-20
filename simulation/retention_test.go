package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Long-run retained-state bounds.
//
// These tests advance well past the point where every ring wraps and check
// the structural bounds at explicit checkpoints. They never retain the
// batches they generate, so the test itself cannot be what grows.

// receivingEngine returns an engine with the maximum fleet and the maximum
// number of co-located all-receiving stations.
func receivingEngine(t *testing.T, aircraft int) (*Engine, []string) {
	t.Helper()

	cfg := stationaryConfig()
	cfg.ID = "retention"
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	ids := make([]string, 0, MaxStations)
	for i := range MaxStations {
		id := string(rune('a' + i))
		ids = append(ids, id)
		station := validStationConfig()
		station.ID = id
		station.LatitudeDegrees = fixedLatitudeDegrees
		station.LongitudeDegrees = fixedLongitudeDegrees
		addStation(t, engine, station)
	}
	if aircraft > 0 {
		_, err := engine.SetCount(t.Context(), aircraft)
		require.NoError(t, err)
	}
	return engine, ids
}

// checkpoint asserts every structural bound and the ordering rules of the
// retained evidence at one instant.
func checkpoint(t *testing.T, engine *Engine, ids []string, label string) {
	t.Helper()

	snapshot := engine.Snapshot()
	require.LessOrEqual(t, len(snapshot.History.Messages), HistoryLimit, "%s: transmission history", label)
	require.Len(t, snapshot.Stations, MaxStations, "%s: active stations", label)
	if len(snapshot.History.Messages) > 0 {
		require.Equal(t, snapshot.History.LatestSequence-uint64(len(snapshot.History.Messages))+1,
			snapshot.History.OldestSequence, "%s: history sequences are contiguous", label)
	}

	raw, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	require.LessOrEqual(t, len(raw.Records), MaxStations*ReceptionHistoryLimit, "%s: raw records", label)
	perStation := map[string]int{}
	var previous Reception
	for i, record := range raw.Records {
		perStation[record.StationID]++
		if i > 0 {
			ordered := record.TransmissionSequence > previous.TransmissionSequence ||
				(record.TransmissionSequence == previous.TransmissionSequence && record.StationID > previous.StationID)
			require.True(t, ordered, "%s: records are ordered by transmission sequence then station", label)
		}
		previous = record
	}
	for _, entry := range raw.Retention {
		require.LessOrEqual(t, perStation[entry.StationID], ReceptionHistoryLimit, "%s: station ring", label)
		require.Equal(t, uint64(perStation[entry.StationID]), entry.LatestSequence-entry.OldestSequence+1,
			"%s: retention matches the captured records", label)
		require.Equal(t, ReceptionHistoryLimit, entry.Limit)
	}
}

func TestRetentionStaysBoundedAcrossManyRingWraps(t *testing.T) {
	t.Parallel()

	engine, ids := receivingEngine(t, MaxAircraft)
	checkpoint(t, engine, ids, "start")

	// Each aircraft emits about 4.2 frames per virtual second, so 100
	// aircraft wrap a 1000-record ring several times per second. Ten seconds
	// is far beyond three complete wraps of every ring.
	for second := range 10 {
		_, err := engine.Advance(t.Context(), time.Second)
		require.NoError(t, err)
		checkpoint(t, engine, ids, "second "+time.Duration(second+1).String())
	}
	// Transmission sequences count every frame of the run, including the
	// creation reports emitted when the fleet was created.
	produced := engine.Snapshot().History.LatestSequence
	require.Greater(t, produced, uint64(3*ReceptionHistoryLimit),
		"the run wraps every ring more than three times")

	raw, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	require.Len(t, raw.Records, MaxStations*ReceptionHistoryLimit)
	for _, entry := range raw.Retention {
		require.True(t, entry.Truncated, "every ring evicted its oldest records")
		require.Equal(t, produced, entry.LatestSequence, "an all-receiving station heard every frame")
	}

	// A cursor before the oldest retained record reports the eviction gap,
	// and the page that follows it starts at the oldest retained record.
	page, err := engine.ReceptionHistory(t.Context(), HistoryRequest{
		StationID: ids[0], Limit: 1,
		Cursor: &ReceptionCursor{RunID: raw.RunID, StationID: ids[0], AfterSequence: 1},
	})
	require.NoError(t, err)
	require.True(t, page.Gap)
	require.Equal(t, raw.Retention[0].OldestSequence, page.Records[0].Sequence)
	require.True(t, page.HasMore)
}

func TestRetentionKeepsChurnedAircraftAndDetachedSnapshots(t *testing.T) {
	t.Parallel()

	engine, ids := receivingEngine(t, 0)
	// Replace the fleet repeatedly without advancing time: creation reports
	// alone leave far more distinct addresses than may ever be active.
	addresses := map[uint32]bool{}
	for range 4 {
		_, err := engine.SetCount(t.Context(), MaxAircraft)
		require.NoError(t, err)
		for _, aircraft := range engine.Snapshot().Aircraft {
			addresses[aircraft.ICAO] = true
		}
		_, err = engine.SetCount(t.Context(), 0)
		require.NoError(t, err)
	}
	_, err := engine.SetCount(t.Context(), MaxAircraft)
	require.NoError(t, err)
	require.Greater(t, len(addresses), MaxAircraft)

	expiry := ObservationExpiry{Identity: time.Minute, Position: time.Minute, Altitude: time.Minute, Velocity: time.Minute}
	observed, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: ids, Expiry: expiry})
	require.NoError(t, err)
	require.Greater(t, len(observed.Aircraft), MaxAircraft,
		"retained evidence names more aircraft than the engine may have active")

	// A returned snapshot is the caller's: editing it cannot change what the
	// next capture returns.
	raw, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	before := len(raw.Records)
	raw.Records[0].StationID = "tampered"
	raw.Records = raw.Records[:1]
	raw.StationIDs[0] = "tampered"

	again, err := engine.ReceptionSnapshot(t.Context(), ReceptionSnapshotRequest{StationIDs: ids})
	require.NoError(t, err)
	require.Len(t, again.Records, before)
	require.Equal(t, ids[0], again.Records[0].StationID)
	require.Equal(t, ids, again.StationIDs)
}

func TestRetentionSurvivesStationEditsAndRejectedCommands(t *testing.T) {
	t.Parallel()

	engine, ids := receivingEngine(t, 10)
	_, err := engine.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)

	// Editing a station keeps its ring and its historical provenance.
	edited := validStationConfig()
	edited.ID = ids[0]
	edited.LatitudeDegrees = fixedLatitudeDegrees
	edited.LongitudeDegrees = fixedLongitudeDegrees
	edited.AntennaGainDBi = 12
	updated, err := engine.UpdateStation(t.Context(), 1, edited)
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Revision)
	_, err = engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	disabled := edited
	disabled.Enabled = false
	_, err = engine.UpdateStation(t.Context(), 2, disabled)
	require.NoError(t, err)
	_, err = engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	records := func(id string) []Reception {
		page, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: id, Limit: MaxHistoryPageSize})
		require.NoError(t, err)
		return page.Records
	}
	gains := map[uint64]float64{}
	for _, record := range records(ids[0]) {
		require.Equal(t, record.StationRevision, record.Receiver.Revision)
		gains[record.StationRevision] = record.Receiver.Config.AntennaGainDBi
	}
	require.Equal(t, map[uint64]float64{1: 3, 2: 12}, gains,
		"current settings never rewrite historical records")

	// A rejected command leaves the retained evidence exactly as it was.
	before := records(ids[0])
	_, err = engine.UpdateStation(t.Context(), 99, edited)
	require.ErrorIs(t, err, ErrConflict)
	_, err = engine.AddStation(t.Context(), namedStation(ids[1]))
	require.ErrorIs(t, err, ErrInvalid)
	require.Equal(t, before, records(ids[0]))

	// The valid suffix still runs, and it appends to the retained records
	// rather than rewriting them.
	after, err := engine.Advance(context.Background(), 2*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, after.Transmissions)
	require.Equal(t, before, records(ids[0])[:len(before)])
}
