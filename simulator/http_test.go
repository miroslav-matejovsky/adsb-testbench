package simulator

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

// mountPrefix is the fixture deployment prefix. Every route must keep
// working below it through http.StripPrefix.
const mountPrefix = "/bench/a/simulator"

// newTestHandler returns a prefix-mounted handler, its service, its clock,
// and the errors the host callback received.
func newTestHandler(t *testing.T) (http.Handler, *API, *runtimeClock, *[]error) {
	t.Helper()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)

	reported := &[]error{}
	config := apiTestConfig()
	config.ReportError = func(err error) { *reported = append(*reported, err) }
	api, err := NewAPI(runtime, config)
	require.NoError(t, err)

	return http.StripPrefix(mountPrefix, api.Handler()), api, clock, reported
}

// call performs one request against the mounted handler.
func call(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, mountPrefix+path, reader)
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

// decodeError reads the shared error envelope of a failed response.
func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) simulatorapi.APIError {
	t.Helper()

	var response simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response.Error
}

func TestHTTPRoutesServeTheDocumentedContract(t *testing.T) {
	t.Parallel()

	handler, api, clock, reported := newTestHandler(t)
	station := `{
		"id":"primary","enabled":true,"latitudeDegrees":50,"longitudeDegrees":14,
		"siteElevationMetres":100,"antennaHeightMetres":20,"antennaGainDBi":3,
		"sensitivityDBm":-95,"systemLossDB":2,"frameLossProbability":0
	}`
	runID := api.RunID()

	created := call(t, handler, http.MethodPost, "/stations",
		`{"runId":"`+runID+`","station":`+station+`}`, nil)
	require.Equal(t, http.StatusCreated, created.Code)
	require.Equal(t, "application/json; charset=utf-8", created.Header().Get("Content-Type"))
	require.Equal(t, "no-store", created.Header().Get("Cache-Control"))

	var stationAck simulatorapi.StationAck
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &stationAck))
	require.Equal(t, "1", stationAck.Station.Revision)
	require.Equal(t, simulatorapi.OperationAddStation, stationAck.Operation)

	clock.Add(2 * time.Second)
	count := call(t, handler, http.MethodPut, "/aircraft/count", `{"runId":"`+runID+`","count":1}`, nil)
	require.Equal(t, http.StatusOK, count.Code)
	clock.Add(2 * time.Second)
	speed := call(t, handler, http.MethodPut, "/time/speed", `{"runId":"`+runID+`","speedHundredths":0}`, nil)
	require.Equal(t, http.StatusOK, speed.Code)

	metadata := call(t, handler, http.MethodGet, "/metadata", "", nil)
	require.Equal(t, http.StatusOK, metadata.Code)
	var decodedMetadata simulatorapi.Metadata
	require.NoError(t, json.Unmarshal(metadata.Body.Bytes(), &decodedMetadata))
	require.Equal(t, runID, decodedMetadata.RunID)
	require.Equal(t, 0, decodedMetadata.Simulation.SpeedHundredths)

	truth := call(t, handler, http.MethodGet, "/truth", "", nil)
	require.Equal(t, http.StatusOK, truth.Code)
	var decodedTruth simulatorapi.TruthSnapshot
	require.NoError(t, json.Unmarshal(truth.Body.Bytes(), &decodedTruth))
	require.Equal(t, 1, decodedTruth.AircraftCount)

	stations := call(t, handler, http.MethodGet, "/stations", "", nil)
	require.Equal(t, http.StatusOK, stations.Code)
	var decodedStations simulatorapi.StationsSnapshot
	require.NoError(t, json.Unmarshal(stations.Body.Bytes(), &decodedStations))
	require.Len(t, decodedStations.Stations, 1)
	require.Equal(t, 35000.0, decodedStations.Stations[0].Coverage.ReferenceAltitudeFeet)

	raw := call(t, handler, http.MethodPost, "/observations/receptions", `{"stationIds":["primary"]}`, nil)
	require.Equal(t, http.StatusOK, raw.Code)
	var decodedRaw simulatorapi.ReceptionSnapshot
	require.NoError(t, json.Unmarshal(raw.Body.Bytes(), &decodedRaw))
	require.NotEmpty(t, decodedRaw.Records)

	observations := call(t, handler, http.MethodPost, "/observations", `{
		"stationIds":["primary"],
		"expiry":{"identityNanoseconds":"60000000000","positionNanoseconds":"60000000000",
		"altitudeNanoseconds":"60000000000","velocityNanoseconds":"60000000000"}
	}`, nil)
	require.Equal(t, http.StatusOK, observations.Code)
	var decodedObservations simulatorapi.ObservationSnapshot
	require.NoError(t, json.Unmarshal(observations.Body.Bytes(), &decodedObservations))
	require.Len(t, decodedObservations.Aircraft, 1)

	history := call(t, handler, http.MethodPost, "/receptions/history",
		`{"stationId":"primary","cursor":null,"limit":1}`, nil)
	require.Equal(t, http.StatusOK, history.Code)
	var page simulatorapi.ReceptionPage
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &page))
	require.Len(t, page.Records, 1)
	require.True(t, page.HasMore)

	resumed := call(t, handler, http.MethodPost, "/receptions/history",
		`{"stationId":"primary","cursor":{"runId":"`+runID+`","stationId":"primary","afterSequence":"`+
			page.NextCursor.AfterSequence+`"},"limit":1}`, nil)
	require.Equal(t, http.StatusOK, resumed.Code)

	updated := call(t, handler, http.MethodPut, "/stations/primary",
		`{"runId":"`+runID+`","expectedRevision":"1","station":`+station+`}`, nil)
	require.Equal(t, http.StatusOK, updated.Code)

	removed := call(t, handler, http.MethodDelete, "/stations/primary",
		`{"runId":"`+runID+`","expectedRevision":"2"}`, nil)
	require.Equal(t, http.StatusOK, removed.Code)

	require.Empty(t, *reported, "successful routes report no host error")
}

func TestHTTPRejectsUnknownRoutesAndMethods(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTestHandler(t)

	notFound := call(t, handler, http.MethodGet, "/nope", "", nil)
	require.Equal(t, http.StatusNotFound, notFound.Code)
	require.Equal(t, simulatorapi.CategoryNotFound, decodeError(t, notFound).Code)

	nested := call(t, handler, http.MethodPut, "/stations/a/b", `{}`, nil)
	require.Equal(t, http.StatusNotFound, nested.Code)

	cases := []struct {
		name   string
		method string
		path   string
		allow  string
	}{
		{name: "metadata post", method: http.MethodPost, path: "/metadata", allow: "GET"},
		{name: "truth delete", method: http.MethodDelete, path: "/truth", allow: "GET"},
		{name: "stations put", method: http.MethodPut, path: "/stations", allow: "GET, POST"},
		{name: "station post", method: http.MethodPost, path: "/stations/primary", allow: "PUT, DELETE"},
		{name: "count post", method: http.MethodPost, path: "/aircraft/count", allow: "PUT"},
		{name: "speed get", method: http.MethodGet, path: "/time/speed", allow: "PUT"},
		{name: "observations get", method: http.MethodGet, path: "/observations", allow: "POST"},
		{name: "receptions options", method: http.MethodOptions, path: "/observations/receptions", allow: "POST"},
		{name: "history head", method: http.MethodHead, path: "/receptions/history", allow: "POST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := call(t, handler, tc.method, tc.path, "", nil)
			require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
			require.Equal(t, tc.allow, recorder.Header().Get("Allow"))
			if tc.method != http.MethodHead {
				require.Equal(t, simulatorapi.CategoryMethodNotAllowed, decodeError(t, recorder).Code)
			}
		})
	}
}

func TestHTTPRejectsQueriesAndBodiesOnReadRoutes(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTestHandler(t)

	request := httptest.NewRequest(http.MethodGet, mountPrefix+"/metadata?verbose=1", strings.NewReader(""))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, simulatorapi.CategoryInvalid, decodeError(t, recorder).Code)

	withBody := httptest.NewRequest(http.MethodGet, mountPrefix+"/truth", strings.NewReader(`{"a":1}`))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, withBody)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestHTTPEnforcesMediaTypeAndBodyBounds(t *testing.T) {
	t.Parallel()

	handler, api, _, _ := newTestHandler(t)
	body := `{"runId":"` + api.RunID() + `","count":1}`

	cases := []struct {
		name    string
		headers map[string]string
		status  int
		code    simulatorapi.Category
	}{
		{
			name: "missing content type", headers: map[string]string{"Content-Type": ""},
			status: http.StatusUnsupportedMediaType, code: simulatorapi.CategoryUnsupportedMediaType,
		},
		{
			name: "wrong content type", headers: map[string]string{"Content-Type": "text/plain"},
			status: http.StatusUnsupportedMediaType, code: simulatorapi.CategoryUnsupportedMediaType,
		},
		{
			name: "malformed content type", headers: map[string]string{"Content-Type": "application/json;;"},
			status: http.StatusUnsupportedMediaType, code: simulatorapi.CategoryUnsupportedMediaType,
		},
		{
			name: "wrong charset", headers: map[string]string{"Content-Type": "application/json; charset=utf-16"},
			status: http.StatusUnsupportedMediaType, code: simulatorapi.CategoryUnsupportedMediaType,
		},
		{
			name: "compressed body", headers: map[string]string{"Content-Encoding": "gzip"},
			status: http.StatusUnsupportedMediaType, code: simulatorapi.CategoryUnsupportedMediaType,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := call(t, handler, http.MethodPut, "/aircraft/count", body, tc.headers)
			require.Equal(t, tc.status, recorder.Code)
			require.Equal(t, tc.code, decodeError(t, recorder).Code)
		})
	}

	accepted := call(t, handler, http.MethodPut, "/aircraft/count", body,
		map[string]string{"Content-Type": "application/json; charset=UTF-8"})
	require.Equal(t, http.StatusOK, accepted.Code)
}

func TestHTTPEnforcesTheRequestByteBound(t *testing.T) {
	t.Parallel()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)

	config := apiTestConfig()
	body := `{"stationIds":[]}`
	config.MaxRequestBytes = len(body)
	api, err := NewAPI(runtime, config)
	require.NoError(t, err)
	handler := http.StripPrefix(mountPrefix, api.Handler())

	atBound := call(t, handler, http.MethodPost, "/observations/receptions", body, nil)
	require.Equal(t, http.StatusOK, atBound.Code)

	oneOver := call(t, handler, http.MethodPost, "/observations/receptions", `{"stationIds":[ ]}`, nil)
	require.Equal(t, http.StatusRequestEntityTooLarge, oneOver.Code)
	require.Equal(t, simulatorapi.CategoryBodyTooLarge, decodeError(t, oneOver).Code)
}

func TestHTTPRejectsMalformedRequestData(t *testing.T) {
	t.Parallel()

	handler, api, _, _ := newTestHandler(t)
	runID := api.RunID()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   simulatorapi.Category
	}{
		{
			name: "missing field", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `"}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "null field", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":null}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "unknown field", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":1,"extra":true}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "duplicate field", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":1,"count":2}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "wrong type", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":"1"}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "malformed json", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "trailing json", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":1}{}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "stale run", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"other","count":1}`, status: http.StatusConflict, code: simulatorapi.CategoryConflict,
		},
		{
			name: "count above engine policy", method: http.MethodPut, path: "/aircraft/count",
			body: `{"runId":"` + runID + `","count":101}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "unknown station", method: http.MethodPost, path: "/observations/receptions",
			body: `{"stationIds":["missing"]}`, status: http.StatusNotFound, code: simulatorapi.CategoryNotFound,
		},
		{
			name: "missing cursor key", method: http.MethodPost, path: "/receptions/history",
			body: `{"stationId":"primary","limit":1}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "zero revision", method: http.MethodDelete, path: "/stations/primary",
			body: `{"runId":"` + runID + `","expectedRevision":"0"}`, status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "path and body station disagree", method: http.MethodPut, path: "/stations/primary",
			body: `{"runId":"` + runID + `","expectedRevision":"1","station":{
				"id":"other","enabled":true,"latitudeDegrees":50,"longitudeDegrees":14,
				"siteElevationMetres":100,"antennaHeightMetres":20,"antennaGainDBi":3,
				"sensitivityDBm":-95,"systemLossDB":2,"frameLossProbability":0}}`,
			status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
		{
			name: "partial station settings", method: http.MethodPost, path: "/stations",
			body:   `{"runId":"` + runID + `","station":{"id":"partial","enabled":true}}`,
			status: http.StatusBadRequest, code: simulatorapi.CategoryInvalid,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := call(t, handler, tc.method, tc.path, tc.body, nil)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.code, decodeError(t, recorder).Code)
		})
	}
}

func TestHTTPEnforcesTheResponseByteBound(t *testing.T) {
	t.Parallel()

	clock := &runtimeClock{
		now:     time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		started: make(chan struct{}), ticks: make(chan time.Time), stopped: make(chan struct{}),
	}
	runtime, err := newSimulator(runtimeTestConfig(), clock)
	require.NoError(t, err)

	config := apiTestConfig()
	config.MaxResponseBytes = simulatorapi.MinResponseBytes
	api, err := NewAPI(runtime, config)
	require.NoError(t, err)
	handler := http.StripPrefix(mountPrefix, api.Handler())

	clock.Add(2 * time.Second)
	counted := call(t, handler, http.MethodPut, "/aircraft/count",
		`{"runId":"`+api.RunID()+`","count":5}`, nil)
	require.Equal(t, http.StatusOK, counted.Code)

	recorder := call(t, handler, http.MethodGet, "/truth", "", nil)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	failure := decodeError(t, recorder)
	require.Equal(t, simulatorapi.CategoryResponseLimit, failure.Code)
	require.LessOrEqual(t, recorder.Body.Len(), simulatorapi.MinResponseBytes)
}

func TestHTTPReportsWriterFailuresToTheHost(t *testing.T) {
	t.Parallel()

	handler, _, _, reported := newTestHandler(t)
	request := httptest.NewRequest(http.MethodGet, mountPrefix+"/metadata", strings.NewReader(""))
	handler.ServeHTTP(failingWriter{header: http.Header{}}, request)
	require.Len(t, *reported, 1)
	require.ErrorContains(t, (*reported)[0], "write simulator response")
}

func TestHTTPMapsAnEarlierCallerDeadlineToGatewayTimeout(t *testing.T) {
	t.Parallel()

	handler, _, _, _ := newTestHandler(t)

	expired, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	request := httptest.NewRequest(http.MethodPost, mountPrefix+"/observations/receptions",
		strings.NewReader(`{"stationIds":[]}`)).WithContext(expired)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	require.Equal(t, simulatorapi.CategoryDeadlineExceeded, decodeError(t, recorder).Code)
}

// failingWriter accepts headers but never writes a body successfully.
type failingWriter struct{ header http.Header }

func (w failingWriter) Header() http.Header     { return w.header }
func (w failingWriter) WriteHeader(int)         {}
func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }
