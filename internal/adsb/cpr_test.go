package adsb_test

import (
	"math"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

var instant = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func sampleAt(t testing.TB, p adsb.Coordinates, odd bool, at time.Time) adsb.PositionSample {
	t.Helper()
	c, err := adsb.EncodeCPR(p, odd)
	require.NoError(t, err)
	f, err := adsb.EncodePosition(adsb.Header{ICAO: 0x40621d, Capability: 5}, adsb.AirbornePosition{TypeCode: 11, CPR: c})
	require.NoError(t, err)
	return adsb.PositionSample{Frame: f, At: at}
}

func TestPublishedCPRPositions(t *testing.T) {
	even := adsb.PositionSample{Frame: frame(t, evenHex), At: instant}
	odd := adsb.PositionSample{Frame: frame(t, oddHex), At: instant.Add(-2 * time.Second)}
	for _, pair := range [][2]adsb.PositionSample{{even, odd}, {odd, even}} {
		fix, err := adsb.DecodeGlobal(pair[0], pair[1], instant)
		require.NoError(t, err)
		require.Equal(t, uint32(0x40621d), fix.ICAO)
		require.Equal(t, instant, fix.At)
		require.InDelta(t, 52.2572021484375, fix.Coordinates.Latitude, 1e-10)
		require.InDelta(t, 3.91937255859375, fix.Coordinates.Longitude, 1e-10)
	}
	c, err := adsb.EncodeCPR(adsb.Coordinates{Latitude: 52.2572021484375, Longitude: 3.91937255859375}, false)
	require.NoError(t, err)
	require.Equal(t, adsb.CPR{Latitude: 93000, Longitude: 51372}, c)

	// The same pair selects the odd solution when the odd frame is newer.
	odd.At = instant
	even.At = instant.Add(-time.Second)
	fix, err := adsb.DecodeGlobal(even, odd, instant)
	require.NoError(t, err)
	require.InDelta(t, 52.26578017412606, fix.Coordinates.Latitude, 1e-10)
	require.InDelta(t, 3.938912527901786, fix.Coordinates.Longitude, 1e-10)
	even.At = instant
	fix, err = adsb.DecodeGlobal(odd, even, instant)
	require.NoError(t, err)
	require.InDelta(t, 52.2572021484375, fix.Coordinates.Latitude, 1e-10)
}

// Rational-input cases are independently calculated from the CPR grid equations:
// at the equator NL=59; at 87 degrees NL=2; one full bin cycle is 131072.
func TestCPRExactGridVectors(t *testing.T) {
	for _, tt := range []struct {
		p        adsb.Coordinates
		odd      bool
		lat, lon uint32
	}{
		{adsb.Coordinates{Latitude: 0, Longitude: 0}, false, 0, 0},
		{adsb.Coordinates{Latitude: 0, Longitude: 180}, false, 0, 65536},
		{adsb.Coordinates{Latitude: 0, Longitude: -180}, false, 0, 65536},
		{adsb.Coordinates{Latitude: 0, Longitude: 180}, true, 0, 0},
		{adsb.Coordinates{Latitude: 87, Longitude: 60}, false, 65536, 43691},
		{adsb.Coordinates{Latitude: 87, Longitude: 60}, true, 33860, 21845},
		{adsb.Coordinates{Latitude: -87, Longitude: -60}, false, 65536, 87381},
		{adsb.Coordinates{Latitude: 90, Longitude: 0}, false, 0, 0},
		{adsb.Coordinates{Latitude: 90, Longitude: 0}, true, 98304, 0},
		{adsb.Coordinates{Latitude: -90, Longitude: 0}, true, 32768, 0},
		{adsb.Coordinates{Latitude: 3.0 / 131072, Longitude: 0}, false, 1, 0},
		{adsb.Coordinates{Latitude: 6 - 0.25*6/131072, Longitude: 0}, false, 0, 0},
	} {
		c, err := adsb.EncodeCPR(tt.p, tt.odd)
		require.NoError(t, err)
		require.Equal(t, adsb.CPR{Odd: tt.odd, Latitude: tt.lat, Longitude: tt.lon}, c)
	}
}

func TestCPRLongitudeWrapAndLatitudeBoundaries(t *testing.T) {
	points := []adsb.Coordinates{
		{Latitude: 0, Longitude: 179.999}, {Latitude: 0, Longitude: -179.999},
		{Latitude: 45, Longitude: 180}, {Latitude: -45, Longitude: -180},
		{Latitude: 10.469, Longitude: 120}, {Latitude: 10.472, Longitude: 120},
		{Latitude: -10.469, Longitude: -120}, {Latitude: -10.472, Longitude: -120},
		{Latitude: 53.09, Longitude: 10}, {Latitude: 53.10, Longitude: 10},
		{Latitude: 86.99, Longitude: 60}, {Latitude: 87, Longitude: 60},
		{Latitude: 87.01, Longitude: 60}, {Latitude: -87, Longitude: -60},
		{Latitude: 89, Longitude: -179.99}, {Latitude: -89, Longitude: 179.99},
		{Latitude: 90, Longitude: 0}, {Latitude: -90, Longitude: 0},
	}
	for _, p := range points {
		even := sampleAt(t, p, false, instant)
		odd := sampleAt(t, p, true, instant.Add(-time.Second))
		fix, err := adsb.DecodeGlobal(even, odd, instant)
		require.NoError(t, err, "position %+v", p)
		require.InDelta(t, p.Latitude, fix.Coordinates.Latitude, 0.0001)
		diff := math.Mod(fix.Coordinates.Longitude-p.Longitude+540, 360) - 180
		require.InDelta(t, 0, diff, 0.003)
		require.GreaterOrEqual(t, fix.Coordinates.Longitude, -180.0)
		require.Less(t, fix.Coordinates.Longitude, 180.0)
	}
}

func TestCPRRejectsCrossingZones(t *testing.T) {
	even := sampleAt(t, adsb.Coordinates{Latitude: 53.095, Longitude: 10}, false, instant.Add(-time.Second))
	odd := sampleAt(t, adsb.Coordinates{Latitude: 53.0955, Longitude: 10}, true, instant)
	_, err := adsb.DecodeGlobal(even, odd, instant)
	require.ErrorIs(t, err, adsb.ErrCPR)
}

func TestCPRPairValidation(t *testing.T) {
	even := adsb.PositionSample{Frame: frame(t, evenHex), At: instant}
	odd := adsb.PositionSample{Frame: frame(t, oddHex), At: instant}
	tests := []struct {
		name   string
		change func(*adsb.PositionSample, *adsb.PositionSample, *time.Time)
		err    error
	}{
		{"same parity", func(a, b *adsb.PositionSample, _ *time.Time) { b.Frame = a.Frame }, adsb.ErrCPR},
		{"different aircraft", func(_, b *adsb.PositionSample, _ *time.Time) { b.Frame = alter(t, b.Frame, 9, 32, 42) }, adsb.ErrCPR},
		{"old frame", func(a, _ *adsb.PositionSample, _ *time.Time) { a.At = instant.Add(-adsb.MaxCPRAge - time.Nanosecond) }, adsb.ErrCPR},
		{"both old", func(a, b *adsb.PositionSample, _ *time.Time) { a.At = instant.Add(-time.Minute); b.At = a.At }, adsb.ErrCPR},
		{"future", func(_, b *adsb.PositionSample, _ *time.Time) { b.At = instant.Add(time.Nanosecond) }, adsb.ErrCPR},
		{"missing time", func(a, _ *adsb.PositionSample, _ *time.Time) { a.At = time.Time{} }, adsb.ErrCPR},
		{"missing now", func(_, _ *adsb.PositionSample, now *time.Time) { *now = time.Time{} }, adsb.ErrCPR},
		{"not position", func(a, _ *adsb.PositionSample, _ *time.Time) { a.Frame = frame(t, identHex) }, adsb.ErrCPR},
		{"bad CRC", func(a, _ *adsb.PositionSample, _ *time.Time) { a.Frame[13] ^= 1 }, adsb.ErrParity},
		{"invalid decoded latitude", func(a, b *adsb.PositionSample, _ *time.Time) {
			a.Frame = alter(t, a.Frame, 55, 71, 0)
			b.Frame = alter(t, b.Frame, 55, 71, 65536)
		}, adsb.ErrCPR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, now := even, odd, instant
			tt.change(&a, &b, &now)
			fix, err := adsb.DecodeGlobal(a, b, now)
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, adsb.Fix{}, fix)
		})
	}
	odd.At = instant.Add(-adsb.MaxCPRAge)
	_, err := adsb.DecodeGlobal(even, odd, instant)
	require.NoError(t, err)
}

func TestCPREncoderRejectsInvalidCoordinates(t *testing.T) {
	for _, p := range []adsb.Coordinates{
		{Latitude: 90.001}, {Latitude: -90.001}, {Longitude: 180.001}, {Longitude: -180.001},
		{Latitude: math.NaN()}, {Latitude: math.Inf(-1)}, {Longitude: math.NaN()}, {Longitude: math.Inf(1)},
	} {
		c, err := adsb.EncodeCPR(p, false)
		require.ErrorIs(t, err, adsb.ErrInvalid)
		require.Equal(t, adsb.CPR{}, c)
	}
}
