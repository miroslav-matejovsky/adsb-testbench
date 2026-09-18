package simulator

import (
	"context"
	"errors"
	"fmt"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// serviceError maps one runtime failure onto a shared category, keeping the
// original cause reachable through errors.Is and errors.As.
//
// Cancellation is returned unchanged, because a canceled caller has no
// promised result and context.Canceled is the answer it is waiting for.
// Anything the engine and driver do not classify stays internal: its message
// is generic, but its cause is retained for the host's error reporting.
func (a *API) serviceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	category := simulatorapi.CategoryInternal
	message := fmt.Sprintf("%s failed unexpectedly", operation)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category, message = simulatorapi.CategoryDeadlineExceeded, operation+" reached its deadline"
	case errors.Is(err, simulation.ErrInvalid):
		category, message = simulatorapi.CategoryInvalid, err.Error()
	case errors.Is(err, simulation.ErrNotFound):
		category, message = simulatorapi.CategoryNotFound, err.Error()
	case errors.Is(err, simulation.ErrConflict):
		category, message = simulatorapi.CategoryConflict, err.Error()
	case errors.Is(err, simulation.ErrLimit):
		category, message = simulatorapi.CategoryLimit, err.Error()
	case errors.Is(err, simdriver.ErrStopped), errors.Is(err, simdriver.ErrAlreadyStarted):
		category, message = simulatorapi.CategoryUnavailable, err.Error()
	}
	return simulatorapi.WrapError(category, err, message).WithRunID(a.runID)
}

// invalidRequest reports caller input this service rejected before reaching
// the runtime. field is the JSON path the failure is attributed to.
func (a *API) invalidRequest(field string, err error) error {
	return simulatorapi.WrapError(simulatorapi.CategoryInvalid, err, err.Error()).
		WithField(field).WithRunID(a.runID)
}

// runConflict reports a command written for a different engine lifetime. The
// current run identifier is returned so a client can resynchronize.
func (a *API) runConflict(commandRunID string) error {
	return simulatorapi.NewError(simulatorapi.CategoryConflict,
		fmt.Sprintf("command run %q is not the served run", commandRunID)).
		WithField("$.runId").WithRunID(a.runID)
}
