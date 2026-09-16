package simulation

import (
	"math"
	"time"
)

// fixtureStart is the virtual start instant shared by package tests.
var fixtureStart = time.Date(2024, time.March, 5, 12, 0, 0, 0, time.UTC)

// validConfig returns a complete configuration with every field assigned
// explicitly. Tests copy it and change only the field under test.
func validConfig() Config {
	return Config{
		ID:                   "fixture",
		StartTime:            fixtureStart,
		Seed:                 0x0123456789abcdef,
		InitialAircraftCount: 3,
		SpeedHundredths:      100,
		Spawn: SpawnConfig{
			LatitudeDegrees:           Range{Min: 50, Max: 51},
			LongitudeDegrees:          Range{Min: 14, Max: 15},
			AltitudeFeet:              Range{Min: 30000, Max: 36000},
			GroundSpeedKnots:          Range{Min: 400, Max: 500},
			TrackDegrees:              Range{Min: 0, Max: 360},
			VerticalRateFeetPerMinute: Range{Min: -1000, Max: 1000},
		},
	}
}

// stationaryConfig returns a configuration whose aircraft are created at one
// exact point with no motion at all. Every range is degenerate, so birth state
// is fully determined and expected frames can be derived by hand.
func stationaryConfig() Config {
	return Config{
		ID:                   "stationary",
		StartTime:            fixtureStart,
		Seed:                 7,
		InitialAircraftCount: 1,
		SpeedHundredths:      100,
		Spawn: SpawnConfig{
			LatitudeDegrees:           Range{Min: 50, Max: 50},
			LongitudeDegrees:          Range{Min: 14, Max: 14},
			AltitudeFeet:              Range{Min: 35000, Max: 35000},
			GroundSpeedKnots:          Range{Min: 0, Max: 0},
			TrackDegrees:              Range{Min: 90, Max: 90},
			VerticalRateFeetPerMinute: Range{Min: 0, Max: 0},
		},
	}
}

// validStationConfig returns a complete station configuration with every field
// assigned explicitly. It sits in the middle of the validConfig spawn box, so
// aircraft created by that configuration are comfortably inside its coverage.
// Tests copy it and change only the field under test.
func validStationConfig() StationConfig {
	return StationConfig{
		ID:                   "primary",
		Enabled:              true,
		LatitudeDegrees:      50.5,
		LongitudeDegrees:     14.5,
		SiteElevationMetres:  100,
		AntennaHeightMetres:  30,
		AntennaGainDBi:       3,
		SensitivityDBm:       -95,
		SystemLossDB:         2,
		FrameLossProbability: 0,
	}
}

// insensitiveStationConfig returns a station whose poor sensitivity makes the
// link budget bind well inside the radio horizon.
func insensitiveStationConfig() StationConfig {
	cfg := validStationConfig()
	cfg.ID = "insensitive"
	cfg.SensitivityDBm = -85
	return cfg
}

// disabledStationConfig returns a fully configured station that is switched
// off, which must hear nothing at all.
func disabledStationConfig() StationConfig {
	cfg := validStationConfig()
	cfg.ID = "disabled"
	cfg.Enabled = false
	return cfg
}

// Fixed geometry fixture. The aircraft sits at one exact coordinate with no
// motion, so a station placed at a known great-circle distance along the same
// meridian has an exactly known range.
const (
	fixedLatitudeDegrees  = 50.0
	fixedLongitudeDegrees = 14.0
)

// fixedGeometryConfig returns a single stationary aircraft at the fixed
// coordinate and the given pressure altitude. Every spawn range is degenerate,
// so the aircraft truth needs no tolerance.
func fixedGeometryConfig(altitudeFeet float64) Config {
	return Config{
		ID:                   "fixed-geometry",
		StartTime:            fixtureStart,
		Seed:                 11,
		InitialAircraftCount: 1,
		SpeedHundredths:      100,
		Spawn: SpawnConfig{
			LatitudeDegrees:           Range{Min: fixedLatitudeDegrees, Max: fixedLatitudeDegrees},
			LongitudeDegrees:          Range{Min: fixedLongitudeDegrees, Max: fixedLongitudeDegrees},
			AltitudeFeet:              Range{Min: altitudeFeet, Max: altitudeFeet},
			GroundSpeedKnots:          Range{Min: 0, Max: 0},
			TrackDegrees:              Range{Min: 0, Max: 0},
			VerticalRateFeetPerMinute: Range{Min: 0, Max: 0},
		},
	}
}

// stationAtDistance places a station due south of the fixed coordinate, so the
// aircraft of fixedGeometryConfig is exactly surfaceMetres away along the
// model sphere.
func stationAtDistance(id string, surfaceMetres float64) StationConfig {
	cfg := validStationConfig()
	cfg.ID = id
	cfg.LongitudeDegrees = fixedLongitudeDegrees
	cfg.LatitudeDegrees = fixedLatitudeDegrees - (surfaceMetres/earthRadiusMetres)*180/math.Pi
	return cfg
}
