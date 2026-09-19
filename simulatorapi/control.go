package simulatorapi

// Control commands.
//
// Every mutation names the run it was written for. A command whose RunID
// differs from the served run is a conflict and changes nothing, so a display
// or manager cannot apply an instruction to a replacement run by accident.
//
// Count and speed are absolute assignments, not increments. Within one run
// the last accepted command wins. Station operations always carry complete
// settings; there is no partial merge.

// CountCommand assigns the absolute number of active aircraft. Zero is a
// valid assignment that removes every aircraft.
type CountCommand struct {
	RunID string `json:"runId"`
	Count int    `json:"count"`
}

// SpeedCommand assigns virtual time per real time in hundredths. Zero pauses
// the run and is a valid assignment.
type SpeedCommand struct {
	RunID           string `json:"runId"`
	SpeedHundredths int    `json:"speedHundredths"`
}

// AddStationCommand creates one station from complete settings. A disabled
// station is fully configured and is a valid creation.
type AddStationCommand struct {
	RunID   string          `json:"runId"`
	Station StationSettings `json:"station"`
}

// UpdateStationCommand replaces every setting of an existing station.
// ExpectedRevision is the positive canonical decimal revision the caller last
// observed; a mismatch is a conflict. The station ID cannot be changed.
type UpdateStationCommand struct {
	RunID            string          `json:"runId"`
	ExpectedRevision string          `json:"expectedRevision"`
	Station          StationSettings `json:"station"`
}

// RemoveStationCommand deletes the station named by the route.
// ExpectedRevision is the positive canonical decimal revision the caller last
// observed; a mismatch is a conflict.
type RemoveStationCommand struct {
	RunID            string `json:"runId"`
	ExpectedRevision string `json:"expectedRevision"`
}

// Accepted operation names. They identify what a command acknowledgement
// describes without repeating the route.
const (
	OperationSetCount      = "setCount"
	OperationSetSpeed      = "setSpeed"
	OperationAddStation    = "addStation"
	OperationUpdateStation = "updateStation"
	OperationRemoveStation = "removeStation"
)

// CommandAck acknowledges one accepted command. It deliberately carries no
// later independently read snapshot, so a caller never mistakes a subsequent
// state for the immediate result of its own command.
type CommandAck struct {
	RunID     string `json:"runId"`
	Operation string `json:"operation"`
}

// StationAck acknowledges an accepted station command and returns the station
// exactly as the command produced it, including its new revision.
type StationAck struct {
	RunID     string  `json:"runId"`
	Operation string  `json:"operation"`
	Station   Station `json:"station"`
}
