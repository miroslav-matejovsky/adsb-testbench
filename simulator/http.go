package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Handler returns the simulator's relative HTTP routes.
//
// Paths are relative to wherever the handler is mounted, so a host can serve
// it below a prefix with http.StripPrefix and every route keeps working. The
// handler owns no listener, no logger, and no background work.
//
// Read queries whose request has a structured shape use POST so the request
// data keeps its required-field rules; they are still read-only operations.
//
//	GET    /metadata               effective settings, limits, virtual time
//	GET    /truth                  simulated state and generated history
//	PUT    /aircraft/count         absolute aircraft count assignment
//	PUT    /time/speed             absolute speed assignment, zero pauses
//	GET    /stations               stations with coverage at one read instant
//	POST   /stations               create a station from complete settings
//	PUT    /stations/{id}          replace every setting of one station
//	DELETE /stations/{id}          remove one station
//	POST   /observations           decoded received snapshot
//	POST   /observations/receptions raw reception snapshot
//	POST   /receptions/history     one page of one station's receptions
//
// Successful responses use 200 except station creation, which uses 201. Every
// response sets Cache-Control: no-store. Failures use the shared error
// envelope and the documented status for their category.
func (a *API) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.serve(w, r); err != nil {
			a.config.ReportError(err)
		}
	})
}

// serve handles one request and returns only failures the client cannot be
// told about, such as a failed response write. Tests use it directly.
func (a *API) serve(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), a.config.RequestTimeout)
	defer cancel()

	outcome, err := a.route(ctx, r)
	if err != nil {
		return a.writeError(w, err)
	}
	return a.writeJSON(w, outcome.status, outcome.allow, outcome.body)
}

// result is one successful route outcome.
type result struct {
	status int
	allow  string
	body   any
}

// route matches the path, checks the method, parses the body, and calls the
// service. It never touches engine state directly.
func (a *API) route(ctx context.Context, r *http.Request) (result, error) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return result{}, a.invalidRequest("query", errors.New("this route accepts no query parameters"))
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	if stationID, ok := strings.CutPrefix(path, "/stations/"); ok {
		return a.routeStation(ctx, r, stationID)
	}

	switch path {
	case "/metadata":
		return a.readRoute(ctx, r, func(ctx context.Context) (any, error) { return a.Metadata(ctx) })
	case "/truth":
		return a.readRoute(ctx, r, func(ctx context.Context) (any, error) { return a.Truth(ctx) })
	case "/stations":
		return a.routeStations(ctx, r)
	case "/aircraft/count":
		return commandRoute(a, ctx, r, http.MethodPut, parseCountCommand, a.SetCount)
	case "/time/speed":
		return commandRoute(a, ctx, r, http.MethodPut, parseSpeedCommand, a.SetSpeed)
	case "/observations":
		return commandRoute(a, ctx, r, http.MethodPost, parseObservationRequest, a.Observations)
	case "/observations/receptions":
		return commandRoute(a, ctx, r, http.MethodPost, parseReceptionSnapshotRequest, a.ReceptionSnapshot)
	case "/receptions/history":
		return commandRoute(a, ctx, r, http.MethodPost, parseHistoryRequest, a.ReceptionHistory)
	default:
		return result{}, simulatorapi.NewError(simulatorapi.CategoryNotFound,
			fmt.Sprintf("no route %q", r.URL.Path)).WithRunID(a.runID)
	}
}

// routeStations serves the collection routes.
func (a *API) routeStations(ctx context.Context, r *http.Request) (result, error) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return a.readRoute(ctx, r, func(ctx context.Context) (any, error) { return a.Stations(ctx) })
	case http.MethodPost:
		body, err := a.requestBody(r)
		if err != nil {
			return result{}, err
		}
		command, err := parseAddStationCommand(body)
		if err != nil {
			return result{}, a.invalidRequest(body.Path(), err)
		}
		ack, err := a.AddStation(ctx, command)
		if err != nil {
			return result{}, err
		}
		return result{status: http.StatusCreated, body: ack}, nil
	default:
		return result{}, a.methodNotAllowed(r, http.MethodGet, http.MethodPost)
	}
}

// routeStation serves the routes addressing one station by identifier.
func (a *API) routeStation(ctx context.Context, r *http.Request, stationID string) (result, error) {
	if stationID == "" || strings.Contains(stationID, "/") {
		return result{}, simulatorapi.NewError(simulatorapi.CategoryNotFound,
			fmt.Sprintf("no route %q", r.URL.Path)).WithRunID(a.runID)
	}
	switch r.Method {
	case http.MethodPut:
		body, err := a.requestBody(r)
		if err != nil {
			return result{}, err
		}
		command, err := parseUpdateStationCommand(body)
		if err != nil {
			return result{}, a.invalidRequest(body.Path(), err)
		}
		ack, err := a.UpdateStation(ctx, stationID, command)
		if err != nil {
			return result{}, err
		}
		return result{status: http.StatusOK, body: ack}, nil
	case http.MethodDelete:
		body, err := a.requestBody(r)
		if err != nil {
			return result{}, err
		}
		command, err := parseRemoveStationCommand(body)
		if err != nil {
			return result{}, a.invalidRequest(body.Path(), err)
		}
		ack, err := a.RemoveStation(ctx, stationID, command)
		if err != nil {
			return result{}, err
		}
		return result{status: http.StatusOK, body: ack}, nil
	default:
		return result{}, a.methodNotAllowed(r, http.MethodPut, http.MethodDelete)
	}
}

// readRoute serves one GET route, which carries no request body.
func (a *API) readRoute(ctx context.Context, r *http.Request, read func(context.Context) (any, error)) (result, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return result{}, a.methodNotAllowed(r, http.MethodGet)
	}
	if r.ContentLength > 0 {
		return result{}, a.invalidRequest("body", errors.New("this route accepts no request body"))
	}
	body, err := read(ctx)
	if err != nil {
		return result{}, err
	}
	return result{status: http.StatusOK, body: body}, nil
}

// commandRoute parses one JSON request body and calls one service operation.
func commandRoute[Request any, Response any](
	a *API, ctx context.Context, r *http.Request, method string,
	parse func(*simulatorapi.Value) (Request, error),
	call func(context.Context, Request) (Response, error),
) (result, error) {
	if r.Method != method {
		return result{}, a.methodNotAllowed(r, method)
	}
	value, err := a.requestBody(r)
	if err != nil {
		return result{}, err
	}
	request, err := parse(value)
	if err != nil {
		return result{}, a.invalidRequest(value.Path(), err)
	}
	response, err := call(ctx, request)
	if err != nil {
		return result{}, err
	}
	return result{status: http.StatusOK, body: response}, nil
}

// requestBody enforces the media type and byte bound, then strictly parses
// exactly one JSON value.
func (a *API) requestBody(r *http.Request) (*simulatorapi.Value, error) {
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" {
		return nil, simulatorapi.NewError(simulatorapi.CategoryUnsupportedMediaType,
			fmt.Sprintf("content encoding %q is not supported", encoding)).WithRunID(a.runID)
	}
	if err := a.checkJSONContentType(r.Header.Get("Content-Type")); err != nil {
		return nil, err
	}
	if r.ContentLength > int64(a.config.MaxRequestBytes) {
		return nil, a.bodyTooLarge()
	}
	value, err := simulatorapi.ParseJSON(r.Body, a.config.MaxRequestBytes)
	if errors.Is(err, simulatorapi.ErrBodyTooLarge) {
		return nil, a.bodyTooLarge()
	}
	if err != nil {
		return nil, a.invalidRequest("body", err)
	}
	return value, nil
}

// checkJSONContentType accepts application/json with an optional UTF-8
// charset parameter and nothing else.
func (a *API) checkJSONContentType(header string) error {
	unsupported := func(reason string) error {
		return simulatorapi.NewError(simulatorapi.CategoryUnsupportedMediaType, reason).WithRunID(a.runID)
	}
	if header == "" {
		return unsupported("this route requires a Content-Type of application/json")
	}
	mediaType, parameters, err := mime.ParseMediaType(header)
	if err != nil {
		return unsupported(fmt.Sprintf("content type %q is malformed", header))
	}
	if mediaType != "application/json" {
		return unsupported(fmt.Sprintf("content type %q is not application/json", mediaType))
	}
	if charset, ok := parameters["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
		return unsupported(fmt.Sprintf("charset %q is not utf-8", charset))
	}
	return nil
}

func (a *API) bodyTooLarge() error {
	return simulatorapi.NewError(simulatorapi.CategoryBodyTooLarge,
		fmt.Sprintf("request body exceeds %d bytes", a.config.MaxRequestBytes)).WithRunID(a.runID)
}

func (a *API) methodNotAllowed(r *http.Request, allowed ...string) error {
	allow := strings.Join(allowed, ", ")
	return simulatorapi.NewError(simulatorapi.CategoryMethodNotAllowed,
		fmt.Sprintf("method %s is not allowed, allowed: %s", r.Method, allow)).
		WithField(allow).WithRunID(a.runID)
}

// writeJSON encodes into a bounded buffer before any status or header is
// written, so an oversized response becomes a bounded error instead of
// truncated successful JSON.
func (a *API) writeJSON(w http.ResponseWriter, status int, allow string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return a.writeError(w, simulatorapi.WrapError(simulatorapi.CategoryInternal, err,
			"the response could not be encoded").WithRunID(a.runID))
	}
	if len(encoded) > a.config.MaxResponseBytes {
		return a.writeError(w, simulatorapi.NewError(simulatorapi.CategoryResponseLimit,
			fmt.Sprintf("the complete response needs %d bytes, above the configured %d",
				len(encoded), a.config.MaxResponseBytes)).WithRunID(a.runID))
	}

	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	if allow != "" {
		header.Set("Allow", allow)
	}
	w.WriteHeader(status)
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("write simulator response: %w", err)
	}
	return nil
}

// writeError renders one shared error envelope with the status its category
// maps to. The envelope always fits the minimum response budget.
func (a *API) writeError(w http.ResponseWriter, err error) error {
	apiError := a.asAPIError(err)
	allow := ""
	if apiError.Code == simulatorapi.CategoryMethodNotAllowed {
		allow = apiError.Field
		apiError = apiError.WithField("")
	}

	encoded, marshalErr := json.Marshal(simulatorapi.ErrorResponse{Error: *apiError})
	if marshalErr != nil {
		return errors.Join(err, fmt.Errorf("encode simulator error response: %w", marshalErr))
	}

	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	if allow != "" {
		header.Set("Allow", allow)
	}
	w.WriteHeader(statusOf(apiError.Code))
	if _, writeErr := w.Write(encoded); writeErr != nil {
		return errors.Join(err, fmt.Errorf("write simulator error response: %w", writeErr))
	}
	if apiError.Code == simulatorapi.CategoryInternal {
		return err
	}
	return nil
}

// asAPIError recovers the shared error of a failure, categorizing anything
// unclassified as internal while keeping its cause for the host callback.
func (a *API) asAPIError(err error) *simulatorapi.APIError {
	var apiError *simulatorapi.APIError
	if errors.As(err, &apiError) {
		return apiError
	}
	if errors.Is(err, context.Canceled) {
		return simulatorapi.WrapError(simulatorapi.CategoryUnavailable, err,
			"the request was canceled").WithRunID(a.runID)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return simulatorapi.WrapError(simulatorapi.CategoryDeadlineExceeded, err,
			"the request reached its deadline").WithRunID(a.runID)
	}
	return simulatorapi.WrapError(simulatorapi.CategoryInternal, err,
		"the request failed unexpectedly").WithRunID(a.runID)
}

// statusOf maps one shared category onto its HTTP status.
func statusOf(category simulatorapi.Category) int {
	switch category {
	case simulatorapi.CategoryInvalid:
		return http.StatusBadRequest
	case simulatorapi.CategoryNotFound:
		return http.StatusNotFound
	case simulatorapi.CategoryMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case simulatorapi.CategoryConflict:
		return http.StatusConflict
	case simulatorapi.CategoryBodyTooLarge:
		return http.StatusRequestEntityTooLarge
	case simulatorapi.CategoryUnsupportedMediaType:
		return http.StatusUnsupportedMediaType
	case simulatorapi.CategoryLimit:
		return http.StatusUnprocessableEntity
	case simulatorapi.CategoryUnavailable, simulatorapi.CategoryResponseLimit:
		return http.StatusServiceUnavailable
	case simulatorapi.CategoryDeadlineExceeded:
		return http.StatusGatewayTimeout
	case simulatorapi.CategorySourceInvalid:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
