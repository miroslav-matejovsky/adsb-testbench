package simulation

import (
	"errors"
	"time"
)

// Engine policy limits. These are fixed implementation bounds of this package.
// They are never inserted into an incomplete Config; every configuration field
// must be assigned by the caller.
const (
	// MaxAircraft is the largest number of simultaneously active aircraft.
	MaxAircraft = 100
	// HistoryLimit is the number of most recent transmissions retained by an
	// engine. Older transmissions are evicted from history but remain complete
	// in the batch returned by the call that emitted them.
	HistoryLimit = 1000
	// ReceptionHistoryLimit is the number of most recent receptions retained
	// independently by each active station.
	ReceptionHistoryLimit = 1000
	// MaxHistoryPageSize is the largest reception-history page a caller may
	// request. Callers must always supply an explicit positive limit.
	MaxHistoryPageSize = ReceptionHistoryLimit
	// MaxAdvance is the largest amount of virtual time one Advance or one
	// scaled Elapse call may cover. Larger requests are rejected without work.
	MaxAdvance = 60 * time.Second
	// MaxBatchFrames is the largest number of transmissions one mutation may
	// emit. It is a safety bound above the worst case reachable within
	// MaxAdvance and MaxAircraft. Receptions have their own derived bound,
	// MaxBatchReceptions.
	MaxBatchFrames = 32000
	// MaxStations is the largest number of simultaneously active receiving
	// stations. Removing a station frees an active slot but never releases its
	// identifier.
	MaxStations = 8
	// MaxBatchReceptions is the largest number of receptions one mutation may
	// return. Reception fan-out is at most one per active station per
	// transmission, so this bound is a guard that MaxBatchFrames always
	// reaches first.
	MaxBatchReceptions = MaxBatchFrames * MaxStations
)

// Error categories are available through errors.Is.
//
// Context failures are returned unchanged and satisfy errors.Is with
// context.Canceled or context.DeadlineExceeded.
var (
	// ErrInvalid identifies caller values outside their documented domain.
	// A duplicate station identifier, or one already used and removed in this
	// run, is an ErrInvalid case: the identifier is outside the domain of
	// identifiers the run still accepts.
	ErrInvalid = errors.New("invalid simulation input")
	// ErrLimit identifies a well-formed request that exceeds representable
	// time, identity, sequence, batch, or station capacity.
	ErrLimit = errors.New("simulation limit exceeded")
	// ErrNotFound identifies a command naming a station that does not exist.
	ErrNotFound = errors.New("simulation station not found")
	// ErrConflict identifies a station command whose supplied revision differs
	// from the current one, so another edit has intervened.
	ErrConflict = errors.New("simulation revision conflict")
)

// MessageKind identifies the ADS-B message family of a transmission.
type MessageKind uint8

// Message families emitted by the engine, in creation and tie-break order.
const (
	// IdentificationMessage is a DF17 TC4 aircraft identification report.
	IdentificationMessage MessageKind = 1
	// PositionMessage is a DF17 TC11 barometric airborne position report.
	PositionMessage MessageKind = 2
	// VelocityMessage is a DF17 TC19 subtype 1 airborne velocity report.
	VelocityMessage MessageKind = 3
)

// String returns a lowercase name for the message family.
func (k MessageKind) String() string {
	switch k {
	case IdentificationMessage:
		return "identification"
	case PositionMessage:
		return "position"
	case VelocityMessage:
		return "velocity"
	default:
		return "unknown"
	}
}

// Range is a sampling interval in one field's own unit. Equal endpoints mean
// an exact value. Otherwise values are drawn uniformly from the half-open
// interval [Min, Max).
type Range struct {
	Min float64
	Max float64
}

// exact reports whether the range denotes a single value.
func (r Range) exact() bool { return r.Min == r.Max }

// SpawnConfig describes the birth state of every aircraft the engine creates.
// Every field must be assigned; the engine supplies no defaults.
type SpawnConfig struct {
	// LatitudeDegrees is birth latitude in degrees, within [-85,85].
	// A later trajectory may cross a pole.
	LatitudeDegrees Range
	// LongitudeDegrees is birth longitude in degrees, within [-180,180].
	// A later trajectory may cross the date line.
	LongitudeDegrees Range
	// AltitudeFeet is birth pressure altitude in feet relative to 1013.25 hPa,
	// within [-1000,50175], matching the codec Q=1 encoder bounds.
	AltitudeFeet Range
	// GroundSpeedKnots is birth ground speed in knots, within [0,1000].
	// Zero is a permitted stationary airborne target.
	GroundSpeedKnots Range
	// TrackDegrees is birth true ground track in degrees, within [0,360).
	// Max may be 360 only when Min < Max, since no track may equal 360.
	TrackDegrees Range
	// VerticalRateFeetPerMinute is birth barometric vertical rate in feet per
	// minute, positive upward, within [-10000,10000].
	VerticalRateFeetPerMinute Range
}

// Config is the complete explicit configuration of one engine run.
// No constructor fills a missing field. Seed 0, InitialAircraftCount 0,
// SpeedHundredths 0, and zero-valued range endpoints have literal meanings;
// a plain Go value cannot distinguish an omitted zero from an assigned zero,
// so any later file or API parser must enforce field presence itself.
type Config struct {
	// ID names the run. It must contain a non-whitespace character and is
	// preserved exactly. Callers pair it with transmission and reception
	// sequences to tell runs apart, and must assign a fresh ID to each engine
	// lifetime, including restarts with otherwise identical configuration.
	ID string
	// StartTime is the virtual instant of zero elapsed time. It must be
	// nonzero and within years 1-9999. The engine normalizes it to UTC and
	// strips its monotonic component.
	StartTime time.Time
	// Seed selects the random streams of the run. Zero is a valid seed.
	Seed uint64
	// InitialAircraftCount is the number of aircraft created by New,
	// within [0,MaxAircraft].
	InitialAircraftCount int
	// SpeedHundredths is virtual time per real time in hundredths, within
	// [0,10000]. Zero pauses, 100 is real time, 10000 is one hundred times.
	SpeedHundredths uint16
	// Spawn describes birth state sampling.
	Spawn SpawnConfig
}

// Aircraft is the truth state of one aircraft evaluated at the snapshot
// instant. Navigation scalars are exact float64 values, not wire-quantized.
type Aircraft struct {
	// ICAO is the allocated 24-bit address, within 000001..FFFFFE.
	ICAO uint32
	// Callsign is TB followed by six uppercase hexadecimal address digits.
	Callsign string
	// CreatedAt is the virtual instant at which the aircraft was created.
	CreatedAt time.Time
	// LatitudeDegrees is current latitude in degrees, within [-90,90].
	LatitudeDegrees float64
	// LongitudeDegrees is current longitude in degrees, within [-180,180).
	LongitudeDegrees float64
	// BarometricAltitudeFeet is current pressure altitude in feet relative to
	// 1013.25 hPa. It is not height above terrain or GNSS height.
	BarometricAltitudeFeet float64
	// GroundSpeedKnots is the constant ground speed in knots.
	GroundSpeedKnots float64
	// TrackDegrees is the current true ground track in degrees, within
	// [0,360). It is not a magnetic heading.
	TrackDegrees float64
	// VerticalRateFeetPerMinute is the current barometric vertical rate in
	// feet per minute, positive upward. It is zero once the aircraft has
	// levelled off at an altitude limit.
	VerticalRateFeetPerMinute float64
}

// Transmission is one generated ADS-B frame and its virtual emission time.
type Transmission struct {
	// Sequence is the position of the transmission in its run, starting at 1.
	// It is unique within one engine; pair it with Config.ID across runs.
	Sequence uint64
	// ICAO is the address of the emitting aircraft.
	ICAO uint32
	// Timestamp is the virtual instant of emission.
	Timestamp time.Time
	// Kind is the message family.
	Kind MessageKind
	// Frame is the complete 14-byte DF17 frame including CRC. It carries no
	// truth fields and no transport wrapper.
	Frame [14]byte
}

// Reception is one transmission as heard by one station.
//
// It is self contained, so a consumer never has to join it against the
// transmission slice of the same batch.
type Reception struct {
	// Sequence is this station's reception sequence, starting at 1. Pair it
	// with the run ID and StationID to identify a reception across queries.
	Sequence uint64
	// TransmissionSequence is the Sequence of the transmission that was heard.
	TransmissionSequence uint64
	// StationID is the identifier of the receiving station.
	StationID string
	// StationRevision is the revision in effect when the decision was made,
	// which is the provenance of the settings that produced it.
	StationRevision uint64
	// Receiver is a value copy of the complete station record in effect when
	// the frame was received. It remains unchanged after station edits.
	Receiver Station
	// ICAO is the address of the emitting aircraft.
	ICAO uint32
	// Kind is the message family.
	Kind MessageKind
	// Timestamp is the virtual instant of reception. No propagation delay is
	// modelled, so it equals the transmission timestamp.
	Timestamp time.Time
	// Frame is the complete 14-byte DF17 frame including CRC, exactly as
	// transmitted.
	Frame [14]byte
	// SlantRangeNauticalMiles is the straight-line distance between the
	// station antenna and the aircraft. Synthetic model output.
	SlantRangeNauticalMiles float64
	// ReceivedPowerDBm is the modelled power arriving at the receiver.
	// Synthetic model output, not a calibrated measurement.
	ReceivedPowerDBm float64
}

// Batch is everything one mutation produced.
//
// A successful mutation returns both slices allocated, empty when nothing was
// emitted. A failed or canceled mutation returns the zero value.
// Transmissions are ordered by sequence; receptions are ordered by
// transmission sequence, then by station creation order.
type Batch struct {
	// Transmissions are the frames the aircraft emitted, complete even when
	// history has already evicted their tail.
	Transmissions []Transmission
	// Receptions are the per-station copies of those transmissions that the
	// reception model accepted. They are returned, not retained.
	Receptions []Reception
}

// HistorySnapshot is the retained transmission history of an engine.
type HistorySnapshot struct {
	// Messages are the retained transmissions, oldest first.
	Messages []Transmission
	// OldestSequence and LatestSequence are the bounds of Messages, or zero
	// when history is empty.
	OldestSequence uint64
	LatestSequence uint64
	// Limit is the retention capacity, always HistoryLimit.
	Limit int
}

// Snapshot is one detached, coherent view of an engine.
// Every slice it contains is owned by the caller.
type Snapshot struct {
	// Config is a copy of the run configuration. Its SpeedHundredths reflects
	// the current speed; its InitialAircraftCount retains the original count
	// given to New. The current count is len(Aircraft).
	Config Config
	// Now is the current virtual instant, StartTime plus Elapsed.
	Now time.Time
	// Elapsed is nonnegative virtual time since StartTime.
	Elapsed time.Duration
	// Aircraft are the active aircraft in creation order, evaluated at Now.
	Aircraft []Aircraft
	// Stations are the active receiving stations in creation order.
	Stations []Station
	// History is the retained transmission history.
	History HistorySnapshot
}
