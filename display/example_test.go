package display_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// exampleProvider stands in for a simulator service. A real host passes
// simulator.NewAPI(...), which satisfies display.ObservationSource and
// display.StationSource without either package importing the other.
type exampleProvider struct{}

func (exampleProvider) ReceptionSnapshot(context.Context, simulatorapi.ReceptionSnapshotRequest) (simulatorapi.ReceptionSnapshot, error) {
	return simulatorapi.ReceptionSnapshot{
		RunID: "example-run", Now: "2024-03-05T12:00:00Z",
		StationIDs: []string{}, Retention: []simulatorapi.StationRetention{},
		Records: []simulatorapi.Reception{},
	}, nil
}

func (exampleProvider) ReceptionHistory(context.Context, simulatorapi.HistoryRequest) (simulatorapi.ReceptionPage, error) {
	return simulatorapi.ReceptionPage{}, nil
}

func (exampleProvider) Stations(context.Context) (simulatorapi.StationsSnapshot, error) {
	return simulatorapi.StationsSnapshot{
		RunID: "example-run", Now: "2024-03-05T12:00:00Z", Stations: []simulatorapi.StationState{},
	}, nil
}

// exampleDisplayConfig is the shared JSON settings document of the examples.
// Every value is a host choice; the parser supplies no default.
const exampleDisplayConfig = `{
  "identityExpiryNanoseconds": "60000000000",
  "positionExpiryNanoseconds": "30000000000",
  "altitudeExpiryNanoseconds": "30000000000",
  "velocityExpiryNanoseconds": "30000000000",
  "maxRequestBytes": 65536,
  "maxResponseBytes": 16777216,
  "requestTimeoutNanoseconds": "5000000000"
}`

// ExampleNewInProcessSource builds a display over a provider in the same
// process and mounts its browser-facing routes below a prefix.
func ExampleNewInProcessSource() {
	source, err := display.NewInProcessSource(exampleProvider{})
	if err != nil {
		log.Fatal(err)
	}
	stations, err := display.NewInProcessStationSource(exampleProvider{})
	if err != nil {
		log.Fatal(err)
	}

	// The error callback is a Go dependency, not a serializable setting.
	config, err := display.ParseConfig(strings.NewReader(exampleDisplayConfig), 4096,
		func(err error) { log.Print(err) })
	if err != nil {
		log.Fatal(err)
	}

	backend, err := display.New(config, source, stations)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/bench/a/display/", http.StripPrefix("/bench/a/display", backend.Handler()))

	snapshot, err := backend.Refresh(context.Background(), []string{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snapshot.Status)
	fmt.Println(snapshot.Observations.RunID)
	fmt.Println(len(snapshot.Observations.Aircraft))
	// Output:
	// fresh
	// example-run
	// 0
}

// ExampleNewHTTPSource reads the same data from a mounted simulator over
// HTTP. The host owns the client, which is passed separately from settings.
func ExampleNewHTTPSource() {
	// A local stand-in for a mounted simulator handler.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"runId":"example-run","now":"2024-03-05T12:00:00Z",` +
			`"stationIds":[],"retention":[],"records":[]}`))
	}))
	defer upstream.Close()

	sourceConfig, err := display.ParseHTTPSourceConfig(strings.NewReader(`{
	  "baseUrl": "`+upstream.URL+`/bench/a/simulator",
	  "timeoutNanoseconds": "5000000000",
	  "maxRequestBytes": 65536,
	  "maxResponseBytes": 16777216
	}`), 4096, upstream.Client())
	if err != nil {
		log.Fatal(err)
	}

	source, err := display.NewHTTPSource(sourceConfig)
	if err != nil {
		log.Fatal(err)
	}

	config, err := display.ParseConfig(strings.NewReader(exampleDisplayConfig), 4096,
		func(err error) { log.Print(err) })
	if err != nil {
		log.Fatal(err)
	}

	// One HTTP source serves both raw evidence and station discovery.
	backend, err := display.New(config, source, source)
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := backend.Refresh(context.Background(), []string{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snapshot.Status)
	// Output:
	// /bench/a/simulator/observations/receptions
	// fresh
}
