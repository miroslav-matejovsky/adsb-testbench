package simulation

import (
	"fmt"
	"math"
	"time"
)

// Station identifier limits. Identifiers are caller supplied, preserved
// exactly, compared case sensitively, and reserved for the whole run.
const (
	minStationIDBytes = 1
	maxStationIDBytes = 64
)

// StationConfig is the complete explicit configuration of one receiving
// station. Every field must be assigned; the engine supplies no defaults and
// performs no normalization, so an accepted configuration is stored exactly as
// given. Zero values are meaningful: 0 dBi gain, 0 dB system loss, 0 m antenna
// height, 0 m site elevation, and probability 0 are all valid settings.
type StationConfig struct {
	// ID names the station. It is 1 to 64 bytes of ASCII letters, digits,
	// hyphen, or underscore, is compared case sensitively, must be unique
	// within a run, and is never reused after the station is removed.
	ID string
	// Enabled selects whether the station evaluates transmissions at all.
	// A disabled station is fully configured and receives nothing.
	Enabled bool
	// LatitudeDegrees is the station latitude in degrees, within [-90,90].
	LatitudeDegrees float64
	// LongitudeDegrees is the station longitude in degrees, within [-180,180].
	LongitudeDegrees float64
	// SiteElevationMetres is the ground elevation of the site in metres above
	// the model sphere, within [-500,9000].
	SiteElevationMetres float64
	// AntennaHeightMetres is the antenna height in metres above the site
	// elevation, within [0,500].
	AntennaHeightMetres float64
	// AntennaGainDBi is the antenna gain in dBi, within [-10,40].
	AntennaGainDBi float64
	// SensitivityDBm is the lowest received power the receiver accepts, in
	// dBm, within [-140,0].
	SensitivityDBm float64
	// SystemLossDB is the fixed receive-path loss in dB, within [0,30].
	SystemLossDB float64
	// FrameLossProbability is the probability that an otherwise receivable
	// transmission is dropped, within [0,1]. Exactly one random value is drawn
	// per eligible transmission whatever this value is.
	FrameLossProbability float64
}

// Station is one configured receiving station as held by the engine.
type Station struct {
	// Config is the accepted configuration, stored exactly as supplied.
	Config StationConfig
	// Revision starts at 1 when the station is created and increments on every
	// accepted update, including one that assigns identical settings.
	// Callers supply the revision they last observed when editing or removing.
	Revision uint64
	// CreatedAt is the virtual instant at which the station was created.
	// A station receives only transmissions emitted at or after it.
	CreatedAt time.Time
}

// ValidateStationConfig checks every station field without changing an engine.
// Runtime command serializers use it to reject invalid edits before settling
// elapsed real time.
func ValidateStationConfig(cfg StationConfig) error {
	return validateStation(cfg)
}

// stationDomains lists every numeric StationConfig field with its accepted
// domain, in declaration order.
var stationDomains = []struct {
	name string
	unit string
	lo   float64
	hi   float64
	get  func(StationConfig) float64
}{
	{"LatitudeDegrees", "degrees", -90, 90, func(s StationConfig) float64 { return s.LatitudeDegrees }},
	{"LongitudeDegrees", "degrees", -180, 180, func(s StationConfig) float64 { return s.LongitudeDegrees }},
	{"SiteElevationMetres", "metres", -500, 9000, func(s StationConfig) float64 { return s.SiteElevationMetres }},
	{"AntennaHeightMetres", "metres", 0, 500, func(s StationConfig) float64 { return s.AntennaHeightMetres }},
	{"AntennaGainDBi", "dBi", -10, 40, func(s StationConfig) float64 { return s.AntennaGainDBi }},
	{"SensitivityDBm", "dBm", -140, 0, func(s StationConfig) float64 { return s.SensitivityDBm }},
	{"SystemLossDB", "dB", 0, 30, func(s StationConfig) float64 { return s.SystemLossDB }},
	{"FrameLossProbability", "probability", 0, 1, func(s StationConfig) float64 { return s.FrameLossProbability }},
}

// validateStation checks the identifier and every numeric domain. It is the
// single rule set shared by AddStation, UpdateStation, and EstimateCoverage.
func validateStation(cfg StationConfig) error {
	if err := validateStationID(cfg.ID); err != nil {
		return err
	}
	for _, domain := range stationDomains {
		v := domain.get(cfg)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: station %s field %s must be a finite number",
				ErrInvalid, cfg.ID, domain.name)
		}
		if v < domain.lo || v > domain.hi {
			return fmt.Errorf("%w: station %s field %s is %g, outside the accepted %s range [%g,%g]",
				ErrInvalid, cfg.ID, domain.name, v, domain.unit, domain.lo, domain.hi)
		}
	}
	return nil
}

// validateStationID checks length and character set. The identifier is neither
// trimmed nor case folded, so leading or trailing spaces are simply invalid.
func validateStationID(id string) error {
	if len(id) < minStationIDBytes || len(id) > maxStationIDBytes {
		return fmt.Errorf("%w: station ID has %d bytes, want %d-%d",
			ErrInvalid, len(id), minStationIDBytes, maxStationIDBytes)
	}
	for i := range len(id) {
		if !stationIDByte(id[i]) {
			return fmt.Errorf("%w: station ID byte %d is %q, want an ASCII letter, digit, hyphen, or underscore",
				ErrInvalid, i, id[i])
		}
	}
	return nil
}

// stationIDByte reports whether one byte is allowed in a station identifier.
func stationIDByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-' || b == '_':
		return true
	default:
		return false
	}
}
