package display

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/qualitytest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Display response capacity.
//
// The bound check extends the simulatorapi worksheet with the display's own
// response wrapper. The cardinality check builds a synthetic but
// semantically valid source of 8000 records with 8000 distinct addresses,
// the largest raw snapshot a conforming source may return. Its frames are
// encoded by this project's codec; they prove cardinality, not protocol.

func loadCapacity(t *testing.T) qualitytest.Capacity {
	t.Helper()

	capacity, err := qualitytest.LoadCapacity(qualityDir + "/capacity.json")
	require.NoError(t, err)
	return capacity
}

func TestCapacityDisplayRefreshBound(t *testing.T) {
	t.Parallel()

	capacity := loadCapacity(t)
	profile := capacity.Profile
	instant := "9999-12-31T23:59:59.999999999Z"
	runID := strings.Repeat("<", profile.MaxRunIDBytes)
	message := strings.Repeat("<", profile.MaxErrorMessageBytes)
	field := strings.Repeat("<", profile.MaxErrorFieldBytes)

	// The wrapper adds the same bytes whatever the observations hold, so its
	// overhead is measured around an empty observation object.
	empty := &simulatorapi.ObservationSnapshot{}
	wrapped, err := json.Marshal(RefreshResponse{
		Snapshot: Snapshot{
			Status: StatusUnavailable, LastUpdatedAt: &instant, Observations: empty,
			Error: &SourceFailure{
				Operation: field, Code: simulatorapi.CategoryDeadlineExceeded, Message: message,
				RunID: runID, Status: math.MinInt64,
			},
		},
		Error: &simulatorapi.APIError{
			Code: simulatorapi.CategoryDeadlineExceeded, Message: message, Field: field, RunID: runID,
		},
	})
	require.NoError(t, err)
	bare, err := json.Marshal(empty)
	require.NoError(t, err)
	bound := capacity.Bounds["observationSnapshotBytes"] + len(wrapped) - len(bare)
	t.Logf("bound displayRefreshBytes = %d", bound)
	require.Equal(t, capacity.Bounds["displayRefreshBytes"], bound,
		"bound displayRefreshBytes changed; update testdata/quality/capacity.json")

	// A display serves refresh bodies, history pages, stations and errors.
	largest := max(bound, capacity.Bounds["historyPageBytes"], capacity.Bounds["stationsBytes"],
		capacity.Bounds["errorEnvelopeBytes"])
	require.Equal(t, capacity.Budget(largest), capacity.Budgets.DisplayMaxResponseBytes)
	require.Equal(t, capacity.Budgets.DisplayMaxResponseBytes, capacity.Budgets.AircraftUIMaxResponseBytes,
		"an aircraft page must accept every display response")
}

// maximumSource builds the largest conforming raw snapshot: eight stations
// with 64-byte identifiers, each holding 1000 records, and 8000 distinct
// addresses. Transmission sequences run 1..8000 in record order and each
// station's local sequences run 1..1000.
func maximumSource(t *testing.T) simulatorapi.ReceptionSnapshot {
	t.Helper()

	stations := make([]simulatorapi.Station, 0, maxSelectedStations)
	ids := make([]string, 0, maxSelectedStations)
	for i := range maxSelectedStations {
		id := string(rune('a'+i)) + strings.Repeat("Z", maxStationIDBytes-1)
		ids = append(ids, id)
		stations = append(stations, fixtureStation(id, 1, fixtureStart))
	}
	records := make([]simulatorapi.Reception, 0, maxSnapshotRecords)
	for k := range maxSnapshotRecords {
		station := stations[k%maxSelectedStations]
		icao := uint32(0x100000 + k)
		record := fixtureReception(uint64(k/maxSelectedStations+1), uint64(k+1), station, icao,
			identificationKind, fixtureStart, identificationFrame(t, icao, fmt.Sprintf("TB%06X", icao)))
		records = append(records, record)
	}
	retention := make([]simulatorapi.StationRetention, 0, len(ids))
	for _, id := range ids {
		retention = append(retention, simulatorapi.StationRetention{
			StationID: id, OldestSequence: "1", LatestSequence: "1000", Truncated: false, Limit: maxStationRecords,
		})
	}
	return simulatorapi.ReceptionSnapshot{
		RunID: fixtureRunID, Now: simulatorapi.FormatTime(fixtureStart.Add(time.Second)),
		StationIDs: ids, Retention: retention, Records: records,
	}
}

func TestCapacityMaximumSourceFitsTheShippedBudgets(t *testing.T) {
	t.Parallel()

	capacity := loadCapacity(t)
	raw := maximumSource(t)
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), capacity.Bounds["rawSnapshotBytes"])
	require.LessOrEqual(t, len(encoded), capacity.Budgets.SourceMaxResponseBytes)

	// The HTTP source reads it within the source budget.
	transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, string(encoded)), nil
	}}
	source, err := NewHTTPSource(HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: 5 * time.Second, MaxRequestBytes: 65536,
		MaxResponseBytes: capacity.Budgets.SourceMaxResponseBytes, Client: &http.Client{Transport: transport},
	})
	require.NoError(t, err)
	request := simulatorapi.ReceptionSnapshotRequest{StationIDs: raw.StationIDs}
	fetched, err := source.ReceptionSnapshot(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, raw, fetched)

	// A display publishes 8000 partial tracks and serves them within the
	// display budget.
	config := displayFixtureConfig()
	config.MaxResponseBytes = capacity.Budgets.DisplayMaxResponseBytes
	backend := newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return fetched, nil
		},
	}, funcSource{}, time.Now)
	handler := http.StripPrefix(displayMount, backend.Handler())
	body, err := json.Marshal(map[string][]string{"stationIds": raw.StationIDs})
	require.NoError(t, err)
	recorder := displayCall(t, handler, http.MethodPost, "/observations", string(body), nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String()[:min(recorder.Body.Len(), 512)])

	var response RefreshResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Snapshot.Observations.Aircraft, maxSnapshotRecords,
		"received tracks are bounded by raw records, not by the engine fleet size")
	for _, aircraft := range response.Snapshot.Observations.Aircraft[:3] {
		require.NotNil(t, aircraft.Identity)
		require.Nil(t, aircraft.Position)
	}
	require.LessOrEqual(t, recorder.Body.Len(), capacity.Bounds["displayRefreshBytes"])
	t.Logf("maximum source: %d records, %d distinct aircraft, %d raw bytes, %d display refresh bytes",
		len(raw.Records), len(response.Snapshot.Observations.Aircraft), len(encoded), recorder.Body.Len())
}

// exactBody returns a valid four-record history page and its exact JSON
// encoding, which is larger than the minimum response budget.
func exactBody(t *testing.T) (simulatorapi.HistoryRequest, simulatorapi.ReceptionPage, []byte) {
	t.Helper()

	alpha := fixtureStation("alpha", 1, fixtureStart)
	frame := identificationFrame(t, fixtureICAO, "TB00ABCD")
	at := fixtureStart.Add(time.Second)
	records := make([]simulatorapi.Reception, 0, 4)
	for sequence := uint64(1); sequence <= 4; sequence++ {
		records = append(records, fixtureReception(sequence, sequence, alpha, fixtureICAO, identificationKind, at, frame))
	}
	request := simulatorapi.HistoryRequest{StationID: "alpha", Limit: 4}
	page := simulatorapi.ReceptionPage{
		RunID: fixtureRunID, StationID: "alpha", Now: simulatorapi.FormatTime(fixtureStart.Add(2 * time.Second)),
		Records: records, OldestSequence: "1", LatestSequence: "4",
		NextCursor: simulatorapi.ReceptionCursor{RunID: fixtureRunID, StationID: "alpha", AfterSequence: "4"},
		Gap:        false, HasMore: false, RetentionLimit: maxStationRecords,
	}
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.Greater(t, len(encoded), simulatorapi.MinResponseBytes)
	return request, page, encoded
}

func TestHTTPSourceResponseLimitIsExact(t *testing.T) {
	t.Parallel()

	request, _, body := exactBody(t)
	cases := []struct {
		name   string
		limit  int
		length int64
		ok     bool
	}{
		{name: "declared length fits exactly", limit: len(body), length: int64(len(body)), ok: true},
		{name: "undeclared length fits exactly", limit: len(body), length: -1, ok: true},
		{name: "declared length one byte over", limit: len(body) - 1, length: int64(len(body))},
		{name: "undeclared length one byte over", limit: len(body) - 1, length: -1},
		{name: "dishonest short length one byte over", limit: len(body) - 1, length: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var served *countingBody
			transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
				response := jsonResponse(http.StatusOK, string(body))
				response.ContentLength = tc.length
				served = response.Body.(*countingBody)
				return response, nil
			}}
			source, err := NewHTTPSource(HTTPSourceConfig{
				BaseURL: sourceBaseURL, Timeout: time.Second, MaxRequestBytes: 4096,
				MaxResponseBytes: tc.limit, Client: &http.Client{Transport: transport},
			})
			require.NoError(t, err)

			page, err := source.ReceptionHistory(t.Context(), request)
			require.Equal(t, 1, served.closed, "the body is closed exactly once")
			if tc.ok {
				require.NoError(t, err)
				require.Len(t, page.Records, 4)
				return
			}
			require.ErrorIs(t, err, simulatorapi.CategoryResponseLimit)
			require.Equal(t, simulatorapi.ReceptionPage{}, page, "no partial success")
		})
	}
}

// failingCloseBody reads normally and fails on close.
type failingCloseBody struct {
	*strings.Reader
	closed int
}

func (b *failingCloseBody) Close() error {
	b.closed++
	return fmt.Errorf("close reset")
}

func TestHTTPSourceTruncatedAndCloseFailuresKeepTheirCause(t *testing.T) {
	t.Parallel()

	request, _, body := exactBody(t)
	cases := []struct {
		name     string
		response func() (*http.Response, func() int)
		category error
		cause    string
	}{
		{
			name: "truncated body", category: simulatorapi.CategorySourceInvalid,
			response: func() (*http.Response, func() int) {
				response := jsonResponse(http.StatusOK, string(body[:len(body)/2]))
				counted := response.Body.(*countingBody)
				return response, func() int { return counted.closed }
			},
		},
		{
			name: "body read failure", category: simulatorapi.CategoryUnavailable, cause: "connection reset",
			response: func() (*http.Response, func() int) {
				response := jsonResponse(http.StatusOK, string(body))
				counted := response.Body.(*countingBody)
				counted.err = fmt.Errorf("connection reset")
				return response, func() int { return counted.closed }
			},
		},
		{
			name: "close failure", category: simulatorapi.CategoryUnavailable, cause: "close reset",
			response: func() (*http.Response, func() int) {
				response := jsonResponse(http.StatusOK, string(body))
				closing := &failingCloseBody{Reader: strings.NewReader(string(body))}
				response.Body = closing
				return response, func() int { return closing.closed }
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			response, closes := tc.response()
			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) { return response, nil })
			page, err := source.ReceptionHistory(t.Context(), request)
			require.ErrorIs(t, err, tc.category)
			if tc.cause != "" {
				require.ErrorContains(t, err, tc.cause, "the original cause is preserved")
			}
			require.Equal(t, simulatorapi.ReceptionPage{}, page)
			require.Equal(t, 1, closes(), "the body is closed exactly once")
		})
	}
}

func TestDisplayHandlerResponseLimitIsExact(t *testing.T) {
	t.Parallel()

	request, page, _ := exactBody(t)
	body, err := json.Marshal(map[string]any{"stationId": request.StationID, "limit": request.Limit, "cursor": nil})
	require.NoError(t, err)
	source := funcSource{history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
		return page, nil
	}}

	// Measure the exact successful body once with a generous budget.
	generous := newDisplay(displayFixtureConfig(), source, funcSource{}, time.Now)
	measured := displayCall(t, http.StripPrefix(displayMount, generous.Handler()), http.MethodPost,
		"/receptions/history", string(body), nil)
	require.Equal(t, http.StatusOK, measured.Code)
	size := measured.Body.Len()
	require.Greater(t, size, simulatorapi.MinResponseBytes)

	for _, tc := range []struct {
		limit int
		ok    bool
	}{{size, true}, {size - 1, false}} {
		config := displayFixtureConfig()
		config.MaxResponseBytes = tc.limit
		backend := newDisplay(config, source, funcSource{}, time.Now)
		recorder := displayCall(t, http.StripPrefix(displayMount, backend.Handler()), http.MethodPost,
			"/receptions/history", string(body), nil)
		if tc.ok {
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, measured.Body.String(), recorder.Body.String())
			continue
		}
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		var failure simulatorapi.ErrorResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &failure), "a complete error envelope")
		require.Equal(t, simulatorapi.CategoryResponseLimit, failure.Error.Code)
		require.NotContains(t, recorder.Body.String(), `"records"`, "no partial success body")
	}
}

// TestDisplayStructuralBoundsMirrorEnginePolicy compares the display's own
// mirrored acceptance constants with the engine policy the simulator
// publishes. The display must not import the engine in production code, so
// the comparison lives in this test.
func TestDisplayStructuralBoundsMirrorEnginePolicy(t *testing.T) {
	t.Parallel()

	require.Equal(t, simulation.MaxStations, maxSelectedStations)
	require.Equal(t, simulation.ReceptionHistoryLimit, maxStationRecords)
	require.Equal(t, simulation.MaxStations*simulation.ReceptionHistoryLimit, maxSnapshotRecords)
	require.Equal(t, 64, maxStationIDBytes, "station identifiers are 1 to 64 bytes")

	capacity := loadCapacity(t)
	require.Equal(t, capacity.Profile.MaxSelectedStations, maxSelectedStations)
	require.Equal(t, capacity.Profile.MaxStationRecords, maxStationRecords)
	require.Equal(t, capacity.Profile.MaxSnapshotRecords, maxSnapshotRecords)
	require.Equal(t, capacity.Profile.MaxStationIDBytes, maxStationIDBytes)
	require.Equal(t, capacity.Profile.MaxTruthAircraft, simulation.MaxAircraft)
	require.Equal(t, capacity.Profile.TransmissionHistoryLimit, simulation.HistoryLimit)
	require.Equal(t, capacity.Profile.MaxHistoryPageRecords, simulation.MaxHistoryPageSize)
}
