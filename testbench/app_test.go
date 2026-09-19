package testbench_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// Explicit fixture settings; none of them is a package default.

func simulatorConfig(runID string) simulator.Config {
	return simulator.Config{Simulation: simulation.Config{
		ID: runID, StartTime: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC), Seed: 7,
		InitialAircraftCount: 2, SpeedHundredths: 0,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: simulation.Range{Min: 49.9, Max: 50.1}, LongitudeDegrees: simulation.Range{Min: 13.9, Max: 14.1},
			AltitudeFeet: simulation.Range{Min: 30000, Max: 35000}, GroundSpeedKnots: simulation.Range{Min: 300, Max: 400},
			TrackDegrees: simulation.Range{Min: 0, Max: 90}, VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
		},
	}}
}

func apiConfig(t *testing.T) simulator.APIConfig {
	return simulator.APIConfig{
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216, RequestTimeout: 5 * time.Second,
		CoverageReferenceAltitudeFeet: 10000, ReportError: func(err error) { t.Error(err) },
	}
}

func displayConfig(t *testing.T) display.Config {
	return display.Config{
		IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second,
		AltitudeExpiry: 30 * time.Second, VelocityExpiry: 30 * time.Second,
		MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
		RequestTimeout: 5 * time.Second, ReportError: func(err error) { t.Error(err) },
	}
}

func managerSettings() ui.ManagerSettings {
	return ui.ManagerSettings{
		PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000,
		MaxResponseBytes: 1 << 20, ResumeSpeedHundredths: 100,
	}
}

func aircraftSettings() ui.AircraftDisplaySettings {
	return ui.AircraftDisplaySettings{
		PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 1 << 20,
		StationIDs: []string{"alpha"}, FreshFor: 10 * time.Second, LostAfter: time.Minute,
		HistoryPageSize: 50, MaxHistoryRecords: 500,
		InitialLatitudeDegrees: 50, InitialLongitudeDegrees: 14, InitialZoom: 7, Tiles: nil,
	}
}

func combinedConfig(t *testing.T, base, runID string) testbench.Config {
	return testbench.Config{
		PublicBasePath: base, Simulator: simulatorConfig(runID), API: apiConfig(t),
		Display: displayConfig(t), Manager: managerSettings(), Aircraft: aircraftSettings(),
	}
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func stationBody(runID, id string) string {
	return `{"runId":"` + runID + `","station":{"id":"` + id + `","enabled":true,"latitudeDegrees":50,` +
		`"longitudeDegrees":14,"siteElevationMetres":250,"antennaHeightMetres":10,"antennaGainDBi":3,` +
		`"sensitivityDBm":-95,"systemLossDB":2,"frameLossProbability":0}}`
}

func TestRoutesPerMode(t *testing.T) {
	t.Parallel()

	combined, err := testbench.New(combinedConfig(t, "/", "routes-combined"))
	require.NoError(t, err)
	simulatorOnly, err := testbench.NewSimulator(testbench.SimulatorConfig{
		PublicBasePath: "/", Simulator: simulatorConfig("routes-simulator"), API: apiConfig(t), Manager: managerSettings(),
	})
	require.NoError(t, err)
	displayOnly, err := testbench.NewDisplay(testbench.DisplayConfig{
		PublicBasePath: "/", Display: displayConfig(t), Aircraft: aircraftSettings(),
		Source: display.HTTPSourceConfig{
			BaseURL: "http://127.0.0.1:1/api/simulator", Timeout: time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216, Client: &http.Client{},
		},
	})
	require.NoError(t, err)

	cases := []struct {
		path                           string
		combined, simulator, displayed int
	}{
		{"/", 200, 200, 200},
		{"/manager/", 200, 200, 404},
		{"/aircraft/", 200, 404, 200},
		{"/api/simulator/metadata", 200, 200, 404},
		{"/api/display/snapshot", 200, 404, 200},
		{"/assets/ui.css", 200, 200, 200},
		{"/status", 200, 200, 200},
		{"/missing", 404, 404, 404},
		{"/api/simulator", 404, 404, 404},
		{"/assets", 404, 404, 404},
	}
	for _, tc := range cases {
		require.Equal(t, tc.combined, call(t, combined.Handler(), http.MethodGet, tc.path, "").Code, "combined "+tc.path)
		require.Equal(t, tc.simulator, call(t, simulatorOnly.Handler(), http.MethodGet, tc.path, "").Code, "simulator "+tc.path)
		require.Equal(t, tc.displayed, call(t, displayOnly.Handler(), http.MethodGet, tc.path, "").Code, "display "+tc.path)
	}

	index := call(t, combined.Handler(), http.MethodGet, "/", "").Body.String()
	require.Contains(t, index, `href="/manager/"`)
	require.Contains(t, index, `href="/aircraft/"`)
	simulatorIndex := call(t, simulatorOnly.Handler(), http.MethodGet, "/", "").Body.String()
	require.NotContains(t, simulatorIndex, "aircraft/")
}

func TestPublicPathsFollowTheConfiguredPrefix(t *testing.T) {
	t.Parallel()

	app, err := testbench.New(combinedConfig(t, "/bench/a", "prefix-run"))
	require.NoError(t, err)
	host := http.NewServeMux()
	host.Handle("/bench/a/", http.StripPrefix("/bench/a", app.Handler()))

	manager := call(t, host, http.MethodGet, "/bench/a/manager/", "").Body.String()
	require.Contains(t, manager, `"apiBaseUrl":"/bench/a/api/simulator/"`)
	require.Contains(t, manager, `"assetBaseUrl":"/bench/a/assets/"`)
	require.Contains(t, manager, `src="/bench/a/assets/manager-page.js"`)
	aircraft := call(t, host, http.MethodGet, "/bench/a/aircraft/", "").Body.String()
	require.Contains(t, aircraft, `"apiBaseUrl":"/bench/a/api/display/"`)
	index := call(t, host, http.MethodGet, "/bench/a/", "").Body.String()
	require.Contains(t, index, `href="/bench/a/manager/"`)
	require.Contains(t, index, `href="/bench/a/status"`)

	redirect := call(t, host, http.MethodGet, "/bench/a/manager", "")
	require.Equal(t, http.StatusMovedPermanently, redirect.Code)
	require.Equal(t, "/bench/a/manager/", redirect.Header().Get("Location"))
	require.Equal(t, http.StatusNotFound, call(t, host, http.MethodPost, "/bench/a/manager", "{}").Code)

	// The host header never changes rendered URLs.
	request := httptest.NewRequest(http.MethodGet, "/bench/a/manager/", nil)
	request.Host = "evil.example"
	request.Header.Set("X-Forwarded-Prefix", "/other/")
	recorder := httptest.NewRecorder()
	host.ServeHTTP(recorder, request)
	require.Contains(t, recorder.Body.String(), `"apiBaseUrl":"/bench/a/api/simulator/"`)
	require.NotContains(t, recorder.Body.String(), "evil.example")
}

func TestConstructorsValidateBeforeCreating(t *testing.T) {
	t.Parallel()

	bad := combinedConfig(t, "bench/a", "invalid")
	_, err := testbench.New(bad)
	require.ErrorIs(t, err, testbench.ErrInvalidConfig)

	missingCallback := combinedConfig(t, "/", "invalid")
	missingCallback.Display.ReportError = nil
	_, err = testbench.New(missingCallback)
	require.ErrorIs(t, err, testbench.ErrInvalidConfig)

	badSimulation := combinedConfig(t, "/", "")
	_, err = testbench.New(badSimulation)
	require.ErrorIs(t, err, testbench.ErrInvalidConfig)

	_, err = testbench.NewSimulator(testbench.SimulatorConfig{PublicBasePath: "/", Simulator: simulatorConfig("x"), API: apiConfig(t)})
	require.ErrorIs(t, err, testbench.ErrInvalidConfig)

	_, err = testbench.NewDisplay(testbench.DisplayConfig{
		PublicBasePath: "/", Display: displayConfig(t), Aircraft: aircraftSettings(),
		Source: display.HTTPSourceConfig{BaseURL: "ftp://host/", Timeout: time.Second, MaxRequestBytes: 1, MaxResponseBytes: 4096, Client: &http.Client{}},
	})
	require.ErrorIs(t, err, testbench.ErrInvalidConfig)
}

func TestStatusReportsLocalStateWithoutAdvancingTime(t *testing.T) {
	t.Parallel()

	app, err := testbench.New(combinedConfig(t, "/", "status-run"))
	require.NoError(t, err)
	before := call(t, app.Handler(), http.MethodGet, "/api/simulator/metadata", "").Body.String()

	var status testbench.Status
	recorder := call(t, app.Handler(), http.MethodGet, "/status", "")
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &status))
	require.Equal(t, testbench.ModeCombined, status.Mode)
	require.Equal(t, testbench.StateConfigured, status.State)
	require.Equal(t, "status-run", *status.RunID)
	require.Equal(t, display.StatusUnavailable, status.Display.Status)
	require.Equal(t, http.StatusMethodNotAllowed, call(t, app.Handler(), http.MethodPost, "/status", "").Code)

	after := call(t, app.Handler(), http.MethodGet, "/api/simulator/metadata", "").Body.String()
	require.Equal(t, before, after, "status reads do not advance virtual time")
}

func TestRunIsSingleUseAndStopsCleanly(t *testing.T) {
	t.Parallel()

	app, err := testbench.New(combinedConfig(t, "/", "lifecycle-run"))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	require.Eventually(t, func() bool { return app.Status().State == testbench.StateRunning }, 5*time.Second, time.Millisecond)
	require.ErrorIs(t, app.Run(t.Context()), testbench.ErrAlreadyRun)
	cancel()
	require.NoError(t, <-done)
	require.Equal(t, testbench.StateStopped, app.Status().State)
	require.ErrorIs(t, app.Run(t.Context()), testbench.ErrAlreadyRun)
}

func TestDisplayOnlyRunWaitsWithoutPolling(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	t.Cleanup(upstream.Close)
	app, err := testbench.NewDisplay(testbench.DisplayConfig{
		PublicBasePath: "/", Display: displayConfig(t), Aircraft: aircraftSettings(),
		Source: display.HTTPSourceConfig{BaseURL: upstream.URL + "/api/simulator", Timeout: time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216, Client: upstream.Client()},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, app.Run(ctx))
	require.Equal(t, testbench.StateStopped, app.Status().State)
	require.Zero(t, requests.Load(), "neither construction nor Run contacts the upstream")
}

func TestCombinedModeReadsInProcess(t *testing.T) {
	t.Parallel()

	app, err := testbench.New(combinedConfig(t, "/", "in-process-run"))
	require.NoError(t, err)
	handler := app.Handler()
	require.Equal(t, http.StatusCreated, call(t, handler, http.MethodPost, "/api/simulator/stations", stationBody("in-process-run", "alpha")).Code)

	stations := call(t, handler, http.MethodGet, "/api/display/stations", "")
	require.Equal(t, http.StatusOK, stations.Code, stations.Body.String())
	require.Contains(t, stations.Body.String(), `"id":"alpha"`)
	refresh := call(t, handler, http.MethodPost, "/api/display/observations", `{"stationIds":["alpha"]}`)
	require.Equal(t, http.StatusOK, refresh.Code, refresh.Body.String())
	require.Contains(t, refresh.Body.String(), `"runId":"in-process-run"`)
	require.Equal(t, app.SimulatorAPI().RunID(), "in-process-run")
}

func TestDisplayModeUsesHTTPEvidence(t *testing.T) {
	t.Parallel()

	sim, err := testbench.NewSimulator(testbench.SimulatorConfig{
		PublicBasePath: "/sim/", Simulator: simulatorConfig("remote-run"), API: apiConfig(t), Manager: managerSettings(),
	})
	require.NoError(t, err)
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		http.StripPrefix("/sim", sim.Handler()).ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)
	require.Equal(t, http.StatusCreated, call(t, sim.Handler(), http.MethodPost, "/api/simulator/stations", stationBody("remote-run", "alpha")).Code)

	app, err := testbench.NewDisplay(testbench.DisplayConfig{
		PublicBasePath: "/", Display: displayConfig(t), Aircraft: aircraftSettings(),
		Source: display.HTTPSourceConfig{BaseURL: upstream.URL + "/sim/api/simulator", Timeout: 5 * time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216, Client: upstream.Client()},
	})
	require.NoError(t, err)
	require.Zero(t, upstreamCalls.Load())
	refresh := call(t, app.Handler(), http.MethodPost, "/api/display/observations", `{"stationIds":["alpha"]}`)
	require.Equal(t, http.StatusOK, refresh.Code, refresh.Body.String())
	require.Contains(t, refresh.Body.String(), `"runId":"remote-run"`)
	stations := call(t, app.Handler(), http.MethodGet, "/api/display/stations", "")
	require.Equal(t, http.StatusOK, stations.Code, stations.Body.String())
	require.Equal(t, int64(2), upstreamCalls.Load())

	var status testbench.Status
	require.NoError(t, json.Unmarshal(call(t, app.Handler(), http.MethodGet, "/status", "").Body.Bytes(), &status))
	require.Equal(t, testbench.ModeDisplay, status.Mode)
	require.Equal(t, "remote-run", *status.RunID)
	require.Equal(t, display.StatusFresh, status.Display.Status)
	require.Nil(t, app.SimulatorAPI())
}

func TestIndependentBenchesNeverCross(t *testing.T) {
	t.Parallel()

	benchA, err := testbench.New(combinedConfig(t, "/bench/a/", "bench-a"))
	require.NoError(t, err)
	benchB, err := testbench.New(combinedConfig(t, "/bench/b/", "bench-b"))
	require.NoError(t, err)
	host := http.NewServeMux()
	host.Handle("/bench/a/", http.StripPrefix("/bench/a", benchA.Handler()))
	host.Handle("/bench/b/", http.StripPrefix("/bench/b", benchB.Handler()))

	require.Equal(t, http.StatusCreated, call(t, host, http.MethodPost, "/bench/a/api/simulator/stations", stationBody("bench-a", "alpha")).Code)
	require.Equal(t, http.StatusConflict, call(t, host, http.MethodPost, "/bench/b/api/simulator/stations", stationBody("bench-a", "alpha")).Code)
	stationsB := call(t, host, http.MethodGet, "/bench/b/api/simulator/stations", "").Body.String()
	require.Contains(t, stationsB, `"stations":[]`)
	require.Contains(t, stationsB, `"runId":"bench-b"`)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- benchA.Run(ctx) }()
	cancel()
	require.NoError(t, <-done)
	require.Equal(t, testbench.StateConfigured, benchB.Status().State)
	count := call(t, host, http.MethodPut, "/bench/b/api/simulator/aircraft/count", `{"runId":"bench-b","count":0}`)
	require.Equal(t, http.StatusOK, count.Code, count.Body.String())
	stopped := call(t, host, http.MethodPut, "/bench/a/api/simulator/aircraft/count", `{"runId":"bench-a","count":0}`)
	require.Equal(t, http.StatusServiceUnavailable, stopped.Code)

	body, err := io.ReadAll(call(t, host, http.MethodGet, "/bench/b/manager/", "").Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "/bench/b/api/simulator/")
	require.NotContains(t, string(body), "/bench/a/")
	var errorBody simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(call(t, host, http.MethodPost, "/bench/b/api/simulator/stations", stationBody("bench-a", "x")).Body.Bytes(), &errorBody))
	require.Equal(t, "bench-b", errorBody.Error.RunID)
}
