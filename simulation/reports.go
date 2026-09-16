package simulation

import (
	"fmt"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
)

// Fixed wire profile of engine-generated traffic. These are deliberate
// synthetic scenario constants, not defaults read from configuration. The
// engine claims no calibrated navigation integrity and no complete ADS-B
// operational-status profile.
const (
	// airborneCapability is CA=5, an airborne transponder.
	airborneCapability = 5
	// identificationTypeCode is TC4 with category 0, meaning unspecified.
	identificationTypeCode = 4
	identificationCategory = 0
	// positionTypeCode is TC11 barometric airborne position.
	positionTypeCode = 11
	// velocitySubtype is TC19 subtype 1, ground components at one knot.
	velocitySubtype = 1
)

// header is the DF17 header shared by every report of one aircraft.
func (a *aircraft) header() adsb.Header {
	return adsb.Header{ICAO: a.icao, Capability: airborneCapability}
}

// encodeReport builds one complete frame for a message family from evaluated
// truth. odd selects CPR parity and is meaningful only for position reports.
//
// Codec errors are wrapped with the aircraft, family, and virtual instant
// while preserving their original causes for errors.Is.
func encodeReport(a *aircraft, kind MessageKind, state navState, odd bool, at time.Time) ([14]byte, error) {
	var (
		frame adsb.Frame
		err   error
	)
	switch kind {
	case IdentificationMessage:
		frame, err = adsb.EncodeIdentification(a.header(), adsb.Identification{
			TypeCode: identificationTypeCode,
			Category: identificationCategory,
			Callsign: a.callsign,
		})
	case PositionMessage:
		frame, err = encodePositionReport(a, state, odd)
	case VelocityMessage:
		frame, err = encodeVelocityReport(a, state)
	default:
		err = fmt.Errorf("%w: message kind %d", ErrInvalid, uint8(kind))
	}
	if err != nil {
		return [14]byte{}, fmt.Errorf("encode %s report for %s at %s: %w",
			kind, a.callsign, at.Format(time.RFC3339Nano), err)
	}
	return frame, nil
}

// encodePositionReport packs a TC11 barometric position with the aircraft's
// own even/odd CPR parity. Altitude is always available and comes from the
// clamped absolute-time trajectory.
func encodePositionReport(a *aircraft, state navState, odd bool) (adsb.Frame, error) {
	cpr, err := adsb.EncodeCPR(adsb.Coordinates{
		Latitude:  state.latitudeDegrees,
		Longitude: state.longitudeDegrees,
	}, odd)
	if err != nil {
		return adsb.Frame{}, err
	}
	altitude := state.altitudeFeet
	return adsb.EncodePosition(a.header(), adsb.AirbornePosition{
		TypeCode:           positionTypeCode,
		SurveillanceStatus: 0,
		Supplement:         false,
		AltitudeFeet:       &altitude,
		TimeSynchronized:   false,
		CPR:                cpr,
	})
}

// encodeVelocityReport packs a TC19 subtype 1 report from the ground
// components projected at the sampled point. A genuine zero component is an
// available measurement, not an unavailable field. Heading, airspeed, and
// GNSS-minus-barometric altitude stay unavailable: the engine models none.
func encodeVelocityReport(a *aircraft, state navState) (adsb.Frame, error) {
	east := adsb.Measurement{Value: state.eastKnots}
	north := adsb.Measurement{Value: state.northKnots}
	rate := adsb.Measurement{Value: state.verticalRateFeetPerMinute}

	return adsb.EncodeVelocity(a.header(), adsb.Velocity{
		Subtype:                   velocitySubtype,
		IntentChange:              false,
		IFRCapability:             false,
		NACv:                      0,
		EastKnots:                 &east,
		NorthKnots:                &north,
		BarometricVerticalRate:    true,
		VerticalRateFeetPerMinute: &rate,
	})
}
