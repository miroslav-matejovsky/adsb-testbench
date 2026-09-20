package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// station is the engine's private record of one receiving station.
// Its generator is stored by value, so a staged copy of engine state shares
// nothing with the committed engine.
type station struct {
	cfg                   StationConfig
	revision              uint64
	createdAt             time.Duration // Creation elapsed time since the run start.
	ordinal               uint64
	rng                   rand.PCG
	lastReceptionSequence uint64
	receptions            receptionHistory
}

// public converts private state into the exported record.
func (s *station) public(start time.Time) Station {
	return Station{
		Config:    s.cfg,
		Revision:  s.revision,
		CreatedAt: start.Add(s.createdAt),
	}
}

// stationRegistry holds the active stations of a run in creation order,
// together with every identifier the run has used.
//
// Identifiers are reserved for the whole run: removing a station frees an
// active slot but never releases its identifier, so a later cursor or record
// naming that identifier can only ever mean the one station that had it.
// The reserved set holds at most MaxStationIDsPerRun identifiers of at most
// 64 bytes each; active holds at most MaxStations stations.
type stationRegistry struct {
	active      []station
	reserved    map[string]struct{}
	nextOrdinal uint64
}

// newStationRegistry returns an empty registry.
func newStationRegistry() stationRegistry {
	return stationRegistry{reserved: make(map[string]struct{}), nextOrdinal: 1}
}

// clone copies the registry, including its slice and its reserved set, so a
// staged mutation shares nothing with committed state.
func (r stationRegistry) clone() stationRegistry {
	copied := r
	copied.active = append([]station(nil), r.active...)
	for i := range copied.active {
		copied.active[i].receptions = copied.active[i].receptions.clone()
	}
	copied.reserved = make(map[string]struct{}, len(r.reserved))
	for id := range r.reserved {
		copied.reserved[id] = struct{}{}
	}
	return copied
}

// find returns the index of the station with an identifier, or -1.
// A linear scan over at most MaxStations entries needs no index.
func (r *stationRegistry) find(id string) int {
	for i := range r.active {
		if r.active[i].cfg.ID == id {
			return i
		}
	}
	return -1
}

// allocate returns the next station creation ordinal. checkAdd rejects
// exhaustion first; this guard keeps the invariant local.
func (r *stationRegistry) allocate() (uint64, error) {
	if r.nextOrdinal == math.MaxUint64 {
		return 0, fmt.Errorf("%w: station ordinals are exhausted", ErrLimit)
	}
	ordinal := r.nextOrdinal
	r.nextOrdinal++
	return ordinal, nil
}

// checkAdd reports whether a station with id may be created now, without
// changing the registry. Precedence is fixed: a used identifier is invalid;
// active capacity, lifetime identifier capacity, and ordinal exhaustion are
// limits, in that order.
func (r *stationRegistry) checkAdd(id string) error {
	if _, used := r.reserved[id]; used {
		return fmt.Errorf("%w: station ID %q is already used in this run", ErrInvalid, id)
	}
	if len(r.active) >= MaxStations {
		return fmt.Errorf("%w: at most %d stations may be active", ErrLimit, MaxStations)
	}
	if len(r.reserved) >= MaxStationIDsPerRun {
		return fmt.Errorf("%w: at most %d station IDs may be created in one run", ErrLimit, MaxStationIDsPerRun)
	}
	if r.nextOrdinal == math.MaxUint64 {
		return fmt.Errorf("%w: station ordinals are exhausted", ErrLimit)
	}
	return nil
}

// add creates a station at an elapsed instant with revision 1 and reserves its
// identifier. The caller has already validated the settings. A rejected
// addition consumes no ordinal, identifier, or random stream.
func (r *stationRegistry) add(seed uint64, cfg StationConfig, createdAt time.Duration) (int, error) {
	if err := r.checkAdd(cfg.ID); err != nil {
		return 0, err
	}

	ordinal, err := r.allocate()
	if err != nil {
		return 0, err
	}
	r.reserved[cfg.ID] = struct{}{}
	r.active = append(r.active, station{
		cfg:        cfg,
		revision:   1,
		createdAt:  createdAt,
		ordinal:    ordinal,
		rng:        newSource(seed, ordinal, stationDomain),
		receptions: newReceptionHistory(),
	})
	return len(r.active) - 1, nil
}

// checkUpdate returns the index of the station an update would change,
// without changing the registry. An unknown identifier is not found, a
// different revision is a conflict, and a revision that cannot increment
// without wrapping is a limit. An exhausted station can still be removed.
func (r *stationRegistry) checkUpdate(expectedRevision uint64, id string) (int, error) {
	index := r.find(id)
	if index < 0 {
		return 0, fmt.Errorf("%w: station %q", ErrNotFound, id)
	}
	current := r.active[index].revision
	if current != expectedRevision {
		return 0, fmt.Errorf("%w: station %q is at revision %d, not %d",
			ErrConflict, id, current, expectedRevision)
	}
	if current == math.MaxUint64 {
		return 0, fmt.Errorf("%w: station %q revision %d cannot increment", ErrLimit, id, current)
	}
	return index, nil
}

// update replaces every setting of a station, keeping its position, creation
// instant, ordinal, and generator state, and increments its revision.
// The caller has already validated the settings.
func (r *stationRegistry) update(expectedRevision uint64, cfg StationConfig) (int, error) {
	index, err := r.checkUpdate(expectedRevision, cfg.ID)
	if err != nil {
		return 0, err
	}

	r.active[index].cfg = cfg
	r.active[index].revision++
	return index, nil
}

// remove deletes a station, keeping its identifier reserved and the relative
// order of the remaining stations. The vacated tail element is cleared, so
// the backing array retains no reference to the removed reception ring.
func (r *stationRegistry) remove(id string, expectedRevision uint64) error {
	index, err := r.checkRemove(id, expectedRevision)
	if err != nil {
		return err
	}

	r.active = slices.Delete(r.active, index, index+1)
	return nil
}

// checkRemove returns the index of the station a removal would delete,
// without changing the registry. Unlike an update, the largest revision is
// acceptable, since removal does not increment it.
func (r *stationRegistry) checkRemove(id string, expectedRevision uint64) (int, error) {
	index := r.find(id)
	if index < 0 {
		return 0, fmt.Errorf("%w: station %q", ErrNotFound, id)
	}
	if current := r.active[index].revision; current != expectedRevision {
		return 0, fmt.Errorf("%w: station %q is at revision %d, not %d",
			ErrConflict, id, current, expectedRevision)
	}
	return index, nil
}

// snapshot returns detached records of the active stations in creation order.
func (r *stationRegistry) snapshot(start time.Time) []Station {
	out := make([]Station, 0, len(r.active))
	for i := range r.active {
		out = append(out, r.active[i].public(start))
	}
	return out
}
