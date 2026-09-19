package adsb_test

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

const (
	identHex  = "8D4840D6202CC371C32CE0576098"
	evenHex   = "8D40621D58C382D690C8AC2863A7"
	oddHex    = "8D40621D58C386435CC412692AD6"
	groundHex = "8D485020994409940838175B284F"
	airHex    = "8DA05F219B06B6AF189400CBC33F"
)

func number(v float64) *float64               { return &v }
func measurement(v float64) *adsb.Measurement { return &adsb.Measurement{Value: v} }

func frame(t testing.TB, text string) adsb.Frame {
	t.Helper()
	data, err := hex.DecodeString(text)
	require.NoError(t, err)
	require.Len(t, data, 14)
	return adsb.Frame(data)
}

// These expectations come from the published examples listed in testdata/README.md.
// Encoding consumes these literal fields, not fields returned by our decoder.
func TestPublishedFrames(t *testing.T) {
	tests := []struct {
		name, hex string
		want      adsb.Message
	}{
		{"identification", identHex, adsb.Message{
			Header:         adsb.Header{ICAO: 0x4840d6, Capability: 5},
			Identification: &adsb.Identification{TypeCode: 4, Category: 0, Callsign: "KLM1023"},
		}},
		{"even position", evenHex, adsb.Message{
			Header:   adsb.Header{ICAO: 0x40621d, Capability: 5},
			Position: &adsb.AirbornePosition{TypeCode: 11, AltitudeFeet: number(38000), CPR: adsb.CPR{Latitude: 93000, Longitude: 51372}},
		}},
		{"odd position", oddHex, adsb.Message{
			Header:   adsb.Header{ICAO: 0x40621d, Capability: 5},
			Position: &adsb.AirbornePosition{TypeCode: 11, AltitudeFeet: number(38000), CPR: adsb.CPR{Odd: true, Latitude: 74158, Longitude: 50194}},
		}},
		{"ground velocity", groundHex, adsb.Message{
			Header: adsb.Header{ICAO: 0x485020, Capability: 5},
			Velocity: &adsb.Velocity{Subtype: 1, IFRCapability: true, EastKnots: measurement(-8), NorthKnots: measurement(-159),
				VerticalRateFeetPerMinute: measurement(-832), GNSSMinusBaroFeet: measurement(550)},
		}},
		{"airspeed", airHex, adsb.Message{
			Header: adsb.Header{ICAO: 0xa05f21, Capability: 5},
			Velocity: &adsb.Velocity{Subtype: 3, HeadingDegrees: number(243.984375), AirspeedKnots: measurement(375),
				TrueAirspeed: true, BarometricVerticalRate: true, VerticalRateFeetPerMinute: measurement(-2304)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := frame(t, tt.hex)
			got, err := adsb.Decode(input[:])
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			var encoded adsb.Frame
			switch {
			case tt.want.Identification != nil:
				encoded, err = adsb.EncodeIdentification(tt.want.Header, *tt.want.Identification)
			case tt.want.Position != nil:
				encoded, err = adsb.EncodePosition(tt.want.Header, *tt.want.Position)
			case tt.want.Velocity != nil:
				encoded, err = adsb.EncodeVelocity(tt.want.Header, *tt.want.Velocity)
			}
			require.NoError(t, err)
			require.Equal(t, input, encoded)
		})
	}
}

func TestDecodeRejectsLengthsAndTransportWrappers(t *testing.T) {
	f := frame(t, identHex)
	for _, length := range []int{0, 1, 7, 13, 15, 28} {
		data := make([]byte, length)
		copy(data, f[:])
		m, err := adsb.Decode(data)
		require.ErrorIs(t, err, adsb.ErrInvalid)
		require.Equal(t, adsb.Message{}, m)
	}
	for _, data := range [][]byte{[]byte(identHex), []byte("*" + identHex + ";\r\n"), append(make([]byte, 9), f[:]...)} {
		_, err := adsb.Decode(data)
		require.Error(t, err)
	}
}

func TestDecodeRejectsEverySingleBitError(t *testing.T) {
	original := frame(t, identHex)
	for i := range 112 {
		f := original
		f[i/8] ^= 1 << uint(7-i%8)
		m, err := adsb.Decode(f[:])
		require.Error(t, err, "bit %d", i+1)
		require.Equal(t, adsb.Message{}, m)
		if i >= 5 {
			require.ErrorIs(t, err, adsb.ErrParity)
		}
	}
}

// alter changes bits using test-only MSB-first masks, then computes
// CRC with polynomial division independent of go-adsb's lookup-table parity.
func alter(t testing.TB, f adsb.Frame, first, last int, value uint64) adsb.Frame {
	t.Helper()
	for pos := first; pos <= last; pos++ {
		mask := byte(1 << uint(7-(pos-1)%8))
		f[(pos-1)/8] &^= mask
		if value&(1<<uint(last-pos)) != 0 {
			f[(pos-1)/8] |= mask
		}
	}
	work := f
	for i := 11; i < 14; i++ {
		work[i] = 0
	}
	const poly uint32 = 0x1fff409
	for i := range 88 {
		if work[i/8]&(1<<uint(7-i%8)) == 0 {
			continue
		}
		for j := range 25 {
			if poly&(1<<uint(24-j)) != 0 {
				p := i + j
				work[p/8] ^= 1 << uint(7-p%8)
			}
		}
	}
	copy(f[11:], work[11:])
	return f
}

func TestUnsupportedAndInvalidWireFields(t *testing.T) {
	tests := []struct {
		name, source string
		first, last  int
		value        uint64
		err          error
	}{
		{"DF18", identHex, 1, 5, 18, adsb.ErrUnsupported},
		{"surface", identHex, 33, 37, 5, adsb.ErrUnsupported},
		{"no position", identHex, 33, 37, 0, adsb.ErrUnsupported},
		{"GNSS height", evenHex, 33, 37, 20, adsb.ErrUnsupported},
		{"operational status", identHex, 33, 37, 31, adsb.ErrUnsupported},
		{"velocity subtype zero", groundHex, 38, 40, 0, adsb.ErrUnsupported},
		{"velocity subtype five", groundHex, 38, 40, 5, adsb.ErrUnsupported},
		{"zero address", identHex, 9, 32, 0, adsb.ErrInvalid},
		{"all ones address", identHex, 9, 32, 0xffffff, adsb.ErrInvalid},
		{"reserved CA", identHex, 6, 8, 2, adsb.ErrInvalid},
		{"invalid callsign", identHex, 41, 46, 0, adsb.ErrInvalid},
		{"reserved velocity bits", groundHex, 79, 80, 1, adsb.ErrInvalid},
		{"invalid Gillham altitude", evenHex, 41, 52, 1, adsb.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := alter(t, frame(t, tt.source), tt.first, tt.last, tt.value)
			m, err := adsb.Decode(f[:])
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, adsb.Message{}, m)
		})
	}
}

func TestUnavailableWireValues(t *testing.T) {
	f := alter(t, frame(t, evenHex), 41, 52, 0)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	require.Nil(t, m.Position.AltitudeFeet)
	require.Equal(t, uint32(93000), m.Position.CPR.Latitude)

	f = frame(t, groundHex)
	for _, span := range [][2]int{{47, 56}, {58, 67}, {70, 78}, {82, 88}} {
		f = alter(t, f, span[0], span[1], 0)
	}
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Nil(t, m.Velocity.EastKnots)
	require.Nil(t, m.Velocity.NorthKnots)
	require.Nil(t, m.Velocity.VerticalRateFeetPerMinute)
	require.Nil(t, m.Velocity.GNSSMinusBaroFeet)
	// Signs have no meaning when their magnitude is unavailable.
	f = alter(t, f, 81, 81, 1)
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Nil(t, m.Velocity.GNSSMinusBaroFeet)

	f = alter(t, frame(t, airHex), 46, 46, 0)
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Nil(t, m.Velocity.HeadingDegrees)
	require.Equal(t, measurement(375), m.Velocity.AirspeedKnots)
}

func TestZeroValuesRemainAvailable(t *testing.T) {
	h := adsb.Header{ICAO: 1, Capability: 5}
	f, err := adsb.EncodeVelocity(h, adsb.Velocity{Subtype: 1, EastKnots: measurement(0), NorthKnots: measurement(0),
		VerticalRateFeetPerMinute: measurement(0), GNSSMinusBaroFeet: measurement(0)})
	require.NoError(t, err)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, measurement(0), m.Velocity.EastKnots)
	require.Equal(t, measurement(0), m.Velocity.NorthKnots)
	require.Equal(t, measurement(0), m.Velocity.VerticalRateFeetPerMinute)
	require.Equal(t, measurement(0), m.Velocity.GNSSMinusBaroFeet)
	f, err = adsb.EncodePosition(h, adsb.AirbornePosition{TypeCode: 9, AltitudeFeet: number(0)})
	require.NoError(t, err)
	m, err = adsb.Decode(f[:])
	require.NoError(t, err)
	require.Equal(t, number(0), m.Position.AltitudeFeet)
}

func TestAltitudeQuantization(t *testing.T) {
	for _, tt := range []struct{ input, want float64 }{
		{-1000, -1000}, {-987.51, -1000}, {-987.5, -975}, {12.49, 0}, {12.5, 25}, {50175, 50175},
	} {
		f, err := adsb.EncodePosition(adsb.Header{ICAO: 1, Capability: 5}, adsb.AirbornePosition{TypeCode: 18, AltitudeFeet: number(tt.input)})
		require.NoError(t, err)
		m, err := adsb.Decode(f[:])
		require.NoError(t, err)
		require.Equal(t, number(tt.want), m.Position.AltitudeFeet)
	}
}

func TestVelocityQuantizationAndSubtypes(t *testing.T) {
	for _, tt := range []struct {
		sub         uint8
		input, want float64
	}{
		{1, 0.49, 0}, {1, 0.5, 1}, {1, -0.5, -1}, {1, 1021, 1021},
		{2, 1.99, 0}, {2, 2, 4}, {2, -2, -4}, {2, 4084, 4084},
		{3, 375.5, 376}, {4, 1502, 1504},
	} {
		v := adsb.Velocity{Subtype: tt.sub, VerticalRateFeetPerMinute: measurement(-32), GNSSMinusBaroFeet: measurement(12.5)}
		if tt.sub <= 2 {
			v.EastKnots = measurement(tt.input)
		} else {
			v.AirspeedKnots = measurement(tt.input)
			v.HeadingDegrees = number(359.99)
		}
		f, err := adsb.EncodeVelocity(adsb.Header{ICAO: 1, Capability: 5}, v)
		require.NoError(t, err)
		m, err := adsb.Decode(f[:])
		require.NoError(t, err)
		if tt.sub <= 2 {
			require.Equal(t, measurement(tt.want), m.Velocity.EastKnots)
		} else {
			require.Equal(t, measurement(tt.want), m.Velocity.AirspeedKnots)
			require.Equal(t, number(0), m.Velocity.HeadingDegrees)
		}
		require.Equal(t, measurement(-64), m.Velocity.VerticalRateFeetPerMinute)
		require.Equal(t, measurement(25), m.Velocity.GNSSMinusBaroFeet)
	}
}

func TestEncoderValidation(t *testing.T) {
	h := adsb.Header{ICAO: 1, Capability: 5}
	for _, bad := range []adsb.Header{{ICAO: 0, Capability: 5}, {ICAO: 0xffffff, Capability: 5}, {ICAO: 0x1000000, Capability: 5}, {ICAO: 1, Capability: 1}, {ICAO: 1, Capability: 8}} {
		f, err := adsb.EncodeIdentification(bad, adsb.Identification{TypeCode: 4, Callsign: "ABC"})
		require.ErrorIs(t, err, adsb.ErrInvalid)
		require.Equal(t, adsb.Frame{}, f)
	}
	for _, id := range []adsb.Identification{{TypeCode: 0}, {TypeCode: 5}, {TypeCode: 4, Category: 8}, {TypeCode: 4, Callsign: "123456789"}, {TypeCode: 4, Callsign: "abc"}, {TypeCode: 4, Callsign: "A_B"}, {TypeCode: 4, Callsign: "é"}} {
		_, err := adsb.EncodeIdentification(h, id)
		require.ErrorIs(t, err, adsb.ErrInvalid)
	}
	for _, alt := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1000.01, 50175.01} {
		_, err := adsb.EncodePosition(h, adsb.AirbornePosition{TypeCode: 9, AltitudeFeet: number(alt)})
		require.ErrorIs(t, err, adsb.ErrInvalid)
	}
	for _, p := range []adsb.AirbornePosition{{TypeCode: 8}, {TypeCode: 20}, {TypeCode: 9, SurveillanceStatus: 4}, {TypeCode: 9, CPR: adsb.CPR{Latitude: 131072}}, {TypeCode: 9, CPR: adsb.CPR{Longitude: 131072}}} {
		_, err := adsb.EncodePosition(h, p)
		require.ErrorIs(t, err, adsb.ErrInvalid)
	}
	for _, v := range []adsb.Velocity{
		{Subtype: 1, NACv: 8}, {Subtype: 1, EastKnots: measurement(math.NaN())},
		{Subtype: 1, EastKnots: &adsb.Measurement{Value: 10, OverRange: true}},
		{Subtype: 1, HeadingDegrees: number(0)}, {Subtype: 1, TrueAirspeed: true},
		{Subtype: 3, EastKnots: measurement(0)}, {Subtype: 3, AirspeedKnots: measurement(-1)},
		{Subtype: 3, AirspeedKnots: measurement(math.Inf(1))}, {Subtype: 3, HeadingDegrees: number(360)},
		{Subtype: 3, HeadingDegrees: number(math.NaN())}, {Subtype: 3, HeadingDegrees: number(-1)},
		{Subtype: 1, VerticalRateFeetPerMinute: measurement(math.Inf(-1))}, {Subtype: 1, GNSSMinusBaroFeet: &adsb.Measurement{Value: 10, OverRange: true}},
	} {
		f, err := adsb.EncodeVelocity(h, v)
		require.ErrorIs(t, err, adsb.ErrInvalid)
		require.Equal(t, adsb.Frame{}, f)
	}
	for _, sub := range []uint8{0, 5, 255} {
		_, err := adsb.EncodeVelocity(h, adsb.Velocity{Subtype: sub})
		require.ErrorIs(t, err, adsb.ErrUnsupported)
	}
}

func TestCallsignPaddingAndEmpty(t *testing.T) {
	for _, call := range []string{"", " ", "A B 19", "ABCDEFGH"} {
		f, err := adsb.EncodeIdentification(adsb.Header{ICAO: 1, Capability: 0}, adsb.Identification{TypeCode: 1, Category: 7, Callsign: call})
		require.NoError(t, err)
		m, err := adsb.Decode(f[:])
		require.NoError(t, err)
		want := call
		if call == " " {
			want = ""
		}
		require.Equal(t, want, m.Identification.Callsign)
	}
}

func TestDecodeOwnsItsValues(t *testing.T) {
	f := frame(t, evenHex)
	m, err := adsb.Decode(f[:])
	require.NoError(t, err)
	for i := range f {
		f[i] = 0
	}
	require.Equal(t, number(38000), m.Position.AltitudeFeet)
	require.Equal(t, uint32(93000), m.Position.CPR.Latitude)
}

func FuzzDecode(f *testing.F) {
	for _, text := range []string{identHex, evenHex, oddHex, groundHex, airHex} {
		data, err := hex.DecodeString(text)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := adsb.Decode(data)
		if err != nil {
			require.Equal(t, adsb.Message{}, m)
			return
		}
		require.Len(t, data, 14)
		count := 0
		if m.Identification != nil {
			count++
		}
		if m.Position != nil {
			count++
		}
		if m.Velocity != nil {
			count++
		}
		require.Equal(t, 1, count)
	})
}
