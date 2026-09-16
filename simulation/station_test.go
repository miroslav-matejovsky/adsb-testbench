package simulation

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStationConfigAccepts(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateStation(validStationConfig()))
}

// Meaningful zero values must be accepted rather than treated as missing.
func TestStationConfigAcceptsZeros(t *testing.T) {
	t.Parallel()

	cfg := validStationConfig()
	cfg.Enabled = false
	cfg.LatitudeDegrees = 0
	cfg.LongitudeDegrees = 0
	cfg.SiteElevationMetres = 0
	cfg.AntennaHeightMetres = 0
	cfg.AntennaGainDBi = 0
	cfg.SystemLossDB = 0
	cfg.FrameLossProbability = 0

	require.NoError(t, validateStation(cfg))
}

func TestStationConfigAcceptsDomainEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*StationConfig)
	}{
		{"latitude low", func(c *StationConfig) { c.LatitudeDegrees = -90 }},
		{"latitude high", func(c *StationConfig) { c.LatitudeDegrees = 90 }},
		{"longitude low", func(c *StationConfig) { c.LongitudeDegrees = -180 }},
		{"longitude high", func(c *StationConfig) { c.LongitudeDegrees = 180 }},
		{"elevation low", func(c *StationConfig) { c.SiteElevationMetres = -500 }},
		{"elevation high", func(c *StationConfig) { c.SiteElevationMetres = 9000 }},
		{"antenna low", func(c *StationConfig) { c.AntennaHeightMetres = 0 }},
		{"antenna high", func(c *StationConfig) { c.AntennaHeightMetres = 500 }},
		{"gain low", func(c *StationConfig) { c.AntennaGainDBi = -10 }},
		{"gain high", func(c *StationConfig) { c.AntennaGainDBi = 40 }},
		{"sensitivity low", func(c *StationConfig) { c.SensitivityDBm = -140 }},
		{"sensitivity high", func(c *StationConfig) { c.SensitivityDBm = 0 }},
		{"loss low", func(c *StationConfig) { c.SystemLossDB = 0 }},
		{"loss high", func(c *StationConfig) { c.SystemLossDB = 30 }},
		{"probability low", func(c *StationConfig) { c.FrameLossProbability = 0 }},
		{"probability high", func(c *StationConfig) { c.FrameLossProbability = 1 }},
		{"shortest id", func(c *StationConfig) { c.ID = "a" }},
		{"longest id", func(c *StationConfig) { c.ID = strings.Repeat("s", maxStationIDBytes) }},
		{"mixed id", func(c *StationConfig) { c.ID = "Station_01-EDDF" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validStationConfig()
			test.change(&cfg)
			require.NoError(t, validateStation(cfg))
		})
	}
}

func TestStationConfigRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*StationConfig)
	}{
		{"latitude below", func(c *StationConfig) { c.LatitudeDegrees = -90.1 }},
		{"latitude above", func(c *StationConfig) { c.LatitudeDegrees = 90.1 }},
		{"longitude below", func(c *StationConfig) { c.LongitudeDegrees = -180.1 }},
		{"longitude above", func(c *StationConfig) { c.LongitudeDegrees = 180.1 }},
		{"elevation below", func(c *StationConfig) { c.SiteElevationMetres = -500.1 }},
		{"elevation above", func(c *StationConfig) { c.SiteElevationMetres = 9000.1 }},
		{"antenna below", func(c *StationConfig) { c.AntennaHeightMetres = -0.1 }},
		{"antenna above", func(c *StationConfig) { c.AntennaHeightMetres = 500.1 }},
		{"gain below", func(c *StationConfig) { c.AntennaGainDBi = -10.1 }},
		{"gain above", func(c *StationConfig) { c.AntennaGainDBi = 40.1 }},
		{"sensitivity below", func(c *StationConfig) { c.SensitivityDBm = -140.1 }},
		{"sensitivity above", func(c *StationConfig) { c.SensitivityDBm = 0.1 }},
		{"loss below", func(c *StationConfig) { c.SystemLossDB = -0.1 }},
		{"loss above", func(c *StationConfig) { c.SystemLossDB = 30.1 }},
		{"probability below", func(c *StationConfig) { c.FrameLossProbability = -0.001 }},
		{"probability above", func(c *StationConfig) { c.FrameLossProbability = 1.001 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validStationConfig()
			test.change(&cfg)
			err := validateStation(cfg)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}

// Every numeric field must reject NaN and both infinities.
func TestStationConfigRejectsNonFinite(t *testing.T) {
	t.Parallel()

	setters := map[string]func(*StationConfig, float64){
		"LatitudeDegrees":      func(c *StationConfig, v float64) { c.LatitudeDegrees = v },
		"LongitudeDegrees":     func(c *StationConfig, v float64) { c.LongitudeDegrees = v },
		"SiteElevationMetres":  func(c *StationConfig, v float64) { c.SiteElevationMetres = v },
		"AntennaHeightMetres":  func(c *StationConfig, v float64) { c.AntennaHeightMetres = v },
		"AntennaGainDBi":       func(c *StationConfig, v float64) { c.AntennaGainDBi = v },
		"SensitivityDBm":       func(c *StationConfig, v float64) { c.SensitivityDBm = v },
		"SystemLossDB":         func(c *StationConfig, v float64) { c.SystemLossDB = v },
		"FrameLossProbability": func(c *StationConfig, v float64) { c.FrameLossProbability = v },
	}
	require.Len(t, setters, len(stationDomains), "every numeric field is covered")

	for name, set := range setters {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			cfg := validStationConfig()
			set(&cfg, value)
			err := validateStation(cfg)
			require.ErrorIs(t, err, ErrInvalid, "%s with %v", name, value)
			require.ErrorContains(t, err, name)
		}
	}
}

func TestStationIdentifierRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("s", maxStationIDBytes+1)},
		{"leading space", " eddf"},
		{"trailing space", "eddf "},
		{"inner space", "ed df"},
		{"dot", "eddf.1"},
		{"slash", "eddf/1"},
		{"colon", "eddf:1"},
		{"non ascii", "eddfé"},
		{"control byte", "eddf\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := validStationConfig()
			cfg.ID = test.id
			require.ErrorIs(t, validateStation(cfg), ErrInvalid)
		})
	}
}

// Identifiers differing only in case are distinct values.
func TestStationIdentifierIsCaseSensitive(t *testing.T) {
	t.Parallel()

	lower := validStationConfig()
	lower.ID = "eddf"
	upper := validStationConfig()
	upper.ID = "EDDF"

	require.NoError(t, validateStation(lower))
	require.NoError(t, validateStation(upper))
	require.NotEqual(t, lower.ID, upper.ID)
}

func TestStationErrorCategoriesAreDistinct(t *testing.T) {
	t.Parallel()

	for _, sentinel := range []error{ErrInvalid, ErrLimit, ErrNotFound, ErrConflict} {
		for _, other := range []error{ErrInvalid, ErrLimit, ErrNotFound, ErrConflict} {
			if sentinel == other {
				continue
			}
			require.False(t, errors.Is(sentinel, other), "%v matches %v", sentinel, other)
		}
	}
}
