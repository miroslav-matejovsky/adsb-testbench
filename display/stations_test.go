package display

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// fixtureStations returns a valid two-station catalog with one disabled
// station.
func fixtureStations(runID string) simulatorapi.StationsSnapshot {
	coverage := simulatorapi.Coverage{
		ReferenceAltitudeFeet:         10000,
		HorizonRadiusNauticalMiles:    130.5,
		LinkBudgetRadiusNauticalMiles: 90.25,
		EffectiveRadiusNauticalMiles:  90.25,
	}
	bravo := fixtureStation("bravo", 3, fixtureStart)
	bravo.Enabled = false
	return simulatorapi.StationsSnapshot{
		RunID: runID, Now: simulatorapi.FormatTime(fixtureStart.Add(time.Minute)),
		Stations: []simulatorapi.StationState{
			{Station: fixtureStation("alpha", 1, fixtureStart), Coverage: coverage},
			{Station: bravo, Coverage: coverage},
		},
	}
}

// stationProvider returns a fixed catalog or error.
type stationProvider struct {
	stations simulatorapi.StationsSnapshot
	err      error
}

func (p stationProvider) Stations(context.Context) (simulatorapi.StationsSnapshot, error) {
	return p.stations, p.err
}

func TestValidateStationsAcceptsACompleteCatalog(t *testing.T) {
	t.Parallel()

	runID, err := validateStations(fixtureStations(fixtureRunID))
	require.NoError(t, err)
	require.Equal(t, fixtureRunID, runID)

	empty := fixtureStations(fixtureRunID)
	empty.Stations = []simulatorapi.StationState{}
	_, err = validateStations(empty)
	require.NoError(t, err)
}

func TestValidateStationsRejectsInconsistentCatalogs(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*simulatorapi.StationsSnapshot){
		"empty run":     func(s *simulatorapi.StationsSnapshot) { s.RunID = "" },
		"bad now":       func(s *simulatorapi.StationsSnapshot) { s.Now = "yesterday" },
		"nil stations":  func(s *simulatorapi.StationsSnapshot) { s.Stations = nil },
		"duplicate id":  func(s *simulatorapi.StationsSnapshot) { s.Stations[1].Station.ID = "alpha" },
		"invalid id":    func(s *simulatorapi.StationsSnapshot) { s.Stations[0].Station.ID = "a b" },
		"zero revision": func(s *simulatorapi.StationsSnapshot) { s.Stations[0].Station.Revision = "0" },
		"padded revision": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Station.Revision = "01"
		},
		"created later": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Station.CreatedAt = simulatorapi.FormatTime(fixtureStart.Add(time.Hour))
		},
		"non finite latitude": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Station.LatitudeDegrees = math.NaN()
		},
		"latitude out of range": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Station.LatitudeDegrees = 90.5
		},
		"probability out of range": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Station.FrameLossProbability = 1.5
		},
		"negative radius": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Coverage.HorizonRadiusNauticalMiles = -1
		},
		"infinite radius": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Coverage.LinkBudgetRadiusNauticalMiles = math.Inf(1)
		},
		"effective radius mismatch": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Coverage.EffectiveRadiusNauticalMiles = 100
		},
		"reference altitude out of range": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[0].Coverage.ReferenceAltitudeFeet = 60000
		},
		"reference altitude differs": func(s *simulatorapi.StationsSnapshot) {
			s.Stations[1].Coverage.ReferenceAltitudeFeet = 5000
		},
		"too many stations": func(s *simulatorapi.StationsSnapshot) {
			for len(s.Stations) <= maxSelectedStations {
				state := s.Stations[0]
				state.Station.ID = "extra" + simulatorapi.FormatUint64(uint64(len(s.Stations)))
				s.Stations = append(s.Stations, state)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			catalog := fixtureStations(fixtureRunID)
			mutate(&catalog)
			_, err := validateStations(catalog)
			require.Error(t, err)
		})
	}
}

func TestBothSourcesReturnEquivalentStations(t *testing.T) {
	t.Parallel()

	fixture := fixtureStations(fixtureRunID)
	local, err := NewInProcessStationSource(stationProvider{stations: fixture})
	require.NoError(t, err)
	fromLocal, err := local.Stations(t.Context())
	require.NoError(t, err)

	remote, transport := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, encodeFixture(t, fixture)), nil
	})
	fromHTTP, err := remote.Stations(t.Context())
	require.NoError(t, err)
	require.Equal(t, fromLocal, fromHTTP)
	require.Equal(t, fixture, fromLocal)

	require.Len(t, transport.requests, 1)
	request := transport.requests[0]
	require.Equal(t, http.MethodGet, request.Method)
	require.Equal(t, "/bench/a/simulator/stations", request.URL.Path)
	require.Nil(t, request.Body)
	require.Empty(t, request.Header.Get("Content-Type"))
}

func TestInProcessStationSourceDetachesProviderData(t *testing.T) {
	t.Parallel()

	fixture := fixtureStations(fixtureRunID)
	local, err := NewInProcessStationSource(stationProvider{stations: fixture})
	require.NoError(t, err)
	got, err := local.Stations(t.Context())
	require.NoError(t, err)
	got.Stations[0].Station.ID = "changed"
	require.Equal(t, "alpha", fixture.Stations[0].Station.ID)
}

func TestBothSourcesClassifyStationFailuresIdentically(t *testing.T) {
	t.Parallel()

	invalid := fixtureStations(fixtureRunID)
	invalid.Stations[0].Coverage.EffectiveRadiusNauticalMiles = 1
	conflict := simulatorapi.NewError(simulatorapi.CategoryUnavailable, "the service stopped")

	cases := []struct {
		name     string
		local    stationProvider
		remote   func(*http.Request) (*http.Response, error)
		category simulatorapi.Category
		runID    string
	}{
		{
			name:  "invalid payload",
			local: stationProvider{stations: invalid},
			remote: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, encodeFixture(t, invalid)), nil
			},
			category: simulatorapi.CategorySourceInvalid, runID: fixtureRunID,
		},
		{
			name:  "unavailable",
			local: stationProvider{err: conflict},
			remote: func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusServiceUnavailable,
					encodeFixture(t, simulatorapi.ErrorResponse{Error: *conflict})), nil
			},
			category: simulatorapi.CategoryUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			local, err := NewInProcessStationSource(tc.local)
			require.NoError(t, err)
			remote, _ := newHTTPSourceFor(t, tc.remote)
			for _, source := range []StationSource{local, remote} {
				_, err := source.Stations(t.Context())
				var failure *SourceError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, tc.category, failure.Category)
				require.Equal(t, tc.runID, failure.RunID)
				require.Equal(t, stationsOperation, failure.Operation)
			}
		})
	}
}

func TestHTTPSourceRejectsMalformedStationResponses(t *testing.T) {
	t.Parallel()

	valid := encodeFixture(t, fixtureStations(fixtureRunID))
	cases := map[string]string{
		"unknown key":          strings.Replace(valid, `"runId"`, `"extra":1,"runId"`, 1),
		"duplicate key":        strings.Replace(valid, `"runId"`, `"now":"x","runId"`, 1),
		"null stations":        `{"runId":"r","now":"2024-03-05T12:00:00Z","stations":null}`,
		"missing stations":     `{"runId":"r","now":"2024-03-05T12:00:00Z"}`,
		"missing coverage key": strings.Replace(valid, `"referenceAltitudeFeet":10000,`, ``, 1),
		"unknown coverage key": strings.Replace(valid, `"referenceAltitudeFeet"`, `"x":1,"referenceAltitudeFeet"`, 1),
		"string number":        strings.Replace(valid, `"referenceAltitudeFeet":10000`, `"referenceAltitudeFeet":"10000"`, 1),
		"trailing value":       valid + `{}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NotEqual(t, valid, body)
			source, _ := newHTTPSourceFor(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, body), nil
			})
			_, err := source.Stations(t.Context())
			require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
		})
	}

	t.Run("oversized", func(t *testing.T) {
		t.Parallel()

		padded := valid + strings.Repeat(" ", simulatorapi.MinResponseBytes)
		transport := &roundTripper{respond: func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, padded), nil
		}}
		source, err := NewHTTPSource(HTTPSourceConfig{
			BaseURL: sourceBaseURL, Timeout: 5 * time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: simulatorapi.MinResponseBytes,
			Client: &http.Client{Transport: transport},
		})
		require.NoError(t, err)
		_, err = source.Stations(t.Context())
		require.ErrorIs(t, err, simulatorapi.CategoryResponseLimit)
	})
}

func TestNewInProcessStationSourceRequiresAProvider(t *testing.T) {
	t.Parallel()

	_, err := NewInProcessStationSource(nil)
	require.Error(t, err)
}

func TestDisplayStationsClearsThePreviousRun(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, "run-1", fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	catalog := fixtureStations("run-2")
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
		stations: func(context.Context) (simulatorapi.StationsSnapshot, error) {
			return catalog, nil
		},
	})
	_, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, StatusFresh, display.Snapshot().Status)

	stations, err := display.Stations(t.Context())
	require.NoError(t, err)
	require.Equal(t, catalog, stations)
	snapshot := display.Snapshot()
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)

	// The returned catalog is detached from the source's value.
	stations.Stations[0].Station.ID = "changed"
	require.Equal(t, "alpha", catalog.Stations[0].Station.ID)
}

func TestDisplayStationsKeepsSameRunState(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	display, _ := newTestDisplay(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
		stations: func(context.Context) (simulatorapi.StationsSnapshot, error) {
			return fixtureStations(fixtureRunID), nil
		},
	})
	published, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	_, err = display.Stations(t.Context())
	require.NoError(t, err)
	require.Equal(t, published, display.Snapshot())
}

func TestDisplayStationsRejectsInvalidCatalogs(t *testing.T) {
	t.Parallel()

	invalid := fixtureStations(fixtureRunID)
	invalid.Stations[0].Station.LatitudeDegrees = math.Inf(-1)
	display, _ := newTestDisplay(t, funcSource{
		stations: func(context.Context) (simulatorapi.StationsSnapshot, error) {
			return invalid, nil
		},
	})
	_, err := display.Stations(t.Context())
	require.ErrorIs(t, err, simulatorapi.CategorySourceInvalid)
}

func TestDisplayHTTPServesStations(t *testing.T) {
	t.Parallel()

	catalog := fixtureStations(fixtureRunID)
	calls := 0
	handler, _, reported := newTestDisplayHandler(t, funcSource{
		stations: func(context.Context) (simulatorapi.StationsSnapshot, error) {
			calls++
			return catalog, nil
		},
	})

	recorder := displayCall(t, handler, http.MethodGet, "/stations", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.JSONEq(t, encodeFixture(t, catalog), recorder.Body.String())

	require.Equal(t, http.StatusMethodNotAllowed,
		displayCall(t, handler, http.MethodPost, "/stations", `{}`, nil).Code)
	require.Equal(t, http.StatusBadRequest,
		displayCall(t, handler, http.MethodGet, "/stations?all=1", "", nil).Code)
	require.Equal(t, http.StatusBadRequest,
		displayCall(t, handler, http.MethodGet, "/stations", `{}`, nil).Code)
	require.Equal(t, 1, calls, "rejected requests never reach the source")
	require.Empty(t, *reported)
}

func TestDisplayHTTPReportsStationSourceFailures(t *testing.T) {
	t.Parallel()

	handler, _, _ := newTestDisplayHandler(t, funcSource{
		stations: func(context.Context) (simulatorapi.StationsSnapshot, error) {
			return simulatorapi.StationsSnapshot{}, context.DeadlineExceeded
		},
	})
	recorder := displayCall(t, handler, http.MethodGet, "/stations", "", nil)
	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"deadline_exceeded"`)
}

// Regression tests for failed-refresh isolation: an early failure must never
// return another selection's data or a fresh status.

func publishedAlpha(t *testing.T, source *funcSource) (*Display, Snapshot) {
	t.Helper()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	source.snapshot = func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
		return raw, nil
	}
	display, _ := newTestDisplay(t, *source)
	published, err := display.Refresh(t.Context(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, StatusFresh, published.Status)
	return display, published
}

func TestRefreshWithInvalidSelectionReturnsNoOtherSelectionData(t *testing.T) {
	t.Parallel()

	display, published := publishedAlpha(t, &funcSource{})
	for name, selection := range map[string][]string{
		"invalid id":   {"bad id"},
		"too many ids": {"a", "b", "c", "d", "e", "f", "g", "h", "i"},
	} {
		snapshot, err := display.Refresh(t.Context(), selection)
		require.ErrorIs(t, err, simulatorapi.CategoryInvalid, name)
		require.Equal(t, StatusUnavailable, snapshot.Status, name)
		require.Nil(t, snapshot.Observations, name)
		require.NotNil(t, snapshot.Error, name)
		require.Equal(t, published, display.Snapshot(), name)
	}
}

func TestRefreshWithCanceledAdmissionReturnsNoOtherSelectionData(t *testing.T) {
	t.Parallel()

	display, published := publishedAlpha(t, &funcSource{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	snapshot, err := display.Refresh(ctx, []string{"bravo"})
	require.ErrorIs(t, err, simulatorapi.CategoryUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)
	require.Equal(t, published, display.Snapshot())
}

func TestRefreshCanceledWhileWaitingForAdmission(t *testing.T) {
	t.Parallel()

	source := &funcSource{}
	display, published := publishedAlpha(t, source)
	entered := make(chan struct{})
	release := make(chan struct{})
	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(20*time.Second), 4)
	display.source = funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			close(entered)
			<-release
			return raw, nil
		},
	}

	first := make(chan error, 1)
	go func() {
		_, err := display.Refresh(context.Background(), []string{"alpha"})
		first <- err
	}()
	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	second := make(chan Snapshot, 1)
	go func() {
		snapshot, err := display.Refresh(ctx, []string{"bravo"})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("waiting refresh error = %v, want cancellation", err)
		}
		second <- snapshot
	}()
	cancel()
	snapshot := <-second
	require.Equal(t, StatusUnavailable, snapshot.Status)
	require.Nil(t, snapshot.Observations)
	require.Equal(t, published, display.Snapshot(), "the admitted refresh has not published yet")

	close(release)
	require.NoError(t, <-first)
	require.Equal(t, StatusFresh, display.Snapshot().Status)
	require.Equal(t, raw.Now, display.Snapshot().Observations.Now)
}

func TestDisplayHTTPInvalidSelectionEnvelopeCarriesNoOtherSelection(t *testing.T) {
	t.Parallel()

	raw := trackedSnapshot(t, fixtureRunID, fixtureICAO, fixtureStart.Add(10*time.Second), 1)
	handler, display, _ := newTestDisplayHandler(t, funcSource{
		snapshot: func(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
			return raw, nil
		},
	})
	require.Equal(t, http.StatusOK,
		displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["alpha"]}`, nil).Code)
	published := display.Snapshot()

	recorder := displayCall(t, handler, http.MethodPost, "/observations", `{"stationIds":["bad id"]}`, nil)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"unavailable"`)
	require.Contains(t, recorder.Body.String(), `"observations":null`)
	require.NotContains(t, recorder.Body.String(), fixtureRunID)
	require.Equal(t, published, display.Snapshot())
}
