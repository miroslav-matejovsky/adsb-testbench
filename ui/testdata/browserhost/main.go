// Command browserhost serves the real embedded UI pages and assets for the
// Playwright browser tests. See doc.go.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

//go:embed harness.html harness.js components.html
var harnessFiles embed.FS

// onePixelPNG is a valid 1x1 transparent PNG used as a deterministic tile.
var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("browserhost", flag.ContinueOnError)
	listen := flags.String("listen", "", "loopback host:port of presentation pages and nested benches (required)")
	listenCombined := flags.String("listen-combined", "", "loopback host:port of the root combined bench (required)")
	listenSimulator := flags.String("listen-simulator", "", "loopback host:port of the separate simulators (required)")
	listenDisplay := flags.String("listen-display", "", "loopback host:port of the separate displays (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	addresses := []string{*listen, *listenCombined, *listenSimulator, *listenDisplay}
	for _, address := range addresses {
		if address == "" {
			return errors.New("-listen, -listen-combined, -listen-simulator and -listen-display are required")
		}
	}
	listeners := make([]net.Listener, 0, len(addresses))
	for _, address := range addresses {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
	}
	simulatorOrigin := "http://" + listeners[2].Addr().String()
	handlers, err := hosts(simulatorOrigin)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	errs := make(chan error, len(listeners))
	servers := make([]*http.Server, len(listeners))
	for index, listener := range listeners {
		server := &http.Server{Handler: handlers[index], ReadHeaderTimeout: 10 * time.Second}
		servers[index] = server
		go func() { errs <- server.Serve(listener) }()
	}
	slog.Info("browserhost listening", "addresses", addresses)
	select {
	case <-ctx.Done():
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, server := range servers {
		_ = server.Shutdown(shutdown)
	}
	return nil
}

// hosts returns the handlers of the four listeners: presentation pages
// with nested integration benches, the root combined bench, the separate
// simulators, and the separate displays reading those simulators.
func hosts(simulatorOrigin string) ([]http.Handler, error) {
	presentation, err := routes()
	if err != nil {
		return nil, err
	}
	benches := map[string]*bench{
		"a": combinedBench("a", "/int/a/"),
		"c": combinedBench("c", "/int/c/"),
		"d": combinedBench("d", "/int/d/"),
		"e": combinedBench("e", "/int/e/"),
	}
	for _, name := range []string{"a", "c", "d", "e"} {
		if err := benches[name].start(); err != nil {
			return nil, err
		}
		presentation.Handle(benches[name].prefix, benches[name])
	}
	presentation.HandleFunc("/int/components/{$}", func(w http.ResponseWriter, _ *http.Request) {
		serveFile(w, "components.html", "text/html; charset=utf-8")
	})
	presentation.HandleFunc("POST /fixture/restart", func(w http.ResponseWriter, r *http.Request) {
		target, ok := benches[r.URL.Query().Get("bench")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := target.start(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	combined := combinedBench("combined-root", "/")
	simulatorRoot := simulatorBench("separate-root", "/")
	simulatorNested := simulatorBench("separate-nested", "/nested/sim/")
	displayRoot := displayBench("display-root", "/", simulatorOrigin+"/api/simulator")
	displayNested := displayBench("display-nested", "/nested/display/", simulatorOrigin+"/nested/sim/api/simulator")
	for _, b := range []*bench{combined, simulatorRoot, simulatorNested, displayRoot, displayNested} {
		if err := b.start(); err != nil {
			return nil, err
		}
	}
	simulators := http.NewServeMux()
	simulators.Handle("/", simulatorRoot)
	simulators.Handle("/nested/sim/", simulatorNested)
	displays := http.NewServeMux()
	displays.Handle("/", displayRoot)
	displays.Handle("/nested/display/", displayNested)
	return []http.Handler{presentation, combined, simulators, displays}, nil
}

func managerConfig(prefix string) ui.ManagerConfig {
	return ui.ManagerConfig{
		APIBaseURL: prefix + "api/simulator/", AssetBaseURL: prefix + "assets/",
		ManagerSettings: ui.ManagerSettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000,
			MaxResponseBytes: 1 << 20, ResumeSpeedHundredths: 100,
		},
	}
}

func aircraftConfig(prefix string, tiles *ui.Tiles) ui.AircraftDisplayConfig {
	return ui.AircraftDisplayConfig{
		APIBaseURL: prefix + "api/display/", AssetBaseURL: prefix + "assets/",
		AircraftDisplaySettings: ui.AircraftDisplaySettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000,
			MaxResponseBytes: 1 << 20, StationIDs: []string{"alpha"},
			FreshFor: 10 * time.Second, LostAfter: 60 * time.Second,
			HistoryPageSize: 3, MaxHistoryRecords: 6,
			InitialLatitudeDegrees: 50, InitialLongitudeDegrees: 14, InitialZoom: 7,
			Tiles: tiles,
		},
	}
}

// routes mounts the pages at the root and below the nested prefix
// /bench/a/, the shared assets, the harness page and local tiles. API
// routes are not served: tests fulfil them with controlled responses.
func routes() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready"))
	})
	localTiles := &ui.Tiles{
		URLTemplate: "/tiles/{z}/{x}/{y}.png", AttributionText: "Local test tiles",
		AttributionURL: "https://example.test/tiles", MinZoom: 0, MaxZoom: 18,
	}
	for _, prefix := range []string{"/", "/bench/a/"} {
		manager, err := ui.NewManager(managerConfig(prefix))
		if err != nil {
			return nil, err
		}
		aircraft, err := ui.NewAircraftDisplay(aircraftConfig(prefix, nil))
		if err != nil {
			return nil, err
		}
		tiled, err := ui.NewAircraftDisplay(aircraftConfig(prefix, localTiles))
		if err != nil {
			return nil, err
		}
		strip := strings.TrimSuffix(prefix, "/")
		mux.Handle(prefix+"manager/", http.StripPrefix(strip+"/manager", manager))
		mux.Handle(prefix+"aircraft/", http.StripPrefix(strip+"/aircraft", aircraft))
		mux.Handle(prefix+"aircraft-tiles/", http.StripPrefix(strip+"/aircraft-tiles", tiled))
		mux.Handle(prefix+"assets/", http.StripPrefix(strip+"/assets", ui.Assets()))
	}
	mux.HandleFunc("/harness/", serveHarness)
	mux.HandleFunc("/tiles/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(onePixelPNG)
	})
	return mux, nil
}

func serveHarness(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/harness/harness.js":
		serveFile(w, "harness.js", "text/javascript; charset=utf-8")
	case "/harness/":
		serveFile(w, "harness.html", "text/html; charset=utf-8")
	default:
		http.NotFound(w, r)
	}
}

func serveFile(w http.ResponseWriter, name, contentType string) {
	data, err := harnessFiles.ReadFile(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
