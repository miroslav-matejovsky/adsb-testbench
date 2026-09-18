package simulation

import (
	"context"
	"fmt"
	"sort"
)

// Observations builds a selected-station snapshot exclusively from retained
// receptions. Station identifiers are deduplicated and returned in lexical order.
func (e *Engine) Observations(ctx context.Context, request ObservationRequest) (ObservationSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ObservationSnapshot{}, err
	}
	if err := validateObservationExpiry(request.Expiry); err != nil {
		return ObservationSnapshot{}, err
	}
	stationIDs := append([]string(nil), request.StationIDs...)
	sort.Strings(stationIDs)
	stationIDs = uniqueStrings(stationIDs)
	if len(stationIDs) > MaxStations {
		return ObservationSnapshot{}, fmt.Errorf("%w: at most %d distinct stations may be selected", ErrInvalid, MaxStations)
	}

	e.mu.Lock()
	if err := ctx.Err(); err != nil {
		e.mu.Unlock()
		return ObservationSnapshot{}, err
	}
	now := e.state.clock.now()
	runID := e.state.cfg.ID
	retention := make([]StationRetention, 0, len(stationIDs))
	recordSets := make([][]Reception, 0, len(stationIDs))
	for _, id := range stationIDs {
		if err := validateStationID(id); err != nil {
			e.mu.Unlock()
			return ObservationSnapshot{}, err
		}
		index := e.state.stations.find(id)
		if index < 0 {
			e.mu.Unlock()
			return ObservationSnapshot{}, fmt.Errorf("%w: station %q", ErrNotFound, id)
		}
		station := &e.state.stations.active[index]
		oldest, latest := station.receptions.bounds()
		retention = append(retention, StationRetention{
			StationID: id, OldestSequence: oldest, LatestSequence: latest,
			Truncated: oldest > 1, Limit: ReceptionHistoryLimit,
		})
		recordSets = append(recordSets, station.receptions.records())
	}
	e.mu.Unlock()

	evidence, err := mergeObservationEvidence(ctx, recordSets)
	if err != nil {
		return ObservationSnapshot{}, err
	}
	aircraft, err := projectObservations(ctx, evidence, now, request.Expiry)
	if err != nil {
		return ObservationSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return ObservationSnapshot{}, err
	}
	return ObservationSnapshot{
		RunID: runID, Now: now, StationIDs: stationIDs, Retention: retention,
		Expiry: request.Expiry, Aircraft: aircraft,
	}, nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func mergeObservationEvidence(ctx context.Context, recordSets [][]Reception) ([]ObservationEvidence, error) {
	byTransmission := make(map[uint64]*ObservationEvidence)
	for _, records := range recordSets {
		for _, reception := range records {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			item := byTransmission[reception.TransmissionSequence]
			if item == nil {
				item = &ObservationEvidence{
					TransmissionSequence: reception.TransmissionSequence, ICAO: reception.ICAO,
					Kind: reception.Kind, Timestamp: reception.Timestamp, Frame: reception.Frame,
					Receptions: []Reception{},
				}
				byTransmission[reception.TransmissionSequence] = item
			} else if item.ICAO != reception.ICAO || item.Kind != reception.Kind ||
				!item.Timestamp.Equal(reception.Timestamp) || item.Frame != reception.Frame {
				return nil, fmt.Errorf("%w: selected receptions disagree on transmission %d", ErrInvalid, reception.TransmissionSequence)
			}
			item.Receptions = append(item.Receptions, reception)
		}
	}
	evidence := make([]ObservationEvidence, 0, len(byTransmission))
	for _, item := range byTransmission {
		sort.Slice(item.Receptions, func(i, j int) bool {
			return item.Receptions[i].StationID < item.Receptions[j].StationID
		})
		evidence = append(evidence, *item)
	}
	sort.Slice(evidence, func(i, j int) bool {
		return evidence[i].TransmissionSequence < evidence[j].TransmissionSequence
	})
	return evidence, nil
}
