package display

import (
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Status describes what a display is currently showing.
type Status string

const (
	// StatusUnavailable means no refresh has ever succeeded for the current
	// state, so there are no observations to show.
	StatusUnavailable Status = "unavailable"
	// StatusFresh means the last refresh succeeded. It describes that
	// refresh, not a promise about how long ago polling last happened.
	StatusFresh Status = "fresh"
	// StatusStale means a refresh failed and the observations shown are the
	// last successful ones for the same selection.
	StatusStale Status = "stale"
)

// SourceFailure is the wire form of the last source failure.
//
// RunID is set only when the source run was validated. Status is the remote
// HTTP status when one was received, and zero otherwise.
type SourceFailure struct {
	Operation string                `json:"operation"`
	Code      simulatorapi.Category `json:"code"`
	Message   string                `json:"message"`
	RunID     string                `json:"runId"`
	Status    int                   `json:"status"`
}

// Snapshot is the detached, browser-facing view of a display.
//
// LastUpdatedAt is the real instant of the last successful refresh, kept
// separate from the source's virtual time so a UI can say how old the data
// is without ageing any received field by wall time. It is null before the
// first success. Observations is null whenever Status is unavailable. Error
// carries the last source failure, and is null after a successful refresh.
type Snapshot struct {
	Status        Status                            `json:"status"`
	LastUpdatedAt *string                           `json:"lastUpdatedAt"`
	Observations  *simulatorapi.ObservationSnapshot `json:"observations"`
	Error         *SourceFailure                    `json:"error"`
}

// failureOf renders one source error for the wire.
func failureOf(err *SourceError) *SourceFailure {
	if err == nil {
		return nil
	}
	return &SourceFailure{
		Operation: err.Operation, Code: err.Category,
		Message: err.Message, RunID: err.RunID, Status: err.Status,
	}
}

// cloneObservations deep-copies a published snapshot so a caller can edit
// what it receives without changing display state or a later response.
func cloneObservations(value *simulatorapi.ObservationSnapshot) *simulatorapi.ObservationSnapshot {
	if value == nil {
		return nil
	}
	copied := *value
	copied.StationIDs = append([]string{}, value.StationIDs...)
	copied.Retention = append([]simulatorapi.StationRetention{}, value.Retention...)
	copied.Aircraft = make([]simulatorapi.Aircraft, 0, len(value.Aircraft))
	for _, aircraft := range value.Aircraft {
		copied.Aircraft = append(copied.Aircraft, cloneAircraft(aircraft))
	}
	return &copied
}

func cloneAircraft(value simulatorapi.Aircraft) simulatorapi.Aircraft {
	copied := value
	if value.Identity != nil {
		identity := *value.Identity
		identity.Evidence = cloneEvidence(value.Identity.Evidence)
		copied.Identity = &identity
	}
	if value.Position != nil {
		position := *value.Position
		position.Evidence = make([]simulatorapi.Evidence, 0, len(value.Position.Evidence))
		for _, evidence := range value.Position.Evidence {
			position.Evidence = append(position.Evidence, cloneEvidence(evidence))
		}
		copied.Position = &position
	}
	if value.BarometricAltitude != nil {
		altitude := *value.BarometricAltitude
		altitude.Evidence = cloneEvidence(value.BarometricAltitude.Evidence)
		copied.BarometricAltitude = &altitude
	}
	if value.Velocity != nil {
		velocity := *value.Velocity
		velocity.EastKnots = cloneMeasurement(value.Velocity.EastKnots)
		velocity.NorthKnots = cloneMeasurement(value.Velocity.NorthKnots)
		velocity.AirspeedKnots = cloneMeasurement(value.Velocity.AirspeedKnots)
		velocity.VerticalRateFeetPerMinute = cloneMeasurement(value.Velocity.VerticalRateFeetPerMinute)
		velocity.GNSSMinusBaroFeet = cloneMeasurement(value.Velocity.GNSSMinusBaroFeet)
		velocity.GroundSpeedKnots = copyFloat(value.Velocity.GroundSpeedKnots)
		velocity.TrackDegrees = copyFloat(value.Velocity.TrackDegrees)
		velocity.HeadingDegrees = copyFloat(value.Velocity.HeadingDegrees)
		velocity.Evidence = cloneEvidence(value.Velocity.Evidence)
		copied.Velocity = &velocity
	}
	return copied
}

func cloneEvidence(value simulatorapi.Evidence) simulatorapi.Evidence {
	copied := value
	copied.Receptions = append([]simulatorapi.Reception{}, value.Receptions...)
	return copied
}

func cloneMeasurement(value *simulatorapi.Measurement) *simulatorapi.Measurement {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
