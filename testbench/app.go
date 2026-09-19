package testbench

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/miroslav-matejovsky/adsb-testbench/display"
	"github.com/miroslav-matejovsky/adsb-testbench/simulator"
	"github.com/miroslav-matejovsky/adsb-testbench/ui"
)

// ErrAlreadyRun reports a second call to App.Run. An App supervises exactly
// one run lifetime.
var ErrAlreadyRun = errors.New("test bench application already ran")

// Lifecycle states reported by App.Status.
const (
	StateConfigured = "configured"
	StateRunning    = "running"
	StateStopped    = "stopped"
)

// App is one composed test bench application. Constructors validate the
// complete configuration and build every service, page and handler, but
// start no listener, goroutine, poller or signal registration. The host
// serves Handler and supervises Run.
type App struct {
	mode    Mode
	paths   paths
	sim     *simulator.Simulator
	api     *simulator.API
	display *display.Display
	handler http.Handler

	mu    sync.Mutex
	state string
}

// New composes a combined application: exactly one simulator and API, and
// one display that reads the same API in process through the raw
// observation and station adapters. No combined read goes through HTTP.
func New(config Config) (*App, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create combined test bench: %w", err)
	}
	paths, _ := derivePaths(config.PublicBasePath)
	sim, api, err := newSimulator(config.Simulator, config.API)
	if err != nil {
		return nil, err
	}
	source, err := display.NewInProcessSource(api)
	if err != nil {
		return nil, fmt.Errorf("create combined test bench: %w", err)
	}
	stations, err := display.NewInProcessStationSource(api)
	if err != nil {
		return nil, fmt.Errorf("create combined test bench: %w", err)
	}
	backend, err := display.New(config.Display, source, stations)
	if err != nil {
		return nil, fmt.Errorf("create combined test bench: %w", err)
	}
	app := &App{mode: ModeCombined, paths: paths, sim: sim, api: api, display: backend, state: StateConfigured}
	if err := app.buildHandler(&config.Manager, &config.Aircraft); err != nil {
		return nil, err
	}
	return app, nil
}

// NewSimulator composes a simulator-only application with the simulator API
// and the manager.
func NewSimulator(config SimulatorConfig) (*App, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create simulator test bench: %w", err)
	}
	paths, _ := derivePaths(config.PublicBasePath)
	sim, api, err := newSimulator(config.Simulator, config.API)
	if err != nil {
		return nil, err
	}
	app := &App{mode: ModeSimulator, paths: paths, sim: sim, api: api, state: StateConfigured}
	if err := app.buildHandler(&config.Manager, nil); err != nil {
		return nil, err
	}
	return app, nil
}

// NewDisplay composes a display-only application. Both received evidence
// and station discovery are read over HTTP from the configured source; the
// browser only ever contacts this application.
func NewDisplay(config DisplayConfig) (*App, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("create display test bench: %w", err)
	}
	paths, _ := derivePaths(config.PublicBasePath)
	source, err := display.NewHTTPSource(config.Source)
	if err != nil {
		return nil, fmt.Errorf("create display test bench: %w", err)
	}
	backend, err := display.New(config.Display, source, source)
	if err != nil {
		return nil, fmt.Errorf("create display test bench: %w", err)
	}
	app := &App{mode: ModeDisplay, paths: paths, display: backend, state: StateConfigured}
	if err := app.buildHandler(nil, &config.Aircraft); err != nil {
		return nil, err
	}
	return app, nil
}

func newSimulator(config simulator.Config, apiConfig simulator.APIConfig) (*simulator.Simulator, *simulator.API, error) {
	sim, err := simulator.New(config)
	if err != nil {
		return nil, nil, fmt.Errorf("create simulator: %w", err)
	}
	api, err := simulator.NewAPI(sim, apiConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create simulator API: %w", err)
	}
	return sim, api, nil
}

func (a *App) buildHandler(manager *ui.ManagerSettings, aircraft *ui.AircraftDisplaySettings) error {
	var managerPage, aircraftPage http.Handler
	var err error
	if manager != nil {
		managerPage, err = ui.NewManager(ui.ManagerConfig{
			APIBaseURL: a.paths.simulatorAPI, AssetBaseURL: a.paths.assets, ManagerSettings: *manager,
		})
		if err != nil {
			return fmt.Errorf("create manager page: %w", err)
		}
	}
	if aircraft != nil {
		aircraftPage, err = ui.NewAircraftDisplay(ui.AircraftDisplayConfig{
			APIBaseURL: a.paths.displayAPI, AssetBaseURL: a.paths.assets, AircraftDisplaySettings: *aircraft,
		})
		if err != nil {
			return fmt.Errorf("create aircraft page: %w", err)
		}
	}
	handler, err := a.routes(managerPage, aircraftPage)
	if err != nil {
		return err
	}
	a.handler = handler
	return nil
}

// SimulatorAPI returns the in-process simulator service, or nil for a
// display-only application. Hosts use it for direct Go calls, for example
// to install initial stations before Run.
func (a *App) SimulatorAPI() *simulator.API { return a.api }

// Handler returns the relative application routes. The host strips its
// public prefix once before dispatching here; see the package
// documentation for the route table.
func (a *App) Handler() http.Handler { return a.handler }

// Run supervises the application until ctx is canceled. Combined and
// simulator applications run their one simulator; a display-only
// application only waits, because a display never polls on its own.
// Cancellation is a clean stop and returns nil. Run may be called once;
// later calls return ErrAlreadyRun.
func (a *App) Run(ctx context.Context) error {
	a.mu.Lock()
	if a.state != StateConfigured {
		a.mu.Unlock()
		return ErrAlreadyRun
	}
	a.state = StateRunning
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.state = StateStopped
		a.mu.Unlock()
	}()

	if a.sim == nil {
		<-ctx.Done()
		return nil
	}
	return a.sim.Run(ctx)
}

func (a *App) lifecycleState() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}
