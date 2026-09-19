package display

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Relative simulator routes this source calls. They are appended to the
// configured base URL, so a nested mount prefix is preserved.
const (
	snapshotPath = "observations/receptions"
	historyPath  = "receptions/history"
	stationsPath = "stations"
)

// errRedirect reports that the upstream tried to move the source elsewhere.
var errRedirect = errors.New("the source refused to follow a redirect")

// HTTPSource reads raw received evidence from a simulator over HTTP.
//
// It owns a copy of the host's client configuration, so setting its own
// redirect policy cannot change the host's client. The host keeps ownership
// of the transport: this source never closes idle connections. Redirects are
// refused rather than followed, because following one would silently change
// which simulator the display is showing.
//
// One call is bounded in every direction: the encoded request obeys
// MaxRequestBytes, the response is read to at most MaxResponseBytes whatever
// its declared length, and the whole call including body reading runs under
// the earlier of the caller's deadline and the configured timeout. Nothing is
// retried automatically.
type HTTPSource struct {
	base             string
	timeout          time.Duration
	maxRequestBytes  int
	maxResponseBytes int
	client           http.Client
}

// NewHTTPSource validates the configuration and returns a source. It starts
// no goroutine, no polling loop, and no background worker.
func NewHTTPSource(config HTTPSourceConfig) (*HTTPSource, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create display HTTP source: %w", err)
	}
	base, err := urlpath.ParseSourceBase(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("create display HTTP source: %w", err)
	}
	client := *config.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirect }
	return &HTTPSource{
		base: base, timeout: config.Timeout,
		maxRequestBytes: config.MaxRequestBytes, maxResponseBytes: config.MaxResponseBytes,
		client: client,
	}, nil
}

// ReceptionSnapshot fetches and validates one raw reception snapshot.
func (s *HTTPSource) ReceptionSnapshot(ctx context.Context, request simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	if err := validateSnapshotRequest(request); err != nil {
		return simulatorapi.ReceptionSnapshot{}, invalidRequestError(snapshotOperation, err)
	}
	body := map[string]any{"stationIds": request.StationIDs}
	value, failure := s.post(ctx, snapshotOperation, snapshotPath, body)
	if failure != nil {
		return simulatorapi.ReceptionSnapshot{}, failure
	}
	raw, err := parseReceptionSnapshot(value)
	if err != nil {
		return simulatorapi.ReceptionSnapshot{}, invalidPayloadError(snapshotOperation, "", err)
	}
	if _, runID, err := validateSnapshot(request, raw); err != nil {
		return simulatorapi.ReceptionSnapshot{}, invalidPayloadError(snapshotOperation, runID, err)
	}
	return raw, nil
}

// ReceptionHistory fetches and validates one page of one station's history.
func (s *HTTPSource) ReceptionHistory(ctx context.Context, request simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	if err := validateHistoryRequest(request); err != nil {
		return simulatorapi.ReceptionPage{}, invalidRequestError(historyOperation, err)
	}
	body := map[string]any{
		"stationId": request.StationID, "limit": request.Limit, "cursor": nil,
	}
	if request.Cursor != nil {
		body["cursor"] = map[string]any{
			"runId": request.Cursor.RunID, "stationId": request.Cursor.StationID,
			"afterSequence": request.Cursor.AfterSequence,
		}
	}
	value, failure := s.post(ctx, historyOperation, historyPath, body)
	if failure != nil {
		return simulatorapi.ReceptionPage{}, failure
	}
	page, err := parseReceptionPage(value)
	if err != nil {
		return simulatorapi.ReceptionPage{}, invalidPayloadError(historyOperation, "", err)
	}
	if runID, err := validatePage(request, page); err != nil {
		return simulatorapi.ReceptionPage{}, invalidPayloadError(historyOperation, runID, err)
	}
	return page, nil
}

// Stations fetches and validates the upstream station catalog with a bodiless
// GET request.
func (s *HTTPSource) Stations(ctx context.Context) (simulatorapi.StationsSnapshot, error) {
	value, failure := s.get(ctx, stationsOperation, stationsPath)
	if failure != nil {
		return simulatorapi.StationsSnapshot{}, failure
	}
	stations, err := parseStationsSnapshot(value)
	if err != nil {
		return simulatorapi.StationsSnapshot{}, invalidPayloadError(stationsOperation, "", err)
	}
	if runID, err := validateStations(stations); err != nil {
		return simulatorapi.StationsSnapshot{}, invalidPayloadError(stationsOperation, runID, err)
	}
	return stations, nil
}

// post performs one bounded JSON POST call and returns its strictly parsed
// body.
func (s *HTTPSource) post(ctx context.Context, operation, path string, body any) (*simulatorapi.Value, *SourceError) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, newSourceError(operation, simulatorapi.CategoryInternal, err,
			"the request could not be encoded")
	}
	if len(encoded) > s.maxRequestBytes {
		return nil, newSourceError(operation, simulatorapi.CategoryBodyTooLarge, nil,
			fmt.Sprintf("the encoded request needs %d bytes, above the configured %d",
				len(encoded), s.maxRequestBytes))
	}
	return s.do(ctx, operation, http.MethodPost, path, encoded)
}

// get performs one bounded bodiless GET call and returns its strictly parsed
// body.
func (s *HTTPSource) get(ctx context.Context, operation, path string) (*simulatorapi.Value, *SourceError) {
	return s.do(ctx, operation, http.MethodGet, path, nil)
}

// do performs one bounded call. A nil encoded body sends no body at all.
func (s *HTTPSource) do(ctx context.Context, operation, method, path string, encoded []byte) (*simulatorapi.Value, *SourceError) {
	// The derived deadline covers headers and body reading, and is canceled
	// after every call. An earlier caller deadline still wins.
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var body io.Reader
	if encoded != nil {
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.base+path, body)
	if err != nil {
		return nil, newSourceError(operation, simulatorapi.CategoryInternal, err,
			"the request could not be built")
	}
	if encoded != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		if errors.Is(err, errRedirect) {
			return nil, newSourceError(operation, simulatorapi.CategorySourceInvalid, err,
				"the source responded with a redirect")
		}
		return nil, sourceErrorFrom(operation, err)
	}
	// The body is closed on every path: success, status failure, decode
	// failure, and overflow. A close failure is kept rather than discarded.
	payload, failure := s.readBody(operation, response)
	closeErr := response.Body.Close()
	if failure != nil {
		if closeErr != nil {
			failure.cause = errors.Join(failure.cause, closeErr)
		}
		return nil, failure
	}
	if closeErr != nil {
		return nil, newSourceError(operation, simulatorapi.CategoryUnavailable, closeErr,
			fmt.Sprintf("the response body could not be closed: %v", closeErr))
	}
	if response.StatusCode != http.StatusOK {
		return nil, s.remoteError(operation, response.StatusCode, payload)
	}
	value, parseErr := simulatorapi.ParseJSONBytes(payload)
	if parseErr != nil {
		return nil, invalidPayloadError(operation, "", parseErr)
	}
	return value, nil
}

// readBody checks the media type and reads at most MaxResponseBytes, whatever
// Content-Length claims.
func (s *HTTPSource) readBody(operation string, response *http.Response) ([]byte, *SourceError) {
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		return nil, newSourceError(operation, simulatorapi.CategorySourceInvalid, nil,
			fmt.Sprintf("content encoding %q is not supported", encoding))
	}
	if err := checkJSONMediaType(response.Header.Get("Content-Type")); err != nil {
		return nil, newSourceError(operation, simulatorapi.CategorySourceInvalid, err, err.Error()).
			withStatus(response.StatusCode)
	}
	if response.ContentLength > int64(s.maxResponseBytes) {
		return nil, s.responseTooLarge(operation, response.StatusCode)
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, int64(s.maxResponseBytes)+1))
	if err != nil {
		return nil, newSourceError(operation, simulatorapi.CategoryUnavailable, err,
			fmt.Sprintf("the response body could not be read: %v", err)).
			withStatus(response.StatusCode)
	}
	if len(payload) > s.maxResponseBytes {
		return nil, s.responseTooLarge(operation, response.StatusCode)
	}
	return payload, nil
}

func (s *HTTPSource) responseTooLarge(operation string, status int) *SourceError {
	return newSourceError(operation, simulatorapi.CategoryResponseLimit, nil,
		fmt.Sprintf("the response exceeds the configured %d bytes", s.maxResponseBytes)).
		withStatus(status)
}

// remoteError recreates a shared failure from a valid error envelope. An
// envelope that is malformed, or whose category contradicts the status, is a
// source protocol failure instead.
func (s *HTTPSource) remoteError(operation string, status int, payload []byte) *SourceError {
	value, err := simulatorapi.ParseJSONBytes(payload)
	if err != nil {
		return newSourceError(operation, simulatorapi.CategorySourceInvalid, err,
			fmt.Sprintf("status %d carried an unreadable error envelope", status)).withStatus(status)
	}
	failure, err := parseErrorEnvelope(value)
	if err != nil {
		return newSourceError(operation, simulatorapi.CategorySourceInvalid, err,
			fmt.Sprintf("status %d carried an invalid error envelope", status)).withStatus(status)
	}
	if statusOfCategory(failure.Code) != status {
		return newSourceError(operation, simulatorapi.CategorySourceInvalid, nil,
			fmt.Sprintf("status %d contradicts error code %q", status, failure.Code)).withStatus(status)
	}
	remote := newSourceError(operation, failure.Code, nil, failure.Message).withStatus(status)
	if failure.RunID != "" {
		remote = remote.withRunID(failure.RunID)
	}
	return remote
}

// checkJSONMediaType accepts application/json with an optional UTF-8 charset.
func checkJSONMediaType(header string) error {
	if header == "" {
		return errors.New("the source response carried no content type")
	}
	mediaType, parameters, err := mime.ParseMediaType(header)
	if err != nil {
		return fmt.Errorf("the source content type %q is malformed", header)
	}
	if mediaType != "application/json" {
		return fmt.Errorf("the source content type %q is not application/json", mediaType)
	}
	if charset, ok := parameters["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
		return fmt.Errorf("the source charset %q is not utf-8", charset)
	}
	return nil
}

// unknownCategory reports a wire code this display does not implement.
func unknownCategory(code string) error {
	return fmt.Errorf("error code %q is not a known category", code)
}

// statusOfCategory maps one shared category onto the status a simulator uses
// for it, so a status and code that disagree are rejected.
func statusOfCategory(category simulatorapi.Category) int {
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
