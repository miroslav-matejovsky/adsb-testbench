package display

import (
	"context"
	"errors"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// InProcessSource adapts a provider in the same process, such as a simulator
// service, to the display's observation source.
//
// It validates requests before calling the provider and validates responses
// afterwards with the same semantic rules the HTTP source uses. Local calls
// are checked too: a Go provider can supply non-finite numbers, inconsistent
// metadata, or nil collections that no JSON document could have carried.
type InProcessSource struct {
	provider ObservationSource
}

// NewInProcessSource validates the provider and returns a source. It starts
// no goroutine and no background work.
func NewInProcessSource(provider ObservationSource) (*InProcessSource, error) {
	if provider == nil {
		return nil, errors.New("display in-process source: provider is nil")
	}
	return &InProcessSource{provider: provider}, nil
}

// ReceptionSnapshot validates the request, calls the provider once, and
// returns a semantically validated snapshot.
func (s *InProcessSource) ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	if err := validateSnapshotRequest(request); err != nil {
		return simulatorapi.ReceptionSnapshot{}, invalidRequestError(snapshotOperation, err)
	}
	raw, err := s.provider.ReceptionSnapshot(ctx, simulatorapi.ReceptionSnapshotRequest{
		StationIDs: append([]string{}, request.StationIDs...),
	})
	if err != nil {
		return simulatorapi.ReceptionSnapshot{}, sourceErrorFrom(snapshotOperation, err)
	}
	if _, runID, err := validateSnapshot(request, raw); err != nil {
		return simulatorapi.ReceptionSnapshot{}, invalidPayloadError(snapshotOperation, runID, err)
	}
	return raw, nil
}

// ReceptionHistory validates the request, calls the provider once, and
// returns a semantically validated page.
func (s *InProcessSource) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	if err := validateHistoryRequest(request); err != nil {
		return simulatorapi.ReceptionPage{}, invalidRequestError(historyOperation, err)
	}
	page, err := s.provider.ReceptionHistory(ctx, copyHistoryRequest(request))
	if err != nil {
		return simulatorapi.ReceptionPage{}, sourceErrorFrom(historyOperation, err)
	}
	if runID, err := validatePage(request, page); err != nil {
		return simulatorapi.ReceptionPage{}, invalidPayloadError(historyOperation, runID, err)
	}
	return page, nil
}

// copyHistoryRequest detaches the caller's cursor from the provider call.
func copyHistoryRequest(request simulatorapi.HistoryRequest) simulatorapi.HistoryRequest {
	copied := simulatorapi.HistoryRequest{StationID: request.StationID, Limit: request.Limit}
	if request.Cursor != nil {
		cursor := *request.Cursor
		copied.Cursor = &cursor
	}
	return copied
}
