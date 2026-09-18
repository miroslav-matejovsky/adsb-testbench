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
//   - [Engine.AddStation], [Engine.UpdateStation], and
//     [Engine.RemoveStation] manage receiving stations.
//   - [Engine.Snapshot] returns one detached, coherent view.
//   - [Engine.ReceptionHistory] pages one station's retained receptions.
//   - [Engine.Observations] decodes a selected-station received-data snapshot.
//   - [Model] and [EstimateCoverage] publish the reception model and the
//     coverage it implies, without needing an engine.
//
// [Engine.Advance], [Engine.Elapse], and [Engine.SetCount] return a [Batch]
// holding the frames the aircraft emitted and the [Reception] records the
// active stations produced from them. A successful mutation returns both
// slices allocated and possibly empty; a failed or canceled one returns the
// zero Batch. Mutations are atomic: nothing is committed unless encoding,
// bounds checks, and the final cancellation check all pass.
//
// Station commands emit no frames and settle no time, exactly like
// [Engine.SetSpeed]. They change station state only: they never touch aircraft
// records, deadlines, aircraft generators, the clock, the fractional carry,
// identity allocation, sequences, or the transmission history, so adding,
// editing, or removing a station cannot change the frames a run generates.
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
// # Station configuration
//
// [StationConfig] is complete and explicit in the same way [Config] is.
// Nothing fills a missing field and nothing is normalized, so an accepted
// configuration is stored exactly as supplied. Zero values are meaningful:
// 0 dBi gain, 0 dB system loss, 0 m antenna height, 0 m site elevation, and
// frame loss probability 0 are all valid settings, and Enabled false is a
// fully configured station that receives nothing.
//
//   - ID is 1 to 64 bytes of ASCII letters, digits, hyphen, or underscore.
//     It is compared case sensitively, preserved exactly, must be unique
//     within a run, and is never reused once its station has been removed.
//   - LatitudeDegrees is within [-90,90] and LongitudeDegrees within
//     [-180,180], in degrees.
//   - SiteElevationMetres is metres above the model sphere, within
//     [-500,9000]. AntennaHeightMetres is metres above that site elevation,
//     within [0,500].
//   - AntennaGainDBi is within [-10,40] dBi.
//   - SensitivityDBm is the lowest accepted received power, within [-140,0].
//   - SystemLossDB is fixed receive-path loss within [0,30] dB.
//   - FrameLossProbability is within [0,1].
//
// [Station] pairs an accepted configuration with a Revision and the virtual
// CreatedAt instant. Revision starts at 1 and increments on every accepted
// update, including one that assigns identical settings.
//
// # Engine policy
//
// [MaxAircraft], [MaxSpeedHundredths], [HistoryLimit],
// [ReceptionHistoryLimit], [MaxHistoryPageSize], [MaxAdvance],
// [MaxBatchFrames], and [MaxStations] are fixed limits of this package, not
// values inserted into configuration.
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
// # Reception model
//
// Whether a station hears a transmission is decided by two documented limits,
// evaluated against the aircraft truth at the exact transmission instant.
// [Model] publishes the fixed parameters both limits use.
//
//   - Radio horizon. The limit in metres is HorizonMetresPerSqrtMetre times
//     the sum of the square roots of the antenna height and the aircraft
//     height, both in metres above the model sphere. Heights below the sphere
//     contribute nothing. The great-circle surface distance must not exceed
//     it. This is what makes reception altitude aware.
//   - Link budget. Free space path loss is 20*log10(slant metres) plus
//     FreeSpacePathLossConstantDB. Received power is TransmitPowerDBm plus
//     AntennaGainDBi minus SystemLossDB minus that loss, and must reach
//     SensitivityDBm. The slant distance is the straight-line chord between
//     the two points.
//
// A station that passes both limits then draws one random value and drops the
// transmission with its configured FrameLossProbability.
//
// [EstimateCoverage] inverts the same two limits into great-circle surface
// radii at an explicit reference altitude, without needing an engine, so a
// caller can preview settings before applying them. Chord length grows
// strictly with angular separation for fixed heights, so a slant limit maps to
// exactly one surface radius and Coverage.EffectiveRadiusNauticalMiles is the
// exact reception boundary rather than an approximation.
//
// The model is synthetic. Coverage radii, slant ranges, and received powers
// are output of this testbench, not calibrated RF predictions and not claims
// about any real receiver. Pressure altitude is used directly as geometric
// height above the model sphere. Terrain, obstructions, antenna patterns,
// multipath, interference, message-rate limits, propagation delay, and Doppler
// are not modelled, and transmit power is one fixed value for every aircraft.
//
// # Determinism
//
// Randomness comes from math/rand/v2 PCG generators seeded explicitly. Each
// aircraft holds four independent streams, one for birth and one per message
// family, and each station holds one. Their two seed words are the first 16
// bytes of SHA-256 over the binary tuple of the run seed (8 bytes,
// little-endian), the creation ordinal (8 bytes, little-endian), and a domain
// tag (one byte: 0 birth, 1 identification, 2 position, 3 velocity,
// 4 station), read as little-endian words. Aircraft ordinals and station
// ordinals are independent counters, so the tag is what keeps the two kinds of
// stream disjoint, and no station can disturb aircraft generation.
//
// Every birth field consumes exactly one 53-bit fraction, including exact
// ranges, so range widths never shift a stream. A station likewise draws
// exactly one fraction per transmission it has already accepted on both
// deterministic limits, whatever its configured FrameLossProbability, so
// editing only that value cannot shift its stream. A disabled station
// evaluates nothing and draws nothing, which freezes its stream until it is
// enabled again.
//
// Identical bytes and identical reception decisions are promised for the same
// engine implementation, Go toolchain, platform, configuration, and ordered
// operations, including ordered station commands. Split-call equivalence also
// requires the same control changes at the same virtual instants. No
// cross-platform floating-point or future-version byte equivalence is claimed,
// and no RF calibration or real receiver behavior is claimed.
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
// Each active station also retains its most recent ReceptionHistoryLimit
// successful receptions. A [Reception] contains the exact frame, virtual time,
// RF model output, and a value copy of the station settings and revision in
// effect at reception. Its station-local Sequence advances only for frames that
// station receives. A missed transmission is never inserted or redelivered.
// Updating or disabling a station preserves its ring; removing it discards the
// ring while keeping the station ID reserved for the run.
//
// [Engine.ReceptionHistory] pages one active station using a [ReceptionCursor]
// bound to the run and station. Gap reports that records after a cursor were
// evicted. An ordinary missed transmission creates no station-sequence gap.
// Config.ID is the run identity in cursors; callers must assign a fresh ID to
// each engine lifetime, including a restart with identical settings.
//
// [Engine.Observations] unions the retained receptions of explicit selected
// stations, deduplicates shared transmissions, and decodes partial aircraft
// state without consulting aircraft truth or generated transmission history.
// Identification, global CPR position, pressure altitude, and velocity have
// independent positive expiry durations measured in virtual time. A usable CPR
// pair may combine halves from different selected stations. Retention eviction
// can remove evidence before its field expiry; StationRetention makes that
// truncation visible. Empty station selection means no observations.
//
// A [Transmission] timestamp is the virtual instant of that emission, which is
// not the current snapshot time; compare a historical frame with truth
// evaluated at its own timestamp. A [Reception] timestamp is the same instant,
// because no propagation delay is modelled. Snapshots, pages, observations,
// and batches copy every slice and frame array they expose, so editing a
// returned value cannot change engine state or later output.
//
// # Concurrency and errors
//
// One mutex serializes mutations, station commands, and snapshot reads.
// Concurrent callers get safety, not a promised operation order; callers
// needing reproducibility must order their own calls. A station edit carries
// the revision the caller last observed, so two concurrent editors cannot
// silently overwrite each other. The engine stores no context. Cancellation is checked
// after the lock is taken, during staged work, and immediately before commit;
// cancellation arriving after that final check may accompany a successful
// return, as with any context-aware API.
//
// Caller values outside their domain wrap [ErrInvalid], including a station
// identifier that is already in use or was used earlier in the run.
// Well-formed requests beyond representable time, identity, sequence, batch,
// or station capacity wrap [ErrLimit]. A command naming a station that does
// not exist wraps [ErrNotFound], and one whose supplied revision differs from
// the current revision wraps [ErrConflict] and changes nothing. Context
// failures are returned unchanged, so errors.Is still matches
// context.Canceled and context.DeadlineExceeded.
package simulation
