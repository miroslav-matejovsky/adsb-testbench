package simulation

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

func TestObservationVirtualTimeAgesDuringEmptyScenario(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	addNamedStation(t, engine, "primary")
	_, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	_, err = engine.SetCount(t.Context(), 0)
	require.NoError(t, err)

	expiry := ObservationExpiry{Identity: time.Second, Position: time.Second, Altitude: time.Second, Velocity: time.Second}
	before, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: expiry})
	require.NoError(t, err)
	require.Len(t, before.Aircraft, 1, "received fields outlive truth removal until expiry")

	batch, err := engine.Advance(t.Context(), time.Second+time.Nanosecond)
	require.NoError(t, err)
	require.Empty(t, batch.Transmissions)
	after, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: expiry})
	require.NoError(t, err)
	require.Empty(t, after.Aircraft)
}

func TestObservationWholeAndSplitAdvancesMatch(t *testing.T) {
	t.Parallel()

	newObservedEngine := func() *Engine {
		cfg := stationaryConfig()
		cfg.InitialAircraftCount = 0
		engine := newEngine(t, cfg)
		addNamedStation(t, engine, "primary")
		_, err := engine.SetCount(t.Context(), 1)
		require.NoError(t, err)
		return engine
	}
	whole := newObservedEngine()
	split := newObservedEngine()
	_, err := whole.Advance(t.Context(), 10*time.Second)
	require.NoError(t, err)
	_, err = split.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	_, err = split.Advance(t.Context(), 6*time.Second)
	require.NoError(t, err)

	wholePage, err := whole.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	splitPage, err := split.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	require.Equal(t, wholePage, splitPage)
	request := ObservationRequest{StationIDs: []string{"primary"}, Expiry: validObservationExpiry()}
	wholeSnapshot, err := whole.Observations(t.Context(), request)
	require.NoError(t, err)
	splitSnapshot, err := split.Observations(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, wholeSnapshot, splitSnapshot)
}

func TestReceptionCancellationRollsBackRetainedState(t *testing.T) {
	t.Parallel()

	newObservedEngine := func() *Engine {
		cfg := stationaryConfig()
		cfg.InitialAircraftCount = 0
		engine := newEngine(t, cfg)
		addNamedStation(t, engine, "primary")
		_, err := engine.SetCount(t.Context(), 1)
		require.NoError(t, err)
		return engine
	}
	subject := newObservedEngine()
	control := newObservedEngine()

	batch, err := subject.Advance(newCountingContext(10, context.Canceled), 20*time.Second)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, Batch{}, batch)
	subjectPage, err := subject.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	controlPage, err := control.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: MaxHistoryPageSize})
	require.NoError(t, err)
	require.Equal(t, controlPage, subjectPage)

	subjectBatch, err := subject.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	controlBatch, err := control.Advance(t.Context(), 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, controlBatch, subjectBatch)
}

func TestReceptionSequenceExhaustionIsAtomic(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	addNamedStation(t, engine, "primary")
	_, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	engine.state.stations.active[0].lastReceptionSequence = math.MaxUint64
	before := engine.state.clone()

	batch, err := engine.Advance(t.Context(), time.Second)
	require.ErrorIs(t, err, ErrLimit)
	require.Equal(t, Batch{}, batch)
	require.Equal(t, before, engine.state)
}

func TestReceptionHistoryAllStationsStayBounded(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	for index := range MaxStations {
		addNamedStation(t, engine, fmt.Sprintf("station%d", index))
	}
	batch, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, batch.Receptions, 3*MaxStations)
	for index := range MaxStations {
		page, queryErr := engine.ReceptionHistory(t.Context(), HistoryRequest{
			StationID: fmt.Sprintf("station%d", index), Limit: MaxHistoryPageSize,
		})
		require.NoError(t, queryErr)
		require.Len(t, page.Records, 3)
		require.Equal(t, ReceptionHistoryLimit, page.RetentionLimit)
	}
}

func TestObservationPauseDoesNotAgeButAdvanceDoes(t *testing.T) {
	t.Parallel()

	cfg := stationaryConfig()
	cfg.InitialAircraftCount = 0
	cfg.SpeedHundredths = 0
	engine := newEngine(t, cfg)
	addNamedStation(t, engine, "primary")
	_, err := engine.SetCount(t.Context(), 1)
	require.NoError(t, err)
	_, err = engine.SetCount(t.Context(), 0)
	require.NoError(t, err)
	expiry := ObservationExpiry{Identity: time.Second, Position: time.Second, Altitude: time.Second, Velocity: time.Second}

	_, err = engine.Elapse(t.Context(), MaxAdvance)
	require.NoError(t, err)
	paused, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: expiry})
	require.NoError(t, err)
	require.Len(t, paused.Aircraft, 1)
	require.Equal(t, fixtureStart, paused.Now)

	_, err = engine.Advance(t.Context(), time.Second+time.Nanosecond)
	require.NoError(t, err)
	advanced, err := engine.Observations(t.Context(), ObservationRequest{StationIDs: []string{"primary"}, Expiry: expiry})
	require.NoError(t, err)
	require.Empty(t, advanced.Aircraft)
}

func TestObservationEvictedEvidenceCannotSupplyFreshField(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	addNamedStation(t, engine, "primary")
	station := &engine.state.stations.active[0]
	identity := encodedIdentification(t, 0xabc123, "LOST")
	velocity := encodedVelocity(t, 0xabc124, 1, 1)
	appendRecord := func(sequence uint64, icao uint32, kind MessageKind, frame adsb.Frame) {
		reception := Reception{
			Sequence: sequence, TransmissionSequence: sequence, StationID: station.cfg.ID,
			StationRevision: station.revision, Receiver: station.public(engine.state.clock.start),
			ICAO: icao, Kind: kind, Timestamp: fixtureStart, Frame: [14]byte(frame),
		}
		station.receptions.append(reception)
		station.lastReceptionSequence = sequence
	}
	appendRecord(1, 0xabc123, IdentificationMessage, identity)
	for sequence := uint64(2); sequence <= ReceptionHistoryLimit+1; sequence++ {
		appendRecord(sequence, 0xabc124, VelocityMessage, velocity)
	}

	snapshot, err := engine.Observations(t.Context(), ObservationRequest{
		StationIDs: []string{"primary"}, Expiry: validObservationExpiry(),
	})
	require.NoError(t, err)
	require.True(t, snapshot.Retention[0].Truncated)
	require.Equal(t, uint64(2), snapshot.Retention[0].OldestSequence)
	require.Len(t, snapshot.Aircraft, 1)
	require.Equal(t, uint32(0xabc124), snapshot.Aircraft[0].ICAO)
}

func TestReceptionCursorRejectsReplacementRun(t *testing.T) {
	t.Parallel()

	first := engineWithSyntheticReceptionHistory(t, 1)
	page, err := first.ReceptionHistory(t.Context(), HistoryRequest{StationID: "primary", Limit: 1})
	require.NoError(t, err)

	cfg := validConfig()
	cfg.ID = "replacement-run"
	replacement := newEngine(t, cfg)
	addStation(t, replacement, validStationConfig())
	_, err = replacement.ReceptionHistory(t.Context(), HistoryRequest{
		StationID: "primary", Cursor: &page.NextCursor, Limit: 1,
	})
	require.ErrorIs(t, err, ErrConflict)
}
