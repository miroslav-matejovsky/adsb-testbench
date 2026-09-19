package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/processtest"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// Smoke tests of the real command in a child process; see
// cmd/adsb-testbench for the waiting and cleanup rules.

func TestMain(m *testing.M) { processtest.Main(m, main) }

func TestSimulatorProcessServesItsAPI(t *testing.T) {
	t.Parallel()

	path := processtest.Config(t, "simulator.json", func(document map[string]any) {
		processtest.Section(document, "server")["listenAddress"] = "127.0.0.1:0"
	})
	process := processtest.Start(t, "-config", path, "-config-max-bytes", "65536")
	require.NotEmpty(t, process.Address, process.Log())

	var status testbench.Status
	processtest.GetJSON(t, "http://"+process.Address+"/status", &status)
	require.Equal(t, testbench.ModeSimulator, status.Mode)
	require.Nil(t, status.Display)
	var stations simulatorapi.StationsSnapshot
	processtest.GetJSON(t, "http://"+process.Address+"/api/simulator/stations", &stations)
	require.Equal(t, *status.RunID, stations.RunID)
	require.True(t, strings.HasPrefix(stations.RunID, "local-bench-"))
	require.Len(t, stations.Stations, 2)
}

func TestSimulatorProcessRejectsDisplaySections(t *testing.T) {
	t.Parallel()

	path := processtest.Config(t, "simulator.json", func(document map[string]any) {
		document["display"] = map[string]any{}
	})
	process := processtest.Start(t, "-config", path, "-config-max-bytes", "65536")
	require.Equal(t, 1, process.Wait(t))
}
