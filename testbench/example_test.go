package testbench_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// ExampleNew mounts a combined bench below a nested prefix on a host-owned
// mux, runs it, and stops it through cancellation.
func ExampleNew() {
	report := func(err error) { log.Print(err) }
	app, err := testbench.New(testbench.Config{
		PublicBasePath: "/bench/a/",
		Simulator: simulator.Config{Simulation: simulation.Config{
			// A fresh ID per App: it identifies one engine lifetime.
			ID: "example-bench-a", StartTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Seed: 1,
			InitialAircraftCount: 1, SpeedHundredths: 100,
			Spawn: simulation.SpawnConfig{
				LatitudeDegrees: simulation.Range{Min: 50, Max: 50}, LongitudeDegrees: simulation.Range{Min: 14, Max: 14},
				AltitudeFeet: simulation.Range{Min: 30000, Max: 30000}, GroundSpeedKnots: simulation.Range{Min: 400, Max: 400},
				TrackDegrees: simulation.Range{Min: 90, Max: 90}, VerticalRateFeetPerMinute: simulation.Range{Min: 0, Max: 0},
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
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/bench/a/", http.StripPrefix("/bench/a", app.Handler()))
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	response, err := server.Client().Get(server.URL + "/bench/a/manager/")
	if err != nil {
		log.Fatal(err)
	}
	_ = response.Body.Close()
	fmt.Println(response.StatusCode)

	cancel()
	fmt.Println(<-done)
	// Output:
	// 200
	// <nil>
}
