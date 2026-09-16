package simulation

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
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
	stations     stationRegistry
}

// clone deep-copies everything a mutation can touch. Aircraft, stations,
// generator states, and clock values are plain values, so copying the slices
// copies them.
func (s state) clone() state {
	copied := s
	copied.fleet = append([]aircraft(nil), s.fleet...)
	copied.history = s.history.clone()
	copied.stations = s.stations.clone()
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
		stations: newStationRegistry(),
	}
	// No station can exist yet, so the creation reports produce no receptions.
	batch := newBatch()
	if err := staged.addAircraft(context.Background(), normalized.InitialAircraftCount, &batch); err != nil {
		return nil, err
	}
	return &Engine{state: staged}, nil
}

// Advance moves virtual time forward by a nonnegative duration of at most
// MaxAdvance and returns every frame emitted in the interval, together with
// the receptions those frames produced at the active stations.
//
// It applies the duration exactly, including while paused, and neither
// consumes nor clears the fractional carry earned by real-time scaling.
func (e *Engine) Advance(ctx context.Context, d time.Duration) (Batch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	target, err := e.state.clock.planAdvance(d)
	if err != nil {
		return Batch{}, err
	}
	return e.runTo(ctx, target)
}

// Elapse converts a nonnegative real duration into virtual time at the
// current speed, preserving the fractional carry, and returns every frame
// emitted in the resulting interval together with its receptions.
//
// While paused it discards the supplied real duration, keeps the carry, and
// performs no later catch-up.
func (e *Engine) Elapse(ctx context.Context, real time.Duration) (Batch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	target, err := e.state.clock.planElapse(real, e.state.cfg.SpeedHundredths)
	if err != nil {
		return Batch{}, err
	}
	return e.runTo(ctx, target)
}

// SetCount changes the number of active aircraft at the committed virtual
// time. Increases return the creation reports of every added aircraft, and the
// receptions those reports produced; decreases remove the newest-created
// aircraft first and return an empty batch. Removal keeps the identities, schedules, and retained transmissions
// of the surviving aircraft, and never reuses a removed address.
func (e *Engine) SetCount(ctx context.Context, count int) (Batch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	if err := validCount(count); err != nil {
		return Batch{}, err
	}

	staged := e.state.clone()
	batch := newBatch()
	if count > len(staged.fleet) {
		if err := staged.addAircraft(ctx, count-len(staged.fleet), &batch); err != nil {
			return Batch{}, err
		}
	} else {
		staged.fleet = staged.fleet[:count]
	}

	if err := ctx.Err(); err != nil {
		return Batch{}, err
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

// AddStation creates a receiving station at the committed virtual time with
// revision 1. The identifier must be unused in this run, including by a
// station that has already been removed.
//
// It emits no frames and settles no time. A station receives only
// transmissions emitted at or after its creation instant; retained
// transmissions are never redelivered.
func (e *Engine) AddStation(ctx context.Context, cfg StationConfig) (Station, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Station{}, err
	}
	if err := validateStation(cfg); err != nil {
		return Station{}, err
	}

	staged := e.state.clone()
	index, err := staged.stations.add(staged.cfg.Seed, cfg, staged.clock.elapsed)
	if err != nil {
		return Station{}, err
	}

	if err := ctx.Err(); err != nil {
		return Station{}, err
	}
	e.state = staged
	return e.state.stations.active[index].public(e.state.clock.start), nil
}

// UpdateStation replaces every setting of the station named by cfg.ID when
// expectedRevision matches its current revision, and returns it with an
// incremented revision.
//
// Disabling a station is an ordinary update with Enabled false, so there is no
// separate method. An update that assigns identical settings still increments
// the revision. The identifier cannot be changed; changing it means removing
// the station and adding another, which allocates a new random stream.
//
// A revision mismatch returns ErrConflict and changes nothing. An unknown
// identifier returns ErrNotFound. Creation instant, ordinal, and generator
// state are preserved.
func (e *Engine) UpdateStation(ctx context.Context, expectedRevision uint64, cfg StationConfig) (Station, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return Station{}, err
	}
	if err := validateStation(cfg); err != nil {
		return Station{}, err
	}

	staged := e.state.clone()
	index, err := staged.stations.update(expectedRevision, cfg)
	if err != nil {
		return Station{}, err
	}

	if err := ctx.Err(); err != nil {
		return Station{}, err
	}
	e.state = staged
	return e.state.stations.active[index].public(e.state.clock.start), nil
}

// RemoveStation deletes a station when expectedRevision matches its current
// revision. The identifier stays reserved for the rest of the run and the
// relative order of the remaining stations is preserved.
//
// Removal discards no retained transmission. A revision mismatch returns
// ErrConflict and an unknown identifier returns ErrNotFound; both change
// nothing.
func (e *Engine) RemoveStation(ctx context.Context, id string, expectedRevision uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	staged := e.state.clone()
	if err := staged.stations.remove(id, expectedRevision); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	e.state = staged
	return nil
}

// runTo drains every event due at or before the target time and commits the
// whole mutation at once. The caller must hold the mutex.
//
// Cancellation observed before the final commit discards all staged work and
// returns the original context error. Cancellation that arrives after that
// final check may still accompany a successful return; this is the usual race
// of any context-aware API.
func (e *Engine) runTo(ctx context.Context, target clock) (Batch, error) {
	staged := e.state.clone()
	batch := newBatch()

	for {
		due, ok := nextEvent(staged.fleet, target.elapsed)
		if !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return Batch{}, err
		}

		craft := &staged.fleet[due.index]
		odd := craft.nextPositionOdd
		if err := staged.record(craft, due.kind, craft.navAt(due.at), odd, due.at, &batch); err != nil {
			return Batch{}, err
		}
		if due.kind == PositionMessage {
			craft.nextPositionOdd = !odd
		}
		if err := craft.rescheduleAfter(due.kind); err != nil {
			return Batch{}, err
		}
	}

	staged.clock = target
	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	e.state = staged
	return batch, nil
}

// addAircraft creates count aircraft at the staged virtual time, emits their
// three creation reports each starting with even CPR, and schedules their
// first deadlines one randomized interval later.
func (s *state) addAircraft(ctx context.Context, count int, batch *Batch) error {
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
func (s *state) record(a *aircraft, kind MessageKind, nav navState, odd bool, at time.Duration, batch *Batch) error {
	if len(batch.Transmissions) >= MaxBatchFrames {
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
	batch.Transmissions = append(batch.Transmissions, emitted)
	return s.deliver(emitted, nav, batch)
}

// deliver evaluates every active station against one emitted transmission and
// appends the receptions it produced.
//
// Stations are scanned in creation order, so receptions stay ordered by
// transmission sequence then by station order. A disabled station evaluates
// nothing and draws nothing, which freezes its stream until it is enabled
// again. An enabled station that passes both deterministic limits draws
// exactly one fraction whatever its configured probability, so editing only
// that probability can never shift the stream.
func (s *state) deliver(t Transmission, nav navState, batch *Batch) error {
	for i := range s.stations.active {
		current := &s.stations.active[i]
		if !current.cfg.Enabled {
			continue
		}

		got := decide(current.cfg, nav.latitudeDegrees, nav.longitudeDegrees, nav.altitudeFeet)
		if !got.received {
			continue
		}
		if fraction(rand.New(&current.rng)) < current.cfg.FrameLossProbability {
			continue
		}

		if len(batch.Receptions) >= MaxBatchReceptions {
			return fmt.Errorf("%w: one mutation may return at most %d receptions, reached at station %q and transmission %d",
				ErrLimit, MaxBatchReceptions, current.cfg.ID, t.Sequence)
		}
		batch.Receptions = append(batch.Receptions, Reception{
			TransmissionSequence:    t.Sequence,
			StationID:               current.cfg.ID,
			StationRevision:         current.revision,
			ICAO:                    t.ICAO,
			Kind:                    t.Kind,
			Timestamp:               t.Timestamp,
			Frame:                   t.Frame,
			SlantRangeNauticalMiles: got.slantMetres / metresPerNauticalMile,
			ReceivedPowerDBm:        got.receivedPowerDBm,
		})
	}
	return nil
}

// newBatch returns an empty batch with both slices allocated, which is the
// documented shape of a successful mutation that emitted nothing.
func newBatch() Batch {
	return Batch{Transmissions: []Transmission{}, Receptions: []Reception{}}
}
