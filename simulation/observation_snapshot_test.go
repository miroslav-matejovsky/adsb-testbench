package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

func addNamedStation(t testing.TB, engine *Engine, id string) Station {
	t.Helper()
	cfg := validStationConfig()
	cfg.ID = id
	station, err := engine.AddStation(context.Background(), cfg)
	require.NoError(t, err)
	return station
}

func TestObservationSelectionDeduplicatesStationsAndTransmissions(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	addNamedStation(t, engine, "bravo")
	addNamedStation(t, engine, "alpha")
	_, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	_, err = engine.Advance(t.Context(), time.Second)
	require.NoError(t, err)

	request := ObservationRequest{
		StationIDs: []string{"bravo", "alpha", "bravo"}, Expiry: validObservationExpiry(),
	}
	got, err := engine.Observations(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "bravo"}, got.StationIDs)
	require.Len(t, got.Aircraft, 1)
	require.Len(t, got.Aircraft[0].Identity.Evidence.Receptions, 2)
	require.Equal(t, "alpha", got.Aircraft[0].Identity.Evidence.Receptions[0].StationID)
	require.NotNil(t, got.Aircraft[0].Position)

	want, err := engine.Observations(t.Context(), request)
	require.NoError(t, err)
	got.StationIDs[0] = "changed"
	got.Aircraft[0].Identity.Callsign = "changed"
	got.Aircraft[0].Identity.Evidence.Receptions[0].Frame[0] ^= 1
	again, err := engine.Observations(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, want, again)
}

func TestObservationSelectionCanPairComplementaryStations(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "alpha")
	addNamedStation(t, engine, "bravo")
	const icao = 0xabc123
	evenFrame := encodedPosition(t, icao, false, 35000)
	oddFrame := encodedPosition(t, icao, true, 35000)
	appendSyntheticReception := func(stationIndex int, sequence, transmission uint64, frame adsb.Frame, at time.Time) {
		station := &engine.state.stations.active[stationIndex]
		reception := Reception{
			Sequence: sequence, TransmissionSequence: transmission, StationID: station.cfg.ID,
			StationRevision: station.revision, Receiver: station.public(engine.state.clock.start),
			ICAO: icao, Kind: PositionMessage, Timestamp: at, Frame: [14]byte(frame),
		}
		station.receptions.append(reception)
		station.lastReceptionSequence = sequence
	}
	appendSyntheticReception(0, 1, 100, evenFrame, fixtureStart)
	appendSyntheticReception(1, 1, 101, oddFrame, fixtureStart.Add(time.Second))
	engine.state.clock.elapsed = time.Second

	alpha, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"alpha"}, Expiry: validObservationExpiry()})
	require.NoError(t, err)
	require.Nil(t, alpha.Aircraft[0].Position)

	both, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"bravo", "alpha"}, Expiry: validObservationExpiry()})
	require.NoError(t, err)
	require.NotNil(t, both.Aircraft[0].Position)
	require.Len(t, both.Aircraft[0].Position.Evidence, 2)
}

func TestObservationMissedTransmissionsDoNotRefreshFields(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	station := addNamedStation(t, engine, "primary")
	_, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	before, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: validObservationExpiry()})
	require.NoError(t, err)
	require.Len(t, before.Aircraft, 1)
	page, err := engine.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: MaxHistoryPageSize})
	require.NoError(t, err)

	dropped := station.Config
	dropped.FrameLossProbability = 1
	_, err = engine.UpdateStation(t.Context(), station.Revision, dropped)
	require.NoError(t, err)
	batch, err := engine.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch.Transmissions)
	require.Empty(t, batch.Receptions)
	unchangedPage, err := engine.ReceptionHistory(t.Context(), HistoryRequest{
		StationID: "primary", Cursor: &page.NextCursor, Limit: MaxHistoryPageSize,
	})
	require.NoError(t, err)
	require.False(t, unchangedPage.Gap)
	require.Empty(t, unchangedPage.Records)

	after, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: validObservationExpiry()})
	require.NoError(t, err)
	require.Equal(t, before.Aircraft[0].Identity.ObservedAt, after.Aircraft[0].Identity.ObservedAt)
	require.Equal(t, before.Aircraft[0].Velocity.ObservedAt, after.Aircraft[0].Velocity.ObservedAt)
}

func TestObservationRemovedStationIsNotSelectable(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	station := addNamedStation(t, engine, "primary")
	require.NoError(t, engine.RemoveStation(t.Context(), station.Config.ID, station.Revision))
	_, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: validObservationExpiry()})
	require.ErrorIs(t, err, ErrNotFound)
}
