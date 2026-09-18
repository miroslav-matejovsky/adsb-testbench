package simulation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func engineWithSyntheticReceptionHistory(t testing.TB, count int) *Engine {
	t.Helper()
	engine := newEngine(t, validConfig())
	addStation(t, engine, validStationConfig())
	station := &engine.state.stations.active[0]
	for sequence := 1; sequence <= count; sequence++ {
		reception := Reception{Sequence: uint64(sequence), StationID: station.cfg.ID, Receiver: station.public(engine.state.clock.start)}
		station.receptions.append(reception)
		station.lastReceptionSequence = uint64(sequence)
	}
	return engine
}

func TestReceptionHistoryPagingAndGap(t *testing.T) {
	t.Parallel()

	engine := engineWithSyntheticReceptionHistory(t, ReceptionHistoryLimit+5)
	first, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: 7})
	require.NoError(t, err)
	require.False(t, first.Gap)
	require.Equal(t, uint64(6), first.OldestSequence)
	require.Equal(t, uint64(12), first.NextCursor.AfterSequence)
	require.True(t, first.HasMore)

	stale := first.NextCursor
	stale.AfterSequence = 1
	gap, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Cursor: &stale, Limit: 2})
	require.NoError(t, err)
	require.True(t, gap.Gap)
	require.Equal(t, uint64(6), gap.Records[0].Sequence)

	next, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Cursor: &first.NextCursor, Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	require.False(t, next.Gap)
	require.False(t, next.HasMore)
	require.Equal(t, uint64(ReceptionHistoryLimit+5), next.NextCursor.AfterSequence)
}

func TestReceptionHistoryRejectsInvalidCursorAndRemovedStation(t *testing.T) {
	t.Parallel()

	engine := engineWithSyntheticReceptionHistory(t, 2)
	request := HistoryRequest{StationID: "primary", Limit: 1}
	page, err := engine.ReceptionHistory(t.Context(), request)
	require.NoError(t, err)

	wrongRun := page.NextCursor
	wrongRun.RunID = "replacement"
	request.Cursor = &wrongRun
	_, err = engine.ReceptionHistory(t.Context(), request)
	require.ErrorIs(t, err, ErrConflict)

	wrongStation := page.NextCursor
	wrongStation.StationID = "other"
	request.Cursor = &wrongStation
	_, err = engine.ReceptionHistory(t.Context(), request)
	require.ErrorIs(t, err, ErrInvalid)

	future := page.NextCursor
	future.AfterSequence = 3
	request.Cursor = &future
	_, err = engine.ReceptionHistory(t.Context(), request)
	require.ErrorIs(t, err, ErrInvalid)

	revision := engine.state.stations.active[0].revision
	require.NoError(t, engine.RemoveStation(t.Context(), "primary", revision))
	request.Cursor = nil
	_, err = engine.ReceptionHistory(t.Context(), request)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestReceptionHistoryRequiresExplicitLimit(t *testing.T) {
	t.Parallel()

	engine := engineWithSyntheticReceptionHistory(t, 0)
	for _, limit := range []int{0, -1, MaxHistoryPageSize + 1} {
		_, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: limit})
		require.ErrorIs(t, err, ErrInvalid)
	}
}
