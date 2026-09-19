package display

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// displayMount is the fixture deployment prefix.
const displayMount = "/bench/a/display"

// newTestDisplayHandler returns a prefix-mounted handler and the errors the
// host callback received.
func newTestDisplayHandler(t *testing.T, source ObservationSource) (http.Handler, *Display, *[]error) {
	t.Helper()

	reported := &[]error{}
	config := displayFixtureConfig()
	config.ReportError = func(err error) { *reported = append(*reported, err) }
	clock := &fixedClock{now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	display := newDisplay(config, source, stationsOf(source), clock.Now)
	return http.StripPrefix(displayMount, display.Handler()), display, reported
}

func displayCall(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, displayMount+path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		if value == "" {
			request.Header.Del(key)
			continue
		}
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestDisplayHTTPServesTheDocumentedRoutes(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	request, page := fixturePage(t)
	handler, _, reported := newTestDisplayHandler(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
		history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
			return page, nil
		},
	})

	empty := displayCall(t, handler, http.MethodGet, "/snapshot", "", nil)
	require.Equal(t, http.StatusOK, empty.Code)
	require.Equal(t, "no-store", empty.Header().Get("Cache-Control"))
	var initial Snapshot
	require.NoError(t, json.Unmarshal(empty.Body.Bytes(), &initial))
	require.Equal(t, StatusUnavailable, initial.Status)
	require.Nil(t, initial.Observations)

	refreshed := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusOK, refreshed.Code)
	var refresh RefreshResponse
	require.NoError(t, json.Unmarshal(refreshed.Body.Bytes(), &refresh))
	require.Nil(t, refresh.Error)
	require.Equal(t, StatusFresh, refresh.Snapshot.Status)
	require.Len(t, refresh.Snapshot.Observations.Aircraft, 1)

	cached := displayCall(t, handler, http.MethodGet, "/snapshot", "", nil)
	require.Equal(t, http.StatusOK, cached.Code)
	var current Snapshot
	require.NoError(t, json.Unmarshal(cached.Body.Bytes(), &current))
	require.Equal(t, StatusFresh, current.Status)

	history := displayCall(t, handler, http.MethodPost, "/receptions/history",
		`{"stationId":"`+request.StationID+`","cursor":null,"limit":2}`, nil)
	require.Equal(t, http.StatusOK, history.Code)
	var got simulatorapi.ReceptionPage
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &got))
	require.Equal(t, page, got)

	require.Empty(t, *reported)
}

func TestDisplayHTTPReturnsStaleDataWithItsErrorStatus(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	fail := false
	handler, _, _ := newTestDisplayHandler(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if fail {
				return simulatorapi.ReceptionSnapshot{},
					newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable, nil, "the source is down")
			}
			return raw, nil
		},
	})

	require.Equal(t, http.StatusOK,
		displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil).Code)

	fail = true
	recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	var refresh RefreshResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refresh))
	require.NotNil(t, refresh.Error)
	require.Equal(t, simulatorapi.CategoryUnavailable, refresh.Error.Code)
	require.Equal(t, StatusStale, refresh.Snapshot.Status)
	require.NotNil(t, refresh.Snapshot.Observations)
}

func TestDisplayHTTPMapsSourceFailuresToStatuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		category simulatorapi.Category
		status   int
	}{
		{name: "upstream contract", category: simulatorapi.CategorySourceInvalid, status: http.StatusBadGateway},
		{name: "source unavailable", category: simulatorapi.CategoryUnavailable, status: http.StatusServiceUnavailable},
		{name: "deadline", category: simulatorapi.CategoryDeadlineExceeded, status: http.StatusGatewayTimeout},
		{name: "not found", category: simulatorapi.CategoryNotFound, status: http.StatusNotFound},
		{name: "conflict", category: simulatorapi.CategoryConflict, status: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, _, _ := newTestDisplayHandler(t, funcSource{
				snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
					return simulatorapi.ReceptionSnapshot{},
						newSourceError(snapshotOperation, tc.category, nil, "upstream said no")
				},
			})
			recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
			require.Equal(t, tc.status, recorder.Code)

			var refresh RefreshResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refresh))
			require.Equal(t, tc.category, refresh.Error.Code)
			require.Equal(t, StatusUnavailable, refresh.Snapshot.Status)
		})
	}
}

func TestDisplayHTTPRejectsInvalidBrowserInputBeforeSourceAccess(t *testing.T) {
	t.Parallel()

	source := funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			t.Fatal("invalid input must not reach the source")
			return simulatorapi.ReceptionSnapshot{}, nil
		},
		history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
			t.Fatal("invalid input must not reach the source")
			return simulatorapi.ReceptionPage{}, nil
		},
	}
	handler, _, _ := newTestDisplayHandler(t, source)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   simulatorapi.Category
	}{
		{name: "unknown route", method: http.MethodGet, path: "/nope", body: "",
			status: http.StatusNotFound, code: simulatorapi.CategoryNotFound},
		{name: "wrong method on snapshot", method: http.MethodPost, path: "/snapshot", body: `{}`,
			status: http.StatusMethodNotAllowed, code: simulatorapi.CategoryMethodNotAllowed},
		{name: "wrong method on refresh", method: http.MethodGet, path: "/observations", body: "",
			status: http.StatusMethodNotAllowed, code: simulatorapi.CategoryMethodNotAllowed},
		{name: "missing selection", method: http.MethodPost, path: "/observations", body: `{}`,
			status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "null selection", method: http.MethodPost, path: "/observations", body: `{"stationIds":null}`,
			status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "unknown field", method: http.MethodPost, path: "/observations",
			body: `{"stationIds":[],"extra":1}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "duplicate field", method: http.MethodPost, path: "/observations",
			body: `{"stationIds":[],"stationIds":[]}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "trailing json", method: http.MethodPost, path: "/observations",
			body: `{"stationIds":[]}{}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "invalid station id", method: http.MethodPost, path: "/observations",
			body: `{"stationIds":["bad id"]}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "missing cursor key", method: http.MethodPost, path: "/receptions/history",
			body: `{"stationId":"alpha","limit":1}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
		{name: "zero limit", method: http.MethodPost, path: "/receptions/history",
			body:   `{"stationId":"alpha","cursor":null,"limit":0}`,
			status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := displayCall(t, handler, tc.method, tc.path, tc.body, nil)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())

			var response simulatorapi.ErrorResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, tc.code, response.Error.Code)
		})
	}
}

func TestDisplayHTTPRejectsQueriesMediaTypesAndOversizedBodies(t *testing.T) {
	t.Parallel()

	handler, _, _ := newTestDisplayHandler(t, funcSource{})

	withQuery := httptest.NewRequest(http.MethodGet, displayMount+"/snapshot?a=1", strings.NewReader(""))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, withQuery)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	for _, headers := range []map[string]string{
		{"Content-Type": ""},
		{"Content-Type": "text/plain"},
		{"Content-Encoding": "gzip"},
	} {
		got := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":[]}`, headers)
		require.Equal(t, http.StatusUnsupportedMediaType, got.Code)
	}

	config := displayFixtureConfig()
	config.MaxRequestBytes = len(`{"stationIds":[]}`)
	small := newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return fixtureSnapshot(fixtureStart, []string{}, []simulatorapi.Reception{}), nil
		},
	}, funcSource{}, time.Now)
	smallHandler := http.StripPrefix(displayMount, small.Handler())

	require.Equal(t, http.StatusOK,
		displayCall(t, smallHandler, http.MethodPost, "/observations", `{"stationIds":[]}`, nil).Code)
	require.Equal(t, http.StatusRequestEntityTooLarge,
		displayCall(t, smallHandler, http.MethodPost, "/observations", `{"stationIds":[ ]}`, nil).Code)
}

func TestDisplayHTTPEnforcesTheResponseByteBound(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	config := displayFixtureConfig()
	config.MaxResponseBytes = simulatorapi.MinResponseBytes
	display := newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	}, funcSource{}, time.Now)
	handler := http.StripPrefix(displayMount, display.Handler())

	recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.LessOrEqual(t, recorder.Body.Len(), simulatorapi.MinResponseBytes)

	var response simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, simulatorapi.CategoryResponseLimit, response.Error.Code)
}

func TestDisplayHTTPReportsWriterFailuresToTheHost(t *testing.T) {
	t.Parallel()

	handler, _, reported := newTestDisplayHandler(t, funcSource{})
	request := httptest.NewRequest(http.MethodGet, displayMount+"/snapshot", strings.NewReader(""))
	handler.ServeHTTP(brokenWriter{header: http.Header{}}, request)
	require.Len(t, *reported, 1)
	require.ErrorContains(t, (*reported)[0], "write display response")
}

// brokenWriter accepts headers but never writes a body successfully.
type brokenWriter struct{ header http.Header }

func (w brokenWriter) Header() http.Header     { return w.header }
func (w brokenWriter) WriteHeader(int)         {}
func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }
