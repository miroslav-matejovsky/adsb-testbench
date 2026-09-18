package display

import (
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Strict browser-facing request parsers.
//
// They use the same presence rules as the simulator's routes: a missing key,
// an explicit null, an unknown key, a duplicate key, or a wrong type is
// rejected before any source is contacted.

// parseSelectionRequest binds a required explicit station selection. An empty
// array selects nothing and is a value, not an omission.
func parseSelectionRequest(value *simulatorapi.Value) ([]string, error) {
	object, err := value.Object()
	if err != nil {
		return nil, err
	}
	stationIDs, err := readTextArray(object, "stationIds")
	if err != nil {
		return nil, err
	}
	if err := object.Done(); err != nil {
		return nil, err
	}
	return stationIDs, nil
}

// parseHistoryRequestBody binds one inspection request, including its
// explicit page size and its optional resume cursor.
func parseHistoryRequestBody(value *simulatorapi.Value) (simulatorapi.HistoryRequest, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.HistoryRequest{}, err
	}
	var request simulatorapi.HistoryRequest
	if request.StationID, err = readText(object, "stationId"); err != nil {
		return simulatorapi.HistoryRequest{}, err
	}
	if request.Limit, err = readInt(object, "limit"); err != nil {
		return simulatorapi.HistoryRequest{}, err
	}
	cursorValue, err := object.Nullable("cursor")
	if err != nil {
		return simulatorapi.HistoryRequest{}, err
	}
	if cursorValue != nil {
		cursor, err := parseCursor(cursorValue)
		if err != nil {
			return simulatorapi.HistoryRequest{}, err
		}
		request.Cursor = &cursor
	}
	if err := object.Done(); err != nil {
		return simulatorapi.HistoryRequest{}, err
	}
	return request, nil
}
