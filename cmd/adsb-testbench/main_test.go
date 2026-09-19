package main

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/processtest"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// These smoke tests run the real command in a child process. They wait for
// the command's own "serving" log line and its status route, never for a
// fixed delay, and kill any child still running at cleanup.

func TestMain(m *testing.M) { processtest.Main(m, main) }

func config(t *testing.T, change func(map[string]any)) string {
	t.Helper()

	return processtest.Config(t, "combined.json", func(document map[string]any) {
		processtest.Section(document, "server")["listenAddress"] = "127.0.0.1:0"
		change(document)
	})
}

func start(t *testing.T, path string) *processtest.Process {
	t.Helper()

	process := processtest.Start(t, "-config", path, "-config-max-bytes", "65536")
	require.NotEmpty(t, process.Address, process.Log())
	return process
}

func TestCombinedProcessServesTheConfiguredPrefix(t *testing.T) {
	t.Parallel()

	process := start(t, config(t, func(document map[string]any) {
		processtest.Section(document, "server")["publicBasePath"] = "/bench/x/"
	}))
	var status testbench.Status
	processtest.GetJSON(t, "http://"+process.Address+"/bench/x/status", &status)
	require.Equal(t, testbench.ModeCombined, status.Mode)
	require.Equal(t, testbench.StateRunning, status.State)
	require.True(t, strings.HasPrefix(*status.RunID, "local-bench-"), *status.RunID)
	require.Contains(t, process.Log(), "runId="+*status.RunID)
}

func TestEveryLaunchGetsAFreshRunIdentity(t *testing.T) {
	t.Parallel()

	path := config(t, func(map[string]any) {})
	var first, second testbench.Status
	processtest.GetJSON(t, "http://"+start(t, path).Address+"/status", &first)
	processtest.GetJSON(t, "http://"+start(t, path).Address+"/status", &second)
	require.NotEqual(t, *first.RunID, *second.RunID)
}

func TestOccupiedAddressFailsWithBindCause(t *testing.T) {
	t.Parallel()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { require.NoError(t, occupied.Close()) }()
	process := processtest.Start(t, "-config", config(t, func(document map[string]any) {
		processtest.Section(document, "server")["listenAddress"] = occupied.Addr().String()
	}), "-config-max-bytes", "65536")
	require.Empty(t, process.Address)
	require.Equal(t, 1, process.Wait(t))
	require.Contains(t, process.Log(), "bind "+occupied.Addr().String())
}

func TestInvalidInputExitsWithoutServing(t *testing.T) {
	t.Parallel()

	invalid := processtest.Start(t, "-config", config(t, func(document map[string]any) {
		document["unknown"] = true
	}), "-config-max-bytes", "65536")
	require.Equal(t, 1, invalid.Wait(t))
	require.Contains(t, invalid.Log(), `unknown section "unknown"`)

	usage := processtest.Start(t, "-config", "missing.json")
	require.Equal(t, 2, usage.Wait(t))

	help := processtest.Start(t, "-help")
	require.Equal(t, 0, help.Wait(t))
}
