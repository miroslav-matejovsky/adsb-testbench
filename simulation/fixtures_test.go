package simulation

import "time"

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
