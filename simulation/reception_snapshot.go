package simulation

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// MaxSnapshotReceptions is the largest number of records one reception
// snapshot can contain: every selected station's full retention.
const MaxSnapshotReceptions = MaxStations * ReceptionHistoryLimit

// ReceptionSnapshot captures every retained reception of an explicit station
// selection at one committed virtual instant.
//
// It exists because independently fetched history pages do not share a read
// instant, and because the decoded observation snapshot omits raw evidence
// that has not yet produced a field, such as an unpaired CPR sample. It
// decodes nothing: consumers own their own decoding.
//
// Station identifiers are deduplicated and sorted. An empty selection
// deliberately selects nothing and returns empty collections. An identifier
// naming no active station returns ErrNotFound and no partial snapshot.
// Records are ordered by transmission sequence, then by station identifier,
// and carry the receiver settings and revision in effect when each frame was
// heard. The returned slices are owned by the caller.
//
// Reading draws no random value, moves no deadline, and advances no time.
func (e *Engine) ReceptionSnapshot(ctx context.Context, request ReceptionSnapshotRequest) (ReceptionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ReceptionSnapshot{}, err
	}
	stationIDs, err := normalizeStationSelection(request.StationIDs)
	if err != nil {
		return ReceptionSnapshot{}, err
	}

	capture, err := e.captureStations(ctx, stationIDs)
	if err != nil {
		return ReceptionSnapshot{}, err
	}

	total := 0
	for _, records := range capture.recordSets {
		total += len(records)
	}
	if total > MaxSnapshotReceptions {
		return ReceptionSnapshot{}, fmt.Errorf("%w: one snapshot may contain at most %d receptions",
			ErrLimit, MaxSnapshotReceptions)
	}

	records := make([]Reception, 0, total)
	for _, set := range capture.recordSets {
		if err := ctx.Err(); err != nil {
			return ReceptionSnapshot{}, err
		}
		records = append(records, set...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].TransmissionSequence != records[j].TransmissionSequence {
			return records[i].TransmissionSequence < records[j].TransmissionSequence
		}
		return records[i].StationID < records[j].StationID
	})

	if err := ctx.Err(); err != nil {
		return ReceptionSnapshot{}, err
	}
	return ReceptionSnapshot{
		RunID: capture.runID, Now: capture.now, StationIDs: stationIDs,
		Retention: capture.retention, Records: records,
	}, nil
}

// stationCapture is one atomic read of selected station histories.
type stationCapture struct {
	runID      string
	now        time.Time
	retention  []StationRetention
	recordSets [][]Reception
}

// captureStations copies run identity, virtual time, retention metadata, and
// all retained receptions of an already normalized selection under one lock,
// so nothing in the result comes from a second read.
func (e *Engine) captureStations(ctx context.Context, stationIDs []string) (stationCapture, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return stationCapture{}, err
	}
	capture := stationCapture{
		runID:      e.state.cfg.ID,
		now:        e.state.clock.now(),
		retention:  make([]StationRetention, 0, len(stationIDs)),
		recordSets: make([][]Reception, 0, len(stationIDs)),
	}
	for _, id := range stationIDs {
		index := e.state.stations.find(id)
		if index < 0 {
			return stationCapture{}, fmt.Errorf("%w: station %q", ErrNotFound, id)
		}
		station := &e.state.stations.active[index]
		oldest, latest := station.receptions.bounds()
		capture.retention = append(capture.retention, StationRetention{
			StationID: id, OldestSequence: oldest, LatestSequence: latest,
			Truncated: oldest > 1, Limit: ReceptionHistoryLimit,
		})
		capture.recordSets = append(capture.recordSets, station.receptions.records())
	}
	return capture, nil
}

// normalizeStationSelection validates, sorts, and deduplicates a selection
// and enforces the engine's station capacity. An empty selection is valid and
// deliberately selects nothing.
func normalizeStationSelection(ids []string) ([]string, error) {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	unique := uniqueStrings(sorted)
	if len(unique) > MaxStations {
		return nil, fmt.Errorf("%w: at most %d distinct stations may be selected", ErrInvalid, MaxStations)
	}
	for _, id := range unique {
		if err := validateStationID(id); err != nil {
			return nil, err
		}
	}
	return unique, nil
}
