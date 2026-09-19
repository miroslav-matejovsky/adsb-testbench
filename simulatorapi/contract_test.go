package simulatorapi_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

func TestStationFlattensSettingsOnTheWire(t *testing.T) {
	t.Parallel()

	station := simulatorapi.Station{
		StationSettings: simulatorapi.StationSettings{
			ID: "north", Enabled: false,
			LatitudeDegrees: 0, LongitudeDegrees: 0,
			SiteElevationMetres: 0, AntennaHeightMetres: 0,
			AntennaGainDBi: 0, SensitivityDBm: -100,
			SystemLossDB: 0, FrameLossProbability: 0,
		},
		Revision: "1", CreatedAt: "2024-03-05T12:00:00Z",
	}

	data, err := json.Marshal(station)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"north","enabled":false,
		"latitudeDegrees":0,"longitudeDegrees":0,
		"siteElevationMetres":0,"antennaHeightMetres":0,
		"antennaGainDBi":0,"sensitivityDBm":-100,
		"systemLossDB":0,"frameLossProbability":0,
		"revision":"1","createdAt":"2024-03-05T12:00:00Z"
	}`, string(data))

	var round simulatorapi.Station
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, station, round)
}

func TestReceptionSnapshotSerializesEmptyCollectionsAsArrays(t *testing.T) {
	t.Parallel()

	snapshot := simulatorapi.ReceptionSnapshot{
		RunID: "run-1", Now: "2024-03-05T12:00:00Z",
		StationIDs: []string{}, Retention: []simulatorapi.StationRetention{},
		Records: []simulatorapi.Reception{},
	}

	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"runId":"run-1","now":"2024-03-05T12:00:00Z","stationIds":[],"retention":[],"records":[]}`,
		string(data))

	var round simulatorapi.ReceptionSnapshot
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, snapshot, round)
}

func TestTruthSnapshotKeepsExactSequenceStringsAndUnits(t *testing.T) {
	t.Parallel()

	snapshot := simulatorapi.TruthSnapshot{
		RunID: "run-1", Now: "2024-03-05T12:00:00Z", ElapsedNanoseconds: "0",
		InitialAircraftCount: 2, AircraftCount: 0, SpeedHundredths: 0,
		Aircraft: []simulatorapi.TruthAircraft{},
		History: simulatorapi.TransmissionHistory{
			Messages: []simulatorapi.Transmission{{
				Sequence: "9007199254740993", ICAO: "00ABCD",
				Timestamp: "2024-03-05T12:00:00Z", Kind: "position",
				Frame: "8D4840D6202CC371C32CE0576098",
			}},
			OldestSequence: "9007199254740993", LatestSequence: "9007199254740993", Limit: 1000,
		},
	}

	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.Contains(t, string(data), `"sequence":"9007199254740993"`)
	require.Contains(t, string(data), `"aircraft":[]`)
	require.Contains(t, string(data), `"aircraftCount":0`)
	require.Contains(t, string(data), `"initialAircraftCount":2`)

	var round simulatorapi.TruthSnapshot
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, snapshot, round)
}

func TestControlCommandsAcceptFullySpecifiedZeroValues(t *testing.T) {
	t.Parallel()

	count, err := json.Marshal(simulatorapi.CountCommand{RunID: "run-1", Count: 0})
	require.NoError(t, err)
	require.JSONEq(t, `{"runId":"run-1","count":0}`, string(count))

	speed, err := json.Marshal(simulatorapi.SpeedCommand{RunID: "run-1", SpeedHundredths: 0})
	require.NoError(t, err)
	require.JSONEq(t, `{"runId":"run-1","speedHundredths":0}`, string(speed))

	remove, err := json.Marshal(simulatorapi.RemoveStationCommand{RunID: "run-1", ExpectedRevision: "3"})
	require.NoError(t, err)
	require.JSONEq(t, `{"runId":"run-1","expectedRevision":"3"}`, string(remove))

	ack, err := json.Marshal(simulatorapi.CommandAck{RunID: "run-1", Operation: simulatorapi.OperationSetSpeed})
	require.NoError(t, err)
	require.JSONEq(t, `{"runId":"run-1","operation":"setSpeed"}`, string(ack))
}

func TestMetadataPublishesEffectiveSettingsAndLimits(t *testing.T) {
	t.Parallel()

	metadata := simulatorapi.Metadata{
		RunID: "run-1", Now: "2024-03-05T12:00:00Z", ElapsedNanoseconds: "0",
		AircraftCount: 3, StationCount: 1,
		Simulation: simulatorapi.SimulationSettings{
			ID: "run-1", StartTime: "2024-03-05T12:00:00Z",
			Seed: "18446744073709551615", InitialAircraftCount: 2, SpeedHundredths: 0,
			Spawn: simulatorapi.SpawnSettings{
				TrackDegrees: simulatorapi.SpawnRange{Min: 0, Max: 360},
			},
		},
		Limits: simulatorapi.EngineLimits{MaxAircraft: 100, MaxAdvanceNanoseconds: "60000000000"},
		Driver: simulatorapi.DriverLimits{
			HeartbeatNanoseconds: "100000000", MaxCatchUpNanoseconds: "3600000000000",
		},
		Service: simulatorapi.ServiceSettings{
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
			RequestTimeoutNanoseconds: "5000000000", CoverageReferenceAltitudeFeet: 0,
		},
	}

	data, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.Contains(t, string(data), `"seed":"18446744073709551615"`)
	require.Contains(t, string(data), `"coverageReferenceAltitudeFeet":0`)
	require.Contains(t, string(data), `"trackDegrees":{"min":0,"max":360}`)

	var round simulatorapi.Metadata
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, metadata, round)
}
