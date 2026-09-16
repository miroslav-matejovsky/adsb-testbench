// Package simulation is a deterministic ADS-B traffic engine.
//
// The engine owns synthetic aircraft truth and the frames they transmit. It
// runs no server, goroutine, or background driver and never reads the wall
// clock: callers supply elapsed durations and receive complete frame batches.
// Frames are encoded with the testbench codec, but no internal or third-party
// type appears in this package's API.
//
// # API
//
//   - [New] validates configuration, creates the initial aircraft, and retains
//     their creation reports.
//   - [Engine.Advance] moves virtual time by a supplied virtual duration.
//   - [Engine.Elapse] converts a supplied real duration at the current speed.
//   - [Engine.SetCount] changes the number of active aircraft.
//   - [Engine.SetSpeed] changes virtual time per real time.
//   - [Engine.Snapshot] returns one detached, coherent view.
//
// Every successful mutation returns an allocated, possibly empty slice; a
// failed one returns nil. Mutations are atomic: nothing is committed unless
// encoding, bounds checks, and the final cancellation check all pass.
//
// # Configuration
//
// [Config] is complete and explicit. Nothing fills a missing field, so Seed 0,
// InitialAircraftCount 0, SpeedHundredths 0, and zero range endpoints keep
// their literal meanings. A plain Go value cannot distinguish an omitted zero
// from an assigned zero; a later file or API parser must enforce presence.
// Invalid configuration is rejected by New before any state exists.
//
// [SpawnConfig] holds six [Range] values sampled in this order:
// LatitudeDegrees, LongitudeDegrees, AltitudeFeet, GroundSpeedKnots,
// TrackDegrees, and VerticalRateFeetPerMinute. Equal endpoints mean an exact
// value; otherwise a value is drawn uniformly from the half-open interval
// [Min, Max) in the field's unit, not uniformly by surface area.
//
// # Units and accepted domains
//
//   - Latitude and longitude are degrees. Birth latitude is within [-85,85]
//     and birth longitude within [-180,180]. A trajectory may later cross the
//     poles or the date line; snapshot longitude is normalized to [-180,180).
//   - Altitude is pressure altitude in feet relative to 1013.25 hPa, within
//     [-1000,50175], matching the codec Q=1 encoder bounds. It is not height
//     above terrain, and no geometric altitude is modelled or inferred.
//   - Ground speed is knots within [0,1000]. Zero is a permitted stationary
//     airborne target.
//   - Track is the true ground track in degrees within [0,360). A sampling
//     range may end at 360 only when Min < Max. No magnetic heading exists,
//     so none is fabricated.
//   - Vertical rate is feet per minute, positive upward, within [-10000,10000].
//   - SpeedHundredths is virtual time per real time in hundredths within
//     [0,10000]: 0 pauses, 100 is real time, 10000 is one hundred times.
//   - StartTime must be nonzero and within years 1-9999. It is normalized to
//     UTC with its monotonic component removed.
//   - ID must contain a non-whitespace character and is preserved exactly.
//
// # Engine policy
//
// [MaxAircraft], [HistoryLimit], [MaxAdvance], and [MaxBatchFrames] are fixed
// limits of this package, not values inserted into configuration.
//
// Addresses are allocated monotonically from 000001 to FFFFFE and are never
// reused within a run, including after a count reduction. A callsign is TB
// followed by the six uppercase hexadecimal digits of the address. Reducing
// the count removes the newest-created aircraft first and keeps the
// identities, schedules, and retained transmissions of the rest.
//
// # Virtual time
//
// Time is a normalized start instant plus a nonnegative elapsed duration.
// Advance applies a virtual duration exactly, including while paused.
// Elapse converts a real duration: the virtual nanoseconds are
// floor((realNS*speed + carry)/100) and the new carry is that remainder, so
// splitting one real duration yields exactly the same total. At speed 0 the
// supplied real duration is discarded, the carry is preserved, and there is no
// later catch-up. Neither Advance nor SetSpeed consumes or clears the carry.
//
// Each call covers at most MaxAdvance of virtual time. Events are processed
// over the open-closed interval (previous time, target time]; a zero-duration
// call emits nothing. After any committed mutation, no aircraft deadline is at
// or before the committed time, so a command issued afterwards follows every
// report already emitted at that instant.
//
// # Motion
//
// Motion is a documented synthetic model, not WGS-84 geodesic navigation. It
// uses a spherical Earth of radius 6371000 metres with 1852 metres per
// nautical mile. Each aircraft keeps immutable birth navigation and a birth
// elapsed time, and every state is evaluated directly at an absolute instant,
// so results never depend on how a caller split its duration calls.
//
// Horizontal motion follows a great circle at constant ground speed from the
// birth track. Pressure altitude is birth altitude plus birth vertical rate
// times age, clamped to the encodable range; the current vertical rate becomes
// zero once a bound has been reached while moving out of it. A position vector
// that lands on a pole uses longitude 0 by convention. At zero ground speed
// the birth track is retained as a display convention and both ground
// components are genuinely zero.
//
// # Schedules and wire profile
//
// Creation emits three reports for the new aircraft, in identification,
// position, and velocity order, with even CPR first. Each family is then
// scheduled independently from its own random stream: identification every
// 4800-5200 milliseconds, position and velocity every 400-600 milliseconds,
// drawn uniformly from those inclusive integer sets. A new deadline is
// measured from the previous deadline. Position parity alternates only on a
// position report, so the first odd half of a CPR pair appears one position
// interval after creation, not at birth. Simultaneous events are ordered by
// deadline, then creation ordinal, then family order.
//
// Frames use DF17 with CA=5, TC4 category 0 identification, TC11 barometric
// position with surveillance status 0 and available pressure altitude, and
// TC19 subtype 1 velocity with NACv 0, signed ground components, and an
// available barometric vertical rate. Heading, airspeed, and GNSS-minus-
// barometric altitude stay unavailable. These are deliberate scenario
// constants; the engine claims no calibrated navigation integrity and no
// complete ADS-B operational-status profile.
//
// # Determinism
//
// Randomness comes from math/rand/v2 PCG generators seeded explicitly. Each
// aircraft holds four independent streams, one for birth and one per message
// family. Their two seed words are the first 16 bytes of SHA-256 over the
// binary tuple of the run seed (8 bytes, little-endian), the aircraft creation
// ordinal (8 bytes, little-endian), and a domain tag (one byte: 0 birth,
// 1 identification, 2 position, 3 velocity), read as little-endian words.
// Every birth field consumes exactly one 53-bit fraction, including exact
// ranges, so range widths never shift a stream.
//
// Identical bytes are promised for the same engine implementation, Go
// toolchain, platform, configuration, and ordered operations. Split-call
// equivalence also requires the same control changes at the same virtual
// instants. No cross-platform floating-point or future-version byte
// equivalence is claimed.
//
// # History and ownership
//
// The engine retains the most recent HistoryLimit transmissions. Because
// creation emits three reports per aircraft and 3*MaxAircraft is below
// HistoryLimit, New always retains every initial report; a later Advance or
// Elapse batch may exceed retention. The returned batch is always complete,
// even for frames already evicted during the same call. A consumer detects
// lost retention when its last processed sequence plus one is below
// HistorySnapshot.OldestSequence within the same run ID.
//
// A [Transmission] timestamp is the virtual instant of that emission, which is
// not the current snapshot time; compare a historical frame with truth
// evaluated at its own timestamp. Snapshots copy every slice and frame array
// they expose, so editing a returned snapshot or batch cannot change engine
// state or later output.
//
// # Concurrency and errors
//
// One mutex serializes mutations and snapshot reads. Concurrent callers get
// safety, not a promised operation order; callers needing reproducibility must
// order their own calls. The engine stores no context. Cancellation is checked
// after the lock is taken, during staged work, and immediately before commit;
// cancellation arriving after that final check may accompany a successful
// return, as with any context-aware API.
//
// Caller values outside their domain wrap [ErrInvalid]. Well-formed requests
// beyond representable time, identity, sequence, or batch capacity wrap
// [ErrLimit]. Context failures are returned unchanged, so errors.Is still
// matches context.Canceled and context.DeadlineExceeded.
package simulation
