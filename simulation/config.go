package simulation

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// MaxSpeedHundredths is the largest accepted speed scale, one hundred
	// times real time.
	MaxSpeedHundredths = 10000
	maxSpeedHundredths = MaxSpeedHundredths
)

// Representable virtual instants. time.Time can hold wider values, but the
// engine keeps every timestamp inside the four-digit year range so that
// formatted output and callers stay unambiguous.
const (
	minYear = 1
	maxYear = 9999
)

// rangeDomain is the accepted domain of one SpawnConfig field.
// When upperExclusive is set, hi is a legal sampling endpoint but never a
// legal sampled value, so the lower endpoint must stay strictly below it.
type rangeDomain struct {
	name           string
	unit           string
	lo             float64
	hi             float64
	upperExclusive bool
}

// spawnDomains lists every SpawnConfig field in its documented draw order.
// The accessor returns the configured range for one config.
var spawnDomains = []struct {
	rangeDomain
	get func(SpawnConfig) Range
}{
	{rangeDomain{"LatitudeDegrees", "degrees", -85, 85, false}, func(s SpawnConfig) Range { return s.LatitudeDegrees }},
	{rangeDomain{"LongitudeDegrees", "degrees", -180, 180, false}, func(s SpawnConfig) Range { return s.LongitudeDegrees }},
	{rangeDomain{"AltitudeFeet", "feet", -1000, 50175, false}, func(s SpawnConfig) Range { return s.AltitudeFeet }},
	{rangeDomain{"GroundSpeedKnots", "knots", 0, 1000, false}, func(s SpawnConfig) Range { return s.GroundSpeedKnots }},
	{rangeDomain{"TrackDegrees", "degrees", 0, 360, true}, func(s SpawnConfig) Range { return s.TrackDegrees }},
	{rangeDomain{"VerticalRateFeetPerMinute", "feet per minute", -10000, 10000, false}, func(s SpawnConfig) Range { return s.VerticalRateFeetPerMinute }},
}

// normalizeConfig validates every configuration field and returns an
// accepted copy. StartTime becomes UTC without a monotonic component.
// Meaningful zero values are preserved exactly.
func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return Config{}, fmt.Errorf("%w: ID must contain a non-whitespace character", ErrInvalid)
	}
	if cfg.StartTime.IsZero() {
		return Config{}, fmt.Errorf("%w: StartTime must be nonzero", ErrInvalid)
	}
	start := cfg.StartTime.UTC().Round(0)
	if year := start.Year(); year < minYear || year > maxYear {
		return Config{}, fmt.Errorf("%w: StartTime year %d is outside %d-%d", ErrInvalid, year, minYear, maxYear)
	}
	if cfg.InitialAircraftCount < 0 || cfg.InitialAircraftCount > MaxAircraft {
		return Config{}, fmt.Errorf("%w: InitialAircraftCount %d is outside 0-%d",
			ErrInvalid, cfg.InitialAircraftCount, MaxAircraft)
	}
	if cfg.SpeedHundredths > maxSpeedHundredths {
		return Config{}, fmt.Errorf("%w: SpeedHundredths %d is outside 0-%d",
			ErrInvalid, cfg.SpeedHundredths, maxSpeedHundredths)
	}
	for _, domain := range spawnDomains {
		if err := validateRange(domain.get(cfg.Spawn), domain.rangeDomain); err != nil {
			return Config{}, err
		}
	}
	cfg.StartTime = start
	return cfg, nil
}

// validateRange checks one sampling range against its documented domain.
func validateRange(r Range, d rangeDomain) error {
	for name, v := range map[string]float64{"Min": r.Min, "Max": r.Max} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: Spawn.%s.%s must be a finite number", ErrInvalid, d.name, name)
		}
	}
	if r.Min > r.Max {
		return fmt.Errorf("%w: Spawn.%s.Min %g is above Max %g", ErrInvalid, d.name, r.Min, r.Max)
	}
	if r.Min < d.lo || r.Max > d.hi {
		return fmt.Errorf("%w: Spawn.%s [%g,%g] leaves the accepted %s range [%g,%g]",
			ErrInvalid, d.name, r.Min, r.Max, d.unit, d.lo, d.hi)
	}
	if d.upperExclusive && r.Min >= d.hi {
		return fmt.Errorf("%w: Spawn.%s.Min %g must be below %g because %g is never a sampled value",
			ErrInvalid, d.name, r.Min, d.hi, d.hi)
	}
	return nil
}

// validCount reports whether an aircraft count is inside the engine policy.
func validCount(count int) error {
	if count < 0 || count > MaxAircraft {
		return fmt.Errorf("%w: aircraft count %d is outside 0-%d", ErrInvalid, count, MaxAircraft)
	}
	return nil
}

// validSpeed reports whether a speed scale is inside the engine policy.
func validSpeed(speed uint16) error {
	if speed > maxSpeedHundredths {
		return fmt.Errorf("%w: speed %d is outside 0-%d hundredths", ErrInvalid, speed, maxSpeedHundredths)
	}
	return nil
}

// withinYearBounds reports whether an instant stays inside years 1-9999.
func withinYearBounds(t time.Time) bool {
	year := t.UTC().Year()
	return year >= minYear && year <= maxYear
}
