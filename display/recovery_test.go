package display

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// Failure and recovery matrix.
//
// Each row states what a browser sees while a refresh fails and what the
// next successful refresh must publish. Rows that exist only on HTTP, such
// as a malformed body, are labeled: a local provider is never made to
// fabricate a transport failure just to claim parity.

// recoverySources builds one local and one HTTP source over the same
// switchable fixture. failing selects what the next call returns.
type recoverySources struct {
	good    simulatorapi.ReceptionSnapshot
	next    simulatorapi.ReceptionSnapshot
	failing bool
	// local returns this error while failing.
	localErr error
	// remote returns this body and status while failing.
	remoteBody   string
	remoteStatus int
}

func (r *recoverySources) localSource(t *testing.T) ObservationSource {
	t.Helper()

	return funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			if r.failing {
				if r.localErr != nil {
					return simulatorapi.ReceptionSnapshot{}, r.localErr
				}
				return r.next, nil
			}
			return r.good, nil
		},
	}
}

func (r *recoverySources) httpSource(t *testing.T, budget int) ObservationSource {
	t.Helper()

	transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
		if r.failing {
			status := r.remoteStatus
			if status == 0 {
				status = http.StatusOK
			}
			body := r.remoteBody
			if body == "" {
				body = encodeFixture(t, r.next)
			}
			return jsonResponse(status, body), nil
		}
		return jsonResponse(http.StatusOK, encodeFixture(t, r.good)), nil
	}}
	source, err := NewHTTPSource(HTTPSourceConfig{
		BaseURL: sourceBaseURL, Timeout: 5 * time.Second, MaxRequestBytes: 65536,
		MaxResponseBytes: budget, Client: &http.Client{Transport: transport},
	})
	require.NoError(t, err)
	return source
}

func TestDisplayFailureAndRecoveryMatrix(t *testing.T) {
	t.Parallel()

	good := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	later := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(30*time.Second), 10)
	corrupt := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(20*time.Second), 4)
	corrupt.Records[1].Frame = strings.Repeat("A", 28)
	regressed := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(time.Second), 1)
	oversized := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(20*time.Second), 4)

	cases := []struct {
		name string
		// transports names the sources the row exists on.
		transports []string
		// setup configures the failure.
		setup  func(*recoverySources)
		budget int
		// category is the failure a browser is told about.
		category error
		// stale reports whether the last good data stays published.
		stale bool
	}{
		{
			name: "same-run transport outage", transports: []string{"local", "http"},
			setup: func(r *recoverySources) {
				r.localErr = newSourceError(snapshotOperation, simulatorapi.CategoryUnavailable,
					errors.New("connection refused"), "the source is down")
				r.remoteStatus = http.StatusServiceUnavailable
				r.remoteBody = `{"error":{"code":"unavailable","message":"the source is down","field":"","runId":""}}`
			},
			category: simulatorapi.CategoryUnavailable, stale: true,
		},
		{
			name: "oversized payload", transports: []string{"http"},
			setup: func(r *recoverySources) {
				// The budget is checked before parsing, so the body only has
				// to be larger than the budget.
				r.remoteBody = `{"runId":"` + strings.Repeat("x", 4000) + `"}`
			},
			budget: simulatorapi.MinResponseBytes, category: simulatorapi.CategoryResponseLimit, stale: true,
		},
		{
			name: "truncated body", transports: []string{"http"},
			setup: func(r *recoverySources) {
				r.remoteBody = encodeFixture(t, oversized)[:200]
			},
			category: simulatorapi.CategorySourceInvalid, stale: true,
		},
		{
			name: "corrupt frame in a valid envelope", transports: []string{"local", "http"},
			setup: func(r *recoverySources) {
				r.next = corrupt
			},
			category: simulatorapi.CategorySourceInvalid, stale: true,
		},
		{
			name: "regressed time in the same run", transports: []string{"local", "http"},
			setup: func(r *recoverySources) {
				r.next = regressed
			},
			category: simulatorapi.CategorySourceInvalid, stale: true,
		},
	}

	for _, tc := range cases {
		for _, transport := range tc.transports {
			t.Run(tc.name+"/"+transport, func(t *testing.T) {
				t.Parallel()

				sources := &recoverySources{good: good, next: good}
				tc.setup(sources)
				budget := tc.budget
				if budget == 0 {
					budget = 16777216
				}
				var source ObservationSource
				if transport == "local" {
					source = sources.localSource(t)
				} else {
					source = sources.httpSource(t, budget)
				}
				config := displayFixtureConfig()
				config.MaxResponseBytes = budget
				backend := newDisplay(config, source, funcSource{}, time.Now)

				first, err := backend.Refresh(t.Context(), good.StationIDs)
				require.NoError(t, err)
				require.Equal(t, StatusFresh, first.Status)

				sources.failing = true
				failed, err := backend.Refresh(t.Context(), good.StationIDs)
				require.ErrorIs(t, err, tc.category)
				if tc.stale {
					require.Equal(t, StatusStale, failed.Status)
					require.Equal(t, first.Observations, failed.Observations,
						"a failed refresh publishes nothing of its own")
				} else {
					require.Equal(t, StatusUnavailable, failed.Status)
					require.Nil(t, failed.Observations)
				}

				// Recovery replaces the published state completely.
				sources.failing = false
				sources.good = later
				recovered, err := backend.Refresh(t.Context(), good.StationIDs)
				require.NoError(t, err)
				require.Equal(t, StatusFresh, recovered.Status)
				require.Nil(t, recovered.Error)
				require.Equal(t, simulatorapi.FormatTime(fixtureStart.Add(30*time.Second)),
					recovered.Observations.Now)
				require.NotEqual(t, first.Observations, recovered.Observations)
			})
		}
	}
}

// TestDisplayHistoryInFlightDuringStationRemoval holds one history request
// at the source while the station it names is removed. The response that
// arrives afterwards belongs to an invalidated selection and must not add
// records to the browser's view of the current run.
func TestDisplayHistoryInFlightDuringStationRemoval(t *testing.T) {
	t.Parallel()

	request, page := fixturePage(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	removed := false
	var mu sync.Mutex

	backend := newDisplay(displayFixtureConfig(), funcSource{
		history: func(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
			// Only the first call is held; later calls run straight through.
			select {
			case entered <- struct{}{}:
				<-release
			default:
			}
			mu.Lock()
			defer mu.Unlock()
			if removed {
				return simulatorapi.ReceptionPage{},
					newSourceError(historyOperation, simulatorapi.CategoryNotFound, nil,
						`station "alpha" was removed`)
			}
			return page, nil
		},
	}, funcSource{}, time.Now)

	result := make(chan error, 1)
	go func() {
		_, err := backend.ReceptionHistory(context.Background(), request)
		result <- err
	}()
	<-entered

	mu.Lock()
	removed = true
	mu.Unlock()
	close(release)

	require.ErrorIs(t, <-result, simulatorapi.CategoryNotFound,
		"a page for a removed station is a failure, not silent extra records")

	// The gate is usable again for the selection that remains.
	_, err := backend.ReceptionHistory(t.Context(), request)
	require.ErrorIs(t, err, simulatorapi.CategoryNotFound)
}

// TestDisplayRecoversAfterAnOversizedResponse checks the browser-facing
// shape of a response-limit failure: a complete error envelope, no partial
// body, and a successful refresh afterwards.
func TestDisplayRecoversAfterAnOversizedResponse(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	budget := simulatorapi.MinResponseBytes
	config := displayFixtureConfig()
	config.MaxResponseBytes = budget
	backend := newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	}, funcSource{}, time.Now)
	handler := http.StripPrefix(displayMount, backend.Handler())

	recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var failure simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &failure))
	require.Equal(t, simulatorapi.CategoryResponseLimit, failure.Error.Code)
	require.NotContains(t, recorder.Body.String(), `"aircraft"`)

	// The refresh itself succeeded, so the published state is fresh and the
	// next request with a sufficient budget serves it.
	config.MaxResponseBytes = 16777216
	larger := newDisplay(config, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	}, funcSource{}, time.Now)
	ok := displayCall(t, http.StripPrefix(displayMount, larger.Handler()), http.MethodPost,
		"/observations", `{"stationIds":["alpha"]}`, nil)
	require.Equal(t, http.StatusOK, ok.Code)
}
