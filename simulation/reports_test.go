package simulation

import (
	"encoding/hex"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

// The helpers below are an independent wire oracle. They pack DF17 bits and
// compute the Mode S CRC from the published field tables listed in
// internal/adsb/doc.go, without calling the codec under test. Comparing engine
// output with a codec round trip alone would only prove self-consistency.

// bitPacker accumulates big-endian bit fields.
type bitPacker struct {
	bits []uint8
}

func (p *bitPacker) put(value uint64, width int) {
	for i := width - 1; i >= 0; i-- {
		p.bits = append(p.bits, uint8((value>>uint(i))&1))
	}
}

func (p *bitPacker) bytes() []byte {
	out := make([]byte, len(p.bits)/8)
	for i, bit := range p.bits {
		out[i/8] |= bit << uint(7-i%8)
	}
	return out
}

// oracleParity computes the 24-bit Mode S CRC of the leading 88 bits using
// the generator 0x1FFF409, bit by bit.
func oracleParity(header []byte) uint32 {
	var crc uint32
	feed := func(bit uint32) {
		top := (crc >> 23) & 1
		crc = ((crc << 1) | bit) & 0xffffff
		if top == 1 {
			crc ^= 0xfff409
		}
	}
	for _, b := range header {
		for i := 7; i >= 0; i-- {
			feed(uint32(b>>uint(i)) & 1)
		}
	}
	for range 24 {
		feed(0)
	}
	return crc
}

// oracleFrame assembles a complete DF17 frame from a 56-bit ME payload.
func oracleFrame(icao uint32, capability uint8, me []byte) [14]byte {
	var p bitPacker
	p.put(17, 5)
	p.put(uint64(capability), 3)
	p.put(uint64(icao), 24)
	head := p.bytes()
	head = append(head, me...)

	crc := oracleParity(head)
	var frame [14]byte
	copy(frame[:], head)
	frame[11] = byte(crc >> 16)
	frame[12] = byte(crc >> 8)
	frame[13] = byte(crc)
	return frame
}

// oracleCharacter maps one callsign character to its six-bit code.
func oracleCharacter(r rune) uint64 {
	switch {
	case r >= 'A' && r <= 'Z':
		return uint64(r-'A') + 1
	case r >= '0' && r <= '9':
		return uint64(r-'0') + 48
	default:
		return 32
	}
}

func oracleIdentificationME(typeCode, category uint8, sign string) []byte {
	var p bitPacker
	p.put(uint64(typeCode), 5)
	p.put(uint64(category), 3)
	padded := sign + strings.Repeat(" ", 8-len(sign))
	for _, r := range padded {
		p.put(oracleCharacter(r), 6)
	}
	return p.bytes()
}

// oracleAltitude encodes pressure altitude with the Q=1 25-foot layout.
func oracleAltitude(feet float64) uint64 {
	n := uint64(math.Floor((feet+1000)/25 + 0.5))
	return ((n >> 4) << 5) | (1 << 4) | (n & 0xf)
}

func cprModulo(x, y float64) float64 { return x - y*math.Floor(x/y) }

// oracleNL is the CPR longitude-zone count.
func oracleNL(latitude float64) float64 {
	a := math.Abs(latitude)
	switch {
	case a == 0:
		return 59
	case a == 87:
		return 2
	case a > 87:
		return 1
	}
	numerator := 1 - math.Cos(math.Pi/30)
	denominator := math.Pow(math.Cos(math.Pi/180*a), 2)
	return math.Floor(2 * math.Pi / math.Acos(1-numerator/denominator))
}

func oracleCPR(latitude, longitude float64, odd bool) (latitudeFraction, longitudeFraction uint64) {
	parity := 0.0
	if odd {
		parity = 1
	}
	const bins = 131072.0

	latZoneSize := 360.0 / (60 - parity)
	yz := math.Floor(bins*cprModulo(latitude, latZoneSize)/latZoneSize + 0.5)
	roundedLatitude := latZoneSize * (yz/bins + math.Floor(latitude/latZoneSize))

	zones := oracleNL(roundedLatitude) - parity
	if zones < 1 {
		zones = 1
	}
	lonZoneSize := 360.0 / zones
	xz := math.Floor(bins*cprModulo(longitude, lonZoneSize)/lonZoneSize + 0.5)

	return uint64(int64(yz) & 0x1ffff), uint64(int64(xz) & 0x1ffff)
}

func oraclePositionME(typeCode uint8, altitudeFeet float64, odd bool, latitude, longitude float64) []byte {
	latFraction, lonFraction := oracleCPR(latitude, longitude, odd)
	var p bitPacker
	p.put(uint64(typeCode), 5)
	p.put(0, 2) // Surveillance status.
	p.put(0, 1) // Supplement.
	p.put(oracleAltitude(altitudeFeet), 12)
	p.put(0, 1) // Time synchronization.
	if odd {
		p.put(1, 1)
	} else {
		p.put(0, 1)
	}
	p.put(latFraction, 17)
	p.put(lonFraction, 17)
	return p.bytes()
}

// oracleComponent encodes a signed one-knot velocity component: code 1 is an
// available zero, and ordinary codes are magnitude plus one.
func oracleComponent(value, step float64) (sign, code uint64) {
	if value < 0 {
		sign = 1
	}
	code = uint64(math.Floor(math.Abs(value)/step+0.5)) + 1
	return sign, code
}

func oracleVelocityME(subtype uint8, eastKnots, northKnots, verticalRate float64) []byte {
	eastSign, eastCode := oracleComponent(eastKnots, 1)
	northSign, northCode := oracleComponent(northKnots, 1)
	rateSign, rateCode := oracleComponent(verticalRate, 64)

	var p bitPacker
	p.put(19, 5)
	p.put(uint64(subtype), 3)
	p.put(0, 1) // Intent change.
	p.put(0, 1) // IFR capability.
	p.put(0, 3) // NACv.
	p.put(eastSign, 1)
	p.put(eastCode, 10)
	p.put(northSign, 1)
	p.put(northCode, 10)
	p.put(1, 1) // Barometric vertical rate source.
	p.put(rateSign, 1)
	p.put(rateCode, 9)
	p.put(0, 2) // Reserved.
	p.put(0, 1) // GNSS minus barometric sign.
	p.put(0, 7) // GNSS minus barometric magnitude, unavailable.
	return p.bytes()
}

func hexFrame(t testing.TB, text string) [14]byte {
	t.Helper()

	raw, err := hex.DecodeString(text)
	require.NoError(t, err)
	require.Len(t, raw, 14)
	return [14]byte(raw)
}

// The oracle CRC must reproduce the externally published frames already used
// by the codec's own tests before it can judge engine output.
func TestReportParityOracle(t *testing.T) {
	t.Parallel()

	published := []string{
		"8D4840D6202CC371C32CE0576098",
		"8D40621D58C382D690C8AC2863A7",
		"8D40621D58C386435CC412692AD6",
		"8D485020994409940838175B284F",
	}
	for _, text := range published {
		frame := hexFrame(t, text)
		want := uint32(frame[11])<<16 | uint32(frame[12])<<8 | uint32(frame[13])
		require.Equal(t, want, oracleParity(frame[:11]), text)
	}
}

// stationaryAircraft returns the fixed single-aircraft scenario used for the
// literal wire expectations below.
func stationaryAircraft(t testing.TB) aircraft {
	t.Helper()

	cfg, err := normalizeConfig(stationaryConfig())
	require.NoError(t, err)
	return newAircraft(cfg, 0x000001, 1, 0)
}

// Literal expectations for the stationary fixture: address 000001, callsign
// TB000001, 50N 14E, 35000 ft pressure altitude, zero ground speed, and zero
// vertical rate. Each frame is also rebuilt by the independent oracle above.
const (
	stationaryIdentificationHex = "8D00000120502C30C30C31A242B6"
	stationaryEvenPositionHex   = "8D00000158B5015556F49F1870D8"
	stationaryOddPositionHex    = "8D00000158B504C71CE0B627BEC9"
	stationaryVelocityHex       = "8D00000199000100300400D9CB25"
)

func TestReportStationaryFrames(t *testing.T) {
	t.Parallel()

	craft := stationaryAircraft(t)
	state := craft.navAt(0)
	at := fixtureStart

	tests := []struct {
		name string
		kind MessageKind
		odd  bool
		me   []byte
		want string
	}{
		{
			"identification", IdentificationMessage, false,
			oracleIdentificationME(identificationTypeCode, identificationCategory, "TB000001"),
			stationaryIdentificationHex,
		},
		{
			"even position", PositionMessage, false,
			oraclePositionME(positionTypeCode, 35000, false, 50, 14),
			stationaryEvenPositionHex,
		},
		{
			"odd position", PositionMessage, true,
			oraclePositionME(positionTypeCode, 35000, true, 50, 14),
			stationaryOddPositionHex,
		},
		{
			"velocity", VelocityMessage, false,
			oracleVelocityME(velocitySubtype, 0, 0, 0),
			stationaryVelocityHex,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := encodeReport(&craft, test.kind, state, test.odd, at)
			require.NoError(t, err)

			require.Equal(t, oracleFrame(craft.icao, airborneCapability, test.me), got, "independent oracle")
			require.Equal(t, hexFrame(t, test.want), got, "recorded literal")
		})
	}
}

// Every emitted frame must decode back to the right aircraft and family.
func TestReportDecodes(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeConfig(validConfig())
	require.NoError(t, err)
	craft := newAircraft(cfg, 0x00c0de, 9, 0)
	state := craft.navAt(45 * time.Second)

	for _, kind := range families {
		frame, err := encodeReport(&craft, kind, state, kind == PositionMessage, fixtureStart)
		require.NoError(t, err)

		message, err := adsb.Decode(frame[:])
		require.NoError(t, err)
		require.Equal(t, craft.icao, message.Header.ICAO)
		require.Equal(t, uint8(airborneCapability), message.Header.Capability)

		switch kind {
		case IdentificationMessage:
			require.NotNil(t, message.Identification)
			require.Equal(t, craft.callsign, message.Identification.Callsign)
			require.Equal(t, uint8(identificationTypeCode), message.Identification.TypeCode)
		case PositionMessage:
			require.NotNil(t, message.Position)
			require.Equal(t, uint8(positionTypeCode), message.Position.TypeCode)
			require.True(t, message.Position.CPR.Odd)
			require.NotNil(t, message.Position.AltitudeFeet)
			require.InDelta(t, state.altitudeFeet, *message.Position.AltitudeFeet, 12.5)
		case VelocityMessage:
			require.NotNil(t, message.Velocity)
			require.Equal(t, uint8(velocitySubtype), message.Velocity.Subtype)
			require.True(t, message.Velocity.BarometricVerticalRate)
			require.NotNil(t, message.Velocity.EastKnots)
			require.NotNil(t, message.Velocity.NorthKnots)
			require.InDelta(t, state.eastKnots, message.Velocity.EastKnots.Value, 0.5)
			require.InDelta(t, state.northKnots, message.Velocity.NorthKnots.Value, 0.5)
			require.NotNil(t, message.Velocity.VerticalRateFeetPerMinute)
			require.InDelta(t, state.verticalRateFeetPerMinute, message.Velocity.VerticalRateFeetPerMinute.Value, 32)
			require.Nil(t, message.Velocity.HeadingDegrees)
			require.Nil(t, message.Velocity.AirspeedKnots)
			require.Nil(t, message.Velocity.GNSSMinusBaroFeet)
		}
	}
}

// A zero ground component is available, not unavailable.
func TestReportZeroComponentsAreAvailable(t *testing.T) {
	t.Parallel()

	craft := stationaryAircraft(t)
	frame, err := encodeReport(&craft, VelocityMessage, craft.navAt(0), false, fixtureStart)
	require.NoError(t, err)

	message, err := adsb.Decode(frame[:])
	require.NoError(t, err)
	require.NotNil(t, message.Velocity.EastKnots)
	require.Equal(t, 0.0, message.Velocity.EastKnots.Value)
	require.NotNil(t, message.Velocity.NorthKnots)
	require.Equal(t, 0.0, message.Velocity.NorthKnots.Value)
	require.NotNil(t, message.Velocity.VerticalRateFeetPerMinute)
	require.Equal(t, 0.0, message.Velocity.VerticalRateFeetPerMinute.Value)
}

func TestReportRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	craft := stationaryAircraft(t)
	_, err := encodeReport(&craft, MessageKind(9), craft.navAt(0), false, fixtureStart)
	require.ErrorIs(t, err, ErrInvalid)
}

// Decoded payloads must agree with the truth evaluated at each frame's own
// timestamp, within the codec's documented quantization.
func TestReportPayloadsMatchTruth(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.InitialAircraftCount = 1
	engine := newEngine(t, cfg)
	craft := engine.state.fleet[0]

	got, err := engine.Advance(t.Context(), 20*time.Second)
	require.NoError(t, err)
	batch := got.Transmissions
	require.NotEmpty(t, batch)

	var (
		positions []adsb.PositionSample
		globals   int
	)
	for _, emitted := range batch {
		elapsed := emitted.Timestamp.Sub(fixtureStart)
		truth := craft.navAt(elapsed)

		message, err := adsb.Decode(emitted.Frame[:])
		require.NoError(t, err)

		switch emitted.Kind {
		case IdentificationMessage:
			require.Equal(t, craft.callsign, message.Identification.Callsign)
		case PositionMessage:
			require.InDelta(t, truth.altitudeFeet, *message.Position.AltitudeFeet, 12.5)
			positions = append(positions, adsb.PositionSample{Frame: adsb.Frame(emitted.Frame), At: emitted.Timestamp})
		case VelocityMessage:
			require.InDelta(t, truth.eastKnots, message.Velocity.EastKnots.Value, 0.5)
			require.InDelta(t, truth.northKnots, message.Velocity.NorthKnots.Value, 0.5)
			require.InDelta(t, truth.verticalRateFeetPerMinute, message.Velocity.VerticalRateFeetPerMinute.Value, 32)
		}
	}

	// Reconstruct positions from consecutive opposite-parity pairs. The codec
	// rejects pairs that straddle a latitude zone boundary; those are skipped
	// rather than treated as engine failures.
	for i := 1; i < len(positions); i++ {
		previous, current := positions[i-1], positions[i]
		fix, err := adsb.DecodeGlobal(previous, current, current.At)
		if err != nil {
			require.ErrorIs(t, err, adsb.ErrCPR)
			continue
		}
		truth := craft.navAt(fix.At.Sub(fixtureStart))
		require.InDelta(t, truth.latitudeDegrees, fix.Coordinates.Latitude, 0.01)
		require.InDelta(t, truth.longitudeDegrees, fix.Coordinates.Longitude, 0.01)
		globals++
	}
	require.NotZero(t, globals, "at least one pair decodes globally")
}
