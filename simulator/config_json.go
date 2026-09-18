package simulator

import (
	"fmt"
	"io"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// ParseConfig reads a complete JSON simulation configuration from r, reading
// at most maxBytes. It supplies no default: every key listed below must be
// present and non-null, and unknown keys are rejected.
//
// The document is an object with one required "simulation" object holding
// "id", "startTime", "seed", "initialAircraftCount", "speedHundredths", and
// "spawn". The spawn object requires "latitudeDegrees", "longitudeDegrees",
// "altitudeFeet", "groundSpeedKnots", "trackDegrees", and
// "verticalRateFeetPerMinute", each an object with "min" and "max".
//
// "seed" is a canonical decimal string so values above 2^53 survive JSON.
// "startTime" is a UTC RFC3339Nano instant. Explicit zero counts, speeds, and
// range endpoints are accepted values, not omissions. The parsed
// configuration is validated by the engine's own rules before it is returned.
func ParseConfig(r io.Reader, maxBytes int) (Config, error) {
	value, err := simulatorapi.ParseJSON(r, maxBytes)
	if err != nil {
		return Config{}, invalidConfig("simulation configuration", err)
	}
	config, err := readConfigDocument(value)
	if err != nil {
		return Config{}, invalidConfig("simulation configuration", err)
	}
	return config, nil
}

func readConfigDocument(value *simulatorapi.Value) (Config, error) {
	document, err := value.Object()
	if err != nil {
		return Config{}, err
	}
	simulationValue, err := document.Field("simulation")
	if err != nil {
		return Config{}, err
	}
	settings, err := readSimulationConfig(simulationValue)
	if err != nil {
		return Config{}, err
	}
	if err := document.Done(); err != nil {
		return Config{}, err
	}
	if _, err := simulation.New(settings); err != nil {
		return Config{}, fmt.Errorf("validate simulation configuration: %w", err)
	}
	return Config{Simulation: settings}, nil
}

// readSimulationConfig binds every engine configuration key.
func readSimulationConfig(value *simulatorapi.Value) (simulation.Config, error) {
	object, err := value.Object()
	if err != nil {
		return simulation.Config{}, err
	}

	var config simulation.Config
	if config.ID, err = readText(object, "id"); err != nil {
		return simulation.Config{}, err
	}
	if config.StartTime, err = readTime(object, "startTime"); err != nil {
		return simulation.Config{}, err
	}
	if config.Seed, err = readUint64(object, "seed"); err != nil {
		return simulation.Config{}, err
	}
	if config.InitialAircraftCount, err = readInt(object, "initialAircraftCount"); err != nil {
		return simulation.Config{}, err
	}
	speed, err := readInt(object, "speedHundredths")
	if err != nil {
		return simulation.Config{}, err
	}
	if speed < 0 || speed > simulation.MaxSpeedHundredths {
		return simulation.Config{}, fmt.Errorf("%s.speedHundredths %d is outside 0-%d",
			object.Path(), speed, simulation.MaxSpeedHundredths)
	}
	config.SpeedHundredths = uint16(speed)

	spawnValue, err := object.Field("spawn")
	if err != nil {
		return simulation.Config{}, err
	}
	if config.Spawn, err = readSpawnConfig(spawnValue); err != nil {
		return simulation.Config{}, err
	}
	if err := object.Done(); err != nil {
		return simulation.Config{}, err
	}
	return config, nil
}

// readSpawnConfig binds all six required spawn ranges.
func readSpawnConfig(value *simulatorapi.Value) (simulation.SpawnConfig, error) {
	object, err := value.Object()
	if err != nil {
		return simulation.SpawnConfig{}, err
	}

	var spawn simulation.SpawnConfig
	targets := []struct {
		key   string
		field *simulation.Range
	}{
		{"latitudeDegrees", &spawn.LatitudeDegrees},
		{"longitudeDegrees", &spawn.LongitudeDegrees},
		{"altitudeFeet", &spawn.AltitudeFeet},
		{"groundSpeedKnots", &spawn.GroundSpeedKnots},
		{"trackDegrees", &spawn.TrackDegrees},
		{"verticalRateFeetPerMinute", &spawn.VerticalRateFeetPerMinute},
	}
	for _, target := range targets {
		rangeValue, err := object.Field(target.key)
		if err != nil {
			return simulation.SpawnConfig{}, err
		}
		if *target.field, err = readRange(rangeValue); err != nil {
			return simulation.SpawnConfig{}, err
		}
	}
	if err := object.Done(); err != nil {
		return simulation.SpawnConfig{}, err
	}
	return spawn, nil
}

// readRange binds one required min/max sampling interval.
func readRange(value *simulatorapi.Value) (simulation.Range, error) {
	object, err := value.Object()
	if err != nil {
		return simulation.Range{}, err
	}
	low, err := readFloat(object, "min")
	if err != nil {
		return simulation.Range{}, err
	}
	high, err := readFloat(object, "max")
	if err != nil {
		return simulation.Range{}, err
	}
	if err := object.Done(); err != nil {
		return simulation.Range{}, err
	}
	return simulation.Range{Min: low, Max: high}, nil
}

// invalidConfig marks every configuration parse failure with the shared
// invalid category while keeping the original cause.
func invalidConfig(what string, err error) error {
	return fmt.Errorf("parse %s: %w: %w", what, simulatorapi.CategoryInvalid, err)
}

// Package-local accessors for one required key of a known wire type. They
// keep every parser in this package reporting the same JSON paths.

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

func readUint64(object *simulatorapi.Object, key string) (uint64, error) {
	text, err := readText(object, key)
	if err != nil {
		return 0, err
	}
	value, err := simulatorapi.ParseUint64(text)
	if err != nil {
		return 0, fmt.Errorf("%s.%s: %w", object.Path(), key, err)
	}
	return value, nil
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

func readTime(object *simulatorapi.Object, key string) (time.Time, error) {
	text, err := readText(object, key)
	if err != nil {
		return time.Time{}, err
	}
	value, err := simulatorapi.ParseTime(text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s.%s: %w", object.Path(), key, err)
	}
	return value, nil
}
