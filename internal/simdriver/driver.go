package simdriver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miroslav-matejovsky/adsb-testbench/simulation"
)

const (
	// Heartbeat is the real interval between pacing wakeups.
	Heartbeat = 100 * time.Millisecond
	// MaxCatchUp is the largest virtual-time backlog accepted by one settlement.
	MaxCatchUp = time.Hour
)

var (
	// ErrStopped rejects mutations after the driver's pacing loop terminates.
	ErrStopped = errors.New("simulation driver stopped")
	// ErrAlreadyStarted rejects a second call to Driver.Run.
	ErrAlreadyStarted = errors.New("simulation driver already started")
)

// Clock supplies measured real time and pacing wakeups to a Driver.
type Clock interface {
	// Now returns the current real instant. System clocks retain their monotonic
	// component so elapsed time is unaffected by wall-clock adjustments.
	Now() time.Time
	// NewTicker returns pacing wakeups and a function which releases the ticker.
	NewTicker(interval time.Duration) (<-chan time.Time, func())
}

// SystemClock is the operating system clock.
type SystemClock struct{}

// Now returns time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// NewTicker creates and stops a time.Ticker.
func (SystemClock) NewTicker(interval time.Duration) (<-chan time.Time, func()) {
	ticker := time.NewTicker(interval)
	return ticker.C, ticker.Stop
}

// Driver serializes measured-time pacing and every engine mutation. Reads can
// use the engine concurrently because Engine provides coherent snapshots.
type Driver struct {
	engine  *simulation.Engine
	clock   Clock
	started atomic.Bool

	mu       sync.Mutex
	stopped  bool
	baseline time.Time
}

// New creates a driver and captures its initial real-time baseline. It starts
// no goroutine. The driver must be the only mutating caller of engine.
func New(engine *simulation.Engine, clock Clock) *Driver {
	return &Driver{engine: engine, clock: clock, baseline: clock.Now()}
}

// Run settles elapsed time on each Heartbeat until cancellation or failure.
// It may be called once. Cancellation is a clean stop; an operational pacing
// error is returned. After Run returns all mutation commands return ErrStopped.
func (d *Driver) Run(ctx context.Context) error {
	if !d.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	defer func() {
		d.mu.Lock()
		d.stopped = true
		d.mu.Unlock()
	}()

	ticks, stopTicker := d.clock.NewTicker(Heartbeat)
	defer stopTicker()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return errors.New("simulation clock ticker closed")
			}
			if err := d.settle(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

// SetCount settles elapsed time at the current speed, then changes the active
// aircraft count at that virtual instant.
func (d *Driver) SetCount(ctx context.Context, count int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return ErrStopped
	}
	if count < 0 || count > simulation.MaxAircraft {
		return fmt.Errorf("%w: aircraft count %d is outside 0-%d", simulation.ErrInvalid, count, simulation.MaxAircraft)
	}
	if err := d.settleLocked(ctx); err != nil {
		return fmt.Errorf("settle before aircraft count change: %w", err)
	}
	if _, err := d.engine.SetCount(ctx, count); err != nil {
		return fmt.Errorf("set aircraft count: %w", err)
	}
	return nil
}

// SetSpeed settles elapsed time at the previous speed, then applies speed to
// later real time. Zero pauses pacing.
func (d *Driver) SetSpeed(ctx context.Context, speed uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return ErrStopped
	}
	if speed > simulation.MaxSpeedHundredths {
		return fmt.Errorf("%w: speed %d is outside 0-%d hundredths", simulation.ErrInvalid, speed, simulation.MaxSpeedHundredths)
	}
	if err := d.settleLocked(ctx); err != nil {
		return fmt.Errorf("settle before speed change: %w", err)
	}
	if err := d.engine.SetSpeed(ctx, speed); err != nil {
		return fmt.Errorf("set simulation speed: %w", err)
	}
	return nil
}

// AddStation validates and settles before creating a station.
func (d *Driver) AddStation(ctx context.Context, cfg simulation.StationConfig) (simulation.Station, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return simulation.Station{}, ErrStopped
	}
	if err := simulation.ValidateStationConfig(cfg); err != nil {
		return simulation.Station{}, err
	}
	snapshot := d.engine.Snapshot()
	if len(snapshot.Stations) >= simulation.MaxStations {
		return simulation.Station{}, fmt.Errorf("%w: at most %d active stations", simulation.ErrLimit, simulation.MaxStations)
	}
	for _, station := range snapshot.Stations {
		if station.Config.ID == cfg.ID {
			return simulation.Station{}, fmt.Errorf("%w: station ID %q is already active", simulation.ErrInvalid, cfg.ID)
		}
	}
	if err := d.settleLocked(ctx); err != nil {
		return simulation.Station{}, fmt.Errorf("settle before station creation: %w", err)
	}
	station, err := d.engine.AddStation(ctx, cfg)
	if err != nil {
		return simulation.Station{}, fmt.Errorf("add station: %w", err)
	}
	return station, nil
}

// UpdateStation validates and settles before replacing a station's settings.
func (d *Driver) UpdateStation(ctx context.Context, expectedRevision uint64, cfg simulation.StationConfig) (simulation.Station, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return simulation.Station{}, ErrStopped
	}
	if err := simulation.ValidateStationConfig(cfg); err != nil {
		return simulation.Station{}, err
	}
	if err := ctx.Err(); err != nil {
		return simulation.Station{}, err
	}
	if err := d.precheckStation(cfg.ID, expectedRevision); err != nil {
		return simulation.Station{}, err
	}
	if err := d.settleLocked(ctx); err != nil {
		return simulation.Station{}, fmt.Errorf("settle before station update: %w", err)
	}
	station, err := d.engine.UpdateStation(ctx, expectedRevision, cfg)
	if err != nil {
		return simulation.Station{}, fmt.Errorf("update station: %w", err)
	}
	return station, nil
}

// RemoveStation settles before removing a revision-matched station.
func (d *Driver) RemoveStation(ctx context.Context, id string, expectedRevision uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return ErrStopped
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.precheckStation(id, expectedRevision); err != nil {
		return err
	}
	if err := d.settleLocked(ctx); err != nil {
		return fmt.Errorf("settle before station removal: %w", err)
	}
	if err := d.engine.RemoveStation(ctx, id, expectedRevision); err != nil {
		return fmt.Errorf("remove station: %w", err)
	}
	return nil
}

func (d *Driver) precheckStation(id string, expectedRevision uint64) error {
	for _, station := range d.engine.Snapshot().Stations {
		if station.Config.ID != id {
			continue
		}
		if station.Revision != expectedRevision {
			return fmt.Errorf("%w: station %q is at revision %d, not %d", simulation.ErrConflict, id, station.Revision, expectedRevision)
		}
		return nil
	}
	return fmt.Errorf("%w: station %q", simulation.ErrNotFound, id)
}

func (d *Driver) settle(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settleLocked(ctx)
}

// settleLocked measures from baseline and delivers complete chunks. Successful
// chunks advance baseline immediately, so cancellation resumes from the first
// undelivered instant. Clock regression neither rewinds nor moves baseline.
func (d *Driver) settleLocked(ctx context.Context) error {
	if d.stopped {
		return ErrStopped
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := d.clock.Now()
	elapsed := now.Sub(d.baseline)
	if elapsed <= 0 {
		return nil
	}
	speed := d.engine.Snapshot().Config.SpeedHundredths
	if speed == 0 {
		d.baseline = now
		return nil
	}
	maxRealBacklog := time.Duration(int64(MaxCatchUp) * 100 / int64(speed))
	if elapsed > maxRealBacklog {
		return fmt.Errorf("simulation is %s of real time behind at speed %d, beyond the %s virtual catch-up limit", elapsed, speed, MaxCatchUp)
	}
	maxRealChunk := time.Duration(int64(simulation.MaxAdvance) * 100 / int64(speed))
	for elapsed > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := min(elapsed, maxRealChunk)
		if _, err := d.engine.Elapse(ctx, chunk); err != nil {
			return fmt.Errorf("deliver elapsed time: %w", err)
		}
		d.baseline = d.baseline.Add(chunk)
		elapsed -= chunk
	}
	return nil
}
