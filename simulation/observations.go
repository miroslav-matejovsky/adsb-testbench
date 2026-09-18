package simulation

import (
	"fmt"
	"time"
)

// ReceptionCursor identifies the last station reception consumed by a caller.
type ReceptionCursor struct {
	RunID         string
	StationID     string
	AfterSequence uint64
}

// HistoryRequest selects one explicit page of one station's reception history.
type HistoryRequest struct {
	StationID string
	Cursor    *ReceptionCursor
	Limit     int
}

// ReceptionPage is one coherent, detached view of retained station receptions.
type ReceptionPage struct {
	RunID          string
	StationID      string
	Now            time.Time
	Records        []Reception
	OldestSequence uint64
	LatestSequence uint64
	NextCursor     ReceptionCursor
	Gap            bool
	HasMore        bool
	RetentionLimit int
}

// ObservationExpiry supplies independent virtual-time lifetimes for received
// fields. Every duration must be positive. A field remains fresh at exactly
// its lifetime and expires once its age exceeds that lifetime.
type ObservationExpiry struct {
	Identity time.Duration
	Position time.Duration
	Altitude time.Duration
	Velocity time.Duration
}

// ObservationRequest selects the stations whose retained receptions form an
// observation snapshot. An empty selection deliberately selects no stations.
type ObservationRequest struct {
	StationIDs []string
	Expiry     ObservationExpiry
}

// StationRetention describes the retained evidence available for one selected
// station. Truncated reports that at least one earlier reception was evicted.
type StationRetention struct {
	StationID      string
	OldestSequence uint64
	LatestSequence uint64
	Truncated      bool
	Limit          int
}

// ObservationEvidence is one distinct received transmission and all selected
// receiver copies which heard it.
type ObservationEvidence struct {
	TransmissionSequence uint64
	ICAO                 uint32
	Kind                 MessageKind
	Timestamp            time.Time
	Frame                [14]byte
	Receptions           []Reception
}

// IdentityObservation is the latest fresh received identification field.
type IdentityObservation struct {
	Callsign   string
	ObservedAt time.Time
	Evidence   ObservationEvidence
}

// PositionObservation is a global CPR fix reconstructed from two received
// position frames. Evidence contains the even and odd samples.
type PositionObservation struct {
	LatitudeDegrees  float64
	LongitudeDegrees float64
	ObservedAt       time.Time
	Evidence         []ObservationEvidence
}

// AltitudeObservation is the latest fresh received pressure altitude.
type AltitudeObservation struct {
	Feet       float64
	ObservedAt time.Time
	Evidence   ObservationEvidence
}

// ObservedMeasurement preserves a decoded numeric value and whether it is an
// over-range threshold rather than an exact measurement.
type ObservedMeasurement struct {
	Value     float64
	OverRange bool
}

// VelocityObservation is the latest fresh TC19 payload. Nil pointers mean the
// corresponding wire field was unavailable.
type VelocityObservation struct {
	Subtype                   uint8
	IntentChange              bool
	IFRCapability             bool
	NACv                      uint8
	EastKnots                 *ObservedMeasurement
	NorthKnots                *ObservedMeasurement
	GroundSpeedKnots          *float64
	TrackDegrees              *float64
	HeadingDegrees            *float64
	AirspeedKnots             *ObservedMeasurement
	TrueAirspeed              bool
	BarometricVerticalRate    bool
	VerticalRateFeetPerMinute *ObservedMeasurement
	GNSSMinusBaroFeet         *ObservedMeasurement
	ObservedAt                time.Time
	Evidence                  ObservationEvidence
}

// ObservedAircraft is received state for one ICAO address. Every optional
// field ages independently. LastReceivedAt never refreshes another field.
type ObservedAircraft struct {
	ICAO               uint32
	LastReceivedAt     time.Time
	Identity           *IdentityObservation
	Position           *PositionObservation
	BarometricAltitude *AltitudeObservation
	Velocity           *VelocityObservation
}

// ObservationSnapshot is a coherent received-data view at one virtual instant.
type ObservationSnapshot struct {
	RunID      string
	Now        time.Time
	StationIDs []string
	Retention  []StationRetention
	Expiry     ObservationExpiry
	Aircraft   []ObservedAircraft
}

func validateObservationExpiry(expiry ObservationExpiry) error {
	if expiry.Identity <= 0 || expiry.Position <= 0 || expiry.Altitude <= 0 || expiry.Velocity <= 0 {
		return fmt.Errorf("%w: all observation expiry durations must be positive", ErrInvalid)
	}
	return nil
}
