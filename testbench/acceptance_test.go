package testbench_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// pair is one paused combined bench served over HTTP and a display-only
// bench reading that bench's simulator API. Both displays therefore observe
// the same source state: one in process, one over HTTP.
type pair struct {
	combined *testbench.App
	remote   *testbench.App
	runID    string
}

func newPair(t *testing.T, runID string) pair {
	t.Helper()

	combined, err := testbench.New(combinedConfig(t, "/bench/a/", runID))
	require.NoError(t, err)
	host := http.NewServeMux()
	host.Handle("/bench/a/", http.StripPrefix("/bench/a", combined.Handler()))
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)

	remote, err := testbench.NewDisplay(testbench.DisplayConfig{
		PublicBasePath: "/", Display: displayConfig(t), Aircraft: aircraftSettings(),
		Source: display.HTTPSourceConfig{
			BaseURL: server.URL + "/bench/a/api/simulator", Timeout: 5 * time.Second,
			MaxRequestBytes: 65536, MaxResponseBytes: 16777216, Client: server.Client(),
		},
	})
	require.NoError(t, err)

	// The simulation is paused (speed 0). Aircraft created after a station
	// exists emit creation reports that the station receives, so received
	// evidence exists without any virtual time passing.
	api := combined.SimulatorAPI()
	for _, id := range []string{"alpha", "bravo"} {
		require.Equal(t, http.StatusCreated,
			call(t, combined.Handler(), http.MethodPost, "/api/simulator/stations", stationBody(runID, id)).Code)
	}
	_, err = api.SetCount(t.Context(), simulatorapi.CountCommand{RunID: runID, Count: 6})
	require.NoError(t, err)
	return pair{combined: combined, remote: remote, runID: runID}
}

// refresh posts one selection to a display and decodes the response.
func refresh(t *testing.T, app *testbench.App, body string) (int, display.RefreshResponse) {
	t.Helper()

	recorder := call(t, app.Handler(), http.MethodPost, "/api/display/observations", body)
	var response display.RefreshResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
}

func TestInProcessAndHTTPDisplaysReceiveIdenticalData(t *testing.T) {
	t.Parallel()

	benches := newPair(t, "parity-run")
	for _, selection := range []string{`{"stationIds":["alpha"]}`, `{"stationIds":["alpha","bravo"]}`, `{"stationIds":[]}`} {
		localCode, local := refresh(t, benches.combined, selection)
		remoteCode, remote := refresh(t, benches.remote, selection)
		require.Equal(t, http.StatusOK, localCode, selection)
		require.Equal(t, localCode, remoteCode, selection)
		require.Equal(t, local.Snapshot.Status, remote.Snapshot.Status, selection)
		require.Nil(t, local.Error)
		require.Nil(t, remote.Error)
		// Only the real update timestamps may differ.
		require.Equal(t, local.Snapshot.Observations, remote.Snapshot.Observations, selection)
		require.Equal(t, "parity-run", local.Snapshot.Observations.RunID)
	}
	_, withAircraft := refresh(t, benches.combined, `{"stationIds":["alpha"]}`)
	require.NotEmpty(t, withAircraft.Snapshot.Observations.Aircraft, "creation reports were received")
	aircraft := withAircraft.Snapshot.Observations.Aircraft[0]
	require.NotNil(t, aircraft.Identity)
	require.Nil(t, aircraft.Position, "one CPR half cannot produce a received position")

	localStations := call(t, benches.combined.Handler(), http.MethodGet, "/api/display/stations", "")
	remoteStations := call(t, benches.remote.Handler(), http.MethodGet, "/api/display/stations", "")
	require.Equal(t, http.StatusOK, localStations.Code)
	require.JSONEq(t, localStations.Body.String(), remoteStations.Body.String())

	history := `{"stationId":"alpha","cursor":null,"limit":5}`
	localHistory := call(t, benches.combined.Handler(), http.MethodPost, "/api/display/receptions/history", history)
	remoteHistory := call(t, benches.remote.Handler(), http.MethodPost, "/api/display/receptions/history", history)
	require.Equal(t, http.StatusOK, localHistory.Code, localHistory.Body.String())
	require.JSONEq(t, localHistory.Body.String(), remoteHistory.Body.String())
}

func TestInProcessAndHTTPDisplaysClassifyFailuresIdentically(t *testing.T) {
	t.Parallel()

	benches := newPair(t, "failure-parity-run")
	localCode, local := refresh(t, benches.combined, `{"stationIds":["zulu"]}`)
	remoteCode, remote := refresh(t, benches.remote, `{"stationIds":["zulu"]}`)
	require.Equal(t, http.StatusNotFound, localCode)
	require.Equal(t, localCode, remoteCode)
	require.Equal(t, local.Error.Code, remote.Error.Code)
	require.Equal(t, local.Error.RunID, remote.Error.RunID)
	require.Equal(t, display.StatusUnavailable, local.Snapshot.Status)
	require.Equal(t, display.StatusUnavailable, remote.Snapshot.Status)

	cursor := `{"stationId":"alpha","cursor":{"runId":"old-run","stationId":"alpha","afterSequence":"1"},"limit":5}`
	localHistory := call(t, benches.combined.Handler(), http.MethodPost, "/api/display/receptions/history", cursor)
	remoteHistory := call(t, benches.remote.Handler(), http.MethodPost, "/api/display/receptions/history", cursor)
	require.Equal(t, http.StatusConflict, localHistory.Code, localHistory.Body.String())
	require.Equal(t, localHistory.Code, remoteHistory.Code)
	var localError, remoteError simulatorapi.ErrorResponse
	require.NoError(t, json.Unmarshal(localHistory.Body.Bytes(), &localError))
	require.NoError(t, json.Unmarshal(remoteHistory.Body.Bytes(), &remoteError))
	require.Equal(t, localError.Error.Code, remoteError.Error.Code)
	require.Equal(t, "failure-parity-run", remoteError.Error.RunID)
}
