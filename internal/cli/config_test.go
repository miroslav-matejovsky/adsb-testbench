package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/cli"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

var shipped = map[testbench.Mode]string{
	testbench.ModeCombined:  "combined.json",
	testbench.ModeSimulator: "simulator.json",
	testbench.ModeDisplay:   "display.json",
}

func shippedConfig(t *testing.T, mode testbench.Mode) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "configs", shipped[mode]))
	require.NoError(t, err)
	return data
}

// edit decodes a shipped configuration, applies change, and re-encodes it.
func edit(t *testing.T, mode testbench.Mode, change func(map[string]any)) []byte {
	t.Helper()

	var document map[string]any
	require.NoError(t, json.Unmarshal(shippedConfig(t, mode), &document))
	change(document)
	data, err := json.Marshal(document)
	require.NoError(t, err)
	return data
}

func section(document map[string]any, name string) map[string]any {
	return document[name].(map[string]any)
}

func deps() cli.Dependencies {
	return cli.Dependencies{ReportError: func(error) {}, Transport: &http.Transport{}}
}

func TestShippedConfigurationsParse(t *testing.T) {
	t.Parallel()

	for mode := range shipped {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			file, err := cli.ParseFile(shippedConfig(t, mode), mode, deps())
			require.NoError(t, err)
			require.Equal(t, mode, file.Mode)
			require.NotEmpty(t, file.Comment)
			require.Equal(t, "/", file.Server.PublicBasePath)
		})
	}
}

func TestParseFileKeepsExplicitZeroAndFalse(t *testing.T) {
	t.Parallel()

	data := edit(t, testbench.ModeCombined, func(document map[string]any) {
		simulation := section(section(document, "simulator"), "simulation")
		simulation["initialAircraftCount"] = 0
		simulation["speedHundredths"] = 0
		station := document["stations"].([]any)[0].(map[string]any)
		station["enabled"] = false
		station["frameLossProbability"] = 0
		station["antennaHeightMetres"] = 0
		section(document, "aircraftUi")["stationIds"] = []any{}
	})
	file, err := cli.ParseFile(data, testbench.ModeCombined, deps())
	require.NoError(t, err)
	require.Zero(t, file.Simulator.Simulation.InitialAircraftCount)
	require.Zero(t, file.Simulator.Simulation.SpeedHundredths)
	require.False(t, file.Stations[0].Enabled)
	require.Zero(t, file.Stations[0].AntennaHeightMetres)
	require.Equal(t, []string{}, file.Aircraft.StationIDs)

	noStations := edit(t, testbench.ModeSimulator, func(document map[string]any) { document["stations"] = []any{} })
	file, err = cli.ParseFile(noStations, testbench.ModeSimulator, deps())
	require.NoError(t, err)
	require.Empty(t, file.Stations)
}

func TestParseFileRejectsIncompleteOrAmbiguousInput(t *testing.T) {
	t.Parallel()

	combined := testbench.ModeCombined
	cases := map[string][]byte{
		"missing section":  edit(t, combined, func(d map[string]any) { delete(d, "display") }),
		"null section":     edit(t, combined, func(d map[string]any) { d["managerUi"] = nil }),
		"unknown section":  edit(t, combined, func(d map[string]any) { d["source"] = map[string]any{} }),
		"missing comment":  edit(t, combined, func(d map[string]any) { delete(d, "$comment") }),
		"empty comment":    edit(t, combined, func(d map[string]any) { d["$comment"] = "  " }),
		"missing key":      edit(t, combined, func(d map[string]any) { delete(section(d, "server"), "idleTimeoutNanoseconds") }),
		"null nested":      edit(t, combined, func(d map[string]any) { section(d, "server")["maxHeaderBytes"] = nil }),
		"unknown nested":   edit(t, combined, func(d map[string]any) { section(d, "logging")["color"] = true }),
		"bad level":        edit(t, combined, func(d map[string]any) { section(d, "logging")["level"] = "loud" }),
		"bad format":       edit(t, combined, func(d map[string]any) { section(d, "logging")["format"] = "xml" }),
		"bad address":      edit(t, combined, func(d map[string]any) { section(d, "server")["listenAddress"] = "http://127.0.0.1:1" }),
		"missing port":     edit(t, combined, func(d map[string]any) { section(d, "server")["listenAddress"] = "127.0.0.1" }),
		"bad prefix":       edit(t, combined, func(d map[string]any) { section(d, "server")["publicBasePath"] = "/a/../b/" }),
		"zero timeout":     edit(t, combined, func(d map[string]any) { section(d, "server")["readTimeoutNanoseconds"] = "0" }),
		"numeric duration": edit(t, combined, func(d map[string]any) { section(d, "server")["readTimeoutNanoseconds"] = 5 }),
		"station key":      edit(t, combined, func(d map[string]any) { delete(d["stations"].([]any)[0].(map[string]any), "systemLossDB") }),
		"duplicate station": edit(t, combined, func(d map[string]any) {
			d["stations"] = append(d["stations"].([]any), d["stations"].([]any)[0])
		}),
		"unknown selection": edit(t, combined, func(d map[string]any) {
			section(d, "aircraftUi")["stationIds"] = []any{"nowhere"}
		}),
		"write before api deadline": edit(t, combined, func(d map[string]any) {
			section(d, "server")["writeTimeoutNanoseconds"] = "5000000000"
		}),
		"manager byte budget": edit(t, combined, func(d map[string]any) { section(d, "managerUi")["maxResponseBytes"] = 4096 }),
		"aircraft byte budget": edit(t, combined, func(d map[string]any) {
			section(d, "aircraftUi")["maxResponseBytes"] = 4096
		}),
		"invalid simulation": edit(t, combined, func(d map[string]any) {
			section(section(d, "simulator"), "simulation")["initialAircraftCount"] = -1
		}),
		"invalid ui tiles": edit(t, combined, func(d map[string]any) { section(d, "aircraftUi")["tiles"] = map[string]any{} }),
		"trailing value":   append(shippedConfig(t, combined), []byte(" {}")...),
		"duplicate key":    []byte(strings.Replace(string(shippedConfig(t, combined)), `"logging": {`, `"logging": {"level": "info",`, 1)),
		"not an object":    []byte(`[]`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := cli.ParseFile(data, combined, deps())
			require.ErrorIs(t, err, cli.ErrInvalidConfig)
		})
	}
}

func TestParseDisplayFileChecksSourceAndTransport(t *testing.T) {
	t.Parallel()

	display := testbench.ModeDisplay
	cases := map[string][]byte{
		"source url":        edit(t, display, func(d map[string]any) { section(d, "source")["baseUrl"] = "/api/simulator" }),
		"source timeout":    edit(t, display, func(d map[string]any) { section(d, "source")["timeoutNanoseconds"] = "9000000000" }),
		"zero dial timeout": edit(t, display, func(d map[string]any) { section(d, "transport")["dialTimeoutNanoseconds"] = "0" }),
		"zero connections":  edit(t, display, func(d map[string]any) { section(d, "transport")["maxConnsPerHost"] = 0 }),
		"simulator section": edit(t, display, func(d map[string]any) { d["stations"] = []any{} }),
	}
	for name, data := range cases {
		_, err := cli.ParseFile(data, display, deps())
		require.ErrorIs(t, err, cli.ErrInvalidConfig, name)
	}
	_, err := cli.ParseFile(shippedConfig(t, display), display, cli.Dependencies{ReportError: func(error) {}})
	require.ErrorIs(t, err, cli.ErrInvalidConfig, "a display file needs a transport")
	_, err = cli.ParseFile(shippedConfig(t, testbench.ModeCombined), testbench.ModeCombined, cli.Dependencies{})
	require.ErrorIs(t, err, cli.ErrInvalidConfig, "the error callback is required")
}

func TestNewTransportUsesEveryExplicitSetting(t *testing.T) {
	t.Parallel()

	settings, err := cli.ParseTransport(shippedConfig(t, testbench.ModeDisplay))
	require.NoError(t, err)
	transport := cli.NewTransport(settings)
	require.Equal(t, settings.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	require.Equal(t, settings.ResponseHeaderTimeout, transport.ResponseHeaderTimeout)
	require.Equal(t, settings.IdleConnTimeout, transport.IdleConnTimeout)
	require.Equal(t, settings.MaxConnsPerHost, transport.MaxConnsPerHost)
	require.Nil(t, transport.Proxy, "environment proxies are never used")
}
