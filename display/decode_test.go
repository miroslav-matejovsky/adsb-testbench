package display

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// decodeFixture validates and decodes one fixture snapshot.
func decodeFixture(t *testing.T, raw simulatorapi.ReceptionSnapshot, expiry lifetimes) (simulatorapi.ObservationSnapshot, error) {
	t.Helper()

	validated, _, err := validateSnapshot(
		simulatorapi.ReceptionSnapshotRequest{StationIDs: raw.StationIDs}, raw)
	require.NoError(t, err)
	return decodeSnapshot(validated, expiry)
}

func TestDecodeBuildsFieldsFromIndependentPublishedFrames(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(10 * time.Second)
	altitude := feet(35000)

	records := []simulatorapi.Reception{
		fixtureReception(1, 1, station, fixtureICAO, identificationKind,
			fixtureStart.Add(time.Second), identificationFrame(t, fixtureICAO, "TB00ABCD")),
		fixtureReception(2, 2, station, fixtureICAO, positionKind,
			fixtureStart.Add(2*time.Second), positionFrame(t, fixtureICAO, 50.0, 14.0, altitude, false)),
		fixtureReception(3, 3, station, fixtureICAO, positionKind,
			fixtureStart.Add(3*time.Second), positionFrame(t, fixtureICAO, 50.0, 14.0, altitude, true)),
		fixtureReception(4, 4, station, fixtureICAO, velocityKind,
			fixtureStart.Add(4*time.Second), velocityFrame(t, fixtureICAO, groundVelocity(100, 0, 640))),
	}

	got, err := decodeFixture(t, fixtureSnapshot(now, []string{"alpha"}, records), fixtureLifetimes)
	require.NoError(t, err)
	require.Equal(t, fixtureRunID, got.RunID)
	require.Equal(t, simulatorapi.FormatTime(now), got.Now)
	require.Len(t, got.Aircraft, 1)

	aircraft := got.Aircraft[0]
	require.Equal(t, "00ABCD", aircraft.ICAO)
	require.Equal(t, simulatorapi.FormatTime(fixtureStart.Add(4*time.Second)), aircraft.LastReceivedAt)

	require.NotNil(t, aircraft.Identity)
	require.Equal(t, "TB00ABCD", aircraft.Identity.Callsign)
	require.Equal(t, simulatorapi.FormatTime(fixtureStart.Add(time.Second)), aircraft.Identity.ObservedAt)

	require.NotNil(t, aircraft.BarometricAltitude)
	require.InDelta(t, 35000.0, aircraft.BarometricAltitude.Feet, 12.5)

	require.NotNil(t, aircraft.Position)
	require.InDelta(t, 50.0, aircraft.Position.LatitudeDegrees, 0.0005)
	require.InDelta(t, 14.0, aircraft.Position.LongitudeDegrees, 0.0005)
	require.Len(t, aircraft.Position.Evidence, 2)
	require.Equal(t, "2", aircraft.Position.Evidence[0].TransmissionSequence)
	require.Equal(t, "3", aircraft.Position.Evidence[1].TransmissionSequence)

	require.NotNil(t, aircraft.Velocity)
	require.NotNil(t, aircraft.Velocity.GroundSpeedKnots)
	require.InDelta(t, 100.0, *aircraft.Velocity.GroundSpeedKnots, 1)
	require.NotNil(t, aircraft.Velocity.TrackDegrees)
	require.InDelta(t, 90.0, *aircraft.Velocity.TrackDegrees, 1)
	require.Nil(t, aircraft.Velocity.HeadingDegrees)
	require.NotNil(t, aircraft.Velocity.VerticalRateFeetPerMinute)
	require.InDelta(t, 640.0, aircraft.Velocity.VerticalRateFeetPerMinute.Value, 64)
	require.Nil(t, aircraft.Velocity.GNSSMinusBaroFeet)
}

func TestDecodeKeepsALoneCPRHalfUnpaired(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(5 * time.Second)
	records := []simulatorapi.Reception{
		fixtureReception(1, 1, station, fixtureICAO, positionKind,
			fixtureStart.Add(time.Second), positionFrame(t, fixtureICAO, 50, 14, feet(35000), false)),
	}

	got, err := decodeFixture(t, fixtureSnapshot(now, []string{"alpha"}, records), fixtureLifetimes)
	require.NoError(t, err)
	require.Len(t, got.Aircraft, 1)
	require.Nil(t, got.Aircraft[0].Position)
	require.NotNil(t, got.Aircraft[0].BarometricAltitude)
}

func TestDecodeHonorsTheTenSecondCPRPairingBound(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	cases := []struct {
		name      string
		separated time.Duration
		paired    bool
	}{
		{name: "at the bound", separated: adsb.MaxCPRAge, paired: true},
		{name: "one nanosecond over", separated: adsb.MaxCPRAge + time.Nanosecond, paired: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			even := fixtureStart.Add(time.Second)
			odd := even.Add(tc.separated)
			records := []simulatorapi.Reception{
				fixtureReception(1, 1, station, fixtureICAO, positionKind, even,
					positionFrame(t, fixtureICAO, 50, 14, feet(35000), false)),
				fixtureReception(2, 2, station, fixtureICAO, positionKind, odd,
					positionFrame(t, fixtureICAO, 50, 14, feet(35000), true)),
			}

			got, err := decodeFixture(t, fixtureSnapshot(odd, []string{"alpha"}, records), fixtureLifetimes)
			require.NoError(t, err)
			require.Len(t, got.Aircraft, 1)
			if tc.paired {
				require.NotNil(t, got.Aircraft[0].Position)
				return
			}
			require.Nil(t, got.Aircraft[0].Position)
		})
	}
}

func TestDecodeAppliesIndependentFieldExpiryAtSnapshotTime(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	observed := fixtureStart.Add(time.Second)
	records := []simulatorapi.Reception{
		fixtureReception(1, 1, station, fixtureICAO, identificationKind, observed,
			identificationFrame(t, fixtureICAO, "TB00ABCD")),
		fixtureReception(2, 2, station, fixtureICAO, velocityKind, observed,
			velocityFrame(t, fixtureICAO, groundVelocity(100, 0, 0))),
	}

	expiry := lifetimes{identity: 10 * time.Second, position: time.Second, altitude: time.Second, velocity: time.Second}

	fresh, err := decodeFixture(t, fixtureSnapshot(observed.Add(time.Second), []string{"alpha"}, records), expiry)
	require.NoError(t, err)
	require.Len(t, fresh.Aircraft, 1)
	require.NotNil(t, fresh.Aircraft[0].Identity)
	require.NotNil(t, fresh.Aircraft[0].Velocity, "a field is fresh at exactly its lifetime")

	expired, err := decodeFixture(t,
		fixtureSnapshot(observed.Add(time.Second+time.Nanosecond), []string{"alpha"}, records), expiry)
	require.NoError(t, err)
	require.Len(t, expired.Aircraft, 1)
	require.NotNil(t, expired.Aircraft[0].Identity)
	require.Nil(t, expired.Aircraft[0].Velocity, "a field is absent once its age exceeds its lifetime")

	gone, err := decodeFixture(t,
		fixtureSnapshot(observed.Add(time.Minute), []string{"alpha"}, records), expiry)
	require.NoError(t, err)
	require.Empty(t, gone.Aircraft, "an aircraft with no fresh field is dropped")
}

func TestDecodeTreatsUnavailableValuesAsReplacements(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(5 * time.Second)
	records := []simulatorapi.Reception{
		fixtureReception(1, 1, station, fixtureICAO, positionKind, fixtureStart.Add(time.Second),
			positionFrame(t, fixtureICAO, 50, 14, feet(35000), false)),
		fixtureReception(2, 2, station, fixtureICAO, positionKind, fixtureStart.Add(2*time.Second),
			positionFrame(t, fixtureICAO, 50, 14, nil, true)),
	}

	got, err := decodeFixture(t, fixtureSnapshot(now, []string{"alpha"}, records), fixtureLifetimes)
	require.NoError(t, err)
	require.Len(t, got.Aircraft, 1)
	require.Nil(t, got.Aircraft[0].BarometricAltitude, "an unavailable altitude clears a known altitude")
	require.NotNil(t, got.Aircraft[0].Position, "the pair still yields a fix")
}

func TestDecodeKeepsUnknownZeroAndOverRangeDistinct(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(2 * time.Second)

	cases := []struct {
		name     string
		velocity adsb.Velocity
		assert   func(*testing.T, *simulatorapi.VelocityObservation)
	}{
		{
			name:     "zero ground speed has no track",
			velocity: groundVelocity(0, 0, 0),
			assert: func(t *testing.T, observed *simulatorapi.VelocityObservation) {
				require.NotNil(t, observed.GroundSpeedKnots)
				require.Equal(t, 0.0, *observed.GroundSpeedKnots)
				require.Nil(t, observed.TrackDegrees)
				require.NotNil(t, observed.VerticalRateFeetPerMinute)
				require.Equal(t, 0.0, observed.VerticalRateFeetPerMinute.Value)
			},
		},
		{
			name:     "unavailable components stay null",
			velocity: adsb.Velocity{Subtype: 1, NACv: 0},
			assert: func(t *testing.T, observed *simulatorapi.VelocityObservation) {
				require.Nil(t, observed.EastKnots)
				require.Nil(t, observed.NorthKnots)
				require.Nil(t, observed.GroundSpeedKnots)
				require.Nil(t, observed.TrackDegrees)
				require.Nil(t, observed.VerticalRateFeetPerMinute)
			},
		},
		{
			name: "over-range components derive no ground speed",
			velocity: adsb.Velocity{
				Subtype:    1,
				EastKnots:  &adsb.Measurement{Value: 1021.5, OverRange: true},
				NorthKnots: &adsb.Measurement{Value: 10},
			},
			assert: func(t *testing.T, observed *simulatorapi.VelocityObservation) {
				require.NotNil(t, observed.EastKnots)
				require.True(t, observed.EastKnots.OverRange)
				require.Equal(t, 1021.5, observed.EastKnots.Value)
				require.Nil(t, observed.GroundSpeedKnots)
				require.Nil(t, observed.TrackDegrees)
			},
		},
		{
			name: "airspeed subtype keeps heading and airspeed type",
			velocity: adsb.Velocity{
				Subtype: 3, HeadingDegrees: feet(90),
				AirspeedKnots: &adsb.Measurement{Value: 250}, TrueAirspeed: true,
			},
			assert: func(t *testing.T, observed *simulatorapi.VelocityObservation) {
				require.Equal(t, uint8(3), observed.Subtype)
				require.NotNil(t, observed.HeadingDegrees)
				require.InDelta(t, 90.0, *observed.HeadingDegrees, 0.4)
				require.NotNil(t, observed.AirspeedKnots)
				require.InDelta(t, 250.0, observed.AirspeedKnots.Value, 1)
				require.True(t, observed.TrueAirspeed)
				require.Nil(t, observed.EastKnots)
				require.Nil(t, observed.GroundSpeedKnots)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			records := []simulatorapi.Reception{
				fixtureReception(1, 1, station, fixtureICAO, velocityKind, fixtureStart.Add(time.Second),
					velocityFrame(t, fixtureICAO, tc.velocity)),
			}
			got, err := decodeFixture(t, fixtureSnapshot(now, []string{"alpha"}, records), fixtureLifetimes)
			require.NoError(t, err)
			require.Len(t, got.Aircraft, 1)
			require.NotNil(t, got.Aircraft[0].Velocity)
			tc.assert(t, got.Aircraft[0].Velocity)
		})
	}
}

func TestDecodeKeepsEveryReceiverCopyInStationOrder(t *testing.T) {
	t.Parallel()

	alpha := fixtureStation("alpha", 1, fixtureStart)
	bravo := fixtureStation("bravo", 4, fixtureStart)
	bravo.AntennaGainDBi = 12
	now := fixtureStart.Add(2 * time.Second)
	frame := identificationFrame(t, fixtureICAO, "TB00ABCD")
	at := fixtureStart.Add(time.Second)

	records := []simulatorapi.Reception{
		fixtureReception(1, 7, alpha, fixtureICAO, identificationKind, at, frame),
		fixtureReception(1, 7, bravo, fixtureICAO, identificationKind, at, frame),
	}

	got, err := decodeFixture(t, fixtureSnapshot(now, []string{"alpha", "bravo"}, records), fixtureLifetimes)
	require.NoError(t, err)
	require.Len(t, got.Aircraft, 1)

	evidence := got.Aircraft[0].Identity.Evidence
	require.Len(t, evidence.Receptions, 2)
	require.Equal(t, "alpha", evidence.Receptions[0].StationID)
	require.Equal(t, "bravo", evidence.Receptions[1].StationID)
	require.Equal(t, 3.0, evidence.Receptions[0].Receiver.AntennaGainDBi)
	require.Equal(t, 12.0, evidence.Receptions[1].Receiver.AntennaGainDBi,
		"reception provenance keeps the historical receiver settings")
	require.Equal(t, "4", evidence.Receptions[1].Receiver.Revision)
}

func TestDecodeFailsTheWholeSnapshotOnCorruptFrames(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(2 * time.Second)
	good := identificationFrame(t, fixtureICAO, "TB00ABCD")

	corrupt := good
	corrupt[13] ^= 0x01

	cases := []struct {
		name   string
		record simulatorapi.Reception
	}{
		{
			name: "bad crc",
			record: fixtureReception(1, 1, station, fixtureICAO, identificationKind,
				fixtureStart.Add(time.Second), corrupt),
		},
		{
			name: "metadata address disagrees",
			record: fixtureReception(1, 1, station, 0x00BEEF, identificationKind,
				fixtureStart.Add(time.Second), good),
		},
		{
			name: "metadata family disagrees",
			record: fixtureReception(1, 1, station, fixtureICAO, velocityKind,
				fixtureStart.Add(time.Second), good),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := fixtureSnapshot(now, []string{"alpha"}, []simulatorapi.Reception{tc.record})
			validated, _, err := validateSnapshot(
				simulatorapi.ReceptionSnapshotRequest{StationIDs: raw.StationIDs}, raw)
			require.NoError(t, err)

			got, err := decodeSnapshot(validated, fixtureLifetimes)
			require.Error(t, err)
			require.Equal(t, simulatorapi.ObservationSnapshot{}, got,
				"a failed decode yields no partial snapshot")
		})
	}
}

func TestDecodeSortsAircraftAndCarriesSelectionUnchanged(t *testing.T) {
	t.Parallel()

	station := fixtureStation("alpha", 1, fixtureStart)
	now := fixtureStart.Add(2 * time.Second)
	at := fixtureStart.Add(time.Second)
	records := []simulatorapi.Reception{
		fixtureReception(1, 1, station, 0x00FFEE, identificationKind, at,
			identificationFrame(t, 0x00FFEE, "TB00FFEE")),
		fixtureReception(2, 2, station, 0x000001, identificationKind, at,
			identificationFrame(t, 0x000001, "TB000001")),
	}

	raw := fixtureSnapshot(now, []string{"alpha"}, records)
	got, err := decodeFixture(t, raw, fixtureLifetimes)
	require.NoError(t, err)
	require.Equal(t, []string{"000001", "00FFEE"},
		[]string{got.Aircraft[0].ICAO, got.Aircraft[1].ICAO})
	require.Equal(t, raw.Retention, got.Retention)
	require.Equal(t, raw.StationIDs, got.StationIDs)
	require.Equal(t, simulatorapi.ObservationExpiry{
		IdentityNanoseconds: "60000000000", PositionNanoseconds: "30000000000",
		AltitudeNanoseconds: "30000000000", VelocityNanoseconds: "30000000000",
	}, got.Expiry)
}

func TestDecodeEmptySnapshotHasNoAircraft(t *testing.T) {
	t.Parallel()

	got, err := decodeFixture(t,
		fixtureSnapshot(fixtureStart, []string{}, []simulatorapi.Reception{}), fixtureLifetimes)
	require.NoError(t, err)
	require.Empty(t, got.Aircraft)
	require.NotNil(t, got.Aircraft)
	require.NotNil(t, got.StationIDs)
	require.NotNil(t, got.Retention)
}
