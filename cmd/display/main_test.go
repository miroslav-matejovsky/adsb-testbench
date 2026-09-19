package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/processtest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// Smoke tests of the real command in a child process; see
// cmd/adsb-testbench for the waiting and cleanup rules. The upstream is an
// in-process simulator application served by httptest.

func TestMain(m *testing.M) { processtest.Main(m, main) }

func upstream(t *testing.T) string {
	t.Helper()

	app, err := testbench.NewSimulator(testbench.SimulatorConfig{
		PublicBasePath: "/sim/",
		Simulator: simulator.Config{Simulation: simulation.Config{
			ID: "display-smoke-upstream", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 3,
			InitialAircraftCount: 0, SpeedHundredths: 0,
			Spawn: simulation.SpawnConfig{
				LatitudeDegrees: simulation.Range{Min: 50, Max: 50}, LongitudeDegrees: simulation.Range{Min: 14, Max: 14},
				AltitudeFeet: simulation.Range{Min: 30000, Max: 30000}, GroundSpeedKnots: simulation.Range{Min: 400, Max: 400},
				TrackDegrees: simulation.Range{Min: 90, Max: 90}, VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
			},
		}},
		API: simulator.APIConfig{
			MaxRequestBytes: 65536, MaxResponseBytes: 4194304, RequestTimeout: 5 * time.Second,
			CoverageReferenceAltitudeFeet: 10000, ReportError: func(err error) { t.Error(err) },
		},
		Manager: ui.ManagerSettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 4194304,
			ResumeSpeedHundredths: 100,
		},
	})
	require.NoError(t, err)
	server := httptest.NewServer(http.StripPrefix("/sim", app.Handler()))
	t.Cleanup(server.Close)
	return server.URL + "/sim/api/simulator"
}

func TestDisplayProcessReadsTheUpstreamOverHTTP(t *testing.T) {
	t.Parallel()

	base := upstream(t)
	path := processtest.Config(t, "display.json", func(document map[string]any) {
		processtest.Section(document, "server")["listenAddress"] = "127.0.0.1:0"
		processtest.Section(document, "server")["publicBasePath"] = "/view/"
		processtest.Section(document, "source")["baseUrl"] = base
	})
	process := processtest.Start(t, "-config", path, "-config-max-bytes", "65536")
	require.NotEmpty(t, process.Address, process.Log())

	var status testbench.Status
	processtest.GetJSON(t, "http://"+process.Address+"/view/status", &status)
	require.Equal(t, testbench.ModeDisplay, status.Mode)
	var stations simulatorapi.StationsSnapshot
	processtest.GetJSON(t, "http://"+process.Address+"/view/api/display/stations", &stations)
	require.Equal(t, "display-smoke-upstream", stations.RunID)
}

func TestDisplayProcessStartsWhileTheUpstreamIsDown(t *testing.T) {
	t.Parallel()

	path := processtest.Config(t, "display.json", func(document map[string]any) {
		processtest.Section(document, "server")["listenAddress"] = "127.0.0.1:0"
		processtest.Section(document, "source")["baseUrl"] = "http://127.0.0.1:1/api/simulator"
	})
	process := processtest.Start(t, "-config", path, "-config-max-bytes", "65536")
	require.NotEmpty(t, process.Address, "an unavailable source is an operational state, not a startup failure")
	var status testbench.Status
	processtest.GetJSON(t, "http://"+process.Address+"/status", &status)
	require.Equal(t, testbench.StateRunning, status.State)
}
