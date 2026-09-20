package simulation

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Station identity and revision bounds.
//
// Exact boundaries are arranged in same-package state so they stay fast.
// Every rejection is compared with a control engine arranged identically,
// through public snapshots and an identical future suffix, so hidden
// ordinal, random, and sequence state is covered as well.

// reserveIDs marks count synthetic identifiers as used and removed in this
// run, as if they had been created and removed earlier.
func reserveIDs(engine *Engine, count int) {
	for i := range count {
		engine.state.stations.reserved[fmt.Sprintf("used-%04d", i)] = struct{}{}
	}
}

// arrangedPair returns a subject and a control engine with zero aircraft, the
// given number of reserved identifiers, and the given active stations.
func arrangedPair(t *testing.T, reserved int, active ...string) (subject, control *Engine) {
	t.Helper()

	cfg := validConfig()
	cfg.InitialAircraftCount = 0
	subject = newEngine(t, cfg)
	control = newEngine(t, cfg)
	for _, engine := range []*Engine{subject, control} {
		reserveIDs(engine, reserved)
		for _, id := range active {
			addStation(t, engine, namedStation(id))
		}
	}
	return subject, control
}

func TestStationLifetimeIDBudget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		reserved int
		active   []string
		add      string
		err      error
		message  string
	}{
		{
			name: "1023 used IDs accept the 1024th", reserved: MaxStationIDsPerRun - 1, add: "last",
		},
		{
			name: "1024 used IDs reject a distinct 1025th", reserved: MaxStationIDsPerRun - 1, active: []string{"last"},
			add: "beyond", err: ErrLimit, message: "at most 1024 station IDs",
		},
		{
			name: "a removed ID is invalid even at lifetime capacity", reserved: MaxStationIDsPerRun,
			add: "used-0007", err: ErrInvalid, message: "already used",
		},
		{
			name: "eight active stations hit the active bound first", reserved: 0,
			active: []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"},
			add:    "s9", err: ErrLimit, message: "at most 8 stations may be active",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			subject, control := arrangedPair(t, tc.reserved, tc.active...)
			ordinal := subject.state.stations.nextOrdinal
			_, err := subject.AddStation(t.Context(), namedStation(tc.add))
			if tc.err == nil {
				require.NoError(t, err)
				require.Len(t, subject.state.stations.reserved, MaxStationIDsPerRun)
				return
			}
			require.ErrorIs(t, err, tc.err)
			require.ErrorContains(t, err, tc.message)
			require.Equal(t, ordinal, subject.state.stations.nextOrdinal, "no ordinal consumed")
			require.Len(t, subject.state.stations.reserved, len(control.state.stations.reserved), "no ID reserved")
			requireIdenticalFuture(t, subject, control)
		})
	}
}

func TestStationLifetimeBudgetKeepsUpdatesAndRemovals(t *testing.T) {
	t.Parallel()

	subject, _ := arrangedPair(t, MaxStationIDsPerRun-1, "last")
	require.Len(t, subject.state.stations.reserved, MaxStationIDsPerRun)

	changed := namedStation("last")
	changed.AntennaGainDBi = 10
	updated, err := subject.UpdateStation(t.Context(), 1, changed)
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Revision)
	require.NoError(t, subject.RemoveStation(t.Context(), "last", 2))

	_, err = subject.AddStation(t.Context(), namedStation("after"))
	require.ErrorIs(t, err, ErrLimit, "removal frees an active slot, never the lifetime budget")
}

// TestStationLifetimeBudgetThroughPublicCycles creates and removes stations
// through the public API until the budget is spent. Only one station is
// active at a time, so the active bound cannot mask the lifetime bound.
func TestStationLifetimeBudgetThroughPublicCycles(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 0
	engine := newEngine(t, cfg)
	for i := range MaxStationIDsPerRun {
		id := fmt.Sprintf("cycle-%04d", i)
		created := addStation(t, engine, namedStation(id))
		require.NoError(t, engine.RemoveStation(t.Context(), id, created.Revision))
	}
	before := engine.Snapshot()
	_, err := engine.AddStation(t.Context(), namedStation("one-too-many"))
	require.ErrorIs(t, err, ErrLimit)
	require.Equal(t, before, engine.Snapshot())
	require.Len(t, engine.state.stations.reserved, MaxStationIDsPerRun)

	// A fresh engine, as after a restart with a new run ID, has a fresh budget.
	fresh := newEngine(t, cfg)
	addStation(t, fresh, namedStation("one-too-many"))
}

func TestStationRevisionNeverWraps(t *testing.T) {
	t.Parallel()

	subject, control := arrangedPair(t, 0, "primary")
	for _, engine := range []*Engine{subject, control} {
		engine.state.stations.active[0].revision = math.MaxUint64 - 1
	}

	// One matching update reaches the largest revision.
	for _, engine := range []*Engine{subject, control} {
		updated, err := engine.UpdateStation(t.Context(), math.MaxUint64-1, namedStation("primary"))
		require.NoError(t, err)
		require.Equal(t, uint64(math.MaxUint64), updated.Revision)
	}

	changed := namedStation("primary")
	changed.AntennaGainDBi = 20
	_, err := subject.UpdateStation(t.Context(), math.MaxUint64, changed)
	require.ErrorIs(t, err, ErrLimit, "the next update would wrap to zero")
	requireIdenticalFuture(t, subject, control)

	_, err = subject.UpdateStation(t.Context(), math.MaxUint64-1, changed)
	require.ErrorIs(t, err, ErrConflict, "a stale revision is still a conflict")
	requireIdenticalFuture(t, subject, control)

	require.NoError(t, subject.RemoveStation(t.Context(), "primary", math.MaxUint64),
		"an exhausted station remains removable")
	require.Empty(t, subject.Snapshot().Stations)
}

func TestStationRemovalClearsTheVacatedTail(t *testing.T) {
	t.Parallel()

	registry := newStationRegistry()
	for _, id := range []string{"first", "second", "third"} {
		_, err := registry.add(1, namedStation(id), 0)
		require.NoError(t, err)
	}
	require.NoError(t, registry.remove("second", 1))

	require.Equal(t, []string{"first", "third"},
		[]string{registry.active[0].cfg.ID, registry.active[1].cfg.ID}, "active order is preserved")
	tail := registry.active[:cap(registry.active)][len(registry.active):]
	for _, vacated := range tail {
		require.Equal(t, station{}, vacated, "the backing array keeps no removed reception ring")
	}
	_, reserved := registry.reserved["second"]
	require.True(t, reserved)
}
