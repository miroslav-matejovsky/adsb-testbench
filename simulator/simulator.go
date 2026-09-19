package simulator

import (
	"context"
	"fmt"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/simdriver"
	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

// Config defines the complete simulation owned by a Simulator.
type Config struct {
	Simulation simulation.Config
}

// Validate reports whether New would accept the configuration, without
// creating an engine.
func (c Config) Validate() error {
	if err := c.Simulation.Validate(); err != nil {
		return fmt.Errorf("validate simulation: %w", err)
	}
	return nil
}

// Simulator owns one engine and the sole driver allowed to mutate it. New
// starts no background work. Run must be supervised by the caller.
type Simulator struct {
	engine *simulation.Engine
	driver *simdriver.Driver
}

// New validates configuration and creates a simulator using the system clock.
// Real time is measured from construction, including time before Run starts.
func New(config Config) (*Simulator, error) {
	return newSimulator(config, simdriver.SystemClock{})
}

func newSimulator(config Config, clock simdriver.Clock) (*Simulator, error) {
	if clock == nil {
		return nil, fmt.Errorf("%w: simulator clock is nil", simulation.ErrInvalid)
	}
	engine, err := simulation.New(config.Simulation)
	if err != nil {
		return nil, fmt.Errorf("create simulation: %w", err)
	}
	return &Simulator{engine: engine, driver: simdriver.New(engine, clock)}, nil
}

// Run paces the owned engine until cancellation or an operational failure. It
// may be called once. After it returns, mutations fail with simdriver.ErrStopped
// while read methods remain available.
func (s *Simulator) Run(ctx context.Context) error {
	if err := s.driver.Run(ctx); err != nil {
		return fmt.Errorf("run simulation: %w", err)
	}
	return nil
}

// SetCount settles measured real time and changes the active aircraft count.
func (s *Simulator) SetCount(ctx context.Context, count int) error {
	return s.driver.SetCount(ctx, count)
}

// SetSpeed settles measured real time at the old speed before applying speed.
func (s *Simulator) SetSpeed(ctx context.Context, speedHundredths uint16) error {
	return s.driver.SetSpeed(ctx, speedHundredths)
}

// AddStation settles measured real time before creating a station.
func (s *Simulator) AddStation(ctx context.Context, config simulation.StationConfig) (simulation.Station, error) {
	return s.driver.AddStation(ctx, config)
}

// UpdateStation settles measured real time before updating a revision-matched station.
func (s *Simulator) UpdateStation(ctx context.Context, expectedRevision uint64, config simulation.StationConfig) (simulation.Station, error) {
	return s.driver.UpdateStation(ctx, expectedRevision, config)
}

// RemoveStation settles measured real time before removing a revision-matched station.
func (s *Simulator) RemoveStation(ctx context.Context, id string, expectedRevision uint64) error {
	return s.driver.RemoveStation(ctx, id, expectedRevision)
}

// Snapshot returns the committed engine state without settling real time.
func (s *Simulator) Snapshot() simulation.Snapshot { return s.engine.Snapshot() }

// ReceptionHistory returns retained received frames without settling real time.
func (s *Simulator) ReceptionHistory(ctx context.Context, request simulation.HistoryRequest) (simulation.ReceptionPage, error) {
	return s.engine.ReceptionHistory(ctx, request)
}

// ReceptionSnapshot returns all retained receptions of a station selection at
// one committed virtual instant, without settling real time.
func (s *Simulator) ReceptionSnapshot(ctx context.Context, request simulation.ReceptionSnapshotRequest) (simulation.ReceptionSnapshot, error) {
	return s.engine.ReceptionSnapshot(ctx, request)
}

// Observations returns decoded received state without settling real time.
func (s *Simulator) Observations(ctx context.Context, request simulation.ObservationRequest) (simulation.ObservationSnapshot, error) {
	return s.engine.Observations(ctx, request)
}
