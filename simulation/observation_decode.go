package simulation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
)

type observedState struct {
	icao           uint32
	lastReceivedAt time.Time
	identity       *IdentityObservation
	position       *PositionObservation
	altitude       *AltitudeObservation
	velocity       *VelocityObservation
	even           *positionEvidence
	odd            *positionEvidence
}

type positionEvidence struct {
	sample   adsb.PositionSample
	evidence ObservationEvidence
}

func projectObservations(ctx context.Context, evidence []ObservationEvidence, now time.Time, expiry ObservationExpiry) ([]ObservedAircraft, error) {
	states := make(map[uint32]*observedState)
	for _, item := range evidence {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item.Timestamp.After(now) {
			return nil, fmt.Errorf("%w: transmission %d is future-dated", ErrInvalid, item.TransmissionSequence)
		}
		message, err := adsb.Decode(item.Frame[:])
		if err != nil {
			return nil, fmt.Errorf("decode received transmission %d: %w", item.TransmissionSequence, err)
		}
		if message.Header.ICAO != item.ICAO {
			return nil, fmt.Errorf("%w: transmission %d metadata ICAO %06X differs from frame %06X", ErrInvalid, item.TransmissionSequence, item.ICAO, message.Header.ICAO)
		}
		if !messageMatchesKind(message, item.Kind) {
			return nil, fmt.Errorf("%w: transmission %d metadata kind %s differs from decoded payload", ErrInvalid, item.TransmissionSequence, item.Kind)
		}

		state := states[item.ICAO]
		if state == nil {
			state = &observedState{icao: item.ICAO}
			states[item.ICAO] = state
		}
		if item.Timestamp.After(state.lastReceivedAt) {
			state.lastReceivedAt = item.Timestamp
		}
		switch item.Kind {
		case IdentificationMessage:
			state.identity = &IdentityObservation{
				Callsign: message.Identification.Callsign, ObservedAt: item.Timestamp, Evidence: item,
			}
		case PositionMessage:
			if message.Position.AltitudeFeet == nil {
				state.altitude = nil
			} else {
				state.altitude = &AltitudeObservation{
					Feet: *message.Position.AltitudeFeet, ObservedAt: item.Timestamp, Evidence: item,
				}
			}
			position := &positionEvidence{
				sample: adsb.PositionSample{Frame: adsb.Frame(item.Frame), At: item.Timestamp}, evidence: item,
			}
			if message.Position.CPR.Odd {
				state.odd = position
			} else {
				state.even = position
			}
			if state.even != nil && state.odd != nil {
				fix, decodeErr := adsb.DecodeGlobal(state.even.sample, state.odd.sample, item.Timestamp)
				switch {
				case decodeErr == nil:
					state.position = &PositionObservation{
						LatitudeDegrees: fix.Coordinates.Latitude, LongitudeDegrees: fix.Coordinates.Longitude,
						ObservedAt: fix.At,
						Evidence:   []ObservationEvidence{state.even.evidence, state.odd.evidence},
					}
				case errors.Is(decodeErr, adsb.ErrCPR):
					// An unusable pair leaves the last accepted fix unchanged.
				default:
					return nil, fmt.Errorf("decode CPR at transmission %d: %w", item.TransmissionSequence, decodeErr)
				}
			}
		case VelocityMessage:
			state.velocity = observedVelocity(*message.Velocity, item)
		}
	}

	aircraft := make([]ObservedAircraft, 0, len(states))
	for _, state := range states {
		if !freshObservation(state.identity, now, expiry.Identity) {
			state.identity = nil
		}
		if !freshObservation(state.position, now, expiry.Position) {
			state.position = nil
		}
		if !freshObservation(state.altitude, now, expiry.Altitude) {
			state.altitude = nil
		}
		if !freshObservation(state.velocity, now, expiry.Velocity) {
			state.velocity = nil
		}
		if state.identity == nil && state.position == nil && state.altitude == nil && state.velocity == nil {
			continue
		}
		aircraft = append(aircraft, ObservedAircraft{
			ICAO: state.icao, LastReceivedAt: state.lastReceivedAt,
			Identity: state.identity, Position: state.position,
			BarometricAltitude: state.altitude, Velocity: state.velocity,
		})
	}
	sort.Slice(aircraft, func(i, j int) bool { return aircraft[i].ICAO < aircraft[j].ICAO })
	return aircraft, nil
}

func messageMatchesKind(message adsb.Message, kind MessageKind) bool {
	switch kind {
	case IdentificationMessage:
		return message.Identification != nil
	case PositionMessage:
		return message.Position != nil
	case VelocityMessage:
		return message.Velocity != nil
	default:
		return false
	}
}

func observedVelocity(value adsb.Velocity, evidence ObservationEvidence) *VelocityObservation {
	result := &VelocityObservation{
		Subtype: value.Subtype, IntentChange: value.IntentChange, IFRCapability: value.IFRCapability,
		NACv: value.NACv, EastKnots: observedMeasurement(value.EastKnots),
		NorthKnots: observedMeasurement(value.NorthKnots), HeadingDegrees: copyFloat(value.HeadingDegrees),
		AirspeedKnots: observedMeasurement(value.AirspeedKnots), TrueAirspeed: value.TrueAirspeed,
		BarometricVerticalRate:    value.BarometricVerticalRate,
		VerticalRateFeetPerMinute: observedMeasurement(value.VerticalRateFeetPerMinute),
		GNSSMinusBaroFeet:         observedMeasurement(value.GNSSMinusBaroFeet),
		ObservedAt:                evidence.Timestamp, Evidence: evidence,
	}
	if value.EastKnots != nil && value.NorthKnots != nil && !value.EastKnots.OverRange && !value.NorthKnots.OverRange {
		speed := math.Hypot(value.EastKnots.Value, value.NorthKnots.Value)
		result.GroundSpeedKnots = &speed
		if speed > 0 {
			track := math.Atan2(value.EastKnots.Value, value.NorthKnots.Value) * 180 / math.Pi
			if track < 0 {
				track += 360
			}
			result.TrackDegrees = &track
		}
	}
	return result
}

func observedMeasurement(value *adsb.Measurement) *ObservedMeasurement {
	if value == nil {
		return nil
	}
	return &ObservedMeasurement{Value: value.Value, OverRange: value.OverRange}
}

func copyFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func freshObservation(value any, now time.Time, lifetime time.Duration) bool {
	var observedAt time.Time
	switch value := value.(type) {
	case *IdentityObservation:
		if value == nil {
			return false
		}
		observedAt = value.ObservedAt
	case *PositionObservation:
		if value == nil {
			return false
		}
		observedAt = value.ObservedAt
	case *AltitudeObservation:
		if value == nil {
			return false
		}
		observedAt = value.ObservedAt
	case *VelocityObservation:
		if value == nil {
			return false
		}
		observedAt = value.ObservedAt
	default:
		return false
	}
	return !observedAt.After(now) && now.Sub(observedAt) <= lifetime
}
