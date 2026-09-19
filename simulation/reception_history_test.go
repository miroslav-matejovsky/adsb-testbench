package simulation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReceptionHistoryRingWrapsAndClones(t *testing.T) {
	t.Parallel()

	history := newReceptionHistory()
	for sequence := uint64(1); sequence <= ReceptionHistoryLimit+2; sequence++ {
		history.append(Reception{Sequence: sequence, StationID: "one"})
	}
	records := history.records()
	require.Len(t, records, ReceptionHistoryLimit)
	require.Equal(t, uint64(3), records[0].Sequence)
	require.Equal(t, uint64(ReceptionHistoryLimit+2), records[len(records)-1].Sequence)

	clone := history.clone()
	clone.buffer[0].StationID = "changed"
	require.NotEqual(t, clone.buffer[0], history.buffer[0])
}

func TestReceptionRetainsReceiverSettingsAndSequence(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	created := addStation(t, engine, validStationConfig())
	batch, err := engine.SetCount(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, batch.Receptions, 3)
	for index, reception := range batch.Receptions {
		require.Equal(t, uint64(index+1), reception.Sequence)
		require.Equal(t, reception.StationID, reception.Receiver.Config.ID)
		require.Equal(t, reception.StationRevision, reception.Receiver.Revision)
		require.Equal(t, created, reception.Receiver)
	}

	updatedConfig := created.Config
	updatedConfig.AntennaGainDBi++
	updated, err := engine.UpdateStation(t.Context(), created.Revision, updatedConfig)
	require.NoError(t, err)
	more, err := engine.Advance(t.Context(), MaxAdvance)
	require.NoError(t, err)
	require.NotEmpty(t, more.Receptions)
	require.Equal(t, updated, more.Receptions[0].Receiver)
	require.Equal(t, created, engine.state.stations.active[0].receptions.records()[0].Receiver)
}
