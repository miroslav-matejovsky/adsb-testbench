package testbench

import (
	"errors"
	"fmt"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// ErrInvalidConfig identifies an application configuration outside its
// accepted domain.
var ErrInvalidConfig = errors.New("invalid test bench configuration")

// Mode names the kind of application an App composes.
type Mode string

const (
	// ModeCombined serves one simulator and one in-process display.
	ModeCombined Mode = "combined"
	// ModeSimulator serves the simulator API and manager only.
	ModeSimulator Mode = "simulator"
	// ModeDisplay serves the aircraft display over a remote simulator.
	ModeDisplay Mode = "display"
)

// Config is the complete configuration of a combined test bench: one
// simulator with its API and manager, and one display that reads the same
// simulator in process.
//
// PublicBasePath is the path prefix the host mounts the application under,
// for example "/" or "/bench/a/". Browser URLs are derived from it and the
// fixed route names; they are never inferred from requests. The simulation
// ID must be fresh for every App: it identifies one engine lifetime.
type Config struct {
	PublicBasePath string
	Simulator      simulator.Config
	API            simulator.APIConfig
	Display        display.Config
	Manager        ui.ManagerSettings
	Aircraft       ui.AircraftDisplaySettings
}

// SimulatorConfig is the complete configuration of a simulator-only App.
type SimulatorConfig struct {
	PublicBasePath string
	Simulator      simulator.Config
	API            simulator.APIConfig
	Manager        ui.ManagerSettings
}

// DisplayConfig is the complete configuration of a display-only App. Source
// addresses the mounted simulator API of another application; the host
// supplies its HTTP client inside Source.
type DisplayConfig struct {
	PublicBasePath string
	Display        display.Config
	Source         display.HTTPSourceConfig
	Aircraft       ui.AircraftDisplaySettings
}

// paths are the public browser URLs derived from one mount prefix.
type paths struct {
	base         string
	simulatorAPI string
	displayAPI   string
	assets       string
}

func derivePaths(publicBasePath string) (paths, error) {
	base, err := urlpath.ParseMountPrefix(publicBasePath)
	if err != nil {
		return paths{}, fmt.Errorf("%w: PublicBasePath: %w", ErrInvalidConfig, err)
	}
	return paths{
		base:         base,
		simulatorAPI: base + "api/simulator/",
		displayAPI:   base + "api/display/",
		assets:       base + "assets/",
	}, nil
}

// Validate checks every setting without creating anything.
func (c Config) Validate() error {
	_, err := derivePaths(c.PublicBasePath)
	return errors.Join(err,
		wrap("Simulator", c.Simulator.Validate()),
		wrap("API", c.API.Validate()),
		wrap("Display", c.Display.Validate()),
		wrap("Manager", c.Manager.Validate()),
		wrap("Aircraft", c.Aircraft.Validate()))
}

// Validate checks every setting without creating anything.
func (c SimulatorConfig) Validate() error {
	_, err := derivePaths(c.PublicBasePath)
	return errors.Join(err,
		wrap("Simulator", c.Simulator.Validate()),
		wrap("API", c.API.Validate()),
		wrap("Manager", c.Manager.Validate()))
}

// Validate checks every setting without creating anything.
func (c DisplayConfig) Validate() error {
	_, err := derivePaths(c.PublicBasePath)
	return errors.Join(err,
		wrap("Display", c.Display.Validate()),
		wrap("Source", c.Source.Validate()),
		wrap("Aircraft", c.Aircraft.Validate()))
}

func wrap(section string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalidConfig, section, err)
}
