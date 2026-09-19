package ui_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// managerConfig returns a complete explicit fixture configuration.
func managerConfig() ui.ManagerConfig {
	return ui.ManagerConfig{
		APIBaseURL: "/api/simulator/", AssetBaseURL: "/assets/",
		ManagerSettings: ui.ManagerSettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000,
			MaxResponseBytes: 1 << 20, ResumeSpeedHundredths: 100,
		},
	}
}

// aircraftConfig returns a complete explicit fixture configuration.
func aircraftConfig() ui.AircraftDisplayConfig {
	return ui.AircraftDisplayConfig{
		APIBaseURL: "/api/display/", AssetBaseURL: "/assets/",
		AircraftDisplaySettings: ui.AircraftDisplaySettings{
			PollIntervalMilliseconds: 1000, RequestTimeoutMilliseconds: 5000,
			MaxResponseBytes: 1 << 20, StationIDs: []string{"alpha"},
			FreshFor: 10 * time.Second, LostAfter: 60 * time.Second,
			HistoryPageSize: 50, MaxHistoryRecords: 500,
			InitialLatitudeDegrees: 50, InitialLongitudeDegrees: 14, InitialZoom: 7,
			Tiles: nil,
		},
	}
}

func tiles() *ui.Tiles {
	return &ui.Tiles{
		URLTemplate:     "https://tiles.example.test/{z}/{x}/{y}.png",
		AttributionText: "Example tiles", AttributionURL: "https://tiles.example.test/about",
		MinZoom: 0, MaxZoom: 18,
	}
}

func TestConfigsAcceptCompleteSettings(t *testing.T) {
	t.Parallel()

	require.NoError(t, managerConfig().Validate())
	require.NoError(t, aircraftConfig().Validate())

	withTiles := aircraftConfig()
	withTiles.Tiles = tiles()
	require.NoError(t, withTiles.Validate())

	empty := aircraftConfig()
	empty.StationIDs = []string{}
	require.NoError(t, empty.Validate(), "an empty selection is explicit")

	absolute := managerConfig()
	absolute.APIBaseURL = "http://127.0.0.1:8080/bench/a/api/simulator"
	require.NoError(t, absolute.Validate())
}

func TestManagerConfigRejectsInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*ui.ManagerConfig){
		"relative api":       func(c *ui.ManagerConfig) { c.APIBaseURL = "api/" },
		"protocol relative":  func(c *ui.ManagerConfig) { c.APIBaseURL = "//evil.test/api/" },
		"script api":         func(c *ui.ManagerConfig) { c.APIBaseURL = "javascript:alert(1)" },
		"backslash asset":    func(c *ui.ManagerConfig) { c.AssetBaseURL = `/assets\x/` },
		"encoded dot asset":  func(c *ui.ManagerConfig) { c.AssetBaseURL = "/a/%2e%2e/assets/" },
		"zero poll":          func(c *ui.ManagerConfig) { c.PollIntervalMilliseconds = 0 },
		"negative timeout":   func(c *ui.ManagerConfig) { c.RequestTimeoutMilliseconds = -1 },
		"small response":     func(c *ui.ManagerConfig) { c.MaxResponseBytes = simulatorapi.MinResponseBytes - 1 },
		"zero resume speed":  func(c *ui.ManagerConfig) { c.ResumeSpeedHundredths = 0 },
		"missing api":        func(c *ui.ManagerConfig) { c.APIBaseURL = "" },
		"query in api":       func(c *ui.ManagerConfig) { c.APIBaseURL = "/api/?x=1" },
		"userinfo api":       func(c *ui.ManagerConfig) { c.APIBaseURL = "http://u:p@host/api/" },
		"control byte asset": func(c *ui.ManagerConfig) { c.AssetBaseURL = "/assets\n/" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := managerConfig()
			mutate(&config)
			require.ErrorIs(t, config.Validate(), ui.ErrInvalidConfig)
		})
	}
}

func TestAircraftConfigRejectsInvalidSettings(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*ui.AircraftDisplayConfig){
		"nil stations":       func(c *ui.AircraftDisplayConfig) { c.StationIDs = nil },
		"duplicate station":  func(c *ui.AircraftDisplayConfig) { c.StationIDs = []string{"a", "a"} },
		"invalid station":    func(c *ui.AircraftDisplayConfig) { c.StationIDs = []string{"a b"} },
		"long station":       func(c *ui.AircraftDisplayConfig) { c.StationIDs = []string{strings.Repeat("a", 65)} },
		"too many stations":  func(c *ui.AircraftDisplayConfig) { c.StationIDs = strings.Split("a,b,c,d,e,f,g,h,i", ",") },
		"zero fresh":         func(c *ui.AircraftDisplayConfig) { c.FreshFor = 0 },
		"lost not after":     func(c *ui.AircraftDisplayConfig) { c.LostAfter = c.FreshFor },
		"zero page":          func(c *ui.AircraftDisplayConfig) { c.HistoryPageSize = 0 },
		"large page":         func(c *ui.AircraftDisplayConfig) { c.HistoryPageSize = 1001 },
		"records below page": func(c *ui.AircraftDisplayConfig) { c.MaxHistoryRecords = 49 },
		"nan latitude":       func(c *ui.AircraftDisplayConfig) { c.InitialLatitudeDegrees = math.NaN() },
		"latitude range":     func(c *ui.AircraftDisplayConfig) { c.InitialLatitudeDegrees = 91 },
		"infinite longitude": func(c *ui.AircraftDisplayConfig) { c.InitialLongitudeDegrees = math.Inf(1) },
		"zoom range":         func(c *ui.AircraftDisplayConfig) { c.InitialZoom = 25 },
		"ftp api":            func(c *ui.AircraftDisplayConfig) { c.APIBaseURL = "ftp://host/api/" },
		"tile without z": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "https://tiles.test/{x}/{y}.png"
		},
		"tile repeated x": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "https://tiles.test/{z}/{x}/{x}/{y}.png"
		},
		"tile other placeholder": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "https://{s}.tiles.test/{z}/{x}/{y}.png"
		},
		"tile script": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "javascript:alert('{z}{x}{y}')"
		},
		"tile userinfo": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "https://u@tiles.test/{z}/{x}/{y}.png"
		},
		"tile fragment": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "https://tiles.test/{z}/{x}/{y}.png#a"
		},
		"tile protocol relative": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.URLTemplate = "//tiles.test/{z}/{x}/{y}.png"
		},
		"empty attribution": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.AttributionText = " "
		},
		"script attribution link": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.AttributionURL = "javascript:alert(1)"
		},
		"zoom outside tiles": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.MinZoom = 8
		},
		"inverted tile zoom": func(c *ui.AircraftDisplayConfig) {
			c.Tiles = tiles()
			c.Tiles.MinZoom = 10
			c.Tiles.MaxZoom = 5
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := aircraftConfig()
			mutate(&config)
			require.ErrorIs(t, config.Validate(), ui.ErrInvalidConfig)
		})
	}
}

func parse(t *testing.T, text string) *simulatorapi.Value {
	t.Helper()

	value, err := simulatorapi.ParseJSONBytes([]byte(text))
	require.NoError(t, err)
	return value
}

const managerSettingsJSON = `{"pollIntervalMilliseconds":1000,"requestTimeoutMilliseconds":5000,` +
	`"maxResponseBytes":1048576,"resumeSpeedHundredths":100}`

const aircraftSettingsJSON = `{"pollIntervalMilliseconds":1000,"requestTimeoutMilliseconds":5000,` +
	`"maxResponseBytes":1048576,"stationIds":["alpha"],"freshForNanoseconds":"10000000000",` +
	`"lostAfterNanoseconds":"60000000000","historyPageSize":50,"maxHistoryRecords":500,` +
	`"initialLatitudeDegrees":50,"initialLongitudeDegrees":14,"initialZoom":7,"tiles":null}`

func TestParseSettingsBindsEveryKey(t *testing.T) {
	t.Parallel()

	manager, err := ui.ParseManagerSettings(parse(t, managerSettingsJSON))
	require.NoError(t, err)
	require.Equal(t, managerConfig().ManagerSettings, manager)

	aircraft, err := ui.ParseAircraftDisplaySettings(parse(t, aircraftSettingsJSON))
	require.NoError(t, err)
	require.Equal(t, aircraftConfig().AircraftDisplaySettings, aircraft)

	withTiles := strings.Replace(aircraftSettingsJSON, `"tiles":null`,
		`"tiles":{"urlTemplate":"https://tiles.example.test/{z}/{x}/{y}.png","attributionText":"Example tiles",`+
			`"attributionUrl":"https://tiles.example.test/about","minZoom":0,"maxZoom":18}`, 1)
	aircraft, err = ui.ParseAircraftDisplaySettings(parse(t, withTiles))
	require.NoError(t, err)
	require.Equal(t, tiles(), aircraft.Tiles)
}

func TestParseSettingsRejectsIncompleteInput(t *testing.T) {
	t.Parallel()

	managerCases := map[string]string{
		"missing key":  strings.Replace(managerSettingsJSON, `,"resumeSpeedHundredths":100`, ``, 1),
		"null value":   strings.Replace(managerSettingsJSON, `"resumeSpeedHundredths":100`, `"resumeSpeedHundredths":null`, 1),
		"unknown key":  strings.Replace(managerSettingsJSON, `{`, `{"extra":1,`, 1),
		"string value": strings.Replace(managerSettingsJSON, `1000`, `"1000"`, 1),
		"fraction":     strings.Replace(managerSettingsJSON, `1000`, `1000.5`, 1),
		"zero speed":   strings.Replace(managerSettingsJSON, `"resumeSpeedHundredths":100`, `"resumeSpeedHundredths":0`, 1),
	}
	for name, text := range managerCases {
		_, err := ui.ParseManagerSettings(parse(t, text))
		require.ErrorIs(t, err, ui.ErrInvalidConfig, name)
	}

	aircraftCases := map[string]string{
		"missing tiles":      strings.Replace(aircraftSettingsJSON, `,"tiles":null`, ``, 1),
		"null stations":      strings.Replace(aircraftSettingsJSON, `["alpha"]`, `null`, 1),
		"numeric duration":   strings.Replace(aircraftSettingsJSON, `"10000000000"`, `10000000000`, 1),
		"padded duration":    strings.Replace(aircraftSettingsJSON, `"10000000000"`, `"010000000000"`, 1),
		"unknown tile key":   strings.Replace(aircraftSettingsJSON, `"tiles":null`, `"tiles":{"x":1}`, 1),
		"fractional zoom":    strings.Replace(aircraftSettingsJSON, `"initialZoom":7`, `"initialZoom":7.5`, 1),
		"unknown key":        strings.Replace(aircraftSettingsJSON, `{`, `{"apiBaseUrl":"/api/",`, 1),
		"thresholds swapped": strings.Replace(aircraftSettingsJSON, `"60000000000"`, `"1"`, 1),
	}
	for name, text := range aircraftCases {
		_, err := ui.ParseAircraftDisplaySettings(parse(t, text))
		require.ErrorIs(t, err, ui.ErrInvalidConfig, name)
	}

	_, err := simulatorapi.ParseJSONBytes([]byte(`{"a":1,"a":2}`))
	require.Error(t, err, "duplicate keys are rejected by the shared strict parser")
}
