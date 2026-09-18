package simulation

import (
	"errors"
	"testing"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/stretchr/testify/require"
)

func encodedIdentification(t testing.TB, icao uint32, callsign string) adsb.Frame {
	t.Helper()
	frame, err := adsb.EncodeIdentification(adsb.Header{ICAO: icao, Capability: 5}, adsb.Identification{TypeCode: 4, Callsign: callsign})
	require.NoError(t, err)
	return frame
}

func encodedPosition(t testing.TB, icao uint32, odd bool, altitude float64) adsb.Frame {
	t.Helper()
	cpr, err := adsb.EncodeCPR(adsb.Coordinates{Latitude: 50, Longitude: 14}, odd)
	require.NoError(t, err)
	frame, err := adsb.EncodePosition(adsb.Header{ICAO: icao, Capability: 5}, adsb.AirbornePosition{
		TypeCode: 11, AltitudeFeet: &altitude, CPR: cpr,
	})
	require.NoError(t, err)
	return frame
}

func encodedVelocity(t testing.TB, icao uint32, east, north float64) adsb.Frame {
	t.Helper()
	frame, err := adsb.EncodeVelocity(adsb.Header{ICAO: icao, Capability: 5}, adsb.Velocity{
		Subtype: 1, EastKnots: &adsb.Measurement{Value: east}, NorthKnots: &adsb.Measurement{Value: north},
	})
	require.NoError(t, err)
	return frame
}

func observedEvidence(frame adsb.Frame, kind MessageKind, sequence uint64, at time.Time) ObservationEvidence {
	message, err := adsb.Decode(frame[:])
	if err != nil {
		panic(err)
	}
	reception := Reception{
		Sequence: sequence, TransmissionSequence: sequence, StationID: "fixture",
		ICAO: message.Header.ICAO, Kind: kind, Timestamp: at, Frame: [14]byte(frame),
	}
	return ObservationEvidence{
		TransmissionSequence: sequence, ICAO: message.Header.ICAO, Kind: kind,
		Timestamp: at, Frame: [14]byte(frame), Receptions: []Reception{reception},
	}
}

func TestObservationDecodeSupportsPartialStateAndCPR(t *testing.T) {
	t.Parallel()

	const icao = 0xabc123
	now := fixtureStart.Add(time.Second)
	even := observedEvidence(encodedPosition(t, icao, false, 35000), PositionMessage, 1, fixtureStart)

	partial, err := projectObservations(t.Context(), []ObservationEvidence{even}, now, validObservationExpiry())
	require.NoError(t, err)
	require.Len(t, partial, 1)
	require.Nil(t, partial[0].Position)
	require.NotNil(t, partial[0].BarometricAltitude)

	odd := observedEvidence(encodedPosition(t, icao, true, 35000), PositionMessage, 2, fixtureStart.Add(500*time.Millisecond))
	full, err := projectObservations(t.Context(), []ObservationEvidence{even, odd}, now, validObservationExpiry())
	require.NoError(t, err)
	require.Len(t, full, 1)
	require.NotNil(t, full[0].Position)
	require.InDelta(t, 50, full[0].Position.LatitudeDegrees, 0.01)
	require.InDelta(t, 14, full[0].Position.LongitudeDegrees, 0.01)
	require.Len(t, full[0].Position.Evidence, 2)
}

func TestObservationExpiryIsIndependentAndInclusive(t *testing.T) {
	t.Parallel()

	const icao = 0xabc123
	evidence := []ObservationEvidence{
		observedEvidence(encodedIdentification(t, icao, "TEST123"), IdentificationMessage, 1, fixtureStart),
		observedEvidence(encodedPosition(t, icao, false, 35000), PositionMessage, 2, fixtureStart),
		observedEvidence(encodedPosition(t, icao, true, 35000), PositionMessage, 3, fixtureStart.Add(time.Second)),
		observedEvidence(encodedVelocity(t, icao, 0, 0), VelocityMessage, 4, fixtureStart.Add(2*time.Second)),
	}
	now := fixtureStart.Add(12 * time.Second)
	expiry := ObservationExpiry{Identity: 12 * time.Second, Position: 10 * time.Second, Altitude: 11 * time.Second, Velocity: 9*time.Second + 999*time.Millisecond}

	got, err := projectObservations(t.Context(), evidence, now, expiry)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Identity, "exact lifetime is fresh")
	require.Nil(t, got[0].Position, "position is one second beyond its lifetime")
	require.NotNil(t, got[0].BarometricAltitude, "altitude ages from the newer position frame")
	require.Nil(t, got[0].Velocity)
	require.Equal(t, fixtureStart.Add(2*time.Second), got[0].LastReceivedAt)
}

func TestObservationVelocityPreservesZeroAndUnknownTrack(t *testing.T) {
	t.Parallel()

	frame := encodedVelocity(t, 0xabc123, 0, 0)
	got, err := projectObservations(t.Context(), []ObservationEvidence{
		observedEvidence(frame, VelocityMessage, 1, fixtureStart),
	}, fixtureStart, validObservationExpiry())
	require.NoError(t, err)
	velocity := got[0].Velocity
	require.NotNil(t, velocity.EastKnots)
	require.Zero(t, velocity.EastKnots.Value)
	require.NotNil(t, velocity.GroundSpeedKnots)
	require.Zero(t, *velocity.GroundSpeedKnots)
	require.Nil(t, velocity.TrackDegrees)
}

func TestObservationDecodeRejectsCorruptAndMismatchedEvidence(t *testing.T) {
	t.Parallel()

	item := observedEvidence(encodedIdentification(t, 0xabc123, "TEST123"), IdentificationMessage, 1, fixtureStart)
	item.Frame[4] ^= 1
	_, err := projectObservations(t.Context(), []ObservationEvidence{item}, fixtureStart, validObservationExpiry())
	require.True(t, errors.Is(err, adsb.ErrParity))

	item = observedEvidence(encodedIdentification(t, 0xabc123, "TEST123"), IdentificationMessage, 1, fixtureStart)
	item.Kind = VelocityMessage
	_, err = projectObservations(t.Context(), []ObservationEvidence{item}, fixtureStart, validObservationExpiry())
	require.ErrorIs(t, err, ErrInvalid)
}
