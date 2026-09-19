package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// addStation adds a station and fails the test if the engine rejects it.
func addStation(t testing.TB, engine *Engine, cfg StationConfig) Station {
	t.Helper()

	got, err := engine.AddStation(context.Background(), cfg)
	require.NoError(t, err)
	return got
}

// namedStation returns the shared fixture with a different identifier.
func namedStation(id string) StationConfig {
	cfg := validStationConfig()
	cfg.ID = id
	return cfg
}

func TestStationLifecycle(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	require.Empty(t, engine.Snapshot().Stations)

	created := addStation(t, engine, validStationConfig())
	require.Equal(t, uint64(1), created.Revision)
	require.Equal(t, validStationConfig(), created.Config)
	require.True(t, created.CreatedAt.Equal(fixtureStart))

	_, err := engine.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)
	now := engine.Snapshot().Now

	second := addStation(t, engine, namedStation("second"))
	third := addStation(t, engine, namedStation("third"))
	require.True(t, second.CreatedAt.Equal(now))

	stations := engine.Snapshot().Stations
	require.Len(t, stations, 3)
	require.Equal(t, []string{"primary", "second", "third"},
		[]string{stations[0].Config.ID, stations[1].Config.ID, stations[2].Config.ID})

	// An update replaces settings, keeps position and creation instant, and
	// increments the revision.
	disabled := namedStation("second")
	disabled.Enabled = false
	disabled.AntennaGainDBi = 12
	updated, err := engine.UpdateStation(t.Context(), second.Revision, disabled)
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Revision)
	require.Equal(t, disabled, updated.Config)
	require.True(t, updated.CreatedAt.Equal(second.CreatedAt))

	// An update that changes nothing still increments the revision.
	again, err := engine.UpdateStation(t.Context(), updated.Revision, disabled)
	require.NoError(t, err)
	require.Equal(t, uint64(3), again.Revision)
	require.Equal(t, disabled, again.Config)

	// Removing the middle station preserves the order of the rest.
	require.NoError(t, engine.RemoveStation(t.Context(), "second", again.Revision))
	stations = engine.Snapshot().Stations
	require.Len(t, stations, 2)
	require.Equal(t, []string{"primary", "third"},
		[]string{stations[0].Config.ID, stations[1].Config.ID})
	require.Equal(t, third.Revision, stations[1].Revision)
}

// A removed identifier stays reserved for the rest of the run.
func TestStationIdentifiersAreReserved(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())

	_, err := engine.AddStation(t.Context(), validStationConfig())
	require.ErrorIs(t, err, ErrInvalid)

	require.NoError(t, engine.RemoveStation(t.Context(), created.Config.ID, created.Revision))
	require.Empty(t, engine.Snapshot().Stations)

	_, err = engine.AddStation(t.Context(), validStationConfig())
	require.ErrorIs(t, err, ErrInvalid)

	// A different identifier is accepted and gets a fresh ordinal.
	replacement := addStation(t, engine, namedStation("replacement"))
	require.Equal(t, uint64(1), replacement.Revision)
	require.Equal(t, uint64(3), engine.state.stations.nextOrdinal)
	require.NotEqual(t, newSource(validConfig().Seed, 1, stationDomain),
		engine.state.stations.active[0].rng)
}

func TestStationRevisionConflict(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())

	changed := validStationConfig()
	changed.SensitivityDBm = -100
	updated, err := engine.UpdateStation(t.Context(), created.Revision, changed)
	require.NoError(t, err)

	before := engine.Snapshot()

	stale := validStationConfig()
	stale.SensitivityDBm = -70
	_, err = engine.UpdateStation(t.Context(), created.Revision, stale)
	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, before, engine.Snapshot())

	require.ErrorIs(t, engine.RemoveStation(t.Context(), created.Config.ID, created.Revision), ErrConflict)
	require.Equal(t, before, engine.Snapshot())

	// A retry with the current revision succeeds.
	retried, err := engine.UpdateStation(t.Context(), updated.Revision, stale)
	require.NoError(t, err)
	require.Equal(t, uint64(3), retried.Revision)
	require.Equal(t, -70.0, retried.Config.SensitivityDBm)
	require.NoError(t, engine.RemoveStation(t.Context(), created.Config.ID, retried.Revision))
}

func TestStationCommandsReject(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())
	before := engine.Snapshot()

	invalid := validStationConfig()
	invalid.SensitivityDBm = 5
	_, err := engine.AddStation(t.Context(), invalid)
	require.ErrorIs(t, err, ErrInvalid)

	invalid.ID = "second"
	_, err = engine.UpdateStation(t.Context(), 1, invalid)
	require.ErrorIs(t, err, ErrInvalid)

	_, err = engine.UpdateStation(t.Context(), 1, namedStation("missing"))
	require.ErrorIs(t, err, ErrNotFound)

	require.ErrorIs(t, engine.RemoveStation(t.Context(), "missing", 1), ErrNotFound)
	require.Equal(t, before, engine.Snapshot())
	require.Equal(t, created.Revision, engine.Snapshot().Stations[0].Revision)
}

func TestStationLimit(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	for i := range MaxStations {
		addStation(t, engine, namedStation(callsign(uint32(i+1))))
	}
	require.Len(t, engine.Snapshot().Stations, MaxStations)

	before := engine.Snapshot()
	_, err := engine.AddStation(t.Context(), namedStation("overflow"))
	require.ErrorIs(t, err, ErrLimit)
	require.Equal(t, before, engine.Snapshot())

	// Removing one frees an active slot for a new identifier.
	require.NoError(t, engine.RemoveStation(t.Context(), before.Stations[0].Config.ID, 1))
	addStation(t, engine, namedStation("overflow"))
	require.Len(t, engine.Snapshot().Stations, MaxStations)
}

func TestStationOrdinalExhaustion(t *testing.T) {
	t.Parallel()

	registry := newStationRegistry()
	registry.nextOrdinal = 1<<64 - 1
	_, err := registry.add(0, validStationConfig(), 0)
	require.ErrorIs(t, err, ErrLimit)
}

func TestStationCommandsCheckContext(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())
	before := engine.Snapshot()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.AddStation(ctx, namedStation("second"))
	require.ErrorIs(t, err, context.Canceled)

	_, err = engine.UpdateStation(ctx, created.Revision, validStationConfig())
	require.ErrorIs(t, err, context.Canceled)

	require.ErrorIs(t, engine.RemoveStation(ctx, created.Config.ID, created.Revision), context.Canceled)
	require.Equal(t, before, engine.Snapshot())
}

// Station commands must not touch aircraft state, time, or history.
func TestStationCommandsLeaveEngineStateAlone(t *testing.T) {
	t.Parallel()

	subject := newEngine(t, validConfig())
	control := newEngine(t, validConfig())

	churn := func(e *Engine) {
		first := addStation(t, e, validStationConfig())
		addStation(t, e, namedStation("second"))
		changed := validStationConfig()
		changed.Enabled = false
		updated, err := e.UpdateStation(t.Context(), first.Revision, changed)
		require.NoError(t, err)
		require.NoError(t, e.RemoveStation(t.Context(), "second", 1))
		_, err = e.UpdateStation(t.Context(), updated.Revision, validStationConfig())
		require.NoError(t, err)
	}
	churn(subject)

	subjectSnapshot := subject.Snapshot()
	controlSnapshot := control.Snapshot()
	require.Equal(t, controlSnapshot.Aircraft, subjectSnapshot.Aircraft)
	require.Equal(t, controlSnapshot.History, subjectSnapshot.History)
	require.Equal(t, controlSnapshot.Elapsed, subjectSnapshot.Elapsed)
	require.Equal(t, control.state.fleet, subject.state.fleet)
	require.Equal(t, control.state.identity, subject.state.identity)
	require.Equal(t, control.state.lastSequence, subject.state.lastSequence)
	require.Equal(t, control.state.clock, subject.state.clock)
}

// A returned station record and the snapshot slice are owned by the caller.
func TestStationSnapshotIsDetached(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	created := addStation(t, engine, validStationConfig())

	created.Config.ID = "tampered"
	created.Config.SensitivityDBm = -1
	created.Revision = 99

	snapshot := engine.Snapshot()
	snapshot.Stations[0].Config.AntennaGainDBi = 40
	snapshot.Stations = append(snapshot.Stations, Station{})

	got := engine.Snapshot().Stations
	require.Len(t, got, 1)
	require.Equal(t, validStationConfig(), got[0].Config)
	require.Equal(t, uint64(1), got[0].Revision)
}

// A cloned registry shares no slice, no reserved set, and no generator.
func TestStationRegistryCloneIsDetached(t *testing.T) {
	t.Parallel()

	original := newStationRegistry()
	_, err := original.add(1, validStationConfig(), 0)
	require.NoError(t, err)

	copied := original.clone()
	_, err = copied.add(1, namedStation("second"), 0)
	require.NoError(t, err)
	copied.active[0].revision = 42

	require.Len(t, original.active, 1)
	require.Equal(t, uint64(1), original.active[0].revision)
	require.Len(t, original.reserved, 1)
	require.Len(t, copied.reserved, 2)
}
