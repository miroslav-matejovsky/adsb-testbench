package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/cli"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// harness is an injected process environment.
type harness struct {
	stdout, stderr bytes.Buffer
	listens        atomic.Int64
	listen         func(network, address string) (net.Listener, error)
	runID          func(label string) (string, error)
	ready          chan string
	mu             sync.Mutex
}

func newHarness() *harness {
	h := &harness{ready: make(chan string, 1)}
	h.listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
	h.runID = func(label string) (string, error) { return label + "-test", nil }
	return h
}

func (h *harness) env() cli.Environment {
	return cli.Environment{
		Stdout: &lockedWriter{h: h, w: &h.stdout}, Stderr: &lockedWriter{h: h, w: &h.stderr},
		ReadFile: os.ReadFile,
		Listen: func(network, address string) (net.Listener, error) {
			h.listens.Add(1)
			return h.listen(network, address)
		},
		NewRunID: func(label string) (string, error) { return h.runID(label) },
		Ready:    func(address string) { h.ready <- address },
	}
}

type lockedWriter struct {
	h *harness
	w io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.h.mu.Lock()
	defer l.h.mu.Unlock()
	return l.w.Write(p)
}

func writeConfig(t *testing.T, data []byte) []string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return []string{"-config", path, "-config-max-bytes", "65536"}
}

func getJSON(t *testing.T, url string, target any) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(target))
}

func TestFlagsAreRequiredAndStrict(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"no flags":          {},
		"missing max bytes": {"-config", "x.json"},
		"zero max bytes":    {"-config", "x.json", "-config-max-bytes", "0"},
		"text max bytes":    {"-config", "x.json", "-config-max-bytes", "many"},
		"missing config":    {"-config-max-bytes", "10"},
		"unknown flag":      {"-config", "x.json", "-config-max-bytes", "10", "-listen", ":1"},
		"positional":        {"-config", "x.json", "-config-max-bytes", "10", "extra"},
	}
	for name, args := range cases {
		h := newHarness()
		err := cli.Run(t.Context(), testbench.ModeCombined, args, h.env())
		require.ErrorIs(t, err, cli.ErrUsage, name)
		require.Equal(t, cli.ExitUsage, cli.ExitStatus(err), name)
		require.Zero(t, h.listens.Load(), name)
	}

	h := newHarness()
	err := cli.Run(t.Context(), testbench.ModeCombined, []string{"-help"}, h.env())
	require.ErrorIs(t, err, flag.ErrHelp)
	require.Equal(t, cli.ExitOK, cli.ExitStatus(err))
	require.Contains(t, h.stdout.String(), "-config-max-bytes")
}

func TestInvalidConfigurationOpensNoListener(t *testing.T) {
	t.Parallel()

	oversized := newHarness()
	args := writeConfig(t, shippedConfig(t, testbench.ModeCombined))
	args[3] = "100"
	err := cli.Run(t.Context(), testbench.ModeCombined, args, oversized.env())
	require.ErrorIs(t, err, cli.ErrInvalidConfig)
	require.Equal(t, cli.ExitFailure, cli.ExitStatus(err))
	require.Zero(t, oversized.listens.Load())

	invalid := newHarness()
	data := edit(t, testbench.ModeCombined, func(d map[string]any) { delete(d, "stations") })
	err = cli.Run(t.Context(), testbench.ModeCombined, writeConfig(t, data), invalid.env())
	require.ErrorIs(t, err, cli.ErrInvalidConfig)
	require.Zero(t, invalid.listens.Load())

	missing := newHarness()
	err = cli.Run(t.Context(), testbench.ModeCombined, []string{"-config", "missing.json", "-config-max-bytes", "10"}, missing.env())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStartupFailuresKeepTheirCauses(t *testing.T) {
	t.Parallel()

	entropy := errors.New("entropy unavailable")
	noEntropy := newHarness()
	noEntropy.runID = func(string) (string, error) { return "", entropy }
	err := cli.Run(t.Context(), testbench.ModeCombined, writeConfig(t, shippedConfig(t, testbench.ModeCombined)), noEntropy.env())
	require.ErrorIs(t, err, entropy)
	require.Zero(t, noEntropy.listens.Load())

	badStation := newHarness()
	data := edit(t, testbench.ModeCombined, func(d map[string]any) {
		d["stations"].([]any)[1].(map[string]any)["latitudeDegrees"] = 95
	})
	err = cli.Run(t.Context(), testbench.ModeCombined, writeConfig(t, data), badStation.env())
	require.ErrorIs(t, err, simulatorapi.CategoryInvalid)
	require.ErrorContains(t, err, `install configured station "brno"`)
	require.Zero(t, badStation.listens.Load())

	occupied := errors.New("address already in use")
	bind := newHarness()
	bind.listen = func(string, string) (net.Listener, error) { return nil, occupied }
	err = cli.Run(t.Context(), testbench.ModeSimulator, writeConfig(t, shippedConfig(t, testbench.ModeSimulator)), bind.env())
	require.ErrorIs(t, err, occupied)
	require.Contains(t, bind.stderr.String(), "command failed")

	accept := errors.New("accept failed")
	serve := newHarness()
	serve.listen = func(string, string) (net.Listener, error) { return &failingListener{err: accept}, nil }
	err = cli.Run(t.Context(), testbench.ModeSimulator, writeConfig(t, shippedConfig(t, testbench.ModeSimulator)), serve.env())
	require.ErrorIs(t, err, accept)
	require.Equal(t, cli.ExitFailure, cli.ExitStatus(err))
}

type failingListener struct {
	err error
}

func (l *failingListener) Accept() (net.Conn, error) { return nil, l.err }
func (l *failingListener) Close() error              { return nil }
func (l *failingListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

// start runs a command in the background and returns its base URL.
func start(t *testing.T, mode testbench.Mode, data []byte, h *harness) (string, func() error) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- cli.Run(ctx, mode, writeConfig(t, data), h.env()) }()
	select {
	case address := <-h.ready:
		return "http://" + address, func() error {
			cancel()
			return <-done
		}
	case err := <-done:
		cancel()
		t.Fatalf("command stopped before serving: %v\n%s", err, h.stderr.String())
		return "", nil
	}
}

func TestCombinedCommandServesAndStopsCleanly(t *testing.T) {
	t.Parallel()

	h := newHarness()
	base, stop := start(t, testbench.ModeCombined, shippedConfig(t, testbench.ModeCombined), h)

	var status testbench.Status
	getJSON(t, base+"/status", &status)
	require.Equal(t, testbench.ModeCombined, status.Mode)
	require.Equal(t, "local-bench-test", *status.RunID)

	var stations simulatorapi.StationsSnapshot
	getJSON(t, base+"/api/simulator/stations", &stations)
	require.Len(t, stations.Stations, 2)
	require.Equal(t, "prague", stations.Stations[0].Station.ID)
	var metadata simulatorapi.Metadata
	getJSON(t, base+"/api/simulator/metadata", &metadata)
	require.Equal(t, 100, metadata.Simulation.SpeedHundredths, "the configured speed is restored before serving")
	var discovered simulatorapi.StationsSnapshot
	getJSON(t, base+"/api/display/stations", &discovered)
	require.Len(t, discovered.Stations, 2)

	require.NoError(t, stop())
	log := h.stderr.String()
	require.Contains(t, log, "msg=serving")
	require.Contains(t, log, "runId=local-bench-test")
	require.Contains(t, log, "msg=stopped")
}

func TestReusedConfigurationGetsAFreshRunIdentity(t *testing.T) {
	t.Parallel()

	first, second := newHarness(), newHarness()
	first.runID, second.runID = cli.FreshRunID, cli.FreshRunID
	data := shippedConfig(t, testbench.ModeSimulator)

	baseA, stopA := start(t, testbench.ModeSimulator, data, first)
	var metadataA simulatorapi.Metadata
	getJSON(t, baseA+"/api/simulator/metadata", &metadataA)
	require.NoError(t, stopA())

	baseB, stopB := start(t, testbench.ModeSimulator, data, second)
	defer func() { require.NoError(t, stopB()) }()
	var metadataB simulatorapi.Metadata
	getJSON(t, baseB+"/api/simulator/metadata", &metadataB)

	require.True(t, strings.HasPrefix(metadataA.RunID, "local-bench-"))
	require.NotEqual(t, metadataA.RunID, metadataB.RunID)
	require.Equal(t, metadataA.Simulation.Seed, metadataB.Simulation.Seed, "seed and start time are unchanged")

	body := strings.NewReader(`{"runId":"` + metadataA.RunID + `","count":0}`)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, baseB+"/api/simulator/aircraft/count", body)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusConflict, response.StatusCode, "a command for the old run is rejected")
}

func TestSeparateSimulatorAndDisplayCommands(t *testing.T) {
	t.Parallel()

	sim := newHarness()
	simulatorBase, stopSimulator := start(t, testbench.ModeSimulator, shippedConfig(t, testbench.ModeSimulator), sim)
	defer func() { require.NoError(t, stopSimulator()) }()

	data := edit(t, testbench.ModeDisplay, func(d map[string]any) {
		section(d, "source")["baseUrl"] = simulatorBase + "/api/simulator"
		section(d, "server")["publicBasePath"] = "/bench/display/"
	})
	display := newHarness()
	displayBase, stopDisplay := start(t, testbench.ModeDisplay, data, display)

	var status testbench.Status
	getJSON(t, displayBase+"/bench/display/status", &status)
	require.Equal(t, testbench.ModeDisplay, status.Mode)
	require.Nil(t, status.RunID, "local readiness never contacts the upstream")

	var stations simulatorapi.StationsSnapshot
	getJSON(t, displayBase+"/bench/display/api/display/stations", &stations)
	require.Equal(t, "local-bench-test", stations.RunID)
	require.Len(t, stations.Stations, 2)
	require.NoError(t, stopDisplay())
}
