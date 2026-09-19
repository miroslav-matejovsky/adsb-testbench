package display

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/adsb"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Shared display fixtures.
//
// Frames are built with the testbench codec, so each fixture states exactly
// which wire values it carries. Expected decoded values are compared with
// codec-appropriate tolerances, never with simulator truth.

const (
	fixtureRunID = "display-fixture"
	fixtureICAO  = uint32(0x00ABCD)
)

var (
	fixtureStart     = time.Date(2024, time.March, 5, 12, 0, 0, 0, time.UTC)
	fixtureLifetimes = lifetimes{
		identity: time.Minute, position: 30 * time.Second,
		altitude: 30 * time.Second, velocity: 30 * time.Second,
	}
)

// fixtureStation returns one complete receiver snapshot.
func fixtureStation(id string, revision uint64, createdAt time.Time) simulatorapi.Station {
	return simulatorapi.Station{
		StationSettings: simulatorapi.StationSettings{
			ID: id, Enabled: true,
			LatitudeDegrees: 50, LongitudeDegrees: 14,
			SiteElevationMetres: 100, AntennaHeightMetres: 20,
			AntennaGainDBi: 3, SensitivityDBm: -95,
			SystemLossDB: 2, FrameLossProbability: 0,
		},
		Revision:  simulatorapi.FormatUint64(revision),
		CreatedAt: simulatorapi.FormatTime(createdAt),
	}
}

// fixtureReception returns one complete raw record.
func fixtureReception(sequence, transmission uint64, station simulatorapi.Station,
	icao uint32, kind string, at time.Time, frame adsb.Frame,
) simulatorapi.Reception {
	return simulatorapi.Reception{
		Sequence:                simulatorapi.FormatUint64(sequence),
		TransmissionSequence:    simulatorapi.FormatUint64(transmission),
		StationID:               station.ID,
		StationRevision:         station.Revision,
		ICAO:                    simulatorapi.FormatICAO(icao),
		Kind:                    kind,
		Timestamp:               simulatorapi.FormatTime(at),
		Frame:                   simulatorapi.FormatFrame(frame),
		SlantRangeNauticalMiles: 12.5,
		ReceivedPowerDBm:        -80,
		Receiver:                station,
	}
}

// fixtureSnapshot assembles a consistent snapshot, deriving retention from
// the records themselves so the fixture is always self-consistent.
func fixtureSnapshot(now time.Time, stationIDs []string, records []simulatorapi.Reception) simulatorapi.ReceptionSnapshot {
	retention := make([]simulatorapi.StationRetention, 0, len(stationIDs))
	for _, id := range stationIDs {
		entry := simulatorapi.StationRetention{
			StationID: id, OldestSequence: "0", LatestSequence: "0", Limit: maxStationRecords,
		}
		for _, record := range records {
			if record.StationID != id {
				continue
			}
			if entry.OldestSequence == "0" {
				entry.OldestSequence = record.Sequence
			}
			entry.LatestSequence = record.Sequence
		}
		oldest, err := simulatorapi.ParseUint64(entry.OldestSequence)
		if err == nil {
			entry.Truncated = oldest > 1
		}
		retention = append(retention, entry)
	}
	return simulatorapi.ReceptionSnapshot{
		RunID: fixtureRunID, Now: simulatorapi.FormatTime(now),
		StationIDs: stationIDs, Retention: retention, Records: records,
	}
}

func identificationFrame(t *testing.T, icao uint32, callsign string) adsb.Frame {
	t.Helper()

	frame, err := adsb.EncodeIdentification(
		adsb.Header{ICAO: icao, Capability: 5},
		adsb.Identification{TypeCode: 4, Category: 1, Callsign: callsign},
	)
	require.NoError(t, err)
	return frame
}

func positionFrame(t *testing.T, icao uint32, latitude, longitude float64, altitude *float64, odd bool) adsb.Frame {
	t.Helper()

	cpr, err := adsb.EncodeCPR(adsb.Coordinates{Latitude: latitude, Longitude: longitude}, odd)
	require.NoError(t, err)
	frame, err := adsb.EncodePosition(
		adsb.Header{ICAO: icao, Capability: 5},
		adsb.AirbornePosition{TypeCode: 11, AltitudeFeet: altitude, CPR: cpr},
	)
	require.NoError(t, err)
	return frame
}

func velocityFrame(t *testing.T, icao uint32, velocity adsb.Velocity) adsb.Frame {
	t.Helper()

	frame, err := adsb.EncodeVelocity(adsb.Header{ICAO: icao, Capability: 5}, velocity)
	require.NoError(t, err)
	return frame
}

// groundVelocity returns a subtype 1 payload with explicit components.
func groundVelocity(east, north, verticalRate float64) adsb.Velocity {
	return adsb.Velocity{
		Subtype: 1, NACv: 1,
		EastKnots:                 &adsb.Measurement{Value: east},
		NorthKnots:                &adsb.Measurement{Value: north},
		BarometricVerticalRate:    true,
		VerticalRateFeetPerMinute: &adsb.Measurement{Value: verticalRate},
	}
}

func feet(value float64) *float64 { return &value }
