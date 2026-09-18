package display

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Display policy bounds on accepted source data. They mirror the simulator's
// published engine limits, which a display cannot import, and exist so one
// malformed or hostile response cannot make a refresh unbounded.
const (
	maxSelectedStations = 8
	maxStationRecords   = 1000
	maxSnapshotRecords  = maxSelectedStations * maxStationRecords
	minStationIDBytes   = 1
	maxStationIDBytes   = 64
)

// Message families a received frame may claim. They must agree with what the
// frame actually decodes to.
const (
	identificationKind = "identification"
	positionKind       = "position"
	velocityKind       = "velocity"
)

// receiverDomains lists every numeric receiver setting with its published
// domain, so historical provenance is checked rather than trusted.
var receiverDomains = []struct {
	name string
	lo   float64
	hi   float64
	get  func(simulatorapi.StationSettings) float64
}{
	{"latitudeDegrees", -90, 90, func(s simulatorapi.StationSettings) float64 { return s.LatitudeDegrees }},
	{"longitudeDegrees", -180, 180, func(s simulatorapi.StationSettings) float64 { return s.LongitudeDegrees }},
	{"siteElevationMetres", -500, 9000, func(s simulatorapi.StationSettings) float64 { return s.SiteElevationMetres }},
	{"antennaHeightMetres", 0, 500, func(s simulatorapi.StationSettings) float64 { return s.AntennaHeightMetres }},
	{"antennaGainDBi", -10, 40, func(s simulatorapi.StationSettings) float64 { return s.AntennaGainDBi }},
	{"sensitivityDBm", -140, 0, func(s simulatorapi.StationSettings) float64 { return s.SensitivityDBm }},
	{"systemLossDB", 0, 30, func(s simulatorapi.StationSettings) float64 { return s.SystemLossDB }},
	{"frameLossProbability", 0, 1, func(s simulatorapi.StationSettings) float64 { return s.FrameLossProbability }},
}

// reception is one semantically validated raw record in native types. It
// keeps its original wire record so returned evidence is exactly what the
// source supplied.
type reception struct {
	sequence             uint64
	transmissionSequence uint64
	stationID            string
	stationRevision      uint64
	icao                 uint32
	kind                 string
	timestamp            time.Time
	frame                [simulatorapi.FrameBytes]byte
	record               simulatorapi.Reception
}

// snapshot is one semantically validated raw reception snapshot.
type snapshot struct {
	runID      string
	now        time.Time
	stationIDs []string
	retention  []simulatorapi.StationRetention
	records    []reception
}

// validateSnapshotRequest checks a request before any source is contacted.
func validateSnapshotRequest(request simulatorapi.ReceptionSnapshotRequest) error {
	if request.StationIDs == nil {
		return errors.New("stationIds must be an array, including when empty")
	}
	return validateSelection(request.StationIDs)
}

// validateHistoryRequest checks a page request before any source is contacted.
func validateHistoryRequest(request simulatorapi.HistoryRequest) error {
	if err := validateStationID(request.StationID); err != nil {
		return fmt.Errorf("stationId: %w", err)
	}
	if request.Limit < 1 || request.Limit > maxStationRecords {
		return fmt.Errorf("limit %d must be within [1,%d]", request.Limit, maxStationRecords)
	}
	if request.Cursor == nil {
		return nil
	}
	if request.Cursor.StationID != request.StationID {
		return fmt.Errorf("cursor station %q does not match request station %q",
			request.Cursor.StationID, request.StationID)
	}
	if request.Cursor.RunID == "" {
		return errors.New("cursor runId must not be empty")
	}
	if _, err := simulatorapi.ParseUint64(request.Cursor.AfterSequence); err != nil {
		return fmt.Errorf("cursor afterSequence: %w", err)
	}
	return nil
}

// validateSelection checks an explicit station selection.
func validateSelection(stationIDs []string) error {
	if len(stationIDs) > maxSelectedStations {
		return fmt.Errorf("at most %d stations may be selected, got %d", maxSelectedStations, len(stationIDs))
	}
	seen := make(map[string]bool, len(stationIDs))
	for _, id := range stationIDs {
		if err := validateStationID(id); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("station %q is selected twice", id)
		}
		seen[id] = true
	}
	return nil
}

// validateStationID applies the identifier rules of the simulator: 1 to 64
// bytes of ASCII letters, digits, hyphen, or underscore, compared exactly.
func validateStationID(id string) error {
	if len(id) < minStationIDBytes || len(id) > maxStationIDBytes {
		return fmt.Errorf("station ID has %d bytes, want %d-%d", len(id), minStationIDBytes, maxStationIDBytes)
	}
	for i := range len(id) {
		b := id[i]
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '-', b == '_':
		default:
			return fmt.Errorf("station ID byte %d is %q, want an ASCII letter, digit, hyphen, or underscore", i, b)
		}
	}
	return nil
}

// validateSnapshotEnvelope checks run identity, selection, time, and
// retention membership without looking at records.
//
// It is separate from record validation so an adapter can report a validated
// run identifier even when the payload itself turns out to be invalid.
func validateSnapshotEnvelope(request simulatorapi.ReceptionSnapshotRequest, raw simulatorapi.ReceptionSnapshot) (snapshot, error) {
	if raw.RunID == "" {
		return snapshot{}, errors.New("runId must not be empty")
	}
	now, err := simulatorapi.ParseTime(raw.Now)
	if err != nil {
		return snapshot{}, fmt.Errorf("now: %w", err)
	}
	if raw.StationIDs == nil || raw.Retention == nil || raw.Records == nil {
		return snapshot{}, errors.New("stationIds, retention, and records must be arrays, including when empty")
	}
	if err := validateSelection(raw.StationIDs); err != nil {
		return snapshot{}, err
	}
	if !sort.StringsAreSorted(raw.StationIDs) {
		return snapshot{}, errors.New("stationIds must be sorted")
	}
	if !sameSelection(normalizeSelection(request.StationIDs), raw.StationIDs) {
		return snapshot{}, fmt.Errorf("stationIds %v do not match the requested selection %v",
			raw.StationIDs, request.StationIDs)
	}
	if len(raw.Retention) != len(raw.StationIDs) {
		return snapshot{}, fmt.Errorf("retention describes %d stations, want %d",
			len(raw.Retention), len(raw.StationIDs))
	}
	for i, entry := range raw.Retention {
		if entry.StationID != raw.StationIDs[i] {
			return snapshot{}, fmt.Errorf("retention entry %d describes station %q, want %q",
				i, entry.StationID, raw.StationIDs[i])
		}
		if entry.Limit < 1 || entry.Limit > maxStationRecords {
			return snapshot{}, fmt.Errorf("station %q retention limit %d must be within [1,%d]",
				entry.StationID, entry.Limit, maxStationRecords)
		}
	}
	if len(raw.Records) > maxSnapshotRecords {
		return snapshot{}, fmt.Errorf("snapshot carries %d records, above the %d bound",
			len(raw.Records), maxSnapshotRecords)
	}
	return snapshot{
		runID: raw.RunID, now: now,
		stationIDs: append([]string(nil), raw.StationIDs...),
		retention:  append([]simulatorapi.StationRetention(nil), raw.Retention...),
	}, nil
}

// validateSnapshot validates the complete snapshot, envelope first. It never
// returns a partially validated result.
func validateSnapshot(request simulatorapi.ReceptionSnapshotRequest, raw simulatorapi.ReceptionSnapshot) (snapshot, string, error) {
	validated, err := validateSnapshotEnvelope(request, raw)
	if err != nil {
		return snapshot{}, "", err
	}
	records, err := validateRecords(validated, raw.Records)
	if err != nil {
		return snapshot{}, validated.runID, err
	}
	validated.records = records
	return validated, validated.runID, nil
}

// validateRecords checks every record, its provenance, its ordering, and its
// agreement with the retention metadata of its station.
func validateRecords(envelope snapshot, raw []simulatorapi.Reception) ([]reception, error) {
	selected := make(map[string]bool, len(envelope.stationIDs))
	for _, id := range envelope.stationIDs {
		selected[id] = true
	}

	records := make([]reception, 0, len(raw))
	lastSequence := make(map[string]uint64, len(envelope.stationIDs))
	settings := make(map[string]simulatorapi.StationSettings, len(envelope.stationIDs))
	transmissions := make(map[uint64]reception, len(raw))
	copies := make(map[string]bool, len(raw))

	for index, record := range raw {
		validated, err := validateReception(record, envelope.now)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", index, err)
		}
		if !selected[validated.stationID] {
			return nil, fmt.Errorf("record %d belongs to unselected station %q", index, validated.stationID)
		}

		if previous, ok := lastSequence[validated.stationID]; ok {
			if validated.sequence != previous+1 {
				return nil, fmt.Errorf("record %d station %q sequence %d does not follow %d",
					index, validated.stationID, validated.sequence, previous)
			}
		}
		lastSequence[validated.stationID] = validated.sequence

		revisionKey := fmt.Sprintf("%s#%d", validated.stationID, validated.stationRevision)
		if known, ok := settings[revisionKey]; ok {
			if known != validated.record.Receiver.StationSettings {
				return nil, fmt.Errorf("record %d station %q revision %d contradicts an earlier receiver snapshot",
					index, validated.stationID, validated.stationRevision)
			}
		} else {
			settings[revisionKey] = validated.record.Receiver.StationSettings
		}

		copyKey := fmt.Sprintf("%s#%d", validated.stationID, validated.transmissionSequence)
		if copies[copyKey] {
			return nil, fmt.Errorf("record %d repeats transmission %d at station %q",
				index, validated.transmissionSequence, validated.stationID)
		}
		copies[copyKey] = true

		if first, ok := transmissions[validated.transmissionSequence]; ok {
			if first.icao != validated.icao || first.kind != validated.kind ||
				!first.timestamp.Equal(validated.timestamp) || first.frame != validated.frame {
				return nil, fmt.Errorf("record %d disagrees with another copy of transmission %d",
					index, validated.transmissionSequence)
			}
		} else {
			transmissions[validated.transmissionSequence] = validated
		}

		records = append(records, validated)
	}

	if err := validateRetentionBounds(envelope, records); err != nil {
		return nil, err
	}
	return records, nil
}

// validateRetentionBounds checks that retention describes exactly the records
// captured for each selected station.
func validateRetentionBounds(envelope snapshot, records []reception) error {
	for _, entry := range envelope.retention {
		var first, last uint64
		count := 0
		for _, record := range records {
			if record.stationID != entry.StationID {
				continue
			}
			if count == 0 {
				first = record.sequence
			}
			last = record.sequence
			count++
		}
		oldest, err := simulatorapi.ParseUint64(entry.OldestSequence)
		if err != nil {
			return fmt.Errorf("station %q oldestSequence: %w", entry.StationID, err)
		}
		latest, err := simulatorapi.ParseUint64(entry.LatestSequence)
		if err != nil {
			return fmt.Errorf("station %q latestSequence: %w", entry.StationID, err)
		}
		if count > entry.Limit {
			return fmt.Errorf("station %q carries %d records, above its retention limit %d",
				entry.StationID, count, entry.Limit)
		}
		if count == 0 {
			if oldest != 0 || latest != 0 {
				return fmt.Errorf("station %q has no records but claims bounds %d-%d",
					entry.StationID, oldest, latest)
			}
			if entry.Truncated {
				return fmt.Errorf("station %q has no records but claims truncation", entry.StationID)
			}
			continue
		}
		if oldest != first || latest != last {
			return fmt.Errorf("station %q claims bounds %d-%d but carries %d-%d",
				entry.StationID, oldest, latest, first, last)
		}
		if entry.Truncated != (oldest > 1) {
			return fmt.Errorf("station %q truncation %t contradicts its oldest sequence %d",
				entry.StationID, entry.Truncated, oldest)
		}
	}
	return nil
}

// validateReception checks one record and its historical receiver provenance.
func validateReception(record simulatorapi.Reception, now time.Time) (reception, error) {
	sequence, err := positiveSequence(record.Sequence, "sequence")
	if err != nil {
		return reception{}, err
	}
	transmission, err := positiveSequence(record.TransmissionSequence, "transmissionSequence")
	if err != nil {
		return reception{}, err
	}
	revision, err := positiveSequence(record.StationRevision, "stationRevision")
	if err != nil {
		return reception{}, err
	}
	if err := validateStationID(record.StationID); err != nil {
		return reception{}, fmt.Errorf("stationId: %w", err)
	}
	icao, err := simulatorapi.ParseICAO(record.ICAO)
	if err != nil {
		return reception{}, err
	}
	switch record.Kind {
	case identificationKind, positionKind, velocityKind:
	default:
		return reception{}, fmt.Errorf("kind %q is not a supported message family", record.Kind)
	}
	timestamp, err := simulatorapi.ParseTime(record.Timestamp)
	if err != nil {
		return reception{}, fmt.Errorf("timestamp: %w", err)
	}
	if timestamp.After(now) {
		return reception{}, fmt.Errorf("timestamp %s is later than the snapshot instant", record.Timestamp)
	}
	frame, err := simulatorapi.ParseFrame(record.Frame)
	if err != nil {
		return reception{}, err
	}
	if !simulatorapi.Finite(record.SlantRangeNauticalMiles) || record.SlantRangeNauticalMiles < 0 {
		return reception{}, fmt.Errorf("slantRangeNauticalMiles %g must be finite and nonnegative",
			record.SlantRangeNauticalMiles)
	}
	if !simulatorapi.Finite(record.ReceivedPowerDBm) {
		return reception{}, errors.New("receivedPowerDBm must be finite")
	}
	if err := validateReceiver(record, revision, timestamp); err != nil {
		return reception{}, err
	}
	return reception{
		sequence: sequence, transmissionSequence: transmission,
		stationID: record.StationID, stationRevision: revision,
		icao: icao, kind: record.Kind, timestamp: timestamp, frame: frame,
		record: record,
	}, nil
}

// validateReceiver checks the historical station snapshot recorded with one
// reception. Its settings are the provenance of that record and are never
// replaced with a current station configuration.
func validateReceiver(record simulatorapi.Reception, revision uint64, timestamp time.Time) error {
	receiver := record.Receiver
	if receiver.ID != record.StationID {
		return fmt.Errorf("receiver id %q differs from record station %q", receiver.ID, record.StationID)
	}
	receiverRevision, err := positiveSequence(receiver.Revision, "receiver revision")
	if err != nil {
		return err
	}
	if receiverRevision != revision {
		return fmt.Errorf("receiver revision %d differs from record revision %d", receiverRevision, revision)
	}
	createdAt, err := simulatorapi.ParseTime(receiver.CreatedAt)
	if err != nil {
		return fmt.Errorf("receiver createdAt: %w", err)
	}
	if createdAt.After(timestamp) {
		return fmt.Errorf("receiver was created at %s, after the reception at %s",
			receiver.CreatedAt, record.Timestamp)
	}
	for _, domain := range receiverDomains {
		value := domain.get(receiver.StationSettings)
		if !simulatorapi.Finite(value) {
			return fmt.Errorf("receiver %s must be finite", domain.name)
		}
		if value < domain.lo || value > domain.hi {
			return fmt.Errorf("receiver %s is %g, outside the accepted range [%g,%g]",
				domain.name, value, domain.lo, domain.hi)
		}
	}
	return nil
}

// validatePage checks one history page against the request that produced it.
func validatePage(request simulatorapi.HistoryRequest, page simulatorapi.ReceptionPage) (string, error) {
	if page.RunID == "" {
		return "", errors.New("runId must not be empty")
	}
	if page.StationID != request.StationID {
		return page.RunID, fmt.Errorf("page station %q does not match request station %q",
			page.StationID, request.StationID)
	}
	if request.Cursor != nil && page.RunID != request.Cursor.RunID {
		return page.RunID, fmt.Errorf("page run %q does not match cursor run %q",
			page.RunID, request.Cursor.RunID)
	}
	now, err := simulatorapi.ParseTime(page.Now)
	if err != nil {
		return page.RunID, fmt.Errorf("now: %w", err)
	}
	if page.Records == nil {
		return page.RunID, errors.New("records must be an array, including when empty")
	}
	if page.RetentionLimit < 1 || page.RetentionLimit > maxStationRecords {
		return page.RunID, fmt.Errorf("retentionLimit %d must be within [1,%d]",
			page.RetentionLimit, maxStationRecords)
	}
	if len(page.Records) > request.Limit {
		return page.RunID, fmt.Errorf("page carries %d records, above the requested limit %d",
			len(page.Records), request.Limit)
	}

	oldest, err := simulatorapi.ParseUint64(page.OldestSequence)
	if err != nil {
		return page.RunID, fmt.Errorf("oldestSequence: %w", err)
	}
	latest, err := simulatorapi.ParseUint64(page.LatestSequence)
	if err != nil {
		return page.RunID, fmt.Errorf("latestSequence: %w", err)
	}
	if oldest > latest {
		return page.RunID, fmt.Errorf("oldestSequence %d is above latestSequence %d", oldest, latest)
	}

	var previous uint64
	for index, record := range page.Records {
		validated, err := validateReception(record, now)
		if err != nil {
			return page.RunID, fmt.Errorf("record %d: %w", index, err)
		}
		if validated.stationID != request.StationID {
			return page.RunID, fmt.Errorf("record %d belongs to station %q, want %q",
				index, validated.stationID, request.StationID)
		}
		if index > 0 && validated.sequence != previous+1 {
			return page.RunID, fmt.Errorf("record %d sequence %d does not follow %d",
				index, validated.sequence, previous)
		}
		if validated.sequence < oldest || validated.sequence > latest {
			return page.RunID, fmt.Errorf("record %d sequence %d is outside the retained bounds %d-%d",
				index, validated.sequence, oldest, latest)
		}
		previous = validated.sequence
	}

	if err := validatePageCursor(request, page, previous, oldest); err != nil {
		return page.RunID, err
	}
	return page.RunID, nil
}

// validatePageCursor checks the returned cursor, gap flag, and hasMore flag.
func validatePageCursor(request simulatorapi.HistoryRequest, page simulatorapi.ReceptionPage, last, oldest uint64) error {
	if page.NextCursor.RunID != page.RunID {
		return fmt.Errorf("next cursor run %q does not match the page run %q",
			page.NextCursor.RunID, page.RunID)
	}
	if page.NextCursor.StationID != page.StationID {
		return fmt.Errorf("next cursor station %q does not match the page station %q",
			page.NextCursor.StationID, page.StationID)
	}
	next, err := simulatorapi.ParseUint64(page.NextCursor.AfterSequence)
	if err != nil {
		return fmt.Errorf("next cursor afterSequence: %w", err)
	}

	var after uint64
	if request.Cursor != nil {
		if after, err = simulatorapi.ParseUint64(request.Cursor.AfterSequence); err != nil {
			return fmt.Errorf("cursor afterSequence: %w", err)
		}
	}
	if len(page.Records) == 0 {
		if next != after {
			return fmt.Errorf("an empty page advanced its cursor from %d to %d", after, next)
		}
		if page.HasMore {
			return errors.New("an empty page cannot have more records")
		}
	} else if next != last {
		return fmt.Errorf("next cursor %d does not name the last record %d", next, last)
	}

	// A gap means records after the caller's cursor were evicted. Without a
	// cursor the engine reports no gap and truncation is visible in bounds.
	wantGap := request.Cursor != nil && oldest > 0 && after < oldest-1
	if page.Gap != wantGap {
		return fmt.Errorf("gap %t contradicts cursor %d and oldest retained sequence %d",
			page.Gap, after, oldest)
	}
	return nil
}

// positiveSequence parses one canonical decimal identifier above zero.
func positiveSequence(text, name string) (uint64, error) {
	value, err := simulatorapi.ParseUint64(text)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if value == 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
}

// normalizeSelection sorts and deduplicates a requested selection so it can
// be compared with what a source returned.
func normalizeSelection(stationIDs []string) []string {
	sorted := append([]string(nil), stationIDs...)
	sort.Strings(sorted)
	unique := make([]string, 0, len(sorted))
	for _, id := range sorted {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	return unique
}

func sameSelection(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}
