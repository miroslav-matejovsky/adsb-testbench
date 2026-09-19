package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
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

// allocate returns the next station creation ordinal.
func (r *stationRegistry) allocate() (uint64, error) {
	if r.nextOrdinal == math.MaxUint64 {
		return 0, fmt.Errorf("%w: station ordinals are exhausted", ErrLimit)
	}
	ordinal := r.nextOrdinal
	r.nextOrdinal++
	return ordinal, nil
}

// add creates a station at an elapsed instant with revision 1 and reserves its
// identifier. The caller has already validated the settings.
func (r *stationRegistry) add(seed uint64, cfg StationConfig, createdAt time.Duration) (int, error) {
	if _, used := r.reserved[cfg.ID]; used {
		return 0, fmt.Errorf("%w: station ID %q is already used in this run", ErrInvalid, cfg.ID)
	}
	if len(r.active) >= MaxStations {
		return 0, fmt.Errorf("%w: at most %d stations may be active", ErrLimit, MaxStations)
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

// update replaces every setting of a station, keeping its position, creation
// instant, ordinal, and generator state, and increments its revision.
// The caller has already validated the settings.
func (r *stationRegistry) update(expectedRevision uint64, cfg StationConfig) (int, error) {
	index := r.find(cfg.ID)
	if index < 0 {
		return 0, fmt.Errorf("%w: station %q", ErrNotFound, cfg.ID)
	}
	if current := r.active[index].revision; current != expectedRevision {
		return 0, fmt.Errorf("%w: station %q is at revision %d, not %d",
			ErrConflict, cfg.ID, current, expectedRevision)
	}

	r.active[index].cfg = cfg
	r.active[index].revision++
	return index, nil
}

// remove deletes a station, keeping its identifier reserved and the relative
// order of the remaining stations.
func (r *stationRegistry) remove(id string, expectedRevision uint64) error {
	index := r.find(id)
	if index < 0 {
		return fmt.Errorf("%w: station %q", ErrNotFound, id)
	}
	if current := r.active[index].revision; current != expectedRevision {
		return fmt.Errorf("%w: station %q is at revision %d, not %d",
			ErrConflict, id, current, expectedRevision)
	}

	r.active = append(r.active[:index], r.active[index+1:]...)
	return nil
}

// snapshot returns detached records of the active stations in creation order.
func (r *stationRegistry) snapshot(start time.Time) []Station {
	out := make([]Station, 0, len(r.active))
	for i := range r.active {
		out = append(out, r.active[i].public(start))
	}
	return out
}
