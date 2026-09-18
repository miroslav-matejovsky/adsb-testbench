package display

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// RefreshResponse is the body of the refresh route.
//
// On success Error is null and Snapshot is fresh. On failure the HTTP status
// is the failure's own status, Error describes it, and Snapshot carries the
// explicitly stale data for the same selection, or unavailable state when
// there is none to show.
type RefreshResponse struct {
	Snapshot Snapshot               `json:"snapshot"`
	Error    *simulatorapi.APIError `json:"error"`
}

// Handler returns the display's relative browser-facing routes.
//
// Paths are relative to wherever the handler is mounted, so a host can serve
// it below a prefix with http.StripPrefix. The browser calls this backend,
// never the upstream simulator directly.
//
//	GET  /snapshot            the cached display state, without refreshing
//	POST /observations        refresh an explicit station selection
//	POST /receptions/history  one page of one station's retained receptions
//
// Successful responses use 200 and set Cache-Control: no-store. Failures use
// the shared error envelope and the status of their category: invalid browser
// input is 400, an unusable upstream contract or frame is 502, an unavailable
// source is 503, and a reached deadline is 504.
func (d *Display) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := d.serve(w, r); err != nil {
			d.config.ReportError(err)
		}
	})
}

// serve handles one request and returns only failures the browser cannot be
// told about, such as a failed response write. Tests use it directly.
func (d *Display) serve(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), d.config.RequestTimeout)
	defer cancel()

	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return d.writeError(w, displayError(simulatorapi.CategoryInvalid,
			errors.New("this route accepts no query parameters")))
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch path {
	case "/snapshot":
		return d.serveSnapshot(w, r)
	case "/observations":
		return d.serveRefresh(ctx, w, r)
	case "/receptions/history":
		return d.serveHistory(ctx, w, r)
	default:
		return d.writeError(w, simulatorapi.NewError(simulatorapi.CategoryNotFound,
			fmt.Sprintf("no route %q", r.URL.Path)))
	}
}

func (d *Display) serveSnapshot(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return d.writeError(w, methodNotAllowed(r, http.MethodGet))
	}
	if r.ContentLength > 0 {
		return d.writeError(w, displayError(simulatorapi.CategoryInvalid,
			errors.New("this route accepts no request body")))
	}
	return d.writeJSON(w, http.StatusOK, "", d.Snapshot())
}

func (d *Display) serveRefresh(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return d.writeError(w, methodNotAllowed(r, http.MethodPost))
	}
	value, apiErr := d.requestBody(r)
	if apiErr != nil {
		return d.writeError(w, apiErr)
	}
	stationIDs, err := parseSelectionRequest(value)
	if err != nil {
		return d.writeError(w, displayError(simulatorapi.CategoryInvalid, err))
	}

	snapshot, refreshErr := d.Refresh(ctx, stationIDs)
	if refreshErr == nil {
		return d.writeJSON(w, http.StatusOK, "", RefreshResponse{Snapshot: snapshot})
	}
	failure := sharedError(refreshErr)
	return d.writeJSON(w, statusOfCategory(failure.Code), "",
		RefreshResponse{Snapshot: snapshot, Error: failure})
}

func (d *Display) serveHistory(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return d.writeError(w, methodNotAllowed(r, http.MethodPost))
	}
	value, apiErr := d.requestBody(r)
	if apiErr != nil {
		return d.writeError(w, apiErr)
	}
	request, err := parseHistoryRequestBody(value)
	if err != nil {
		return d.writeError(w, displayError(simulatorapi.CategoryInvalid, err))
	}

	page, historyErr := d.ReceptionHistory(ctx, request)
	if historyErr != nil {
		return d.writeError(w, sharedError(historyErr))
	}
	return d.writeJSON(w, http.StatusOK, "", page)
}

// requestBody enforces the media type and byte bound, then strictly parses
// exactly one JSON value.
func (d *Display) requestBody(r *http.Request) (*simulatorapi.Value, *simulatorapi.APIError) {
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" {
		return nil, simulatorapi.NewError(simulatorapi.CategoryUnsupportedMediaType,
			fmt.Sprintf("content encoding %q is not supported", encoding))
	}
	if err := checkJSONMediaType(r.Header.Get("Content-Type")); err != nil {
		return nil, simulatorapi.NewError(simulatorapi.CategoryUnsupportedMediaType, err.Error())
	}
	if r.ContentLength > int64(d.config.MaxRequestBytes) {
		return nil, d.bodyTooLarge()
	}
	value, err := simulatorapi.ParseJSON(r.Body, d.config.MaxRequestBytes)
	if errors.Is(err, simulatorapi.ErrBodyTooLarge) {
		return nil, d.bodyTooLarge()
	}
	if err != nil {
		return nil, displayError(simulatorapi.CategoryInvalid, err)
	}
	return value, nil
}

func (d *Display) bodyTooLarge() *simulatorapi.APIError {
	return simulatorapi.NewError(simulatorapi.CategoryBodyTooLarge,
		fmt.Sprintf("request body exceeds %d bytes", d.config.MaxRequestBytes))
}

func displayError(category simulatorapi.Category, err error) *simulatorapi.APIError {
	return simulatorapi.WrapError(category, err, err.Error())
}

func methodNotAllowed(r *http.Request, allowed ...string) *simulatorapi.APIError {
	allow := strings.Join(allowed, ", ")
	return simulatorapi.NewError(simulatorapi.CategoryMethodNotAllowed,
		fmt.Sprintf("method %s is not allowed, allowed: %s", r.Method, allow)).WithField(allow)
}

// sharedError renders a display or source failure as a shared error.
func sharedError(err error) *simulatorapi.APIError {
	var failure *SourceError
	if errors.As(err, &failure) {
		return simulatorapi.WrapError(failure.Category, err, failure.Message).
			WithField(failure.Operation).WithRunID(failure.RunID)
	}
	var apiError *simulatorapi.APIError
	if errors.As(err, &apiError) {
		return apiError
	}
	return simulatorapi.WrapError(simulatorapi.CategoryInternal, err, "the request failed unexpectedly")
}

// writeJSON encodes into a bounded buffer before any status or header is
// written, so an oversized response becomes a bounded error instead of
// truncated successful JSON.
func (d *Display) writeJSON(w http.ResponseWriter, status int, allow string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return d.writeError(w, simulatorapi.WrapError(simulatorapi.CategoryInternal, err,
			"the response could not be encoded"))
	}
	if len(encoded) > d.config.MaxResponseBytes {
		return d.writeError(w, simulatorapi.NewError(simulatorapi.CategoryResponseLimit,
			fmt.Sprintf("the complete response needs %d bytes, above the configured %d",
				len(encoded), d.config.MaxResponseBytes)))
	}

	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	if allow != "" {
		header.Set("Allow", allow)
	}
	w.WriteHeader(status)
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("write display response: %w", err)
	}
	return nil
}

// writeError renders one shared error envelope with the status its category
// maps to.
func (d *Display) writeError(w http.ResponseWriter, apiError *simulatorapi.APIError) error {
	allow := ""
	if apiError.Code == simulatorapi.CategoryMethodNotAllowed {
		allow = apiError.Field
		apiError = apiError.WithField("")
	}
	encoded, marshalErr := json.Marshal(simulatorapi.ErrorResponse{Error: *apiError})
	if marshalErr != nil {
		return errors.Join(apiError, fmt.Errorf("encode display error response: %w", marshalErr))
	}

	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	if allow != "" {
		header.Set("Allow", allow)
	}
	w.WriteHeader(statusOfCategory(apiError.Code))
	if _, writeErr := w.Write(encoded); writeErr != nil {
		return errors.Join(apiError, fmt.Errorf("write display error response: %w", writeErr))
	}
	if apiError.Code == simulatorapi.CategoryInternal {
		return apiError
	}
	return nil
}
