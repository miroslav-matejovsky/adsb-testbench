package simulation

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// state is the engine's complete mutable state. Mutations clone it, work on
// the clone, and replace the committed value only once everything succeeded,
// so no caller ever observes a partial change.
type state struct {
	cfg          Config
	clock        clock
	fleet        []aircraft
	identity     identityAllocator
	lastSequence uint64
	history      history
}

// clone deep-copies everything a mutation can touch. Aircraft, generator
// states, and clock values are plain values, so copying the slice copies them.
func (s state) clone() state {
	copied := s
	copied.fleet = append([]aircraft(nil), s.fleet...)
	copied.history = s.history.clone()
	return copied
}

// Engine is a deterministic ADS-B traffic generator.
//
// The same configuration and the same ordered operations always produce the
// same transmissions and the same snapshots. The engine holds no context,
// starts no goroutine, and never reads the wall clock: callers supply every
// duration. One mutex serializes mutations and snapshot reads, so concurrent
// callers get safety, not a promised operation order; callers that need
// reproducibility must order their own calls.
type Engine struct {
	mu    sync.Mutex
	state state
}

// New validates the configuration, creates the initial aircraft, and retains
// their creation reports. It returns nil and an error on any failure.
//
// Creation emits three reports per aircraft, so an initial fleet retains at
// most 3*MaxAircraft reports, well inside HistoryLimit.
func New(cfg Config) (*Engine, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	staged := state{
		cfg:      normalized,
		clock:    clock{start: normalized.StartTime},
		identity: newIdentityAllocator(),
		history:  newHistory(),
	}
	batch := make([]Transmission, 0, 3*normalized.InitialAircraftCount)
	if err := staged.addAircraft(context.Background(), normalized.InitialAircraftCount, &batch); err != nil {
		return nil, err
	}
	return &Engine{state: staged}, nil
}

// Advance moves virtual time forward by a nonnegative duration of at most
// MaxAdvance and returns every frame emitted in the interval.
//
// It applies the duration exactly, including while paused, and neither
// consumes nor clears the fractional carry earned by real-time scaling.
func (e *Engine) Advance(ctx context.Context, d time.Duration) ([]Transmission, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := e.state.clock.planAdvance(d)
	if err != nil {
		return nil, err
	}
	return e.runTo(ctx, target)
}

// Elapse converts a nonnegative real duration into virtual time at the
// current speed, preserving the fractional carry, and returns every frame
// emitted in the resulting interval.
//
// While paused it discards the supplied real duration, keeps the carry, and
// performs no later catch-up.
func (e *Engine) Elapse(ctx context.Context, real time.Duration) ([]Transmission, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := e.state.clock.planElapse(real, e.state.cfg.SpeedHundredths)
	if err != nil {
		return nil, err
	}
	return e.runTo(ctx, target)
}

// SetCount changes the number of active aircraft at the committed virtual
// time. Increases return the creation reports of every added aircraft;
// decreases remove the newest-created aircraft first and return an empty
// batch. Removal keeps the identities, schedules, and retained transmissions
// of the surviving aircraft, and never reuses a removed address.
func (e *Engine) SetCount(ctx context.Context, count int) ([]Transmission, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validCount(count); err != nil {
		return nil, err
	}

	staged := e.state.clone()
	batch := make([]Transmission, 0)
	if count > len(staged.fleet) {
		if err := staged.addAircraft(ctx, count-len(staged.fleet), &batch); err != nil {
			return nil, err
		}
	} else {
		staged.fleet = staged.fleet[:count]
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.state = staged
	return batch, nil
}

// SetSpeed sets virtual time per real time in hundredths, 0 through 10000.
// It emits nothing, settles no real time, and changes neither aircraft speed
// nor virtual time. A caller that needs pending real time applied at the old
// speed must call Elapse before changing the speed.
func (e *Engine) SetSpeed(ctx context.Context, speed uint16) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validSpeed(speed); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e.state.cfg.SpeedHundredths = speed
	return nil
}

// runTo drains every event due at or before the target time and commits the
// whole mutation at once. The caller must hold the mutex.
//
// Cancellation observed before the final commit discards all staged work and
// returns the original context error. Cancellation that arrives after that
// final check may still accompany a successful return; this is the usual race
// of any context-aware API.
func (e *Engine) runTo(ctx context.Context, target clock) ([]Transmission, error) {
	staged := e.state.clone()
	batch := make([]Transmission, 0)

	for {
		due, ok := nextEvent(staged.fleet, target.elapsed)
		if !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		craft := &staged.fleet[due.index]
		odd := craft.nextPositionOdd
		if err := staged.record(craft, due.kind, craft.navAt(due.at), odd, due.at, &batch); err != nil {
			return nil, err
		}
		if due.kind == PositionMessage {
			craft.nextPositionOdd = !odd
		}
		if err := craft.rescheduleAfter(due.kind); err != nil {
			return nil, err
		}
	}

	staged.clock = target
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.state = staged
	return batch, nil
}

// addAircraft creates count aircraft at the staged virtual time, emits their
// three creation reports each starting with even CPR, and schedules their
// first deadlines one randomized interval later.
func (s *state) addAircraft(ctx context.Context, count int, batch *[]Transmission) error {
	for range count {
		if err := ctx.Err(); err != nil {
			return err
		}
		icao, ordinal, err := s.identity.allocate()
		if err != nil {
			return err
		}

		craft := newAircraft(s.cfg, icao, ordinal, s.clock.elapsed)
		nav := craft.navAt(s.clock.elapsed)
		for _, kind := range families {
			if err := s.record(&craft, kind, nav, false, s.clock.elapsed, batch); err != nil {
				return err
			}
		}
		if err := craft.scheduleFirst(); err != nil {
			return err
		}
		s.fleet = append(s.fleet, craft)
	}
	return nil
}

// record encodes one report, assigns its sequence, retains it, and appends it
// to the returned batch. Sequence and batch capacity are checked before any
// allocation or counter change.
func (s *state) record(a *aircraft, kind MessageKind, nav navState, odd bool, at time.Duration, batch *[]Transmission) error {
	if len(*batch) >= MaxBatchFrames {
		return fmt.Errorf("%w: one mutation may emit at most %d transmissions", ErrLimit, MaxBatchFrames)
	}
	if s.lastSequence == math.MaxUint64 {
		return fmt.Errorf("%w: transmission sequences are exhausted", ErrLimit)
	}

	timestamp := s.clock.start.Add(at)
	frame, err := encodeReport(a, kind, nav, odd, timestamp)
	if err != nil {
		return err
	}

	s.lastSequence++
	emitted := Transmission{
		Sequence:  s.lastSequence,
		ICAO:      a.icao,
		Timestamp: timestamp,
		Kind:      kind,
		Frame:     frame,
	}
	s.history.append(emitted)
	*batch = append(*batch, emitted)
	return nil
}
