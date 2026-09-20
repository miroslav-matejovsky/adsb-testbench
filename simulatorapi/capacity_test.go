package simulatorapi_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Conservative response-size worksheet.
//
// Every bound below is the marshaled size of a worst-case DTO whose fields
// have the widest value the supported profile allows: 20-digit unsigned
// decimals, 30-byte timestamps, 25-byte finite numbers, maximum-length
// station identifiers, and a run identifier whose every byte escapes to six
// JSON bytes. Because the bounds are computed from the DTOs themselves, a
// changed JSON key or response shape changes them and this test fails until
// testdata/quality/capacity.json is updated.

const capacityFile = "../testdata/quality/capacity.json"

// worst holds maximum-width field values of the supported profile.
type worst struct {
	profile  qualitytest.CapacityProfile
	number   float64
	uint64   string
	instant  string
	runID    string
	station  []string
	message  string
	field    string
	callsign string
}

func newWorst(t *testing.T, profile qualitytest.CapacityProfile) worst {
	t.Helper()

	w := worst{
		profile: profile,
		// Go encodes finite values in [1e-6,1e21) with 'f' formatting; this
		// value needs the most bytes: sign, "0.00000", 17 significant digits.
		number:   -1.2345678901234567e-06,
		uint64:   simulatorapi.FormatUint64(math.MaxUint64),
		instant:  "9999-12-31T23:59:59.999999999Z",
		runID:    strings.Repeat("<", profile.MaxRunIDBytes),
		message:  strings.Repeat("<", profile.MaxErrorMessageBytes),
		field:    strings.Repeat("<", profile.MaxErrorFieldBytes),
		callsign: "ABCDEFGH",
	}
	for i := range profile.MaxSelectedStations {
		w.station = append(w.station, string(rune('a'+i))+strings.Repeat("Z", profile.MaxStationIDBytes-1))
	}

	// The worksheet widths are facts of the encoder, checked here.
	require.Len(t, encode(t, w.number), profile.FiniteNumberBytes)
	require.Len(t, w.uint64, profile.Uint64Digits)
	require.Len(t, w.instant, profile.TimestampBytes)
	require.Len(t, encode(t, w.runID), 2+profile.MaxRunIDBytes*profile.JSONEscapeFactor)
	return w
}

func encode(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func (w worst) settings(id string) simulatorapi.StationSettings {
	return simulatorapi.StationSettings{
		ID: id, Enabled: false,
		LatitudeDegrees: w.number, LongitudeDegrees: w.number, SiteElevationMetres: w.number,
		AntennaHeightMetres: w.number, AntennaGainDBi: w.number, SensitivityDBm: w.number,
		SystemLossDB: w.number, FrameLossProbability: w.number,
	}
}

func (w worst) stationDTO(id string) simulatorapi.Station {
	return simulatorapi.Station{StationSettings: w.settings(id), Revision: w.uint64, CreatedAt: w.instant}
}

func (w worst) reception() simulatorapi.Reception {
	id := w.station[0]
	return simulatorapi.Reception{
		Sequence: w.uint64, TransmissionSequence: w.uint64, StationID: id, StationRevision: w.uint64,
		ICAO: "FFFFFE", Kind: "identification", Timestamp: w.instant, Frame: strings.Repeat("F", 28),
		SlantRangeNauticalMiles: w.number, ReceivedPowerDBm: w.number, Receiver: w.stationDTO(id),
	}
}

func (w worst) retention() []simulatorapi.StationRetention {
	entries := make([]simulatorapi.StationRetention, 0, len(w.station))
	for _, id := range w.station {
		entries = append(entries, simulatorapi.StationRetention{
			StationID: id, OldestSequence: w.uint64, LatestSequence: w.uint64, Truncated: false,
			Limit: w.profile.MaxStationRecords,
		})
	}
	return entries
}

func (w worst) cursor() simulatorapi.ReceptionCursor {
	return simulatorapi.ReceptionCursor{RunID: w.runID, StationID: w.station[0], AfterSequence: w.uint64}
}

func (w worst) measurement() *simulatorapi.Measurement {
	return &simulatorapi.Measurement{Value: w.number, OverRange: false}
}

// rawSnapshotBytes bounds one raw reception snapshot of every selected
// station at full retention.
func (w worst) rawSnapshotBytes(t *testing.T) int {
	records := make([]simulatorapi.Reception, w.profile.MaxSnapshotRecords)
	for i := range records {
		records[i] = w.reception()
	}
	return len(encode(t, simulatorapi.ReceptionSnapshot{
		RunID: w.runID, Now: w.instant, StationIDs: w.station, Retention: w.retention(), Records: records,
	}))
}

// historyPageBytes bounds one history page of the largest page size.
func (w worst) historyPageBytes(t *testing.T) int {
	records := make([]simulatorapi.Reception, w.profile.MaxHistoryPageRecords)
	for i := range records {
		records[i] = w.reception()
	}
	return len(encode(t, simulatorapi.ReceptionPage{
		RunID: w.runID, StationID: w.station[0], Now: w.instant, Records: records,
		OldestSequence: w.uint64, LatestSequence: w.uint64, NextCursor: w.cursor(),
		Gap: false, HasMore: false, RetentionLimit: w.profile.MaxStationRecords,
	}))
}

// observationHeaderBytes is an observation snapshot with no aircraft.
func (w worst) observationHeaderBytes(t *testing.T) int {
	return len(encode(t, w.observationHeader()))
}

func (w worst) observationHeader() simulatorapi.ObservationSnapshot {
	return simulatorapi.ObservationSnapshot{
		RunID: w.runID, Now: w.instant, StationIDs: w.station, Retention: w.retention(),
		Expiry: simulatorapi.ObservationExpiry{
			IdentityNanoseconds: w.uint64, PositionNanoseconds: w.uint64,
			AltitudeNanoseconds: w.uint64, VelocityNanoseconds: w.uint64,
		},
		Aircraft: []simulatorapi.Aircraft{},
	}
}

// observationRecordBytes is the most output one raw record can cause in an
// observation snapshot.
//
// Every aircraft needs at least one record, and so does every field: an
// identity or velocity field uses its own record, and a position record
// serves at most a position and an altitude field of the same aircraft,
// while a position fix needs two records. Each evidence object is one
// transmission in one role, and a transmission has at most two roles, so
// evidence objects and reception copies are each at most twice the records.
// Charging every record one aircraft wrapper, the largest field object, and
// two evidence objects with a copy each therefore bounds the whole array.
func (w worst) observationRecordBytes(t *testing.T) int {
	aircraft := len(encode(t, simulatorapi.Aircraft{ICAO: "FFFFFE", LastReceivedAt: w.instant}))
	evidence := len(encode(t, simulatorapi.Evidence{
		TransmissionSequence: w.uint64, ICAO: "FFFFFE", Kind: "identification", Timestamp: w.instant,
		Frame: strings.Repeat("F", 28), Receptions: []simulatorapi.Reception{},
	}))
	reception := len(encode(t, w.reception()))
	fields := []any{
		simulatorapi.IdentityObservation{Callsign: w.callsign, ObservedAt: w.instant},
		simulatorapi.PositionObservation{
			LatitudeDegrees: w.number, LongitudeDegrees: w.number, ObservedAt: w.instant,
			Evidence: []simulatorapi.Evidence{},
		},
		simulatorapi.AltitudeObservation{Feet: w.number, ObservedAt: w.instant},
		simulatorapi.VelocityObservation{
			Subtype: math.MaxUint8, NACv: math.MaxUint8,
			EastKnots: w.measurement(), NorthKnots: w.measurement(),
			GroundSpeedKnots: &w.number, TrackDegrees: &w.number, HeadingDegrees: &w.number,
			AirspeedKnots: w.measurement(), VerticalRateFeetPerMinute: w.measurement(),
			GNSSMinusBaroFeet: w.measurement(), ObservedAt: w.instant,
		},
	}
	field := 0
	for _, value := range fields {
		field = max(field, len(encode(t, value)))
	}
	// One separator per array element and per object member added.
	return aircraft + 1 + field + 1 + 2*(evidence+1+reception+1)
}

// observationSnapshotBytes bounds a decoded observation snapshot of a full
// raw selection, served by the simulator and wrapped by a display.
func (w worst) observationSnapshotBytes(t *testing.T) int {
	return w.observationHeaderBytes(t) + w.profile.MaxSnapshotRecords*w.observationRecordBytes(t)
}

// truthBytes bounds the manager's truth snapshot: every active aircraft and
// the whole retained transmission history.
func (w worst) truthBytes(t *testing.T) int {
	aircraft := make([]simulatorapi.TruthAircraft, w.profile.MaxTruthAircraft)
	for i := range aircraft {
		aircraft[i] = simulatorapi.TruthAircraft{
			ICAO: "FFFFFE", Callsign: w.callsign, CreatedAt: w.instant,
			LatitudeDegrees: w.number, LongitudeDegrees: w.number, BarometricAltitudeFeet: w.number,
			GroundSpeedKnots: w.number, TrackDegrees: w.number, VerticalRateFeetPerMinute: w.number,
		}
	}
	messages := make([]simulatorapi.Transmission, w.profile.TransmissionHistoryLimit)
	for i := range messages {
		messages[i] = simulatorapi.Transmission{
			Sequence: w.uint64, ICAO: "FFFFFE", Timestamp: w.instant, Kind: "identification",
			Frame: strings.Repeat("F", 28),
		}
	}
	return len(encode(t, simulatorapi.TruthSnapshot{
		RunID: w.runID, Now: w.instant, ElapsedNanoseconds: w.uint64,
		InitialAircraftCount: w.profile.MaxTruthAircraft, AircraftCount: w.profile.MaxTruthAircraft,
		SpeedHundredths: math.MinInt64, Aircraft: aircraft,
		History: simulatorapi.TransmissionHistory{
			Messages: messages, OldestSequence: w.uint64, LatestSequence: w.uint64,
			Limit: w.profile.TransmissionHistoryLimit,
		},
	}))
}

// stationsBytes bounds the station catalog with coverage.
func (w worst) stationsBytes(t *testing.T) int {
	states := make([]simulatorapi.StationState, 0, len(w.station))
	for _, id := range w.station {
		states = append(states, simulatorapi.StationState{
			Station: w.stationDTO(id),
			Coverage: simulatorapi.Coverage{
				ReferenceAltitudeFeet: w.number, HorizonRadiusNauticalMiles: w.number,
				LinkBudgetRadiusNauticalMiles: w.number, EffectiveRadiusNauticalMiles: w.number,
			},
		})
	}
	return len(encode(t, simulatorapi.StationsSnapshot{RunID: w.runID, Now: w.instant, Stations: states}))
}

// metadataBytes bounds the metadata document. Integers use the widest
// signed 64-bit representation.
func (w worst) metadataBytes(t *testing.T) int {
	wide := math.MinInt64
	span := simulatorapi.SpawnRange{Min: w.number, Max: w.number}
	return len(encode(t, simulatorapi.Metadata{
		RunID: w.runID, Now: w.instant, ElapsedNanoseconds: w.uint64,
		AircraftCount: wide, StationCount: wide,
		Simulation: simulatorapi.SimulationSettings{
			ID: w.runID, StartTime: w.instant, Seed: w.uint64,
			InitialAircraftCount: wide, SpeedHundredths: wide,
			Spawn: simulatorapi.SpawnSettings{
				LatitudeDegrees: span, LongitudeDegrees: span, AltitudeFeet: span,
				GroundSpeedKnots: span, TrackDegrees: span, VerticalRateFeetPerMinute: span,
			},
		},
		Model: simulatorapi.ReceptionModel{
			TransmitPowerDBm: w.number, FrequencyMHz: w.number, FreeSpacePathLossConstantDB: w.number,
			RefractionFactor: w.number, HorizonMetresPerSqrtMetre: w.number, EarthRadiusMetres: w.number,
		},
		Limits: simulatorapi.EngineLimits{
			MaxAircraft: wide, MaxStations: wide, MaxStationIDsPerRun: wide, MaxSpeedHundredths: wide,
			HistoryLimit: wide, ReceptionHistoryLimit: wide, MaxHistoryPageSize: wide,
			MaxBatchFrames: wide, MaxBatchReceptions: wide, MaxAdvanceNanoseconds: w.uint64,
		},
		Driver: simulatorapi.DriverLimits{HeartbeatNanoseconds: w.uint64, MaxCatchUpNanoseconds: w.uint64},
		Service: simulatorapi.ServiceSettings{
			MaxRequestBytes: wide, MaxResponseBytes: wide, RequestTimeoutNanoseconds: w.uint64,
			CoverageReferenceAltitudeFeet: w.number,
		},
	}))
}

// errorEnvelopeBytes bounds one shared error envelope.
func (w worst) errorEnvelopeBytes(t *testing.T) int {
	return len(encode(t, simulatorapi.ErrorResponse{Error: w.apiError()}))
}

func (w worst) apiError() simulatorapi.APIError {
	return simulatorapi.APIError{
		Code: simulatorapi.CategoryDeadlineExceeded, Message: w.message, Field: w.field, RunID: w.runID,
	}
}

func TestCapacityWorksheetMatchesTheDTOs(t *testing.T) {
	t.Parallel()

	capacity, err := qualitytest.LoadCapacity(capacityFile)
	require.NoError(t, err)
	w := newWorst(t, capacity.Profile)

	got := map[string]int{
		"rawSnapshotBytes":         w.rawSnapshotBytes(t),
		"historyPageBytes":         w.historyPageBytes(t),
		"observationHeaderBytes":   w.observationHeaderBytes(t),
		"observationRecordBytes":   w.observationRecordBytes(t),
		"observationSnapshotBytes": w.observationSnapshotBytes(t),
		"truthBytes":               w.truthBytes(t),
		"stationsBytes":            w.stationsBytes(t),
		"metadataBytes":            w.metadataBytes(t),
		"errorEnvelopeBytes":       w.errorEnvelopeBytes(t),
	}
	for name, bytes := range got {
		t.Logf("bound %s = %d", name, bytes)
	}
	for name, bytes := range got {
		recorded, ok := capacity.Bounds[name]
		require.True(t, ok, "capacity.json records bound %s", name)
		require.Equal(t, recorded, bytes, "bound %s changed; update testdata/quality/capacity.json", name)
	}

	// The simulator API serves every document above; the largest decides.
	simulatorLargest := 0
	for _, name := range []string{"rawSnapshotBytes", "historyPageBytes", "observationSnapshotBytes",
		"truthBytes", "stationsBytes", "metadataBytes", "errorEnvelopeBytes"} {
		simulatorLargest = max(simulatorLargest, got[name])
	}
	require.Equal(t, capacity.Budget(simulatorLargest), capacity.Budgets.SimulatorAPIMaxResponseBytes)
	require.Equal(t, capacity.Budgets.SimulatorAPIMaxResponseBytes, capacity.Budgets.ManagerUIMaxResponseBytes,
		"a manager must accept every simulator response")

	// A display source reads raw snapshots, history pages, stations and errors.
	sourceLargest := max(got["rawSnapshotBytes"], got["historyPageBytes"], got["stationsBytes"], got["errorEnvelopeBytes"])
	require.Equal(t, capacity.Budget(sourceLargest), capacity.Budgets.SourceMaxResponseBytes)
}
