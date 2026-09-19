package ui

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
)

// ErrInvalidConfig identifies a UI configuration outside its accepted domain.
var ErrInvalidConfig = errors.New("invalid UI configuration")

// Bounds shared by the Go validators and the browser validators in
// static/config.js. They are view policy, not simulator limits: the servers
// stay authoritative for every submitted value.
const (
	// MaxSelectedStations mirrors the display's selection bound.
	MaxSelectedStations = 8
	// MaxHistoryPageSize mirrors the reception history page bound.
	MaxHistoryPageSize = 1000
	// MaxHistoryRecords bounds the reception rows one browser component keeps.
	MaxHistoryRecords = 100000
	// MaxZoom is the largest accepted map zoom level.
	MaxZoom = 24
)

// ManagerSettings are the serializable manager component settings. Commands
// read them from configuration files; URL bases are derived separately.
type ManagerSettings struct {
	// PollIntervalMilliseconds is the pause between the end of one refresh
	// and the start of the next. It must be positive.
	PollIntervalMilliseconds int
	// RequestTimeoutMilliseconds bounds one browser request. It must be
	// positive.
	RequestTimeoutMilliseconds int
	// MaxResponseBytes bounds one response body read by the browser. It must
	// be at least simulatorapi.MinResponseBytes.
	MaxResponseBytes int
	// ResumeSpeedHundredths is the positive speed "Resume" sends when the
	// page has never observed a positive speed in the current run.
	ResumeSpeedHundredths int
}

// ManagerConfig is the complete configuration of one manager page or
// component. Both bases are root-relative paths or absolute http(s) URLs
// that must resolve to the page's own origin.
type ManagerConfig struct {
	// APIBaseURL is the mounted simulator API, for example "/api/simulator/".
	APIBaseURL string
	// AssetBaseURL is the mounted Assets handler, for example "/assets/".
	AssetBaseURL string
	ManagerSettings
}

// Tiles configures an optional host-approved map tile provider. A nil *Tiles
// disables tiles and the map then makes no tile request at all.
type Tiles struct {
	// URLTemplate is an http(s) or root-relative URL containing the {z},
	// {x} and {y} placeholders and no other placeholder.
	URLTemplate string
	// AttributionText is the provider attribution, shown as plain text.
	AttributionText string
	// AttributionURL is an absolute http(s) link for the attribution.
	AttributionURL string
	// MinZoom and MaxZoom bound the tile zoom levels, 0 <= min <= max <= 24.
	MinZoom int
	MaxZoom int
}

// AircraftDisplaySettings are the serializable aircraft display settings.
type AircraftDisplaySettings struct {
	// PollIntervalMilliseconds is the pause between the end of one refresh
	// and the start of the next. It must be positive.
	PollIntervalMilliseconds int
	// RequestTimeoutMilliseconds bounds one browser request.
	RequestTimeoutMilliseconds int
	// MaxResponseBytes bounds one response body read by the browser.
	MaxResponseBytes int
	// StationIDs is the initial explicit station selection. An empty,
	// non-nil slice selects no station.
	StationIDs []string
	// FreshFor and LostAfter classify a track by the virtual age of its last
	// received frame: fresh at age <= FreshFor, stale up to LostAfter, and
	// lost beyond. 0 < FreshFor < LostAfter. They never extend a field
	// lifetime.
	FreshFor  time.Duration
	LostAfter time.Duration
	// HistoryPageSize is the reception history page size, 1..1000.
	HistoryPageSize int
	// MaxHistoryRecords bounds loaded inspection rows. It is at least
	// HistoryPageSize.
	MaxHistoryRecords int
	// InitialLatitudeDegrees, InitialLongitudeDegrees and InitialZoom set
	// the initial map view.
	InitialLatitudeDegrees  float64
	InitialLongitudeDegrees float64
	InitialZoom             int
	// Tiles is the optional tile provider; nil disables tiles.
	Tiles *Tiles
}

// AircraftDisplayConfig is the complete configuration of one aircraft
// display page or component. APIBaseURL addresses the display backend.
type AircraftDisplayConfig struct {
	APIBaseURL   string
	AssetBaseURL string
	AircraftDisplaySettings
}

// Validate checks every manager setting.
func (c ManagerConfig) Validate() error {
	return errors.Join(validateBases(c.APIBaseURL, c.AssetBaseURL), c.ManagerSettings.Validate())
}

// Validate checks every manager setting except URL bases.
func (s ManagerSettings) Validate() error {
	return errors.Join(
		validateCommon(s.PollIntervalMilliseconds, s.RequestTimeoutMilliseconds, s.MaxResponseBytes),
		positive("resumeSpeedHundredths", s.ResumeSpeedHundredths),
	)
}

// Validate checks every aircraft display setting.
func (c AircraftDisplayConfig) Validate() error {
	return errors.Join(validateBases(c.APIBaseURL, c.AssetBaseURL), c.AircraftDisplaySettings.Validate())
}

// Validate checks every aircraft display setting except URL bases.
func (s AircraftDisplaySettings) Validate() error {
	errs := []error{
		validateCommon(s.PollIntervalMilliseconds, s.RequestTimeoutMilliseconds, s.MaxResponseBytes),
		validateStationIDs(s.StationIDs),
	}
	if s.FreshFor <= 0 || s.LostAfter <= s.FreshFor {
		errs = append(errs, invalid("freshForNanoseconds %d and lostAfterNanoseconds %d must satisfy 0 < fresh < lost",
			s.FreshFor, s.LostAfter))
	}
	if s.HistoryPageSize < 1 || s.HistoryPageSize > MaxHistoryPageSize {
		errs = append(errs, invalid("historyPageSize %d is outside [1,%d]", s.HistoryPageSize, MaxHistoryPageSize))
	}
	if s.MaxHistoryRecords < s.HistoryPageSize || s.MaxHistoryRecords > MaxHistoryRecords {
		errs = append(errs, invalid("maxHistoryRecords %d is outside [historyPageSize,%d]",
			s.MaxHistoryRecords, MaxHistoryRecords))
	}
	if !finiteWithin(s.InitialLatitudeDegrees, -90, 90) {
		errs = append(errs, invalid("initialLatitudeDegrees %g is outside [-90,90]", s.InitialLatitudeDegrees))
	}
	if !finiteWithin(s.InitialLongitudeDegrees, -180, 180) {
		errs = append(errs, invalid("initialLongitudeDegrees %g is outside [-180,180]", s.InitialLongitudeDegrees))
	}
	if s.InitialZoom < 0 || s.InitialZoom > MaxZoom {
		errs = append(errs, invalid("initialZoom %d is outside [0,%d]", s.InitialZoom, MaxZoom))
	}
	if s.Tiles != nil {
		errs = append(errs, s.Tiles.Validate())
		if s.InitialZoom < s.Tiles.MinZoom || s.InitialZoom > s.Tiles.MaxZoom {
			errs = append(errs, invalid("initialZoom %d is outside the tile zoom range [%d,%d]",
				s.InitialZoom, s.Tiles.MinZoom, s.Tiles.MaxZoom))
		}
	}
	return errors.Join(errs...)
}

// Validate checks a tile provider.
func (t Tiles) Validate() error {
	var errs []error
	if err := validateTileTemplate(t.URLTemplate); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(t.AttributionText) == "" || hasControl(t.AttributionText) {
		errs = append(errs, invalid("tiles.attributionText must be nonempty text without control characters"))
	}
	if err := validateLink(t.AttributionURL); err != nil {
		errs = append(errs, invalid("tiles.attributionUrl: %v", err))
	}
	if t.MinZoom < 0 || t.MinZoom > t.MaxZoom || t.MaxZoom > MaxZoom {
		errs = append(errs, invalid("tiles zoom range [%d,%d] must satisfy 0 <= min <= max <= %d",
			t.MinZoom, t.MaxZoom, MaxZoom))
	}
	return errors.Join(errs...)
}

func validateBases(api, asset string) error {
	var errs []error
	if _, err := urlpath.ParseBrowserBase(api); err != nil {
		errs = append(errs, fmt.Errorf("%w: apiBaseUrl: %w", ErrInvalidConfig, err))
	}
	if _, err := urlpath.ParseBrowserBase(asset); err != nil {
		errs = append(errs, fmt.Errorf("%w: assetBaseUrl: %w", ErrInvalidConfig, err))
	}
	return errors.Join(errs...)
}

func validateCommon(pollMilliseconds, timeoutMilliseconds, maxResponseBytes int) error {
	errs := []error{
		positive("pollIntervalMilliseconds", pollMilliseconds),
		positive("requestTimeoutMilliseconds", timeoutMilliseconds),
	}
	if maxResponseBytes < simulatorapi.MinResponseBytes {
		errs = append(errs, invalid("maxResponseBytes %d is below the %d byte error envelope budget",
			maxResponseBytes, simulatorapi.MinResponseBytes))
	}
	return errors.Join(errs...)
}

// validateStationIDs applies the simulator's station identifier rules to an
// explicit, possibly empty selection.
func validateStationIDs(ids []string) error {
	if ids == nil {
		return invalid("stationIds must be an array, including when empty")
	}
	if len(ids) > MaxSelectedStations {
		return invalid("at most %d stations may be selected, got %d", MaxSelectedStations, len(ids))
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !validStationID(id) {
			return invalid("station ID %q must be 1-64 ASCII letters, digits, hyphens, or underscores", id)
		}
		if seen[id] {
			return invalid("station %q is selected twice", id)
		}
		seen[id] = true
	}
	return nil
}

func validStationID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := range len(id) {
		if !stationIDByte(id[i]) {
			return false
		}
	}
	return true
}

func stationIDByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// validateTileTemplate requires the {z}, {x} and {y} placeholders exactly
// once each, rejects any other brace, and validates the URL produced by
// substituting safe numbers.
func validateTileTemplate(template string) error {
	for _, placeholder := range []string{"{z}", "{x}", "{y}"} {
		if strings.Count(template, placeholder) != 1 {
			return invalid("tiles.urlTemplate must contain %s exactly once", placeholder)
		}
	}
	substituted := strings.NewReplacer("{z}", "0", "{x}", "0", "{y}", "0").Replace(template)
	if strings.ContainsAny(substituted, "{}") {
		return invalid("tiles.urlTemplate contains an unsupported placeholder")
	}
	if hasControl(substituted) || strings.ContainsAny(substituted, `\#`) {
		return invalid("tiles.urlTemplate contains a control character, backslash, or fragment")
	}
	if strings.HasPrefix(substituted, "/") && !strings.HasPrefix(substituted, "//") {
		return nil
	}
	parsed, err := url.Parse(substituted)
	if err != nil {
		return invalid("tiles.urlTemplate: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return invalid("tiles.urlTemplate must be root-relative or an absolute http(s) URL")
	}
	if parsed.User != nil {
		return invalid("tiles.urlTemplate must not carry userinfo")
	}
	return nil
}

func validateLink(raw string) error {
	if hasControl(raw) || strings.ContainsAny(raw, `\`) {
		return errors.New("contains a control character or backslash")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("must be an absolute http(s) URL without userinfo")
	}
	return nil
}

func hasControl(text string) bool {
	for _, r := range text {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return true
		}
	}
	return false
}

func finiteWithin(value, lo, hi float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= lo && value <= hi
}

func positive(name string, value int) error {
	if value <= 0 {
		return invalid("%s %d must be positive", name, value)
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}
