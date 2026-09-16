package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Editing anything a snapshot or a batch returns must not reach engine state.
func TestSnapshotOwnership(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	batch, err := engine.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, batch)

	original := engine.Snapshot()
	mutated := engine.Snapshot()

	mutated.Config.ID = "tampered"
	mutated.Config.SpeedHundredths = 9999
	mutated.Config.Spawn.AltitudeFeet = Range{Min: 1, Max: 2}
	mutated.Aircraft[0].ICAO = 0xabcdef
	mutated.Aircraft[0].Callsign = "TAMPERED"
	mutated.History.Messages[0].Sequence = 12345
	mutated.History.Messages[0].Frame[0] = 0
	mutated.Elapsed = time.Hour

	batch[0].Frame[3] = 0xff
	batch[0].Sequence = 999999

	require.Equal(t, original, engine.Snapshot())

	// Subsequent output is unaffected too.
	control := newEngine(t, validConfig())
	_, err = control.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)

	tampered, err := engine.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)
	clean, err := control.Advance(t.Context(), 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, clean, tampered)
}

// Snapshot evaluates every aircraft at the same committed instant.
func TestSnapshotIsCoherent(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	_, err := engine.Advance(t.Context(), 12500*time.Millisecond)
	require.NoError(t, err)

	got := engine.Snapshot()
	require.True(t, got.Now.Equal(got.Config.StartTime.Add(got.Elapsed)))
	require.Equal(t, 12500*time.Millisecond, got.Elapsed)

	for i, craft := range got.Aircraft {
		want := engine.state.fleet[i].public(engine.state.clock.start, engine.state.clock.elapsed)
		require.Equal(t, want, craft)
		require.False(t, craft.CreatedAt.After(got.Now))
	}
}

// Reading a snapshot must not consume randomness or move a deadline.
func TestSnapshotDoesNotDisturbState(t *testing.T) {
	t.Parallel()

	engine := newEngine(t, validConfig())
	control := newEngine(t, validConfig())

	for range 20 {
		engine.Snapshot()
	}

	first, err := engine.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	second, err := control.Advance(t.Context(), 4*time.Second)
	require.NoError(t, err)
	require.Equal(t, second, first)
}

// Config in a snapshot reports the current speed but the original count.
func TestSnapshotConfigReflectsCurrentSpeed(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 2
	engine := newEngine(t, cfg)

	require.NoError(t, engine.SetSpeed(t.Context(), 4200))
	_, err := engine.SetCount(t.Context(), 6)
	require.NoError(t, err)

	got := engine.Snapshot()
	require.Equal(t, uint16(4200), got.Config.SpeedHundredths)
	require.Equal(t, 2, got.Config.InitialAircraftCount)
	require.Len(t, got.Aircraft, 6)
}
