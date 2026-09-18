package simulatorapi

import "strings"

// Category is a stable, transport-neutral failure class. It is the JSON
// "code" of an error envelope and is also an error value, so callers match a
// class with errors.Is regardless of how the failure was produced.
type Category string

// Failure classes. Every service and source error carries exactly one.
const (
	// CategoryInvalid marks malformed or semantically invalid input.
	CategoryInvalid Category = "invalid"
	// CategoryNotFound marks an unknown route or station.
	CategoryNotFound Category = "not_found"
	// CategoryMethodNotAllowed marks a wrong HTTP method.
	CategoryMethodNotAllowed Category = "method_not_allowed"
	// CategoryConflict marks a stale run or station revision.
	CategoryConflict Category = "conflict"
	// CategoryBodyTooLarge marks a request body above the configured bound.
	CategoryBodyTooLarge Category = "body_too_large"
	// CategoryUnsupportedMediaType marks a body that is not JSON.
	CategoryUnsupportedMediaType Category = "unsupported_media_type"
	// CategoryLimit marks an engine capacity or representability limit.
	CategoryLimit Category = "limit"
	// CategoryUnavailable marks a stopped driver or unavailable upstream.
	CategoryUnavailable Category = "unavailable"
	// CategoryResponseLimit marks a complete response that cannot fit the
	// configured response bound. Successful JSON is never truncated.
	CategoryResponseLimit Category = "response_limit"
	// CategoryDeadlineExceeded marks a reached service or source deadline.
	CategoryDeadlineExceeded Category = "deadline_exceeded"
	// CategorySourceInvalid marks an upstream contract or frame a display
	// source accepted syntactically but could not validate semantically.
	CategorySourceInvalid Category = "source_invalid"
	// CategoryInternal marks an unexpected server failure.
	CategoryInternal Category = "internal"
)

// Error implements error so errors.Is(err, CategoryInvalid) matches any
// APIError of that class.
func (c Category) Error() string { return string(c) }

// Valid reports whether c is one of the defined categories. Wire input from a
// remote service is checked with it before a category is adopted.
func (c Category) Valid() bool {
	switch c {
	case CategoryInvalid, CategoryNotFound, CategoryMethodNotAllowed, CategoryConflict,
		CategoryBodyTooLarge, CategoryUnsupportedMediaType, CategoryLimit, CategoryUnavailable,
		CategoryResponseLimit, CategoryDeadlineExceeded, CategorySourceInvalid, CategoryInternal:
		return true
	default:
		return false
	}
}

// APIError is the shared service failure. The exported fields are the
// complete wire representation; the local cause is never serialized because
// an HTTP client cannot rebuild a remote Go error object.
//
// Field is the JSON path or parameter the failure is attributed to, empty
// when the failure is not attributable to one input. RunID is the run the
// service was serving, empty when it is unknown.
type APIError struct {
	Code    Category `json:"code"`
	Message string   `json:"message"`
	Field   string   `json:"field"`
	RunID   string   `json:"runId"`

	cause error
}

// ErrorResponse is the complete JSON body of every failed request.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// NewError returns a categorized error with clamped external text.
func NewError(code Category, message string) *APIError {
	return &APIError{Code: code, Message: ClampMessage(message)}
}

// WrapError returns a categorized error that keeps cause for local callers.
// The cause is available through errors.Is and errors.As but never encoded.
func WrapError(code Category, cause error, message string) *APIError {
	return &APIError{Code: code, Message: ClampMessage(message), cause: cause}
}

// WithField returns a copy attributed to one input path.
func (e *APIError) WithField(field string) *APIError {
	copied := *e
	copied.Field = field
	return &copied
}

// WithRunID returns a copy naming the run the service was serving.
func (e *APIError) WithRunID(runID string) *APIError {
	copied := *e
	copied.RunID = runID
	return &copied
}

// Error reports the category, message, and any attributed input or run.
func (e *APIError) Error() string {
	var builder strings.Builder
	builder.WriteString(string(e.Code))
	builder.WriteString(": ")
	builder.WriteString(e.Message)
	if e.Field != "" {
		builder.WriteString(" (field ")
		builder.WriteString(e.Field)
		builder.WriteString(")")
	}
	if e.RunID != "" {
		builder.WriteString(" (run ")
		builder.WriteString(e.RunID)
		builder.WriteString(")")
	}
	return builder.String()
}

// Unwrap returns the local cause, which is nil for a recreated remote error.
func (e *APIError) Unwrap() error { return e.cause }

// Is matches a category so callers can classify without type assertions.
func (e *APIError) Is(target error) bool {
	category, ok := target.(Category)
	return ok && e.Code == category
}
