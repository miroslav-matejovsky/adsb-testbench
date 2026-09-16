# Deterministic aircraft engine and virtual time

## Status and source

Detailed implementation plan for the deterministic engine, originally backlog
item 02. Implementation is complete and the backlog item has been removed.
Step-by-step status is recorded in [progress.md](progress.md).
The commands listed per step were run as acceptance checks.

Baseline reviewed: `bf221890eb71c9915b7e8829119bdb0fd17de827`, when the public
`simulation` package contained only `doc.go`. The implemented package is
documented in [simulation/doc.go](../../../simulation/doc.go).
The existing [codec](../../../internal/adsb/doc.go) supplies frame encoding,
CRC, field validation, and CPR. Its public API was inspected with `go doc`.

## Goal

Provide an importable simulation engine that generates the same ordered ADS-B
bytes from the same explicit configuration and ordered operations. Other Go
programs supply elapsed durations and receive complete batches without running
an HTTP server or background driver.

For example, one 10-second advance and ten 1-second advances must produce
identical concatenated transmissions and identical final snapshots. Failed or
canceled operations must leave both visible state and subsequent output unchanged.

## Scope and architecture

Implement synthetic aircraft creation, three-dimensional movement, independent
identification/position/velocity schedules, explicit virtual time, aircraft
count changes, speed scaling, bounded transmission history, and detached snapshots.

Keep implementation in `simulation`. Its only first-party dependency remains
`internal/adsb`, as already allowed by `.go-arch-lint.yml`. Use standard-library
randomness, arithmetic, synchronization, and contexts. Add no production
dependency, configuration file, server, goroutine, or process-wide logger.

The engine owns aircraft truth and generated transmissions. Backlog 03 owns
receiving stations and reception decisions; backlog 04 owns received
observations. Backlog 05 owns wall-clock pacing and serialization with elapsed
real time. Aircraft heading, wind, routes, surface movement, and unavailable
navigation scenarios are outside this increment.

## Public contract

The identifiers below are the implemented public API.

| API | Contract |
| --- | --- |
| `New(Config) (*Engine, error)` | Validate all configuration, create the initial aircraft and their creation reports, and return a usable engine or nil on failure. |
| `Advance(context.Context, time.Duration) ([]Transmission, error)` | Advance by a nonnegative virtual duration, including while paused; return every emitted frame. |
| `Elapse(context.Context, time.Duration) ([]Transmission, error)` | Convert supplied nonnegative real duration using the current speed and fractional carry; emit every due frame. |
| `SetCount(context.Context, int) ([]Transmission, error)` | Change active count at committed virtual time; increases return all creation reports; decreases return an empty batch. |
| `SetSpeed(context.Context, uint16) error` | Set hundredths-of-normal speed, 0 through 10000 inclusive. This changes no aircraft speed or virtual time. |
| `Snapshot() Snapshot` | Return one detached, coherent view of configuration, current time, aircraft truth, and retained transmission history. |

`Config` contains `ID string`, `StartTime time.Time`, `Seed uint64`,
`InitialAircraftCount int`, `SpeedHundredths uint16`, and `Spawn SpawnConfig`.
No constructor fills missing fields. Seed 0, count 0, speed 0, and zero-valued
range endpoints have literal meanings. A plain Go value cannot distinguish an
omitted valid zero from an explicitly assigned zero; later file/API parsers must
enforce field presence if their format requires it.

`SpawnConfig` contains six `Range{Min, Max float64}` values:
`LatitudeDegrees`, `LongitudeDegrees`, `AltitudeFeet`,
`GroundSpeedKnots`, `TrackDegrees`, and `VerticalRateFeetPerMinute`.
Use equal endpoints for an exact value. Otherwise sample uniformly from
the half-open interval [Min, Max) in the stated units, not uniformly by surface area.

`Aircraft` contains `ICAO uint32`, `Callsign string`, `CreatedAt time.Time`,
`LatitudeDegrees`, `LongitudeDegrees`, `BarometricAltitudeFeet`,
`GroundSpeedKnots`, `TrackDegrees`, and `VerticalRateFeetPerMinute`.
Navigation scalars are float64 values evaluated at the snapshot time.
Track is the current true ground track. No magnetic heading is fabricated.

`Transmission` contains `Sequence uint64`, `ICAO uint32`,
`Timestamp time.Time`, `Kind MessageKind`, and `Frame [14]byte`.
Define `IdentificationMessage`, `PositionMessage`, and `VelocityMessage`
as the three kinds. A sequence identifies a transmission within a run;
the caller pairs it with `Snapshot.Config.ID` across runs. The frame contains
no truth fields or transport wrapper.

`Snapshot` contains a value copy of `Config`, `Now time.Time`,
`Elapsed time.Duration`, `Aircraft []Aircraft`, and `History HistorySnapshot`.
Its `Config.SpeedHundredths` reflects current speed and
`Config.InitialAircraftCount` retains the constructor's original count.
Current count is the length of `Aircraft`.
`HistorySnapshot` contains `Messages []Transmission`,
`OldestSequence uint64`, `LatestSequence uint64`, and `Limit int`.
Empty history uses zero bounds; generated sequences begin at 1.
Every successful operation returns an allocated empty slice when no frames emit.

## Fixed engine policy

These are documented implementation limits and scenario choices, not defaults
inserted into configuration.

| Policy | Decision |
| --- | --- |
| Active aircraft | 0-100; export `MaxAircraft = 100`. |
| History | Retain the latest 1000 transmissions; export `HistoryLimit = 1000`. |
| Work per time call | At most 60 virtual seconds; export `MaxAdvance = 60s`. Reject larger calls without work. |
| Batch safety bound | At most 32000 transmissions in one mutation; export `MaxBatchFrames = 32000`. |
| Virtual lifetime | Nonnegative time.Duration elapsed, checked for int64 overflow; timestamps remain within years 1-9999. |
| Speed | Integer hundredths: 0 pauses, 1 means 0.01x, 100 means 1x, 10000 means 100x. |
| Initial latitude/longitude | Latitude [-85,85], longitude [-180,180]; the trajectory may subsequently cross poles or the date line. |
| Initial pressure altitude | [-1000,50175] feet, matching codec Q=1 encoder bounds. |
| Ground speed | [0,1000] knots; zero is a permitted synthetic stationary airborne target. |
| Initial true track | [0,360) degrees; a non-degenerate sampling range may have Max=360, but an exact value of 360 is invalid. |
| Initial vertical rate | [-10000,10000] feet/minute. |
| Altitude limit behavior | Move linearly to a codec altitude limit, then level off there; current vertical rate becomes zero at the limit. |
| Identity | Monotonic addresses 000001..FFFFFE, never reused in a run; callsign is TB followed by six uppercase hexadecimal address digits. |
| Count reduction | Remove newest-created aircraft first; keep remaining identities, schedules, and histories. |

Start time must be nonzero and within years 1-9999. Normalize it to UTC and remove
its monotonic component. ID must contain a non-whitespace character. Preserve the
supplied ID exactly. Reject invalid input before changing any state.

Use one `sync.Mutex` per engine. Mutations and snapshot reads are serialized.
Callers needing reproducibility must order operations; concurrent callers get
safety, not a promised scheduling order.

## Determinism and time decisions

1. Each aircraft retains immutable birth navigation and birth elapsed time.
   Evaluate motion directly at absolute elapsed instants. Do not repeatedly
   integrate from the result of the previous caller's duration.
2. Store three independent random states and deadlines per aircraft. Deadlines
   advance from the preceding deadline, never from the end of the caller's batch.
3. Select events by virtual deadline, then creation ordinal, then family order:
   identification, position, velocity. Do not let map iteration choose ordering.
4. Emit those three reports at aircraft creation, with even CPR first. Then
   schedule each family separately. Creation is an explicit startup scenario
   policy, not randomized traffic already in steady state.
5. Identification intervals are integer milliseconds 4800-5200 inclusive.
   Position and velocity intervals are integer milliseconds 400-600 inclusive.
   Draw uniformly from these integer sets. Position alternates even/odd;
   unrelated families never advance its parity.
6. Use `math/rand/v2.PCG` with explicit seeds. Derive two seed words from the
   first 16 bytes of SHA-256 over a fixed binary tuple: run seed (8 bytes,
   little-endian), aircraft creation ordinal (8 bytes, little-endian), and
   domain tag (one byte: 0 for birth, 1 identification, 2 position, 3 velocity).
   Birth uses its own generator. Draw its six fields in the SpawnConfig order above,
   including for equal ranges. Use `rand.New(&pcg).Uint64N` for unbiased interval
   selection and a fixed 53-bit fraction for floating ranges.
7. Compute scaled nanoseconds as the quotient of real nanoseconds times speed
   plus the saved remainder, divided by 100. Retain remainder 0-99. Validate
   the scaled-duration bound before multiplication. `Advance` and `SetSpeed`
   preserve that carry. Paused `Elapse` discards its supplied real duration and
   preserves existing carry; it creates no later catch-up.
8. Process the open-closed event interval (previous time, target time].
   Zero-duration operations generate no reports. At return, no deadline is at
   or before committed time. A command at that time occurs after any reports
   already emitted there.
9. Commit aircraft, deadlines, RNG states, time, carry, sequences, identity
   allocation, and history together. Cancellation is checked after acquiring
   the lock, during staged work, and immediately before commit.

Identical bytes are promised for the same engine implementation, Go toolchain,
platform, configuration, and ordered operations. Do not claim cross-platform
floating-point or future-version byte equivalence. Split-call equivalence also
requires the same control changes at the same virtual instants.

## Implementation sequence

All steps are complete; see [progress.md](progress.md).

| Step | Deliverable |
| --- | --- |
| [01](01-contract-and-configuration.md) | Public value types, documented bounds, validation, errors |
| [02](02-aircraft-identity-and-motion.md) | Reproducible creation and absolute-time movement |
| [03](03-frame-generation-and-scheduling.md) | Codec integration and independent event scheduling |
| [04](04-virtual-clock-and-scaling.md) | Checked virtual time and exact speed scaling |
| [05](05-engine-commands-and-history.md) | Atomic engine API, detached snapshots, bounded history |
| [06](06-determinism-and-failure-tests.md) | End-to-end replay, cancellation, limits, and ownership evidence |
| [07](07-public-documentation-and-acceptance.md) | Public examples, external consumer check, synchronized documentation |

Implement in that order. Each step specifies its actual dependencies; keep
unfinished public method bodies out of earlier steps.

## Completion criteria

- Every backlog 02 acceptance condition is covered by named tests in step 06.
- Encoded frames pass the existing codec decoder and match independent or
  analytically derived engine expectations.
- Snapshot and concatenated output equality hold for split duration calls,
  mixed time operations, pauses, and failed-command retry scenarios.
- No library API exposes an `internal/adsb` or third-party type.
- Root and package documentation describe the implemented engine honestly.
- The implementation passes the commands in step 07, including `task all`.
