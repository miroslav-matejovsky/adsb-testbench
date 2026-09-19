package ui

import (
	"fmt"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// ParseManagerSettings strictly binds one JSON manager settings object.
// Every key is required; missing, null, unknown and duplicate keys are
// rejected, and the parsed settings are validated.
//
// Keys: pollIntervalMilliseconds, requestTimeoutMilliseconds,
// maxResponseBytes, resumeSpeedHundredths.
func ParseManagerSettings(value *simulatorapi.Value) (ManagerSettings, error) {
	object, err := value.Object()
	if err != nil {
		return ManagerSettings{}, parseError("manager settings", err)
	}
	var settings ManagerSettings
	for _, field := range []struct {
		key    string
		target *int
	}{
		{"pollIntervalMilliseconds", &settings.PollIntervalMilliseconds},
		{"requestTimeoutMilliseconds", &settings.RequestTimeoutMilliseconds},
		{"maxResponseBytes", &settings.MaxResponseBytes},
		{"resumeSpeedHundredths", &settings.ResumeSpeedHundredths},
	} {
		if *field.target, err = readInt(object, field.key); err != nil {
			return ManagerSettings{}, parseError("manager settings", err)
		}
	}
	if err := object.Done(); err != nil {
		return ManagerSettings{}, parseError("manager settings", err)
	}
	if err := settings.Validate(); err != nil {
		return ManagerSettings{}, parseError("manager settings", err)
	}
	return settings, nil
}

// ParseAircraftDisplaySettings strictly binds one JSON aircraft display
// settings object. Every key is required; only tiles may be an explicit null,
// which disables tiles.
//
// Keys: pollIntervalMilliseconds, requestTimeoutMilliseconds,
// maxResponseBytes, stationIds, freshForNanoseconds, lostAfterNanoseconds,
// historyPageSize, maxHistoryRecords, initialLatitudeDegrees,
// initialLongitudeDegrees, initialZoom, tiles. A tiles object has the keys
// urlTemplate, attributionText, attributionUrl, minZoom and maxZoom.
func ParseAircraftDisplaySettings(value *simulatorapi.Value) (AircraftDisplaySettings, error) {
	settings, err := parseAircraftDisplaySettings(value)
	if err != nil {
		return AircraftDisplaySettings{}, parseError("aircraft display settings", err)
	}
	if err := settings.Validate(); err != nil {
		return AircraftDisplaySettings{}, parseError("aircraft display settings", err)
	}
	return settings, nil
}

func parseAircraftDisplaySettings(value *simulatorapi.Value) (AircraftDisplaySettings, error) {
	object, err := value.Object()
	if err != nil {
		return AircraftDisplaySettings{}, err
	}
	var settings AircraftDisplaySettings
	ints := []struct {
		key    string
		target *int
	}{
		{"pollIntervalMilliseconds", &settings.PollIntervalMilliseconds},
		{"requestTimeoutMilliseconds", &settings.RequestTimeoutMilliseconds},
		{"maxResponseBytes", &settings.MaxResponseBytes},
	}
	for _, field := range ints {
		if *field.target, err = readInt(object, field.key); err != nil {
			return AircraftDisplaySettings{}, err
		}
	}
	if settings.StationIDs, err = readTextArray(object, "stationIds"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.FreshFor, err = readDuration(object, "freshForNanoseconds"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.LostAfter, err = readDuration(object, "lostAfterNanoseconds"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.HistoryPageSize, err = readInt(object, "historyPageSize"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.MaxHistoryRecords, err = readInt(object, "maxHistoryRecords"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.InitialLatitudeDegrees, err = readFloat(object, "initialLatitudeDegrees"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.InitialLongitudeDegrees, err = readFloat(object, "initialLongitudeDegrees"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	if settings.InitialZoom, err = readInt(object, "initialZoom"); err != nil {
		return AircraftDisplaySettings{}, err
	}
	tilesValue, err := object.Nullable("tiles")
	if err != nil {
		return AircraftDisplaySettings{}, err
	}
	if tilesValue != nil {
		tiles, err := parseTiles(tilesValue)
		if err != nil {
			return AircraftDisplaySettings{}, err
		}
		settings.Tiles = &tiles
	}
	if err := object.Done(); err != nil {
		return AircraftDisplaySettings{}, err
	}
	return settings, nil
}

func parseTiles(value *simulatorapi.Value) (Tiles, error) {
	object, err := value.Object()
	if err != nil {
		return Tiles{}, err
	}
	var tiles Tiles
	if tiles.URLTemplate, err = readText(object, "urlTemplate"); err != nil {
		return Tiles{}, err
	}
	if tiles.AttributionText, err = readText(object, "attributionText"); err != nil {
		return Tiles{}, err
	}
	if tiles.AttributionURL, err = readText(object, "attributionUrl"); err != nil {
		return Tiles{}, err
	}
	if tiles.MinZoom, err = readInt(object, "minZoom"); err != nil {
		return Tiles{}, err
	}
	if tiles.MaxZoom, err = readInt(object, "maxZoom"); err != nil {
		return Tiles{}, err
	}
	if err := object.Done(); err != nil {
		return Tiles{}, err
	}
	return tiles, nil
}

// browserManagerConfig is the exact JSON document the manager module reads.
type browserManagerConfig struct {
	APIBaseURL                 string `json:"apiBaseUrl"`
	AssetBaseURL               string `json:"assetBaseUrl"`
	PollIntervalMilliseconds   int    `json:"pollIntervalMilliseconds"`
	RequestTimeoutMilliseconds int    `json:"requestTimeoutMilliseconds"`
	MaxResponseBytes           int    `json:"maxResponseBytes"`
	ResumeSpeedHundredths      int    `json:"resumeSpeedHundredths"`
}

// browserTiles is the JSON form of Tiles.
type browserTiles struct {
	URLTemplate     string `json:"urlTemplate"`
	AttributionText string `json:"attributionText"`
	AttributionURL  string `json:"attributionUrl"`
	MinZoom         int    `json:"minZoom"`
	MaxZoom         int    `json:"maxZoom"`
}

// browserAircraftConfig is the exact JSON document the aircraft module reads.
// Nanosecond durations are canonical decimal strings.
type browserAircraftConfig struct {
	APIBaseURL                 string        `json:"apiBaseUrl"`
	AssetBaseURL               string        `json:"assetBaseUrl"`
	PollIntervalMilliseconds   int           `json:"pollIntervalMilliseconds"`
	RequestTimeoutMilliseconds int           `json:"requestTimeoutMilliseconds"`
	MaxResponseBytes           int           `json:"maxResponseBytes"`
	StationIDs                 []string      `json:"stationIds"`
	FreshForNanoseconds        string        `json:"freshForNanoseconds"`
	LostAfterNanoseconds       string        `json:"lostAfterNanoseconds"`
	HistoryPageSize            int           `json:"historyPageSize"`
	MaxHistoryRecords          int           `json:"maxHistoryRecords"`
	InitialLatitudeDegrees     float64       `json:"initialLatitudeDegrees"`
	InitialLongitudeDegrees    float64       `json:"initialLongitudeDegrees"`
	InitialZoom                int           `json:"initialZoom"`
	Tiles                      *browserTiles `json:"tiles"`
}

func (c ManagerConfig) browser(api, asset string) browserManagerConfig {
	return browserManagerConfig{
		APIBaseURL: api, AssetBaseURL: asset,
		PollIntervalMilliseconds:   c.PollIntervalMilliseconds,
		RequestTimeoutMilliseconds: c.RequestTimeoutMilliseconds,
		MaxResponseBytes:           c.MaxResponseBytes,
		ResumeSpeedHundredths:      c.ResumeSpeedHundredths,
	}
}

func (c AircraftDisplayConfig) browser(api, asset string) browserAircraftConfig {
	config := browserAircraftConfig{
		APIBaseURL: api, AssetBaseURL: asset,
		PollIntervalMilliseconds:   c.PollIntervalMilliseconds,
		RequestTimeoutMilliseconds: c.RequestTimeoutMilliseconds,
		MaxResponseBytes:           c.MaxResponseBytes,
		StationIDs:                 append([]string{}, c.StationIDs...),
		FreshForNanoseconds:        simulatorapi.FormatDuration(c.FreshFor),
		LostAfterNanoseconds:       simulatorapi.FormatDuration(c.LostAfter),
		HistoryPageSize:            c.HistoryPageSize,
		MaxHistoryRecords:          c.MaxHistoryRecords,
		InitialLatitudeDegrees:     c.InitialLatitudeDegrees,
		InitialLongitudeDegrees:    c.InitialLongitudeDegrees,
		InitialZoom:                c.InitialZoom,
	}
	if c.Tiles != nil {
		config.Tiles = &browserTiles{
			URLTemplate: c.Tiles.URLTemplate, AttributionText: c.Tiles.AttributionText,
			AttributionURL: c.Tiles.AttributionURL, MinZoom: c.Tiles.MinZoom, MaxZoom: c.Tiles.MaxZoom,
		}
	}
	return config
}

func parseError(what string, err error) error {
	return fmt.Errorf("parse %s: %w: %w", what, ErrInvalidConfig, err)
}

func readText(object *simulatorapi.Object, key string) (string, error) {
	field, err := object.Field(key)
	if err != nil {
		return "", err
	}
	return field.Text()
}

func readInt(object *simulatorapi.Object, key string) (int, error) {
	field, err := object.Field(key)
	if err != nil {
		return 0, err
	}
	return field.Int()
}

func readFloat(object *simulatorapi.Object, key string) (float64, error) {
	field, err := object.Field(key)
	if err != nil {
		return 0, err
	}
	return field.Float()
}

func readDuration(object *simulatorapi.Object, key string) (time.Duration, error) {
	text, err := readText(object, key)
	if err != nil {
		return 0, err
	}
	value, err := simulatorapi.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("%s.%s: %w", object.Path(), key, err)
	}
	return value, nil
}

func readTextArray(object *simulatorapi.Object, key string) ([]string, error) {
	field, err := object.Field(key)
	if err != nil {
		return nil, err
	}
	items, err := field.Array()
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		text, err := item.Text()
		if err != nil {
			return nil, err
		}
		values = append(values, text)
	}
	return values, nil
}
