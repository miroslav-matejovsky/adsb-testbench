// Package main is an external host that embeds two independent ADS-B test
// benches and a custom page built from the public UI components.
//
// It imports only public packages of the test bench module. The host owns
// its listener, logger, signal handling and shutdown; the benches only
// provide handlers and Run.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// customPage lays out its own page and mounts one manager from bench A and
// one aircraft display from bench B with explicit URLs.
const customPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Host dashboard</title>
</head>
<body>
<h1>Host dashboard</h1>
<div id="manager-a"></div>
<div id="aircraft-b"></div>
<script type="module">
import { mountManager, mountAircraftDisplay } from "/bench/a/assets/components.js";
const common = { pollIntervalMilliseconds: 1000, requestTimeoutMilliseconds: 5000, maxResponseBytes: 1048576 };
window.hostComponents = [
  mountManager(document.getElementById("manager-a"), {
    ...common, apiBaseUrl: "/bench/a/api/simulator/", assetBaseUrl: "/bench/a/assets/", resumeSpeedHundredths: 100,
  }),
  mountAircraftDisplay(document.getElementById("aircraft-b"), {
    ...common, apiBaseUrl: "/bench/b/api/display/", assetBaseUrl: "/bench/b/assets/", stationIds: [],
    freshForNanoseconds: "10000000000", lostAfterNanoseconds: "60000000000", historyPageSize: 50,
    maxHistoryRecords: 500, initialLatitudeDegrees: 50, initialLongitudeDegrees: 14, initialZoom: 7, tiles: null,
  }),
];
</script>
</body>
</html>
`

// benchConfig returns a complete combined configuration. Every value is an
// explicit host choice.
func benchConfig(base, runID string, logger *slog.Logger) testbench.Config {
	report := func(err error) { logger.Error("bench reported an error", "bench", base, "error", err) }
	return testbench.Config{
		PublicBasePath: base,
		Simulator: simulator.Config{Simulation: simulation.Config{
			ID: runID, StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 42,
			InitialAircraftCount: 3, SpeedHundredths: 100,
			Spawn: simulation.SpawnConfig{
				LatitudeDegrees: simulation.Range{Min: 49.5, Max: 50.5}, LongitudeDegrees: simulation.Range{Min: 13.5, Max: 14.5},
				AltitudeFeet: simulation.Range{Min: 5000, Max: 38000}, GroundSpeedKnots: simulation.Range{Min: 150, Max: 480},
				TrackDegrees: simulation.Range{Min: 0, Max: 359}, VerticalRateFeetPerMinute: simulation.Range{Min: -1500, Max: 1500},
			},
		}},
		API: simulator.APIConfig{
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216, RequestTimeout: 5 * time.Second,
			CoverageReferenceAltitudeFeet: 10000, ReportError: report,
		},
		Display: display.Config{
			IdentityExpiry: time.Minute, PositionExpiry: 30 * time.Second, AltitudeExpiry: 30 * time.Second,
			VelocityExpiry: 30 * time.Second, MaxRequestBytes: 65536, MaxResponseBytes: 16777216,
			RequestTimeout: 5 * time.Second, ReportError: report,
		},
		Manager: ui.ManagerSettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 1 << 20,
			ResumeSpeedHundredths: 100,
		},
		Aircraft: ui.AircraftDisplaySettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000, MaxResponseBytes: 1 << 20,
			StationIDs: []string{}, FreshFor: 10 * time.Second, LostAfter: time.Minute,
			HistoryPageSize: 50, MaxHistoryRecords: 500,
			InitialLatitudeDegrees: 50, InitialLongitudeDegrees: 14, InitialZoom: 7, Tiles: nil,
		},
	}
}

// host is two independent benches and the custom page on one mux.
type host struct {
	handler http.Handler
	benches []*testbench.App
}

// newHost composes both benches. runIDs must be fresh for every call.
func newHost(logger *slog.Logger, runA, runB string) (*host, error) {
	benchA, err := testbench.New(benchConfig("/bench/a/", runA, logger))
	if err != nil {
		return nil, fmt.Errorf("bench a: %w", err)
	}
	benchB, err := testbench.New(benchConfig("/bench/b/", runB, logger))
	if err != nil {
		return nil, fmt.Errorf("bench b: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/bench/a/", http.StripPrefix("/bench/a", benchA.Handler()))
	mux.Handle("/bench/b/", http.StripPrefix("/bench/b", benchB.Handler()))
	mux.HandleFunc("/custom/{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(customPage))
	})
	return &host{handler: mux, benches: []*testbench.App{benchA, benchB}}, nil
}

// run supervises every bench until ctx is canceled and joins them. Any
// bench failure cancels the others; all failures are returned.
func (h *host) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var group sync.WaitGroup
	errs := make([]error, len(h.benches))
	for index, bench := range h.benches {
		group.Go(func() {
			if err := bench.Run(ctx); err != nil {
				errs[index] = err
				cancel()
			}
		})
	}
	group.Wait()
	return errors.Join(errs...)
}
