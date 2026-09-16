package simulation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAircraftIdentityAllocation(t *testing.T) {
	t.Parallel()

	allocator := newIdentityAllocator()
	seenAddresses := make(map[uint32]bool)
	seenCallsigns := make(map[string]bool)

	for want := uint64(1); want <= 250; want++ {
		icao, ordinal, err := allocator.allocate()
		require.NoError(t, err)
		require.Equal(t, want, ordinal)
		require.GreaterOrEqual(t, icao, minAddress)
		require.LessOrEqual(t, icao, maxAddress)
		require.False(t, seenAddresses[icao])
		seenAddresses[icao] = true

		sign := callsign(icao)
		require.Len(t, sign, 8)
		require.Equal(t, "TB", sign[:2])
		require.Equal(t, sign, upper(sign), "callsign is uppercase")
		require.False(t, seenCallsigns[sign])
		seenCallsigns[sign] = true
	}
}

// upper returns the uppercase form of s using a path independent of the
// production formatter.
func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 'a' + 'A'
		}
	}
	return string(out)
}

func TestAircraftCallsignFormat(t *testing.T) {
	t.Parallel()

	require.Equal(t, "TB000001", callsign(minAddress))
	require.Equal(t, "TBFFFFFE", callsign(maxAddress))
	require.Equal(t, "TB0ABCDE", callsign(0x0abcde))
}

// The last legal address is allocatable; the next allocation fails and the
// allocator stays exhausted.
func TestAircraftIdentityExhaustion(t *testing.T) {
	t.Parallel()

	allocator := identityAllocator{nextAddress: maxAddress, nextOrdinal: 900}
	icao, ordinal, err := allocator.allocate()
	require.NoError(t, err)
	require.Equal(t, maxAddress, icao)
	require.Equal(t, uint64(900), ordinal)

	_, _, err = allocator.allocate()
	require.ErrorIs(t, err, ErrLimit)
	_, _, err = allocator.allocate()
	require.ErrorIs(t, err, ErrLimit)
}

func TestAircraftCreationIsReproducible(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(validConfig())
	require.NoError(t, err)

	first := newAircraft(cfg, 0x000010, 4, 3*time.Second)
	second := newAircraft(cfg, 0x000010, 4, 3*time.Second)
	require.Equal(t, first, second)

	other := newAircraft(cfg, 0x000011, 5, 3*time.Second)
	require.NotEqual(t, first.traj.birth, other.traj.birth)

	changed := cfg
	changed.Seed++
	require.NotEqual(t, first.traj.birth, newAircraft(changed, 0x000010, 4, 3*time.Second).traj.birth)
}

func TestAircraftBirthStaysInsideConfiguredRanges(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(validConfig())
	require.NoError(t, err)

	for ordinal := uint64(1); ordinal <= 200; ordinal++ {
		birth := newAircraft(cfg, uint32(ordinal), ordinal, 0).traj.birth
		requireInRange(t, cfg.Spawn.LatitudeDegrees, birth.latitudeDegrees)
		requireInRange(t, cfg.Spawn.LongitudeDegrees, birth.longitudeDegrees)
		requireInRange(t, cfg.Spawn.AltitudeFeet, birth.altitudeFeet)
		requireInRange(t, cfg.Spawn.GroundSpeedKnots, birth.groundSpeedKnots)
		requireInRange(t, cfg.Spawn.TrackDegrees, birth.trackDegrees)
		requireInRange(t, cfg.Spawn.VerticalRateFeetPerMinute, birth.verticalRateFeetPerMinute)
	}
}

func requireInRange(t *testing.T, r Range, v float64) {
	t.Helper()

	require.GreaterOrEqual(t, v, r.Min)
	if r.exact() {
		require.Equal(t, r.Min, v)
		return
	}
	require.Less(t, v, r.Max)
}

// An exact spawn configuration fixes the birth state completely.
func TestAircraftExactSpawn(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(stationaryConfig())
	require.NoError(t, err)

	got := newAircraft(cfg, 0x000001, 1, 0)
	require.Equal(t, birthState{
		latitudeDegrees:  50,
		longitudeDegrees: 14,
		altitudeFeet:     35000,
		groundSpeedKnots: 0,
		trackDegrees:     90,
	}, got.traj.birth)
}

func TestAircraftPublicSnapshot(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(stationaryConfig())
	require.NoError(t, err)

	craft := newAircraft(cfg, 0x00beef, 3, 20*time.Second)
	got := craft.public(fixtureStart, 80*time.Second)

	require.Equal(t, uint32(0x00beef), got.ICAO)
	require.Equal(t, "TB00BEEF", got.Callsign)
	require.True(t, got.CreatedAt.Equal(fixtureStart.Add(20*time.Second)))
	require.Equal(t, 50.0, got.LatitudeDegrees)
	require.Equal(t, 14.0, got.LongitudeDegrees)
	require.Equal(t, 35000.0, got.BarometricAltitudeFeet)
	require.Equal(t, 0.0, got.GroundSpeedKnots)
	require.Equal(t, 90.0, got.TrackDegrees)
	require.Equal(t, 0.0, got.VerticalRateFeetPerMinute)
}

// Motion depends on age since creation, not on absolute run time.
func TestAircraftMotionUsesAgeSinceCreation(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(validConfig())
	require.NoError(t, err)

	early := newAircraft(cfg, 0x000002, 2, 0)
	late := newAircraft(cfg, 0x000002, 2, 30*time.Second)

	require.Equal(t, early.navAt(45*time.Second), late.navAt(75*time.Second))
	require.NotEqual(t, early.navAt(45*time.Second), late.navAt(45*time.Second))
}
