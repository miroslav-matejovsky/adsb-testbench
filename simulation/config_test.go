package simulation

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigNormalizeAccepts(t *testing.T) {
	t.Parallel()

	local := time.FixedZone("test", 2*60*60)
	cfg := validConfig()
	cfg.StartTime = time.Date(2024, time.March, 5, 14, 0, 0, 0, local)

	got, err := normalizeConfig(cfg)
	require.NoError(t, err)
	require.Equal(t, time.UTC, got.StartTime.Location())
	require.True(t, got.StartTime.Equal(fixtureStart))
	require.Equal(t, cfg.ID, got.ID)
	require.Equal(t, cfg.Seed, got.Seed)
	require.Equal(t, cfg.Spawn, got.Spawn)
}

// Meaningful zeros must survive normalization untouched.
func TestConfigNormalizeKeepsZeros(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Seed = 0
	cfg.InitialAircraftCount = 0
	cfg.SpeedHundredths = 0
	cfg.Spawn.LatitudeDegrees = Range{Min: 0, Max: 0}
	cfg.Spawn.LongitudeDegrees = Range{Min: 0, Max: 0}
	cfg.Spawn.GroundSpeedKnots = Range{Min: 0, Max: 0}
	cfg.Spawn.TrackDegrees = Range{Min: 0, Max: 0}
	cfg.Spawn.VerticalRateFeetPerMinute = Range{Min: 0, Max: 0}

	got, err := normalizeConfig(cfg)
	require.NoError(t, err)
	cfg.StartTime = cfg.StartTime.UTC()
	require.Equal(t, cfg, got)
}

func TestConfigNormalizeRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"empty id", func(c *Config) { c.ID = "" }},
		{"blank id", func(c *Config) { c.ID = " \t\n" }},
		{"zero start", func(c *Config) { c.StartTime = time.Time{} }},
		{"year zero", func(c *Config) { c.StartTime = time.Date(0, time.December, 31, 0, 0, 0, 0, time.UTC) }},
		{"year ten thousand", func(c *Config) { c.StartTime = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC) }},
		{"negative count", func(c *Config) { c.InitialAircraftCount = -1 }},
		{"count above max", func(c *Config) { c.InitialAircraftCount = MaxAircraft + 1 }},
		{"speed above max", func(c *Config) { c.SpeedHundredths = maxSpeedHundredths + 1 }},
		{"latitude below", func(c *Config) { c.Spawn.LatitudeDegrees = Range{Min: -85.1, Max: 0} }},
		{"latitude above", func(c *Config) { c.Spawn.LatitudeDegrees = Range{Min: 0, Max: 85.1} }},
		{"longitude below", func(c *Config) { c.Spawn.LongitudeDegrees = Range{Min: -180.1, Max: 0} }},
		{"longitude above", func(c *Config) { c.Spawn.LongitudeDegrees = Range{Min: 0, Max: 180.1} }},
		{"altitude below", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: -1000.5, Max: 0} }},
		{"altitude above", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: 0, Max: 50175.5} }},
		{"ground speed below", func(c *Config) { c.Spawn.GroundSpeedKnots = Range{Min: -1, Max: 10} }},
		{"ground speed above", func(c *Config) { c.Spawn.GroundSpeedKnots = Range{Min: 0, Max: 1000.5} }},
		{"track below", func(c *Config) { c.Spawn.TrackDegrees = Range{Min: -0.5, Max: 10} }},
		{"track above", func(c *Config) { c.Spawn.TrackDegrees = Range{Min: 0, Max: 360.5} }},
		{"track exactly 360", func(c *Config) { c.Spawn.TrackDegrees = Range{Min: 360, Max: 360} }},
		{"vertical rate below", func(c *Config) { c.Spawn.VerticalRateFeetPerMinute = Range{Min: -10001, Max: 0} }},
		{"vertical rate above", func(c *Config) { c.Spawn.VerticalRateFeetPerMinute = Range{Min: 0, Max: 10001} }},
		{"inverted interval", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: 10000, Max: 9000} }},
		{"nan min", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: math.NaN(), Max: 9000} }},
		{"nan max", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: 9000, Max: math.NaN()} }},
		{"positive infinity", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: 0, Max: math.Inf(1)} }},
		{"negative infinity", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: math.Inf(-1), Max: 0} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			test.change(&cfg)
			got, err := normalizeConfig(cfg)
			require.ErrorIs(t, err, ErrInvalid)
			require.Equal(t, Config{}, got)
			require.ErrorIs(t, cfg.Validate(), ErrInvalid)
		})
	}
	require.NoError(t, validConfig().Validate())
}

// Exact domain endpoints and degenerate ranges are accepted.
func TestConfigNormalizeEdgeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"latitude endpoints", func(c *Config) { c.Spawn.LatitudeDegrees = Range{Min: -85, Max: 85} }},
		{"longitude endpoints", func(c *Config) { c.Spawn.LongitudeDegrees = Range{Min: -180, Max: 180} }},
		{"altitude endpoints", func(c *Config) { c.Spawn.AltitudeFeet = Range{Min: -1000, Max: 50175} }},
		{"ground speed endpoints", func(c *Config) { c.Spawn.GroundSpeedKnots = Range{Min: 0, Max: 1000} }},
		{"track open upper endpoint", func(c *Config) { c.Spawn.TrackDegrees = Range{Min: 359.9, Max: 360} }},
		{"track exact value", func(c *Config) { c.Spawn.TrackDegrees = Range{Min: 359.999, Max: 359.999} }},
		{"vertical rate endpoints", func(c *Config) { c.Spawn.VerticalRateFeetPerMinute = Range{Min: -10000, Max: 10000} }},
		{"maximum count", func(c *Config) { c.InitialAircraftCount = MaxAircraft }},
		{"maximum speed", func(c *Config) { c.SpeedHundredths = maxSpeedHundredths }},
		{"first year", func(c *Config) { c.StartTime = time.Date(1, time.January, 1, 0, 0, 1, 0, time.UTC) }},
		{"last year", func(c *Config) { c.StartTime = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			test.change(&cfg)
			_, err := normalizeConfig(cfg)
			require.NoError(t, err)
		})
	}
}

func TestRangeExact(t *testing.T) {
	t.Parallel()

	require.True(t, Range{Min: 5, Max: 5}.exact())
	require.True(t, Range{}.exact())
	require.False(t, Range{Min: 5, Max: 5.000001}.exact())
}

func TestValidCountAndSpeed(t *testing.T) {
	t.Parallel()

	require.NoError(t, validCount(0))
	require.NoError(t, validCount(MaxAircraft))
	require.ErrorIs(t, validCount(-1), ErrInvalid)
	require.ErrorIs(t, validCount(MaxAircraft+1), ErrInvalid)

	require.NoError(t, validSpeed(0))
	require.NoError(t, validSpeed(maxSpeedHundredths))
	require.ErrorIs(t, validSpeed(maxSpeedHundredths+1), ErrInvalid)
}

func TestWithinYearBounds(t *testing.T) {
	t.Parallel()

	require.True(t, withinYearBounds(fixtureStart))
	require.True(t, withinYearBounds(time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)))
	require.False(t, withinYearBounds(time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)))
	require.False(t, withinYearBounds(time.Date(0, time.December, 31, 23, 59, 59, 0, time.UTC)))
}
