package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/httpserver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulatorapi"
	"github.com/miroslav-matejovsky/adsb-testbench/testbench"
)

// ErrUsage identifies invalid command-line arguments.
var ErrUsage = errors.New("invalid command line")

// Exit statuses.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Environment is everything a command run needs from its process. Tests
// inject each field; Main supplies the real process values.
type Environment struct {
	// Stdout receives help output; Stderr receives usage errors and logs.
	Stdout io.Writer
	Stderr io.Writer
	// ReadFile reads the configuration file.
	ReadFile func(path string) ([]byte, error)
	// Listen binds the validated listen address.
	Listen func(network, address string) (net.Listener, error)
	// NewRunID returns the effective run ID for a configured label. It must
	// return a fresh identity on every call.
	NewRunID func(label string) (string, error)
	// Ready, when not nil, receives the bound address once serving started.
	Ready func(address string)
}

// FreshRunID appends a cryptographically random suffix to label, so a
// configuration reused verbatim still names a new engine lifetime.
func FreshRunID(label string) (string, error) {
	suffix := make([]byte, 10)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate run identity: %w", err)
	}
	return fmt.Sprintf("%s-%x", label, suffix), nil
}

// ProcessEnvironment returns the real process environment.
func ProcessEnvironment() Environment {
	return Environment{
		Stdout: os.Stdout, Stderr: os.Stderr, ReadFile: os.ReadFile,
		Listen: net.Listen, NewRunID: FreshRunID,
	}
}

// Main runs one command in the current process and returns its exit status.
// Interrupt and termination signals cancel the run.
func Main(mode testbench.Mode, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ExitStatus(Run(ctx, mode, args, ProcessEnvironment()))
}

// ExitStatus maps a Run result to a process exit status: success and -help
// are 0, usage errors 2, and every other failure 1.
func ExitStatus(err error) int {
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return ExitOK
	case errors.Is(err, ErrUsage):
		return ExitUsage
	default:
		return ExitFailure
	}
}

// arguments are the parsed command-line flags.
type arguments struct {
	configPath     string
	configMaxBytes int
}

func parseArguments(mode testbench.Mode, args []string, env Environment) (arguments, error) {
	flags := flag.NewFlagSet(string(mode), flag.ContinueOnError)
	flags.SetOutput(env.Stderr)
	configPath := flags.String("config", "", "path of the complete JSON configuration file (required)")
	maxBytes := flags.String("config-max-bytes", "", "positive upper bound on the configuration file size in bytes (required)")
	flags.Usage = func() {
		_, _ = fmt.Fprintf(flags.Output(), "usage: %s -config FILE -config-max-bytes N\n", mode)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(env.Stdout)
			flags.Usage()
			return arguments{}, err
		}
		return arguments{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}
	if flags.NArg() > 0 {
		return arguments{}, fmt.Errorf("%w: unexpected positional arguments %q", ErrUsage, flags.Args())
	}
	if *configPath == "" {
		return arguments{}, fmt.Errorf("%w: -config is required", ErrUsage)
	}
	limit, err := strconv.Atoi(*maxBytes)
	if err != nil || limit <= 0 {
		return arguments{}, fmt.Errorf("%w: -config-max-bytes must be a positive integer", ErrUsage)
	}
	return arguments{configPath: *configPath, configMaxBytes: limit}, nil
}

// Run executes one command: it parses flags and the complete configuration,
// validates everything before creating a runtime or binding a listener,
// composes the application, binds the address, and serves until ctx is
// canceled or a failure occurs. HTTP drains before the simulator stops.
// Every cause is returned; fatal errors are logged once.
func Run(ctx context.Context, mode testbench.Mode, args []string, env Environment) error {
	parsed, err := parseArguments(mode, args, env)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprintln(env.Stderr, err)
		}
		return err
	}
	data, err := readBounded(env, parsed)
	if err != nil {
		_, _ = fmt.Fprintln(env.Stderr, err)
		return err
	}
	logging, err := ParseLogging(data, mode)
	if err != nil {
		_, _ = fmt.Fprintln(env.Stderr, err)
		return err
	}
	logger := newLogger(env.Stderr, logging)
	if err := run(ctx, mode, data, env, logger); err != nil {
		logger.Error("command failed", "mode", mode, "error", err)
		return err
	}
	return nil
}

func readBounded(env Environment, parsed arguments) ([]byte, error) {
	data, err := env.ReadFile(parsed.configPath)
	if err != nil {
		return nil, fmt.Errorf("read configuration %q: %w", parsed.configPath, err)
	}
	if len(data) > parsed.configMaxBytes {
		return nil, fmt.Errorf("%w: configuration %q has %d bytes, above -config-max-bytes %d",
			ErrInvalidConfig, parsed.configPath, len(data), parsed.configMaxBytes)
	}
	return data, nil
}

func newLogger(output io.Writer, settings LoggingSettings) *slog.Logger {
	options := &slog.HandlerOptions{Level: settings.Level}
	if settings.Format == "json" {
		return slog.New(slog.NewJSONHandler(output, options))
	}
	return slog.New(slog.NewTextHandler(output, options))
}

func run(ctx context.Context, mode testbench.Mode, data []byte, env Environment, logger *slog.Logger) error {
	report := func(err error) { logger.Error("service error", "error", err) }
	deps := Dependencies{ReportError: report}
	if mode == testbench.ModeDisplay {
		settings, err := ParseTransport(data)
		if err != nil {
			return err
		}
		transport := NewTransport(settings)
		defer transport.CloseIdleConnections()
		deps.Transport = transport
	}
	file, err := ParseFile(data, mode, deps)
	if err != nil {
		return err
	}

	app, start, err := compose(ctx, file, env)
	if err != nil {
		return err
	}
	listener, err := env.Listen("tcp", file.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("bind %s: %w", file.Server.ListenAddress, err)
	}
	// The listener is closed here on every failure before serving starts;
	// once httpserver.Run starts it owns the listener.
	if err := start(ctx); err != nil {
		return errors.Join(err, closeListener(listener))
	}
	attributes := []any{"mode", mode, "address", listener.Addr().String(), "publicBasePath", file.Server.PublicBasePath}
	if api := app.SimulatorAPI(); api != nil {
		attributes = append(attributes, "runId", api.RunID())
	}
	logger.Info("serving", attributes...)
	if env.Ready != nil {
		env.Ready(listener.Addr().String())
	}
	if err := httpserver.Run(ctx, file.Server.HTTP, logger, listener, mount(file.Server.PublicBasePath, app.Handler()), app.Run); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("stopped", "mode", mode)
	return nil
}

// mount serves the application below its public prefix and strips the
// prefix once, as any embedding host would. Paths outside the prefix are
// 404.
func mount(prefix string, handler http.Handler) http.Handler {
	if prefix == "/" {
		return handler
	}
	mux := http.NewServeMux()
	mux.Handle(prefix, http.StripPrefix(strings.TrimSuffix(prefix, "/"), handler))
	return mux
}

func closeListener(listener net.Listener) error {
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close listener: %w", err)
	}
	return nil
}

// compose builds the application for file. For simulator modes the
// runtime is created paused with a fresh run ID and every configured
// station is installed; the returned start function restores the
// configured speed immediately before serving, so construction and station
// installation never advance traffic.
func compose(ctx context.Context, file File, env Environment) (*testbench.App, func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	switch file.Mode {
	case testbench.ModeDisplay:
		app, err := testbench.NewDisplay(testbench.DisplayConfig{
			PublicBasePath: file.Server.PublicBasePath, Display: file.Display,
			Source: file.Source, Aircraft: file.Aircraft,
		})
		return app, noop, err
	case testbench.ModeCombined, testbench.ModeSimulator:
	default:
		return nil, nil, fmt.Errorf("%w: unknown mode %q", ErrInvalidConfig, file.Mode)
	}

	runID, err := env.NewRunID(file.Simulator.Simulation.ID)
	if err != nil {
		return nil, nil, err
	}
	paused := file.Simulator
	paused.Simulation.ID = runID
	speed := int(paused.Simulation.SpeedHundredths)
	paused.Simulation.SpeedHundredths = 0

	var app *testbench.App
	if file.Mode == testbench.ModeCombined {
		app, err = testbench.New(testbench.Config{
			PublicBasePath: file.Server.PublicBasePath, Simulator: paused, API: file.API,
			Display: file.Display, Manager: file.Manager, Aircraft: file.Aircraft,
		})
	} else {
		app, err = testbench.NewSimulator(testbench.SimulatorConfig{
			PublicBasePath: file.Server.PublicBasePath, Simulator: paused, API: file.API, Manager: file.Manager,
		})
	}
	if err != nil {
		return nil, nil, err
	}
	api := app.SimulatorAPI()
	for _, station := range file.Stations {
		if _, err := api.AddStation(ctx, simulatorapi.AddStationCommand{RunID: runID, Station: station}); err != nil {
			return nil, nil, fmt.Errorf("install configured station %q: %w", station.ID, err)
		}
	}
	start := func(ctx context.Context) error {
		if _, err := api.SetSpeed(ctx, simulatorapi.SpeedCommand{RunID: runID, SpeedHundredths: speed}); err != nil {
			return fmt.Errorf("restore configured speed: %w", err)
		}
		return nil
	}
	return app, start, nil
}
