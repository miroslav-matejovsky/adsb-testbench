package display

import (
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Strict response parsers.
//
// A remote response is bound key by key, so a missing key, an explicit null,
// an unknown key, a duplicate key, or a wrong type is rejected before any
// semantic validation runs. Nothing is inferred from an absent field.

func parseReceptionSnapshot(value *simulatorapi.Value) (simulatorapi.ReceptionSnapshot, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	var snapshot simulatorapi.ReceptionSnapshot
	if snapshot.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	if snapshot.Now, err = readText(object, "now"); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	if snapshot.StationIDs, err = readTextArray(object, "stationIds"); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	if snapshot.Retention, err = readRetention(object); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	if snapshot.Records, err = readReceptions(object, "records"); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.ReceptionSnapshot{}, err
	}
	return snapshot, nil
}

func parseReceptionPage(value *simulatorapi.Value) (simulatorapi.ReceptionPage, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	var page simulatorapi.ReceptionPage
	if page.RunID, err = readText(object, "runId"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.StationID, err = readText(object, "stationId"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.Now, err = readText(object, "now"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.Records, err = readReceptions(object, "records"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.OldestSequence, err = readText(object, "oldestSequence"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.LatestSequence, err = readText(object, "latestSequence"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	cursorValue, err := object.Field("nextCursor")
	if err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.NextCursor, err = parseCursor(cursorValue); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.Gap, err = readBool(object, "gap"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.HasMore, err = readBool(object, "hasMore"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if page.RetentionLimit, err = readInt(object, "retentionLimit"); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.ReceptionPage{}, err
	}
	return page, nil
}

func parseErrorEnvelope(value *simulatorapi.Value) (simulatorapi.APIError, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.APIError{}, err
	}
	errorValue, err := object.Field("error")
	if err != nil {
		return simulatorapi.APIError{}, err
	}
	errorObject, err := errorValue.Object()
	if err != nil {
		return simulatorapi.APIError{}, err
	}
	var failure simulatorapi.APIError
	code, err := readText(errorObject, "code")
	if err != nil {
		return simulatorapi.APIError{}, err
	}
	failure.Code = simulatorapi.Category(code)
	if !failure.Code.Valid() {
		return simulatorapi.APIError{}, unknownCategory(code)
	}
	if failure.Message, err = readText(errorObject, "message"); err != nil {
		return simulatorapi.APIError{}, err
	}
	if failure.Field, err = readText(errorObject, "field"); err != nil {
		return simulatorapi.APIError{}, err
	}
	if failure.RunID, err = readText(errorObject, "runId"); err != nil {
		return simulatorapi.APIError{}, err
	}
	if err := errorObject.Done(); err != nil {
		return simulatorapi.APIError{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.APIError{}, err
	}
	failure.Message = simulatorapi.ClampMessage(failure.Message)
	return failure, nil
}

func parseCursor(value *simulatorapi.Value) (simulatorapi.ReceptionCursor, error) {
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

func readRetention(object *simulatorapi.Object) ([]simulatorapi.StationRetention, error) {
	field, err := object.Field("retention")
	if err != nil {
		return nil, err
	}
	items, err := field.Array()
	if err != nil {
		return nil, err
	}
	retention := make([]simulatorapi.StationRetention, 0, len(items))
	for _, item := range items {
		entryObject, err := item.Object()
		if err != nil {
			return nil, err
		}
		var entry simulatorapi.StationRetention
		if entry.StationID, err = readText(entryObject, "stationId"); err != nil {
			return nil, err
		}
		if entry.OldestSequence, err = readText(entryObject, "oldestSequence"); err != nil {
			return nil, err
		}
		if entry.LatestSequence, err = readText(entryObject, "latestSequence"); err != nil {
			return nil, err
		}
		if entry.Truncated, err = readBool(entryObject, "truncated"); err != nil {
			return nil, err
		}
		if entry.Limit, err = readInt(entryObject, "limit"); err != nil {
			return nil, err
		}
		if err := entryObject.Done(); err != nil {
			return nil, err
		}
		retention = append(retention, entry)
	}
	return retention, nil
}

func readReceptions(object *simulatorapi.Object, key string) ([]simulatorapi.Reception, error) {
	field, err := object.Field(key)
	if err != nil {
		return nil, err
	}
	items, err := field.Array()
	if err != nil {
		return nil, err
	}
	records := make([]simulatorapi.Reception, 0, len(items))
	for _, item := range items {
		record, err := parseReception(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func parseReception(value *simulatorapi.Value) (simulatorapi.Reception, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.Reception{}, err
	}
	var record simulatorapi.Reception
	texts := []struct {
		key    string
		target *string
	}{
		{"sequence", &record.Sequence},
		{"transmissionSequence", &record.TransmissionSequence},
		{"stationId", &record.StationID},
		{"stationRevision", &record.StationRevision},
		{"icao", &record.ICAO},
		{"kind", &record.Kind},
		{"timestamp", &record.Timestamp},
		{"frame", &record.Frame},
	}
	for _, text := range texts {
		if *text.target, err = readText(object, text.key); err != nil {
			return simulatorapi.Reception{}, err
		}
	}
	if record.SlantRangeNauticalMiles, err = readFloat(object, "slantRangeNauticalMiles"); err != nil {
		return simulatorapi.Reception{}, err
	}
	if record.ReceivedPowerDBm, err = readFloat(object, "receivedPowerDBm"); err != nil {
		return simulatorapi.Reception{}, err
	}
	receiverValue, err := object.Field("receiver")
	if err != nil {
		return simulatorapi.Reception{}, err
	}
	if record.Receiver, err = parseStation(receiverValue); err != nil {
		return simulatorapi.Reception{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.Reception{}, err
	}
	return record, nil
}

func parseStation(value *simulatorapi.Value) (simulatorapi.Station, error) {
	object, err := value.Object()
	if err != nil {
		return simulatorapi.Station{}, err
	}
	var station simulatorapi.Station
	if station.ID, err = readText(object, "id"); err != nil {
		return simulatorapi.Station{}, err
	}
	if station.Enabled, err = readBool(object, "enabled"); err != nil {
		return simulatorapi.Station{}, err
	}
	numbers := []struct {
		key    string
		target *float64
	}{
		{"latitudeDegrees", &station.LatitudeDegrees},
		{"longitudeDegrees", &station.LongitudeDegrees},
		{"siteElevationMetres", &station.SiteElevationMetres},
		{"antennaHeightMetres", &station.AntennaHeightMetres},
		{"antennaGainDBi", &station.AntennaGainDBi},
		{"sensitivityDBm", &station.SensitivityDBm},
		{"systemLossDB", &station.SystemLossDB},
		{"frameLossProbability", &station.FrameLossProbability},
	}
	for _, number := range numbers {
		if *number.target, err = readFloat(object, number.key); err != nil {
			return simulatorapi.Station{}, err
		}
	}
	if station.Revision, err = readText(object, "revision"); err != nil {
		return simulatorapi.Station{}, err
	}
	if station.CreatedAt, err = readText(object, "createdAt"); err != nil {
		return simulatorapi.Station{}, err
	}
	if err := object.Done(); err != nil {
		return simulatorapi.Station{}, err
	}
	return station, nil
}

func readTextArray(object *simulatorapi.Object, key string) ([]string, error) {
	field, err := object.Field(key)
	if err != nil {
		return nil, err
	}
	items, err := field.Array()
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text, err := item.Text()
		if err != nil {
			return nil, err
		}
		values = append(values, text)
	}
	return values, nil
}

func readBool(object *simulatorapi.Object, key string) (bool, error) {
	field, err := object.Field(key)
	if err != nil {
		return false, err
	}
	return field.Bool()
}

func readFloat(object *simulatorapi.Object, key string) (float64, error) {
	field, err := object.Field(key)
	if err != nil {
		return 0, err
	}
	return field.Float()
}
