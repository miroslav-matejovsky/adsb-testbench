package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// Integration benches are real testbench applications. Their simulations
// are paused (speed 0) so virtual time never moves on its own; aircraft
// created by a count change emit creation reports that the installed
// stations receive, which gives deterministic received data.

func reportTo(name string) func(error) {
	return func(err error) { slog.Error("bench error", "bench", name, "error", err) }
}

func simulationConfig(runID string) simulator.Config {
	return simulator.Config{Simulation: simulation.Config{
		ID: runID, StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 11,
		InitialAircraftCount: 0, SpeedHundredths: 0,
		Spawn: simulation.SpawnConfig{
			LatitudeDegrees: simulation.Range{Min: 49.8, Max: 50.2}, LongitudeDegrees: simulation.Range{Min: 13.8, Max: 14.2},
			AltitudeFeet: simulation.Range{Min: 20000, Max: 30000}, GroundSpeedKnots: simulation.Range{Min: 300, Max: 400},
			TrackDegrees: simulation.Range{Min: 0, Max: 359}, VerticalRateFeetPerMinute: simulation.Range{Min: -500, Max: 500},
		},
	}}
}

func apiConfig(name string) simulator.APIConfig {
	return simulator.APIConfig{
		MaxRequestBytes: 65536, MaxResponseBytes: 1 << 22, RequestTimeout: 5 * time.Second,
		CoverageReferenceAltitudeFeet: 10000, ReportError: reportTo(name),
	}
}

func displayConfig(name string) display.Config {
	return display.Config{
		IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second, AltitudeExpiry: 30 * time.Second,
		VelocityExpiry: 30 * time.Second, MaxRequestBytes: 65536, MaxResponseBytes: 1 << 22,
		RequestTimeout: 5 * time.Second, ReportError: reportTo(name),
	}
}

func managerSettings() ui.ManagerSettings {
	return ui.ManagerSettings{
		PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 1 << 22,
		ResumeSpeedHundredths: 100,
	}
}

func aircraftSettings() ui.AircraftDisplaySettings {
	return ui.AircraftDisplaySettings{
		PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 1 << 22,
		StationIDs: []string{"alpha"}, FreshFor: 10 * time.Second, LostAfter: time.Minute,
		HistoryPageSize: 5, MaxHistoryRecords: 50,
		InitialLatitudeDegrees: 50, InitialLongitudeDegrees: 14, InitialZoom: 7, Tiles: nil,
	}
}

func installStations(api *simulator.API) error {
	for _, id := range []string{"alpha", "bravo"} {
		_, err := api.AddStation(context.Background(), simulatorapi.AddStationCommand{
			RunID: api.RunID(),
			Station: simulatorapi.StationSettings{
				ID: id, Enabled: true, LatitudeDegrees: 50, LongitudeDegrees: 14, SiteElevationMetres: 300,
				AntennaHeightMetres: 20, AntennaGainDBi: 3, SensitivityDBm: -100, SystemLossDB: 1, FrameLossProbability: 0,
			},
		})
		if err != nil {
			return fmt.Errorf("install station %s: %w", id, err)
		}
	}
	return nil
}

// bench is one replaceable integration application mounted at a prefix.
// The test-only restart action swaps in a new App with a new run ID.
type bench struct {
	name    string
	prefix  string
	build   func(runID string) (*testbench.App, error)
	mu      sync.Mutex
	app     *testbench.App
	stop    context.CancelFunc
	restart int
}

func (b *bench) start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	runID := fmt.Sprintf("%s-run-%d", b.name, b.restart+1)
	app, err := b.build(runID)
	if err != nil {
		return err
	}
	if api := app.SimulatorAPI(); api != nil {
		if err := installStations(api); err != nil {
			return err
		}
	}
	if b.stop != nil {
		b.stop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := app.Run(ctx); err != nil {
			slog.Error("bench stopped", "bench", b.name, "error", err)
		}
	}()
	b.app, b.stop = app, cancel
	b.restart++
	return nil
}

func (b *bench) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	app := b.app
	b.mu.Unlock()
	http.StripPrefix(strings.TrimSuffix(b.prefix, "/"), app.Handler()).ServeHTTP(w, r)
}

func combinedBench(name, prefix string) *bench {
	return &bench{name: name, prefix: prefix, build: func(runID string) (*testbench.App, error) {
		return testbench.New(testbench.Config{
			PublicBasePath: prefix, Simulator: simulationConfig(runID), API: apiConfig(name),
			Display: displayConfig(name), Manager: managerSettings(), Aircraft: aircraftSettings(),
		})
	}}
}

func simulatorBench(name, prefix string) *bench {
	return &bench{name: name, prefix: prefix, build: func(runID string) (*testbench.App, error) {
		return testbench.NewSimulator(testbench.SimulatorConfig{
			PublicBasePath: prefix, Simulator: simulationConfig(runID), API: apiConfig(name), Manager: managerSettings(),
		})
	}}
}

func displayBench(name, prefix, sourceBase string) *bench {
	return &bench{name: name, prefix: prefix, build: func(string) (*testbench.App, error) {
		return testbench.NewDisplay(testbench.DisplayConfig{
			PublicBasePath: prefix, Display: displayConfig(name), Aircraft: aircraftSettings(),
			Source: display.HTTPSourceConfig{
				BaseURL: sourceBase, Timeout: 4 * time.Second, MaxRequestBytes: 65536, MaxResponseBytes: 1 << 22,
				Client: &http.Client{Transport: &http.Transport{MaxConnsPerHost: 16}},
			},
		})
	}}
}
