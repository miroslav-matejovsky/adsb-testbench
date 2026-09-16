package simulation

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// Allocatable ICAO address range. The codec rejects 000000 and FFFFFF, so the
// engine never allocates them.
const (
	minAddress uint32 = 0x000001
	maxAddress uint32 = 0xFFFFFE
)

// identityAllocator hands out unique addresses and creation ordinals.
// Neither counter is ever rewound, so an address freed by a count reduction is
// not reused within a run and an ordinal keeps identifying its random streams.
type identityAllocator struct {
	nextAddress uint32
	nextOrdinal uint64
}

// newIdentityAllocator starts allocation at the first legal address.
func newIdentityAllocator() identityAllocator {
	return identityAllocator{nextAddress: minAddress, nextOrdinal: 1}
}

// allocate returns the next address and creation ordinal.
// The last legal address can be allocated; the allocation after it fails.
func (a *identityAllocator) allocate() (icao uint32, ordinal uint64, err error) {
	if a.nextAddress > maxAddress {
		return 0, 0, fmt.Errorf("%w: ICAO addresses past %06X are exhausted", ErrLimit, maxAddress)
	}
	icao, ordinal = a.nextAddress, a.nextOrdinal
	a.nextAddress++
	a.nextOrdinal++
	return icao, ordinal, nil
}

// callsign formats the synthetic callsign of an address: TB followed by the
// six uppercase hexadecimal digits of that address.
func callsign(icao uint32) string {
	return fmt.Sprintf("TB%06X", icao)
}

// aircraft is the engine's private record of one active aircraft. It holds
// immutable birth state plus the three independent message schedules. All
// generator state is stored by value so a staged copy shares nothing with the
// committed engine.
type aircraft struct {
	icao      uint32
	callsign  string
	ordinal   uint64
	createdAt time.Duration // Birth elapsed time since the run start.
	traj      trajectory

	identificationRNG rand.PCG
	positionRNG       rand.PCG
	velocityRNG       rand.PCG

	identificationDeadline time.Duration
	positionDeadline       time.Duration
	velocityDeadline       time.Duration

	// nextPositionOdd selects the CPR parity of the next position report.
	// Creation emits even, so the next scheduled report is odd.
	nextPositionOdd bool
}

// newAircraft creates one aircraft from its own birth random stream.
// The six birth fields are drawn in the documented SpawnConfig order, one
// 53-bit fraction each, so identical seeds and ordinals reproduce the record.
func newAircraft(cfg Config, icao uint32, ordinal uint64, createdAt time.Duration) aircraft {
	source := newSource(cfg.Seed, ordinal, birthDomain)
	draw := rand.New(&source)

	birth := birthState{
		latitudeDegrees:           sampleRange(draw, cfg.Spawn.LatitudeDegrees),
		longitudeDegrees:          sampleRange(draw, cfg.Spawn.LongitudeDegrees),
		altitudeFeet:              sampleRange(draw, cfg.Spawn.AltitudeFeet),
		groundSpeedKnots:          sampleRange(draw, cfg.Spawn.GroundSpeedKnots),
		trackDegrees:              sampleRange(draw, cfg.Spawn.TrackDegrees),
		verticalRateFeetPerMinute: sampleRange(draw, cfg.Spawn.VerticalRateFeetPerMinute),
	}

	return aircraft{
		icao:              icao,
		callsign:          callsign(icao),
		ordinal:           ordinal,
		createdAt:         createdAt,
		traj:              newTrajectory(birth),
		identificationRNG: newSource(cfg.Seed, ordinal, identificationDomain),
		positionRNG:       newSource(cfg.Seed, ordinal, positionDomain),
		velocityRNG:       newSource(cfg.Seed, ordinal, velocityDomain),
	}
}

// navAt evaluates truth at an absolute elapsed instant of the run.
func (a *aircraft) navAt(elapsed time.Duration) navState {
	return a.traj.at((elapsed - a.createdAt).Seconds())
}

// public converts private state into the exported snapshot record.
func (a *aircraft) public(start time.Time, elapsed time.Duration) Aircraft {
	state := a.navAt(elapsed)
	return Aircraft{
		ICAO:                      a.icao,
		Callsign:                  a.callsign,
		CreatedAt:                 start.Add(a.createdAt),
		LatitudeDegrees:           state.latitudeDegrees,
		LongitudeDegrees:          state.longitudeDegrees,
		BarometricAltitudeFeet:    state.altitudeFeet,
		GroundSpeedKnots:          state.groundSpeedKnots,
		TrackDegrees:              state.trackDegrees,
		VerticalRateFeetPerMinute: state.verticalRateFeetPerMinute,
	}
}
