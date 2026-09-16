package adsb_test

import (
	"math"
	"testing"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

func TestOverRangeVelocity(t *testing.T) {
	for _, tt := range []struct {
		name        string
		first, last int
		code        uint64
		want        float64
		get         func(*adsb.Velocity) *adsb.Measurement
	}{
		{"east", 47, 56, 1023, -1021.5, func(v *adsb.Velocity) *adsb.Measurement { return v.EastKnots }},
		{"north", 58, 67, 1023, -1021.5, func(v *adsb.Velocity) *adsb.Measurement { return v.NorthKnots }},
		{"vertical", 70, 78, 511, -32608, func(v *adsb.Velocity) *adsb.Measurement { return v.VerticalRateFeetPerMinute }},
		{"altitude difference", 82, 88, 127, 3137.5, func(v *adsb.Velocity) *adsb.Measurement { return v.GNSSMinusBaroFeet }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := alter(t, frame(t, groundHex), tt.first, tt.last, tt.code)
			m, err := adsb.Decode(f[:])
			require.NoError(t, err)
			require.Equal(t, &adsb.Measurement{Value: tt.want, OverRange: true}, tt.get(m.Velocity))
			encoded, err := adsb.EncodeVelocity(m.Header, *m.Velocity)
			require.NoError(t, err)
			require.Equal(t, f, encoded)
		})
	}
	// All eight final bits set represent a negative over-range difference,
	// not an unavailable field. This follows the primary table in ATC-334.
	f := alter(t, frame(t, groundHex), 81, 88, 255)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, &adsb.Measurement{Value: -3137.5, OverRange: true}, m.Velocity.GNSSMinusBaroFeet)

	f = alter(t, frame(t, airHex), 58, 67, 1023)
	f = alter(t, f, 38, 40, 4)
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, &adsb.Measurement{Value: 4086, OverRange: true}, m.Velocity.AirspeedKnots)
}

func TestVelocityTopBinThresholds(t *testing.T) {
	for _, tt := range []struct {
		sub   uint8
		value float64
		want  adsb.Measurement
	}{
		{1, 1021.5, adsb.Measurement{Value: 1021}},
		{1, math.Nextafter(1021.5, math.Inf(1)), adsb.Measurement{Value: 1021.5, OverRange: true}},
		{1, -2000, adsb.Measurement{Value: -1021.5, OverRange: true}},
		{2, 4086, adsb.Measurement{Value: 4084}},
		{2, math.Nextafter(4086, math.Inf(1)), adsb.Measurement{Value: 4086, OverRange: true}},
		{2, math.MaxFloat64, adsb.Measurement{Value: 4086, OverRange: true}},
	} {
		f, err := adsb.EncodeVelocity(adsb.Header{ICAO: 1, Capability: 5}, adsb.Velocity{Subtype: tt.sub, EastKnots: measurement(tt.value)})
		require.NoError(t, err)
		m, err := adsb.Decode(f[:])
		require.NoError(t, err)
		require.Equal(t, &tt.want, m.Velocity.EastKnots)
	}
	for _, v := range []adsb.Velocity{
		{Subtype: 1, NorthKnots: measurement(math.NaN())},
		{Subtype: 3, AirspeedKnots: &adsb.Measurement{Value: 12, OverRange: true}},
	} {
		_, err := adsb.EncodeVelocity(adsb.Header{ICAO: 1, Capability: 5}, v)
		require.ErrorIs(t, err, adsb.ErrInvalid)
	}
}

func TestSupersonicWireFields(t *testing.T) {
	f := alter(t, frame(t, groundHex), 38, 40, 2)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, measurement(-32), m.Velocity.EastKnots)
	require.Equal(t, measurement(-636), m.Velocity.NorthKnots)
	require.Equal(t, measurement(-832), m.Velocity.VerticalRateFeetPerMinute)
	f = alter(t, frame(t, airHex), 38, 40, 4)
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, measurement(1500), m.Velocity.AirspeedKnots)
	require.Equal(t, number(243.984375), m.Velocity.HeadingDegrees)
}

func TestGillhamAltitude(t *testing.T) {
	// go-adsb's independent DF4 fixture 2000102a10fc86 has AC=0x102a
	// and altitude 1300 ft. Removing the M bit gives DF17 ALT=0x82a.
	f := alter(t, frame(t, evenHex), 41, 52, 0x82a)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, number(1300), m.Position.AltitudeFeet)
}

func TestPreservePositionFlags(t *testing.T) {
	for tc := uint8(9); tc <= 18; tc++ {
		p := adsb.AirbornePosition{TypeCode: tc, SurveillanceStatus: 3, Supplement: true, TimeSynchronized: true, CPR: adsb.CPR{Odd: true, Latitude: 131071, Longitude: 131071}}
		f, err := adsb.EncodePosition(adsb.Header{ICAO: 0xfffffe, Capability: 7}, p)
		require.NoError(t, err)
		m, err := adsb.Decode(f[:])
		require.NoError(t, err)
		require.Equal(t, &p, m.Position)
	}
}
