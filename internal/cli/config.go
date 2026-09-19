package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// ErrInvalidConfig identifies a configuration file outside its accepted
// domain. Every parse and cross-field failure wraps it.
var ErrInvalidConfig = errors.New("invalid configuration")

// Section names of a configuration file.
const (
	sectionComment      = "$comment"
	sectionServer       = "server"
	sectionLogging      = "logging"
	sectionSimulator    = "simulator"
	sectionSimulatorAPI = "simulatorApi"
	sectionStations     = "stations"
	sectionDisplay      = "display"
	sectionManagerUI    = "managerUi"
	sectionAircraftUI   = "aircraftUi"
	sectionSource       = "source"
	sectionTransport    = "transport"
)

// requiredSections lists the exact top-level keys of each command's file.
var requiredSections = map[testbench.Mode][]string{
	testbench.ModeCombined: {sectionComment, sectionServer, sectionLogging, sectionSimulator, sectionSimulatorAPI,
		sectionStations, sectionDisplay, sectionManagerUI, sectionAircraftUI},
	testbench.ModeSimulator: {sectionComment, sectionServer, sectionLogging, sectionSimulator, sectionSimulatorAPI,
		sectionStations, sectionManagerUI},
	testbench.ModeDisplay: {sectionComment, sectionServer, sectionLogging, sectionDisplay, sectionAircraftUI,
		sectionSource, sectionTransport},
}

// ServerSettings is the server section: the listen address, the public
// mount prefix, and every HTTP server limit.
type ServerSettings struct {
	ListenAddress  string
	PublicBasePath string
	HTTP           httpserver.Config
}

// LoggingSettings is the logging section.
type LoggingSettings struct {
	Level  slog.Level
	Format string
}

// TransportSettings is the display command's outbound HTTP transport.
type TransportSettings struct {
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
}

// File is one parsed and validated configuration file. Only the sections of
// its mode are set. Go callbacks and the HTTP client are attached by the
// parser from the supplied dependencies.
type File struct {
	Mode      testbench.Mode
	Comment   string
	Server    ServerSettings
	Logging   LoggingSettings
	Simulator simulator.Config
	API       simulator.APIConfig
	Stations  []simulatorapi.StationSettings
	Display   display.Config
	Manager   ui.ManagerSettings
	Aircraft  ui.AircraftDisplaySettings
	Source    display.HTTPSourceConfig
	Transport TransportSettings
}

// Dependencies are the Go values a configuration file cannot carry.
type Dependencies struct {
	// ReportError receives service failures no client can observe.
	ReportError func(error)
	// Transport is configured from the transport section and used by the
	// display source client. It may be nil outside display mode.
	Transport *http.Transport
}

// document is a strictly parsed file: the presence-tracked object and the
// raw bytes of each section for the reader-based parsers.
type document struct {
	object *simulatorapi.Object
	raw    map[string]json.RawMessage
}

func invalid(section string, err error) error {
	if section == "" {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalidConfig, section, err)
}

// parseDocument strictly parses one complete JSON object and checks that
// its keys are exactly the sections of mode.
func parseDocument(data []byte, mode testbench.Mode) (document, error) {
	required, ok := requiredSections[mode]
	if !ok {
		return document{}, invalid("", fmt.Errorf("unknown mode %q", mode))
	}
	value, err := simulatorapi.ParseJSONBytes(data)
	if err != nil {
		return document{}, invalid("", err)
	}
	object, err := value.Object()
	if err != nil {
		return document{}, invalid("", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return document{}, invalid("", err)
	}
	for key := range raw {
		if !slices.Contains(required, key) {
			return document{}, invalid("", fmt.Errorf("unknown section %q for the %s command", key, mode))
		}
	}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			return document{}, invalid("", fmt.Errorf("missing required section %q", key))
		}
		if bytes.Equal(bytes.TrimSpace(raw[key]), []byte("null")) {
			return document{}, invalid("", fmt.Errorf("section %q must not be null", key))
		}
	}
	return document{object: object, raw: raw}, nil
}

// ParseLogging reads only the logging section so a command can build its
// logger before the remaining sections are bound.
func ParseLogging(data []byte, mode testbench.Mode) (LoggingSettings, error) {
	doc, err := parseDocument(data, mode)
	if err != nil {
		return LoggingSettings{}, err
	}
	return parseLogging(doc)
}

func parseLogging(doc document) (LoggingSettings, error) {
	field, err := doc.object.Field(sectionLogging)
	if err != nil {
		return LoggingSettings{}, invalid(sectionLogging, err)
	}
	object, err := field.Object()
	if err != nil {
		return LoggingSettings{}, invalid(sectionLogging, err)
	}
	level, err := readText(object, "level")
	if err != nil {
		return LoggingSettings{}, invalid(sectionLogging, err)
	}
	format, err := readText(object, "format")
	if err != nil {
		return LoggingSettings{}, invalid(sectionLogging, err)
	}
	if err := object.Done(); err != nil {
		return LoggingSettings{}, invalid(sectionLogging, err)
	}
	settings := LoggingSettings{Format: format}
	switch level {
	case "debug":
		settings.Level = slog.LevelDebug
	case "info":
		settings.Level = slog.LevelInfo
	case "warn":
		settings.Level = slog.LevelWarn
	case "error":
		settings.Level = slog.LevelError
	default:
		return LoggingSettings{}, invalid(sectionLogging, fmt.Errorf("level %q is not debug, info, warn or error", level))
	}
	if format != "text" && format != "json" {
		return LoggingSettings{}, invalid(sectionLogging, fmt.Errorf("format %q is not text or json", format))
	}
	return settings, nil
}

// ParseFile binds and validates every section of a configuration file for
// mode, including cross-field constraints. It creates no runtime, listener
// or connection.
func ParseFile(data []byte, mode testbench.Mode, deps Dependencies) (File, error) {
	doc, err := parseDocument(data, mode)
	if err != nil {
		return File{}, err
	}
	if deps.ReportError == nil {
		return File{}, invalid("", errors.New("the error callback is nil"))
	}
	file := File{Mode: mode}
	if file.Comment, err = readText(doc.object, sectionComment); err != nil || strings.TrimSpace(file.Comment) == "" {
		return File{}, invalid(sectionComment, errors.New("a nonempty string documenting the settings is required"))
	}
	if file.Logging, err = parseLogging(doc); err != nil {
		return File{}, err
	}
	if file.Server, err = parseServer(doc); err != nil {
		return File{}, err
	}
	if slices.Contains(requiredSections[mode], sectionSimulator) {
		if err := parseSimulatorSections(doc, &file, deps); err != nil {
			return File{}, err
		}
	}
	if slices.Contains(requiredSections[mode], sectionDisplay) {
		if file.Display, err = display.ParseConfig(bytes.NewReader(doc.raw[sectionDisplay]), len(doc.raw[sectionDisplay]), deps.ReportError); err != nil {
			return File{}, invalid(sectionDisplay, err)
		}
		if file.Aircraft, err = parseAircraft(doc); err != nil {
			return File{}, err
		}
	}
	if mode == testbench.ModeDisplay {
		if file.Transport, err = parseTransport(doc); err != nil {
			return File{}, err
		}
		if deps.Transport == nil {
			return File{}, invalid(sectionTransport, errors.New("no transport was supplied"))
		}
		client := &http.Client{Transport: deps.Transport}
		raw := doc.raw[sectionSource]
		if file.Source, err = display.ParseHTTPSourceConfig(bytes.NewReader(raw), len(raw), client); err != nil {
			return File{}, invalid(sectionSource, err)
		}
	}
	if err := validateFile(file); err != nil {
		return File{}, err
	}
	return file, nil
}

func parseSimulatorSections(doc document, file *File, deps Dependencies) error {
	var err error
	raw := doc.raw[sectionSimulator]
	if file.Simulator, err = simulator.ParseConfig(bytes.NewReader(raw), len(raw)); err != nil {
		return invalid(sectionSimulator, err)
	}
	raw = doc.raw[sectionSimulatorAPI]
	if file.API, err = simulator.ParseAPIConfig(bytes.NewReader(raw), len(raw), deps.ReportError); err != nil {
		return invalid(sectionSimulatorAPI, err)
	}
	if file.Stations, err = parseStations(doc); err != nil {
		return err
	}
	field, err := doc.object.Field(sectionManagerUI)
	if err != nil {
		return invalid(sectionManagerUI, err)
	}
	if file.Manager, err = ui.ParseManagerSettings(field); err != nil {
		return invalid(sectionManagerUI, err)
	}
	return nil
}

func parseAircraft(doc document) (ui.AircraftDisplaySettings, error) {
	field, err := doc.object.Field(sectionAircraftUI)
	if err != nil {
		return ui.AircraftDisplaySettings{}, invalid(sectionAircraftUI, err)
	}
	settings, err := ui.ParseAircraftDisplaySettings(field)
	if err != nil {
		return ui.AircraftDisplaySettings{}, invalid(sectionAircraftUI, err)
	}
	return settings, nil
}

func parseServer(doc document) (ServerSettings, error) {
	field, err := doc.object.Field(sectionServer)
	if err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	object, err := field.Object()
	if err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	var settings ServerSettings
	if settings.ListenAddress, err = readText(object, "listenAddress"); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	if settings.PublicBasePath, err = readText(object, "publicBasePath"); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	durations := []struct {
		key    string
		target *time.Duration
	}{
		{"readHeaderTimeoutNanoseconds", &settings.HTTP.ReadHeaderTimeout},
		{"readTimeoutNanoseconds", &settings.HTTP.ReadTimeout},
		{"writeTimeoutNanoseconds", &settings.HTTP.WriteTimeout},
		{"idleTimeoutNanoseconds", &settings.HTTP.IdleTimeout},
		{"shutdownTimeoutNanoseconds", &settings.HTTP.ShutdownTimeout},
	}
	for _, duration := range durations {
		if *duration.target, err = readDuration(object, duration.key); err != nil {
			return ServerSettings{}, invalid(sectionServer, err)
		}
	}
	if settings.HTTP.MaxHeaderBytes, err = readInt(object, "maxHeaderBytes"); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	if err := object.Done(); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	if err := ValidateListenAddress(settings.ListenAddress); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	if _, err := urlpath.ParseMountPrefix(settings.PublicBasePath); err != nil {
		return ServerSettings{}, invalid(sectionServer, fmt.Errorf("publicBasePath: %w", err))
	}
	if err := settings.HTTP.Validate(); err != nil {
		return ServerSettings{}, invalid(sectionServer, err)
	}
	return settings, nil
}

func parseStations(doc document) ([]simulatorapi.StationSettings, error) {
	field, err := doc.object.Field(sectionStations)
	if err != nil {
		return nil, invalid(sectionStations, err)
	}
	items, err := field.Array()
	if err != nil {
		return nil, invalid(sectionStations, err)
	}
	stations := make([]simulatorapi.StationSettings, 0, len(items))
	for _, item := range items {
		object, err := item.Object()
		if err != nil {
			return nil, invalid(sectionStations, err)
		}
		var station simulatorapi.StationSettings
		if station.ID, err = readText(object, "id"); err != nil {
			return nil, invalid(sectionStations, err)
		}
		enabled, err := object.Field("enabled")
		if err != nil {
			return nil, invalid(sectionStations, err)
		}
		if station.Enabled, err = enabled.Bool(); err != nil {
			return nil, invalid(sectionStations, err)
		}
		for _, number := range []struct {
			key    string
			target *float64
		}{
			{"latitudeDegrees", &station.LatitudeDegrees},
			{"longitudeDegrees", &station.LongitudeDegrees},
			{"siteElevationMetres", &station.SiteElevationMetres},
			{"antennaHeightMetres", &station.AntennaHeightMetres},
			{"antennaGainDBi", &station.AntennaGainDBi},
			{"sensitivityDBm", &station.SensitivityDBm},
			{"systemLossDB", &station.SystemLossDB},
			{"frameLossProbability", &station.FrameLossProbability},
		} {
			value, err := object.Field(number.key)
			if err != nil {
				return nil, invalid(sectionStations, err)
			}
			if *number.target, err = value.Float(); err != nil {
				return nil, invalid(sectionStations, err)
			}
		}
		if err := object.Done(); err != nil {
			return nil, invalid(sectionStations, err)
		}
		stations = append(stations, station)
	}
	return stations, nil
}

func parseTransport(doc document) (TransportSettings, error) {
	field, err := doc.object.Field(sectionTransport)
	if err != nil {
		return TransportSettings{}, invalid(sectionTransport, err)
	}
	object, err := field.Object()
	if err != nil {
		return TransportSettings{}, invalid(sectionTransport, err)
	}
	var settings TransportSettings
	for _, duration := range []struct {
		key    string
		target *time.Duration
	}{
		{"dialTimeoutNanoseconds", &settings.DialTimeout},
		{"keepAliveNanoseconds", &settings.KeepAlive},
		{"tlsHandshakeTimeoutNanoseconds", &settings.TLSHandshakeTimeout},
		{"responseHeaderTimeoutNanoseconds", &settings.ResponseHeaderTimeout},
		{"idleConnTimeoutNanoseconds", &settings.IdleConnTimeout},
	} {
		if *duration.target, err = readDuration(object, duration.key); err != nil {
			return TransportSettings{}, invalid(sectionTransport, err)
		}
		if *duration.target <= 0 {
			return TransportSettings{}, invalid(sectionTransport, fmt.Errorf("%s must be positive", duration.key))
		}
	}
	for _, count := range []struct {
		key    string
		target *int
	}{
		{"maxIdleConns", &settings.MaxIdleConns},
		{"maxIdleConnsPerHost", &settings.MaxIdleConnsPerHost},
		{"maxConnsPerHost", &settings.MaxConnsPerHost},
	} {
		if *count.target, err = readInt(object, count.key); err != nil {
			return TransportSettings{}, invalid(sectionTransport, err)
		}
		if *count.target <= 0 {
			return TransportSettings{}, invalid(sectionTransport, fmt.Errorf("%s must be positive", count.key))
		}
	}
	if err := object.Done(); err != nil {
		return TransportSettings{}, invalid(sectionTransport, err)
	}
	return settings, nil
}

// ParseTransport reads only the transport section of a display file so the
// command can build its HTTP transport before binding the source section.
func ParseTransport(data []byte) (TransportSettings, error) {
	doc, err := parseDocument(data, testbench.ModeDisplay)
	if err != nil {
		return TransportSettings{}, err
	}
	return parseTransport(doc)
}

// NewTransport builds an HTTP transport from explicit settings. Proxies from
// the environment are not used: the configured source URL is contacted
// directly.
func NewTransport(settings TransportSettings) *http.Transport {
	dialer := &net.Dialer{Timeout: settings.DialTimeout, KeepAlive: settings.KeepAlive}
	return &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   settings.TLSHandshakeTimeout,
		ResponseHeaderTimeout: settings.ResponseHeaderTimeout,
		IdleConnTimeout:       settings.IdleConnTimeout,
		MaxIdleConns:          settings.MaxIdleConns,
		MaxIdleConnsPerHost:   settings.MaxIdleConnsPerHost,
		MaxConnsPerHost:       settings.MaxConnsPerHost,
		ForceAttemptHTTP2:     true,
	}
}

// validateFile checks constraints that span sections.
func validateFile(file File) error {
	var errs []error
	switch file.Mode {
	case testbench.ModeCombined, testbench.ModeSimulator:
		seen := map[string]bool{}
		for _, station := range file.Stations {
			if seen[station.ID] {
				errs = append(errs, fmt.Errorf("station %q is configured twice", station.ID))
			}
			seen[station.ID] = true
		}
		if file.Manager.MaxResponseBytes < file.API.MaxResponseBytes {
			errs = append(errs, fmt.Errorf("managerUi.maxResponseBytes %d is below simulatorApi.maxResponseBytes %d",
				file.Manager.MaxResponseBytes, file.API.MaxResponseBytes))
		}
		if file.Server.HTTP.WriteTimeout <= file.API.RequestTimeout {
			errs = append(errs, fmt.Errorf("server write timeout %s must exceed simulatorApi request timeout %s",
				file.Server.HTTP.WriteTimeout, file.API.RequestTimeout))
		}
		if file.Mode == testbench.ModeCombined {
			for _, id := range file.Aircraft.StationIDs {
				if !seen[id] {
					errs = append(errs, fmt.Errorf("aircraftUi.stationIds selects %q, which is not a configured station", id))
				}
			}
		}
	}
	if file.Mode == testbench.ModeCombined || file.Mode == testbench.ModeDisplay {
		if file.Aircraft.MaxResponseBytes < file.Display.MaxResponseBytes {
			errs = append(errs, fmt.Errorf("aircraftUi.maxResponseBytes %d is below display.maxResponseBytes %d",
				file.Aircraft.MaxResponseBytes, file.Display.MaxResponseBytes))
		}
		if file.Server.HTTP.WriteTimeout <= file.Display.RequestTimeout {
			errs = append(errs, fmt.Errorf("server write timeout %s must exceed display request timeout %s",
				file.Server.HTTP.WriteTimeout, file.Display.RequestTimeout))
		}
	}
	if file.Mode == testbench.ModeDisplay && file.Source.Timeout > file.Display.RequestTimeout {
		errs = append(errs, fmt.Errorf("source timeout %s exceeds display request timeout %s",
			file.Source.Timeout, file.Display.RequestTimeout))
	}
	if len(errs) > 0 {
		return invalid("cross-section", errors.Join(errs...))
	}
	return nil
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
