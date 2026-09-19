package simulation

import (
	"context"
	"fmt"
)

// ReceptionHistory returns one coherent page of one active station's retained
// receptions. A cursor from another run is rejected with ErrConflict.
func (e *Engine) ReceptionHistory(ctx context.Context, request HistoryRequest) (ReceptionPage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return ReceptionPage{}, err
	}
	if request.Limit < 1 || request.Limit > MaxHistoryPageSize {
		return ReceptionPage{}, fmt.Errorf("%w: history limit must be within [1,%d]", ErrInvalid, MaxHistoryPageSize)
	}
	if err := validateStationID(request.StationID); err != nil {
		return ReceptionPage{}, err
	}
	after := uint64(0)
	if request.Cursor != nil {
		if request.Cursor.StationID != request.StationID {
			return ReceptionPage{}, fmt.Errorf("%w: cursor station %q does not match request station %q", ErrInvalid, request.Cursor.StationID, request.StationID)
		}
		if request.Cursor.RunID != e.state.cfg.ID {
			return ReceptionPage{}, fmt.Errorf("%w: cursor run %q does not match current run %q", ErrConflict, request.Cursor.RunID, e.state.cfg.ID)
		}
		after = request.Cursor.AfterSequence
	}
	index := e.state.stations.find(request.StationID)
	if index < 0 {
		return ReceptionPage{}, fmt.Errorf("%w: station %q", ErrNotFound, request.StationID)
	}

	station := &e.state.stations.active[index]
	if after > station.lastReceptionSequence {
		return ReceptionPage{}, fmt.Errorf("%w: cursor sequence %d is newer than station latest sequence %d", ErrInvalid, after, station.lastReceptionSequence)
	}
	records := station.receptions.records()
	oldest, latest := station.receptions.bounds()
	gap := request.Cursor != nil && oldest > 0 && after < oldest-1

	start := 0
	for start < len(records) && records[start].Sequence <= after {
		start++
	}
	end := min(start+request.Limit, len(records))
	pageRecords := append([]Reception(nil), records[start:end]...)
	next := after
	if len(pageRecords) > 0 {
		next = pageRecords[len(pageRecords)-1].Sequence
	}
	page := ReceptionPage{
		RunID:          e.state.cfg.ID,
		StationID:      request.StationID,
		Now:            e.state.clock.now(),
		Records:        pageRecords,
		OldestSequence: oldest,
		LatestSequence: latest,
		NextCursor: ReceptionCursor{
			RunID: e.state.cfg.ID, StationID: request.StationID, AfterSequence: next,
		},
		Gap: gap, HasMore: end < len(records), RetentionLimit: ReceptionHistoryLimit,
	}
	if err := ctx.Err(); err != nil {
		return ReceptionPage{}, err
	}
	return page, nil
}
