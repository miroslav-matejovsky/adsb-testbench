package display

import (
	"context"
	"errors"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Operation names carried by every SourceError.
const (
	snapshotOperation = "read reception snapshot"
	historyOperation  = "read reception history"
	stationsOperation = "read stations"
)

// ObservationSource supplies raw received evidence to a display.
//
// It is deliberately narrow: a display builds tracks only from retained
// receptions, so a source exposes no simulator truth and no decoded state.
// The interface is defined here, by the consumer, and a simulator service
// satisfies it structurally without importing this package.
//
// Implementations must return detached values and must not retain the
// caller's request slices.
type ObservationSource interface {
	// ReceptionSnapshot returns all retained receptions of an explicit
	// station selection at one virtual instant.
	ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error)
	// ReceptionHistory returns one explicit page of one station's retained
	// receptions.
	ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error)
}

// StationSource supplies the station catalog with synthetic coverage to a
// display, so a browser can discover station choices through the display
// backend without contacting the simulator.
//
// It is separate from ObservationSource so a host can supply raw evidence
// without also supplying station discovery, and so neither interface grows
// beyond what one consumer needs. Station discovery carries no aircraft truth.
//
// Implementations must return detached values.
type StationSource interface {
	// Stations returns every active station with its estimated coverage at
	// one virtual instant.
	Stations(ctx context.Context) (simulatorapi.StationsSnapshot, error)
}

// sourceErrorFrom maps one underlying failure onto a shared category while
// keeping the original cause reachable through errors.Is and errors.As.
func sourceErrorFrom(operation string, err error) *SourceError {
	var apiError *simulatorapi.APIError
	switch {
	case errors.As(err, &apiError):
		failure := newSourceError(operation, apiError.Code, err, apiError.Message)
		if apiError.RunID != "" {
			failure = failure.withRunID(apiError.RunID)
		}
		return failure
	case errors.Is(err, context.Canceled):
		return newSourceError(operation, simulatorapi.CategoryUnavailable, err, "the request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return newSourceError(operation, simulatorapi.CategoryDeadlineExceeded, err,
			"the source reached its deadline")
	default:
		return newSourceError(operation, simulatorapi.CategoryUnavailable, err, err.Error())
	}
}

// invalidRequestError reports caller input rejected before any source access.
func invalidRequestError(operation string, err error) *SourceError {
	return newSourceError(operation, simulatorapi.CategoryInvalid, err, err.Error())
}

// invalidPayloadError reports a source response this display could not
// validate. runID is empty unless the envelope itself was valid.
func invalidPayloadError(operation, runID string, err error) *SourceError {
	failure := newSourceError(operation, simulatorapi.CategorySourceInvalid, err, err.Error())
	if runID != "" {
		failure = failure.withRunID(runID)
	}
	return failure
}
