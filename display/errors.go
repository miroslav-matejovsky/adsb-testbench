package display

import (
	"fmt"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// SourceError is one failed observation-source operation.
//
// Operation names what was attempted, so a caller can tell a snapshot read
// from a history read without inspecting the message. Category is the shared
// failure class and matches through errors.Is.
//
// RunID is the source run identifier, and is set only when it was validated.
// An adapter that accepted a well-formed envelope but rejected its payload
// still reports the run, so a display can invalidate obsolete state after a
// restart. Identity is never salvaged from malformed or oversized data, so an
// empty RunID means the run is genuinely unknown.
//
// The local cause is kept for errors.Is and errors.As. An error recreated
// from a remote envelope has no Go cause, because one cannot cross a wire.
type SourceError struct {
	Operation string
	Category  simulatorapi.Category
	Message   string
	RunID     string
	Status    int

	cause error
}

// newSourceError returns a categorized source failure keeping cause locally.
func newSourceError(operation string, category simulatorapi.Category, cause error, message string) *SourceError {
	return &SourceError{
		Operation: operation,
		Category:  category,
		Message:   simulatorapi.ClampMessage(message),
		cause:     cause,
	}
}

// withRunID returns a copy naming a run identifier that was validated.
func (e *SourceError) withRunID(runID string) *SourceError {
	copied := *e
	copied.RunID = runID
	return &copied
}

// withStatus returns a copy recording the remote HTTP status.
func (e *SourceError) withStatus(status int) *SourceError {
	copied := *e
	copied.Status = status
	return &copied
}

// Error reports the operation, category, message, and any known run.
func (e *SourceError) Error() string {
	var builder strings.Builder
	builder.WriteString(e.Operation)
	builder.WriteString(": ")
	builder.WriteString(string(e.Category))
	builder.WriteString(": ")
	builder.WriteString(e.Message)
	if e.Status != 0 {
		fmt.Fprintf(&builder, " (status %d)", e.Status)
	}
	if e.RunID != "" {
		builder.WriteString(" (run ")
		builder.WriteString(e.RunID)
		builder.WriteString(")")
	}
	return builder.String()
}

// Unwrap returns the local cause, which is nil for a recreated remote error.
func (e *SourceError) Unwrap() error { return e.cause }

// Is matches the shared category so callers classify without type assertions.
func (e *SourceError) Is(target error) bool {
	category, ok := target.(simulatorapi.Category)
	return ok && e.Category == category
}
