package adsb

import (
	"errors"
	"fmt"
	"math"
	"strings"

	wire "kreklow.us/go/go-adsb/adsb"
)

// Frame is one 112-bit DF17 message, in transmission byte order, including CRC.
// It contains no RF preamble, timestamps, or AVR/Beast transport framing.
type Frame [14]byte

// Error categories are available through errors.Is.
var (
	// ErrInvalid identifies malformed fields, lengths, and input values.
	ErrInvalid = errors.New("invalid ADS-B data")
	// ErrUnsupported identifies formats outside the implemented wire subset.
	ErrUnsupported = errors.New("unsupported ADS-B format")
	// ErrParity identifies a received CRC that differs from the computed CRC.
	ErrParity = errors.New("ADS-B parity mismatch")
	// ErrCPR identifies a pair or reference that cannot yield an accepted fix.
	ErrCPR = errors.New("unusable CPR position")
)

// Header contains the DF17 fields preceding the extended squitter payload.
type Header struct {
	ICAO       uint32 // 24-bit address; 000000 and FFFFFF are rejected.
	Capability uint8  // CA: 0 or 4-7; 1-3 are reserved.
}

// Identification contains TC 1-4 aircraft identification and category.
type Identification struct {
	TypeCode uint8  // 1-4; category interpretation depends on this value.
	Category uint8  // Three-bit category, retained even when reserved.
	Callsign string // Up to eight uppercase ASCII letters, digits, or spaces; empty is unavailable.
}

// AirbornePosition contains TC 9-18 barometric position data.
// CPR holds encoded fractions, not an independently usable global position.
type AirbornePosition struct {
	TypeCode           uint8    // 9-18, retained without inferring navigation integrity.
	SurveillanceStatus uint8    // 0-3.
	Supplement         bool     // ME bit 8: version-dependent SAF/NIC supplement.
	AltitudeFeet       *float64 // Pressure altitude relative to 1013.25 hPa; nil is unavailable.
	TimeSynchronized   bool     // UTC synchronization flag, not a timestamp.
	CPR                CPR
}

// Message is a validated supported DF17 message. Exactly one payload is non-nil.
type Message struct {
	Header         Header
	Identification *Identification
	Position       *AirbornePosition
	Velocity       *Velocity
}

// Decode validates length, DF17, CRC, address, and the supported payload.
// It returns a zero Message on failure and never repairs corrupted bits.
func Decode(data []byte) (Message, error) {
	if len(data) != len(Frame{}) {
		return Message{}, fmt.Errorf("%w: frame has %d bytes, want 14", ErrInvalid, len(data))
	}
	if data[0]>>3 != 17 {
		return Message{}, fmt.Errorf("%w: downlink format %d", ErrUnsupported, data[0]>>3)
	}
	var raw wire.RawMessage
	if err := raw.UnmarshalBinary(data); err != nil {
		return Message{}, fmt.Errorf("%w: read frame: %w", ErrInvalid, err)
	}
	if raw.Parity() != raw.Bits(89, 112) {
		return Message{}, ErrParity
	}
	h := Header{ICAO: uint32(raw.Bits(9, 32)), Capability: uint8(raw.Bits(6, 8))}
	if err := validateHeader(h); err != nil {
		return Message{}, err
	}
	decoded, err := wire.NewMessage(&raw)
	if err != nil {
		return Message{}, fmt.Errorf("%w: decode frame: %w", ErrInvalid, err)
	}
	m := Message{Header: h}
	tc := uint8(raw.Bits(33, 37))
	switch {
	case tc >= 1 && tc <= 4:
		call, err := decoded.Call()
		if err != nil {
			return Message{}, fmt.Errorf("%w: callsign: %w", ErrInvalid, err)
		}
		if err := validateCallsign(call); err != nil {
			return Message{}, err
		}
		m.Identification = &Identification{TypeCode: tc, Category: uint8(raw.Bits(38, 40)), Callsign: call}
	case tc >= 9 && tc <= 18:
		p := &AirbornePosition{
			TypeCode: tc, SurveillanceStatus: uint8(raw.Bits(38, 39)),
			Supplement: raw.Bit(40) != 0, TimeSynchronized: raw.Bit(53) != 0,
			CPR: CPR{Odd: raw.Bit(54) != 0, Latitude: uint32(raw.Bits(55, 71)), Longitude: uint32(raw.Bits(72, 88))},
		}
		// The library rejects zero altitude. On the wire it means unavailable.
		if raw.Bits(41, 52) != 0 {
			alt, err := decoded.Alt()
			if err != nil {
				return Message{}, fmt.Errorf("%w: barometric altitude: %w", ErrInvalid, err)
			}
			value := float64(alt)
			p.AltitudeFeet = &value
		}
		m.Position = p
	case tc == 19:
		v, err := decodeVelocity(&raw)
		if err != nil {
			return Message{}, err
		}
		m.Velocity = &v
	default:
		return Message{}, fmt.Errorf("%w: type code %d", ErrUnsupported, tc)
	}
	return m, nil
}

// EncodeIdentification packs an identification payload and computes its CRC.
// Callsigns are right-padded with spaces; decoding trims only trailing spaces.
func EncodeIdentification(h Header, id Identification) (Frame, error) {
	if id.TypeCode < 1 || id.TypeCode > 4 || id.Category > 7 {
		return Frame{}, fmt.Errorf("%w: identification type/category", ErrInvalid)
	}
	if err := validateCallsign(id.Callsign); err != nil {
		return Frame{}, err
	}
	var f Frame
	put(&f, 33, 37, uint64(id.TypeCode))
	put(&f, 38, 40, uint64(id.Category))
	call := id.Callsign + strings.Repeat(" ", 8-len(id.Callsign))
	for i := range 8 {
		put(&f, 41+i*6, 46+i*6, uint64(call[i]&63))
	}
	return finish(h, f)
}

// EncodePosition packs barometric altitude and already encoded CPR fractions.
// Available altitude is rounded to the nearest 25 feet, ties upward, and must
// lie in [-1000, 50175] feet before rounding. Encoding always uses Q=1;
// Decode also accepts valid Q=0 Gillham altitude supplied by other sources.
func EncodePosition(h Header, p AirbornePosition) (Frame, error) {
	if p.TypeCode < 9 || p.TypeCode > 18 || p.SurveillanceStatus > 3 {
		return Frame{}, fmt.Errorf("%w: airborne position type/status", ErrInvalid)
	}
	if err := p.CPR.validate(); err != nil {
		return Frame{}, err
	}
	var alt uint64
	if p.AltitudeFeet != nil {
		value := *p.AltitudeFeet
		if !finite(value) || value < -1000 || value > 50175 {
			return Frame{}, fmt.Errorf("%w: altitude must be -1000..50175 feet", ErrInvalid)
		}
		n := uint64(math.Floor((value+1000)/25 + 0.5))
		alt = (n&0x7f0)<<1 | 0x10 | n&15
	}
	var f Frame
	put(&f, 33, 37, uint64(p.TypeCode))
	put(&f, 38, 39, uint64(p.SurveillanceStatus))
	put(&f, 40, 40, bit(p.Supplement))
	put(&f, 41, 52, alt)
	put(&f, 53, 53, bit(p.TimeSynchronized))
	put(&f, 54, 54, bit(p.CPR.Odd))
	put(&f, 55, 71, uint64(p.CPR.Latitude))
	put(&f, 72, 88, uint64(p.CPR.Longitude))
	return finish(h, f)
}

func validateHeader(h Header) error {
	if h.ICAO == 0 || h.ICAO >= 0xffffff {
		return fmt.Errorf("%w: ICAO address must be 000001..FFFFFE", ErrInvalid)
	}
	if h.Capability > 7 || (h.Capability > 0 && h.Capability < 4) {
		return fmt.Errorf("%w: reserved or out-of-range capability %d", ErrInvalid, h.Capability)
	}
	return nil
}

func validateCallsign(call string) error {
	if len(call) > 8 {
		return fmt.Errorf("%w: callsign exceeds eight characters", ErrInvalid)
	}
	for _, c := range call {
		if c != ' ' && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return fmt.Errorf("%w: unsupported callsign character %q", ErrInvalid, c)
		}
	}
	return nil
}

func finish(h Header, f Frame) (Frame, error) {
	if err := validateHeader(h); err != nil {
		return Frame{}, err
	}
	put(&f, 1, 5, 17)
	put(&f, 6, 8, uint64(h.Capability))
	put(&f, 9, 32, uint64(h.ICAO))
	var raw wire.RawMessage
	if err := raw.UnmarshalBinary(f[:]); err != nil {
		return Frame{}, fmt.Errorf("prepare parity: %w", err)
	}
	put(&f, 89, 112, raw.Parity())
	return f, nil
}

// put uses one-based, inclusive, MSB-first bit positions from the wire tables.
// All call sites use fixed valid widths and validate values before packing.
func put(f *Frame, first, last int, value uint64) {
	for p := last; p >= first; p-- {
		mask := byte(1 << (7 - (p-1)%8))
		f[(p-1)/8] &^= mask
		if value&1 != 0 {
			f[(p-1)/8] |= mask
		}
		value >>= 1
	}
}

func bit(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
