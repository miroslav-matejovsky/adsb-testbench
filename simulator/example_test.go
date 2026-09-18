package simulator_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
)

// ExampleNewAPI builds a simulator service from explicit settings and mounts
// its routes below a prefix. Every setting is a host choice; the constructors
// supply no default.
func ExampleNewAPI() {
	config, err := simulator.ParseConfig(strings.NewReader(`{
	  "simulation": {
	    "id": "example-run",
	    "startTime": "2024-03-05T12:00:00Z",
	    "seed": "7",
	    "initialAircraftCount": 0,
	    "speedHundredths": 100,
	    "spawn": {
	      "latitudeDegrees": {"min": 49, "max": 51},
	      "longitudeDegrees": {"min": 13, "max": 15},
	      "altitudeFeet": {"min": 30000, "max": 36000},
	      "groundSpeedKnots": {"min": 400, "max": 500},
	      "trackDegrees": {"min": 0, "max": 360},
	      "verticalRateFeetPerMinute": {"min": -1000, "max": 1000}
	    }
	  }
	}`), 8192)
	if err != nil {
		log.Fatal(err)
	}

	runtime, err := simulator.New(config)
	if err != nil {
		log.Fatal(err)
	}

	// The error callback is a Go dependency, not a serializable setting.
	reportError := func(err error) { log.Print(err) }
	apiConfig, err := simulator.ParseAPIConfig(strings.NewReader(`{
	  "maxRequestBytes": 65536,
	  "maxResponseBytes": 16777216,
	  "requestTimeoutNanoseconds": "5000000000",
	  "coverageReferenceAltitudeFeet": 35000
	}`), 4096, reportError)
	if err != nil {
		log.Fatal(err)
	}

	api, err := simulator.NewAPI(runtime, apiConfig)
	if err != nil {
		log.Fatal(err)
	}

	// The host owns the listener. Routes are relative, so any prefix works.
	mux := http.NewServeMux()
	mux.Handle("/bench/a/simulator/", http.StripPrefix("/bench/a/simulator", api.Handler()))

	server := httptest.NewServer(mux)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/bench/a/simulator/metadata")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	fmt.Println(api.RunID())
	fmt.Println(response.StatusCode)
	fmt.Println(response.Header.Get("Cache-Control"))
	// Output:
	// example-run
	// 200
	// no-store
}

// ExampleAPIConfig shows the same service settings supplied as Go values,
// including the required host error callback.
func ExampleAPIConfig() {
	config := simulator.APIConfig{
		MaxRequestBytes:               65536,
		MaxResponseBytes:              16777216,
		RequestTimeout:                5 * time.Second,
		CoverageReferenceAltitudeFeet: 35000,
		ReportError:                   func(err error) { log.Print(err) },
	}
	fmt.Println(config.Validate())
	// Output: <nil>
}
