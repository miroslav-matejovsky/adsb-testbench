package display

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Literal reference corpus.
//
// These tests read the shared corpus in testdata/quality. The frames are
// published examples with their original addresses; receiver settings,
// sequences and instants are synthetic. Expected observations are stored
// data, not output of this decoder.

const qualityDir = "../testdata/quality"

// loadReference returns the literal reference snapshot, its parsed wire DTO,
// and the scenario naming its assertions.
func loadReference(t *testing.T) (qualitytest.DisplayReference, simulatorapi.ReceptionSnapshot, qualitytest.Scenario) {
	t.Helper()

	scenarios, err := qualitytest.LoadScenarios(qualityDir + "/scenarios.json")
	require.NoError(t, err)
	scenario, err := qualitytest.FindScenario(scenarios, "reference-fields")
	require.NoError(t, err)
	require.Equal(t, qualitytest.ReferenceKind, scenario.Kind)

	reference, err := qualitytest.LoadDisplayReference(qualityDir + "/" + scenario.Reference)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(reference.Snapshot))
	decoder.DisallowUnknownFields()
	var raw simulatorapi.ReceptionSnapshot
	require.NoError(t, decoder.Decode(&raw))
	return reference, raw, scenario
}

// referenceLifetimes converts stored field lifetimes.
func referenceLifetimes(expiry qualitytest.Expiry) lifetimes {
	return lifetimes{
		identity: expiry.Identity, position: expiry.Position,
		altitude: expiry.Altitude, velocity: expiry.Velocity,
	}
}

// requireReferenceObservations compares decoded tracks with the stored
// expectations. Wire values compare exactly; derived values use their stated
// tolerance.
func requireReferenceObservations(t *testing.T, scenario qualitytest.Scenario, reference qualitytest.DisplayReference, got simulatorapi.ObservationSnapshot) {
	t.Helper()

	require.Len(t, got.Aircraft, len(reference.Aircraft))
	icaos := map[string]bool{}
	for i, want := range reference.Aircraft {
		aircraft := got.Aircraft[i]
		require.Equal(t, want.ICAO, aircraft.ICAO)
		icaos[aircraft.ICAO] = true
		require.Equal(t, want.LastReceivedAt, aircraft.LastReceivedAt, want.ICAO)

		if scenario.Asserts("partial-tracks") {
			require.Equal(t, want.Identity == nil, aircraft.Identity == nil, "%s identity availability", want.ICAO)
			require.Equal(t, want.Position == nil, aircraft.Position == nil, "%s position availability", want.ICAO)
			require.Equal(t, want.BarometricAltitude == nil, aircraft.BarometricAltitude == nil, "%s altitude availability", want.ICAO)
			require.Equal(t, want.Velocity == nil, aircraft.Velocity == nil, "%s velocity availability", want.ICAO)
		}
		if scenario.Asserts("known-fields") {
			requireReferenceFields(t, scenario, want, aircraft)
		}
	}
	if scenario.Asserts("distinct-original-icaos") {
		require.Equal(t, map[string]bool{"40621D": true, "4840D6": true, "485020": true, "A05F21": true}, icaos,
			"published examples keep their own addresses and are never merged into one aircraft")
	}
}

// requireReferenceFields compares the known fields of one track.
func requireReferenceFields(t *testing.T, scenario qualitytest.Scenario, want qualitytest.ReferenceAircraft, aircraft simulatorapi.Aircraft) {
	t.Helper()

	if want.Identity != nil {
		require.Equal(t, want.Identity.Callsign, aircraft.Identity.Callsign)
		require.Equal(t, want.Identity.ObservedAt, aircraft.Identity.ObservedAt)
		require.Equal(t, want.Identity.EvidenceTransmissionSequence, aircraft.Identity.Evidence.TransmissionSequence)
	}
	if want.Position != nil {
		if scenario.Asserts("cpr-coordinate-tolerance") {
			require.InDelta(t, want.Position.LatitudeDegrees, aircraft.Position.LatitudeDegrees, want.Position.ToleranceDegrees)
			require.InDelta(t, want.Position.LongitudeDegrees, aircraft.Position.LongitudeDegrees, want.Position.ToleranceDegrees)
		}
		require.Equal(t, want.Position.ObservedAt, aircraft.Position.ObservedAt)
		sequences := make([]string, 0, len(aircraft.Position.Evidence))
		for _, item := range aircraft.Position.Evidence {
			sequences = append(sequences, item.TransmissionSequence)
		}
		require.Equal(t, want.Position.EvidenceTransmissionSequences, sequences)
	}
	if want.BarometricAltitude != nil {
		require.Equal(t, want.BarometricAltitude.Feet, aircraft.BarometricAltitude.Feet)
		require.Equal(t, want.BarometricAltitude.ObservedAt, aircraft.BarometricAltitude.ObservedAt)
		require.Equal(t, want.BarometricAltitude.EvidenceTransmissionSequence,
			aircraft.BarometricAltitude.Evidence.TransmissionSequence)
	}
	if want.Velocity == nil {
		return
	}
	measurement := func(value *qualitytest.Measurement) *simulatorapi.Measurement {
		if value == nil {
			return nil
		}
		return &simulatorapi.Measurement{Value: value.Value, OverRange: value.OverRange}
	}
	derived := func(expected, actual *float64, name string) {
		if expected == nil {
			require.Nil(t, actual, name)
			return
		}
		require.NotNil(t, actual, name)
		require.InDelta(t, *expected, *actual, want.Velocity.DerivedTolerance, name)
	}
	v := aircraft.Velocity
	require.Equal(t, want.Velocity.Subtype, v.Subtype)
	require.Equal(t, measurement(want.Velocity.EastKnots), v.EastKnots)
	require.Equal(t, measurement(want.Velocity.NorthKnots), v.NorthKnots)
	derived(want.Velocity.GroundSpeedKnots, v.GroundSpeedKnots, "ground speed")
	derived(want.Velocity.TrackDegrees, v.TrackDegrees, "track")
	require.Equal(t, want.Velocity.HeadingDegrees, v.HeadingDegrees)
	require.Equal(t, measurement(want.Velocity.AirspeedKnots), v.AirspeedKnots)
	require.Equal(t, want.Velocity.TrueAirspeed, v.TrueAirspeed)
	require.Equal(t, want.Velocity.BarometricVerticalRate, v.BarometricVerticalRate)
	require.Equal(t, measurement(want.Velocity.VerticalRateFeetPerMinute), v.VerticalRateFeetPerMinute)
	require.Equal(t, measurement(want.Velocity.GNSSMinusBaroFeet), v.GNSSMinusBaroFeet)
	require.Equal(t, want.Velocity.ObservedAt, v.ObservedAt)
	require.Equal(t, want.Velocity.EvidenceTransmissionSequence, v.Evidence.TransmissionSequence)
}

func TestDecodeLiteralReferenceSnapshot(t *testing.T) {
	t.Parallel()

	reference, raw, scenario := loadReference(t)
	frames, err := qualitytest.LoadFrames(qualityDir + "/frames.json")
	require.NoError(t, err)
	for i, id := range reference.Frames {
		frame, err := frames.Frame(id)
		require.NoError(t, err)
		require.Equal(t, frame.Hex, raw.Records[i].Frame, "the snapshot carries the literal corpus bytes")
	}

	got, err := decodeFixture(t, raw, referenceLifetimes(reference.Expiry))
	require.NoError(t, err)
	requireReferenceObservations(t, scenario, reference, got)
}

// TestDecodeLiteralCPRPairAgeBound pairs the published even and odd halves
// at exactly the pairing bound and one nanosecond beyond it.
func TestDecodeLiteralCPRPairAgeBound(t *testing.T) {
	t.Parallel()

	scenarios, err := qualitytest.LoadScenarios(qualityDir + "/scenarios.json")
	require.NoError(t, err)
	scenario, err := qualitytest.FindScenario(scenarios, "expiry-boundaries")
	require.NoError(t, err)
	require.True(t, scenario.Asserts("cpr-pair-age-bound"))

	frames, err := qualitytest.LoadFrames(qualityDir + "/frames.json")
	require.NoError(t, err)
	pair := frames.CPRPairs[0]
	even, err := frames.Frame(pair.Even)
	require.NoError(t, err)
	odd, err := frames.Frame(pair.Odd)
	require.NoError(t, err)
	evenNewer := pair.Cases[0]
	require.Equal(t, "even-newer", evenNewer.Name)

	station := fixtureStation("alpha", 1, fixtureStart)
	for _, tc := range []struct {
		separation time.Duration
		paired     bool
	}{
		{10 * time.Second, true},
		{10*time.Second + time.Nanosecond, false},
	} {
		oddAt := fixtureStart.Add(time.Second)
		evenAt := oddAt.Add(tc.separation)
		records := []simulatorapi.Reception{
			fixtureReception(1, 1, station, odd.Expected.ICAO, positionKind, oddAt, odd.Bytes),
			fixtureReception(2, 2, station, even.Expected.ICAO, positionKind, evenAt, even.Bytes),
		}
		got, err := decodeFixture(t, fixtureSnapshot(evenAt, []string{"alpha"}, records),
			referenceLifetimes(scenario.Expiry))
		require.NoError(t, err)
		require.Len(t, got.Aircraft, 1)
		if !tc.paired {
			require.Nil(t, got.Aircraft[0].Position, "separation %s exceeds the pairing bound", tc.separation)
			continue
		}
		require.NotNil(t, got.Aircraft[0].Position, "separation %s is inside the pairing bound", tc.separation)
		require.InDelta(t, evenNewer.LatitudeDegrees, got.Aircraft[0].Position.LatitudeDegrees, evenNewer.ToleranceDegrees)
		require.InDelta(t, evenNewer.LongitudeDegrees, got.Aircraft[0].Position.LongitudeDegrees, evenNewer.ToleranceDegrees)
	}
}
