package adsb

import (
	"fmt"
	"math"

	wire "kreklow.us/go/go-adsb/adsb"
)

// Measurement is an available velocity or altitude-difference value.
// When OverRange is true, the absolute value is a strict lower bound on
// magnitude rather than an exact measurement. The sign retains direction.
type Measurement struct {
	Value     float64
	OverRange bool
}

// Velocity is TC19 airborne velocity. Nil numeric fields mean unavailable.
// Subtypes 1/2 carry ground components; 3/4 carry heading and airspeed.
// Subtypes 2/4 use four-knot resolution; 1/3 use one-knot resolution.
type Velocity struct {
	Subtype                   uint8 // 1-4.
	IntentChange              bool
	IFRCapability             bool         // ME bit 10: legacy IFR capability; interpretation depends on version.
	NACv                      uint8        // Raw three-bit velocity quality category; no accuracy inferred.
	EastKnots                 *Measurement // Signed east component, only subtypes 1/2.
	NorthKnots                *Measurement // Signed north component, only subtypes 1/2.
	HeadingDegrees            *float64     // Magnetic heading [0,360), only subtypes 3/4, not ground track.
	AirspeedKnots             *Measurement // Nonnegative, only subtypes 3/4.
	TrueAirspeed              bool         // For subtypes 3/4: true airspeed if set, indicated otherwise.
	BarometricVerticalRate    bool         // True: pressure source; false: geometric source.
	VerticalRateFeetPerMinute *Measurement // Signed upward, 64 ft/min resolution.
	GNSSMinusBaroFeet         *Measurement // Signed geometric minus pressure altitude, 25 ft resolution.
}

// EncodeVelocity packs TC19 velocity and computes CRC. Speeds and rates round
// to the nearest representable magnitude, ties away from zero. Values above
// the top exact bin use the wire's over-range code. An explicit OverRange
// measurement must contain the exact signed threshold documented in doc.go.
// Heading rounds to the nearest 360/1024 degree, wrapping 360 to zero.
// Fields belonging to another subtype are rejected. Encoded zero magnitudes
// represent unavailable, not zero speed.
func EncodeVelocity(h Header, v Velocity) (Frame, error) {
	if v.Subtype < 1 || v.Subtype > 4 {
		return Frame{}, fmt.Errorf("%w: velocity subtype %d", ErrUnsupported, v.Subtype)
	}
	if v.NACv > 7 {
		return Frame{}, fmt.Errorf("%w: NACv exceeds three bits", ErrInvalid)
	}
	step := velocityStep(v.Subtype)
	var f Frame
	put(&f, 33, 37, 19)
	put(&f, 38, 40, uint64(v.Subtype))
	put(&f, 41, 41, bit(v.IntentChange))
	put(&f, 42, 42, bit(v.IFRCapability))
	put(&f, 43, 45, uint64(v.NACv))
	if v.Subtype <= 2 {
		if v.HeadingDegrees != nil || v.AirspeedKnots != nil || v.TrueAirspeed {
			return Frame{}, fmt.Errorf("%w: airspeed fields in ground velocity", ErrInvalid)
		}
		east, err := magnitude(v.EastKnots, step, 1023, true)
		if err != nil {
			return Frame{}, fmt.Errorf("east velocity: %w", err)
		}
		north, err := magnitude(v.NorthKnots, step, 1023, true)
		if err != nil {
			return Frame{}, fmt.Errorf("north velocity: %w", err)
		}
		put(&f, 46, 46, sign(v.EastKnots))
		put(&f, 47, 56, east)
		put(&f, 57, 57, sign(v.NorthKnots))
		put(&f, 58, 67, north)
	} else {
		if v.EastKnots != nil || v.NorthKnots != nil {
			return Frame{}, fmt.Errorf("%w: ground components in airspeed velocity", ErrInvalid)
		}
		if v.HeadingDegrees != nil {
			heading := *v.HeadingDegrees
			if !finite(heading) || heading < 0 || heading >= 360 {
				return Frame{}, fmt.Errorf("%w: heading must be 0..<360 degrees", ErrInvalid)
			}
			put(&f, 46, 46, 1)
			put(&f, 47, 56, uint64(math.Floor(heading*1024/360+0.5))%1024)
		}
		speed, err := magnitude(v.AirspeedKnots, step, 1023, false)
		if err != nil {
			return Frame{}, fmt.Errorf("airspeed: %w", err)
		}
		put(&f, 57, 57, bit(v.TrueAirspeed))
		put(&f, 58, 67, speed)
	}
	rate, err := magnitude(v.VerticalRateFeetPerMinute, 64, 511, true)
	if err != nil {
		return Frame{}, fmt.Errorf("vertical rate: %w", err)
	}
	diff, err := magnitude(v.GNSSMinusBaroFeet, 25, 127, true)
	if err != nil {
		return Frame{}, fmt.Errorf("altitude difference: %w", err)
	}
	put(&f, 68, 68, bit(v.BarometricVerticalRate))
	put(&f, 69, 69, sign(v.VerticalRateFeetPerMinute))
	put(&f, 70, 78, rate)
	put(&f, 81, 81, sign(v.GNSSMinusBaroFeet))
	put(&f, 82, 88, diff)
	return finish(h, f)
}

func decodeVelocity(r *wire.RawMessage) (Velocity, error) {
	sub := uint8(r.Bits(38, 40))
	if sub < 1 || sub > 4 {
		return Velocity{}, fmt.Errorf("%w: velocity subtype %d", ErrUnsupported, sub)
	}
	if r.Bits(79, 80) != 0 {
		return Velocity{}, fmt.Errorf("%w: reserved velocity bits are nonzero", ErrInvalid)
	}
	v := Velocity{
		Subtype: sub, IntentChange: r.Bit(41) != 0, IFRCapability: r.Bit(42) != 0,
		NACv: uint8(r.Bits(43, 45)), BarometricVerticalRate: r.Bit(68) != 0,
		VerticalRateFeetPerMinute: signedValue(r.Bits(70, 78), r.Bit(69), 64, 511),
		GNSSMinusBaroFeet:         signedValue(r.Bits(82, 88), r.Bit(81), 25, 127),
	}
	step := velocityStep(sub)
	if sub <= 2 {
		v.EastKnots = signedValue(r.Bits(47, 56), r.Bit(46), step, 1023)
		v.NorthKnots = signedValue(r.Bits(58, 67), r.Bit(57), step, 1023)
	} else {
		if r.Bit(46) != 0 {
			heading := float64(r.Bits(47, 56)) * 360 / 1024
			v.HeadingDegrees = &heading
		}
		v.TrueAirspeed = r.Bit(57) != 0
		v.AirspeedKnots = signedValue(r.Bits(58, 67), 0, step, 1023)
	}
	return v, nil
}

func velocityStep(subtype uint8) float64 {
	if subtype == 2 || subtype == 4 {
		return 4
	}
	return 1
}

func magnitude(v *Measurement, step float64, maxCode uint64, signed bool) (uint64, error) {
	if v == nil {
		return 0, nil
	}
	if !finite(v.Value) || (!signed && v.Value < 0) {
		return 0, fmt.Errorf("%w: measurement is non-finite or has an invalid sign", ErrInvalid)
	}
	threshold := (float64(maxCode) - 1.5) * step
	if v.OverRange {
		if math.Abs(v.Value) != threshold {
			return 0, fmt.Errorf("%w: over-range measurement must use magnitude %g", ErrInvalid, threshold)
		}
		return maxCode, nil
	}
	if math.Abs(v.Value) > threshold {
		return maxCode, nil
	}
	// At the exact threshold the standard still selects the top exact bin.
	return min(uint64(math.Floor(math.Abs(v.Value)/step+0.5))+1, maxCode-1), nil
}

func sign(v *Measurement) uint64 {
	return bit(v != nil && v.Value < 0)
}

func signedValue(code uint64, negative uint8, step float64, maxCode uint64) *Measurement {
	if code == 0 {
		return nil
	}
	value := float64(code-1) * step
	over := code == maxCode
	if over {
		value = (float64(maxCode) - 1.5) * step
	}
	if negative != 0 {
		value = -value
	}
	return &Measurement{Value: value, OverRange: over}
}
