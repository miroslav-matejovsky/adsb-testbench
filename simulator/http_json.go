package simulator

import (
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Strict request-body parsers.
//
// Each parser binds one wire type from an already strictly parsed value tree,
// so missing keys, explicit nulls, unknown keys, duplicate keys, and wrong
// types are all rejected before any engine value exists. Parsing never
// narrows an untrusted number without checking it first.

func parseCountCommand(value *simulatorapi.Value) (simulatorapi.CountCommand, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.CountCommand{}, err
	}
	var command simulatorapi.CountCommand
	if command.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.CountCommand{}, err
	}
	if command.Count, err = readInt(object, "count"); err != nil {
		return simulatorapi.CountCommand{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.CountCommand{}, err
	}
	return command, nil
}

func parseSpeedCommand(value *simulatorapi.Value) (simulatorapi.SpeedCommand, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.SpeedCommand{}, err
	}
	var command simulatorapi.SpeedCommand
	if command.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.SpeedCommand{}, err
	}
	if command.SpeedHundredths, err = readInt(object, "speedHundredths"); err != nil {
		return simulatorapi.SpeedCommand{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.SpeedCommand{}, err
	}
	return command, nil
}

func parseAddStationCommand(value *simulatorapi.Value) (simulatorapi.AddStationCommand, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.AddStationCommand{}, err
	}
	var command simulatorapi.AddStationCommand
	if command.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.AddStationCommand{}, err
	}
	station, err := object.Field("station")
	if err != nil {
		return simulatorapi.AddStationCommand{}, err
	}
	if command.Station, err = parseStationSettings(station); err != nil {
		return simulatorapi.AddStationCommand{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.AddStationCommand{}, err
	}
	return command, nil
}

func parseUpdateStationCommand(value *simulatorapi.Value) (simulatorapi.UpdateStationCommand, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	var command simulatorapi.UpdateStationCommand
	if command.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	if command.ExpectedRevision, err = readText(object, "expectedRevision"); err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	station, err := object.Field("station")
	if err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	if command.Station, err = parseStationSettings(station); err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.UpdateStationCommand{}, err
	}
	return command, nil
}

func parseRemoveStationCommand(value *simulatorapi.Value) (simulatorapi.RemoveStationCommand, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.RemoveStationCommand{}, err
	}
	var command simulatorapi.RemoveStationCommand
	if command.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.RemoveStationCommand{}, err
	}
	if command.ExpectedRevision, err = readText(object, "expectedRevision"); err != nil {
		return simulatorapi.RemoveStationCommand{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.RemoveStationCommand{}, err
	}
	return command, nil
}

// parseStationSettings requires every station setting. There is no partial
// update, so an omitted key is an error rather than an unchanged value.
func parseStationSettings(value *simulatorapi.Value) (simulatorapi.StationSettings, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.StationSettings{}, err
	}
	var settings simulatorapi.StationSettings
	if settings.ID, err = readText(object, "id"); err != nil {
		return simulatorapi.StationSettings{}, err
	}
	if settings.Enabled, err = readBool(object, "enabled"); err != nil {
		return simulatorapi.StationSettings{}, err
	}
	numbers := []struct {
		key    string
		target *float64
	}{
		{"latitudeDegrees", &settings.LatitudeDegrees},
		{"longitudeDegrees", &settings.LongitudeDegrees},
		{"siteElevationMetres", &settings.SiteElevationMetres},
		{"antennaHeightMetres", &settings.AntennaHeightMetres},
		{"antennaGainDBi", &settings.AntennaGainDBi},
		{"sensitivityDBm", &settings.SensitivityDBm},
		{"systemLossDB", &settings.SystemLossDB},
		{"frameLossProbability", &settings.FrameLossProbability},
	}
	for _, number := range numbers {
		if *number.target, err = readFloat(object, number.key); err != nil {
			return simulatorapi.StationSettings{}, err
		}
	}
	if err := object.Done(); err != nil {
		return simulatorapi.StationSettings{}, err
	}
	return settings, nil
}

func parseObservationRequest(value *simulatorapi.Value) (simulatorapi.ObservationRequest, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	stationIDs, err := readStationIDs(object)
	if err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	expiryValue, err := object.Field("expiry")
	if err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	expiryObject, err := expiryValue.Object()
	if err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	var expiry simulatorapi.ObservationExpiry
	lifetimes := []struct {
		key    string
		target *string
	}{
		{"identityNanoseconds", &expiry.IdentityNanoseconds},
		{"positionNanoseconds", &expiry.PositionNanoseconds},
		{"altitudeNanoseconds", &expiry.AltitudeNanoseconds},
		{"velocityNanoseconds", &expiry.VelocityNanoseconds},
	}
	for _, lifetime := range lifetimes {
		if *lifetime.target, err = readText(expiryObject, lifetime.key); err != nil {
			return simulatorapi.ObservationRequest{}, err
		}
	}
	if err := expiryObject.Done(); err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.ObservationRequest{}, err
	}
	return simulatorapi.ObservationRequest{StationIDs: stationIDs, Expiry: expiry}, nil
}

func parseReceptionSnapshotRequest(value *simulatorapi.Value) (simulatorapi.ReceptionSnapshotRequest, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.ReceptionSnapshotRequest{}, err
	}
	stationIDs, err := readStationIDs(object)
	if err != nil {
		return simulatorapi.ReceptionSnapshotRequest{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.ReceptionSnapshotRequest{}, err
	}
	return simulatorapi.ReceptionSnapshotRequest{StationIDs: stationIDs}, nil
}

func parseHistoryRequest(value *simulatorapi.Value) (simulatorapi.HistoryRequest, error) {
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
		cursor, err := parseReceptionCursor(cursorValue)
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

func parseReceptionCursor(value *simulatorapi.Value) (simulatorapi.ReceptionCursor, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.ReceptionCursor{}, err
	}
	var cursor simulatorapi.ReceptionCursor
	if cursor.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.ReceptionCursor{}, err
	}
	if cursor.StationID, err = readText(object, "stationId"); err != nil {
		return simulatorapi.ReceptionCursor{}, err
	}
	if cursor.AfterSequence, err = readText(object, "afterSequence"); err != nil {
		return simulatorapi.ReceptionCursor{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.ReceptionCursor{}, err
	}
	return cursor, nil
}

// readStationIDs binds a required station selection. An explicit empty array
// is a selection of nothing, not an omitted field.
func readStationIDs(object *simulatorapi.Object) ([]string, error) {
	field, err := object.Field("stationIds")
	if err != nil {
		return nil, err
	}
	items, err := field.Array()
	if err != nil {
		return nil, err
	}
	stationIDs := make([]string, 0, len(items))
	for _, item := range items {
		id, err := item.Text()
		if err != nil {
			return nil, err
		}
		stationIDs = append(stationIDs, id)
	}
	return stationIDs, nil
}

func readBool(object *simulatorapi.Object, key string) (bool, error) {
	field, err := object.Field(key)
	if err != nil {
		return false, err
	}
	return field.Bool()
}
