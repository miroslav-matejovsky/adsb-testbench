# Receiving stations and altitude-aware reception

## Status and source

Detailed implementation plan for
[backlog 03](../../backlog/03-receiving-stations.md).
Implementation is not started. This planning change adds documentation only.
No build, test, lint, or `task all` execution is part of preparing this plan.
The commands listed in each step are acceptance checks for the subsequent
implementation.

Baseline reviewed: `653374f0bd5412f3a92f259990c34523434b928a`.
The `simulation` package already implements the deterministic engine documented
in [simulation/doc.go](../../../simulation/doc.go): explicit configuration,
checked virtual time, aircraft truth, generated DF17 frames, ordered
transmission sequences, and a bounded transmission history. It has no station
or reception concept. Its public API and the
[codec](../../../internal/adsb/doc.go) were inspected with `go doc`.

## Goal

Add receiving stations to the same engine and decide, for every generated
transmission, which stations hear it. The decision is a documented synthetic
model driven by aircraft altitude, station antenna height, range, receiver
sensitivity, and configured impairments. It is not a calibrated RF prediction.

The same configuration and the same ordered operations must produce the same
receptions. Station creation, editing, disabling, and deletion must never change
the generated aircraft frames, aircraft truth, virtual time, carry, identity
allocation, or transmission history of the same run. Published coverage for an
explicit reference altitude must agree with the reception decision.

## Scope and architecture

Implement station value types and validation, a station registry with
revisions, station commands, the reception model, published model parameters
and coverage, and per-transmission reception records returned with each
mutation batch.

Keep everything in `simulation`. Its only first-party dependency remains
`internal/adsb`, so [.go-arch-lint.yml](../../../.go-arch-lint.yml) needs no
change. Use standard-library mathematics, randomness, synchronization, and
contexts. Add no production dependency, configuration file, server, goroutine,
or process-wide logger.

| File | Contents |
| --- | --- |
| `simulation/types.go` | Add `MaxStations`, `MaxBatchReceptions`, `ErrNotFound`, `ErrConflict`, `Batch`, and `Reception`; add `Stations` to `Snapshot`. |
| `simulation/station.go` | New. `StationConfig`, `Station`, and station validation. |
| `simulation/reception.go` | New. `ReceptionModel`, `Coverage`, `Model`, `EstimateCoverage`, and the private geometry and link-budget helpers. |
| `simulation/registry.go` | New. Private ordered station registry, ordinal allocation, reserved identifiers, and cloning. |
| `simulation/random.go` | Add the station domain tag to the existing seed derivation. |
| `simulation/engine.go` | Station commands, reception evaluation during emission, and the `Batch` return type. |
| `simulation/snapshot.go` | Detached station records in `Snapshot`. |

The engine owns stations and reception decisions. Backlog 04 owns retained
per-station reception history, cursors, gaps, and observed aircraft state.
Backlog 06 owns the HTTP contract. Backlog 09 owns the reception inspector.
Terrain, obstruction masks, antenna patterns, multipath, interference,
message-rate limits, propagation delay, and Doppler are outside this increment.

## Public contract to implement

All identifiers below are proposed APIs, not existing code.

| API | Contract |
| --- | --- |
| `AddStation(context.Context, StationConfig) (Station, error)` | Validate settings, reject a duplicate or previously used identifier, allocate a station ordinal, and create the station at the committed virtual time with revision 1. |
| `UpdateStation(context.Context, uint64, StationConfig) (Station, error)` | Replace every setting of the station named by `StationConfig.ID` when the supplied revision matches the current one, and return the station with an incremented revision. |
| `RemoveStation(context.Context, string, uint64) error` | Remove the named station when the supplied revision matches. The identifier stays reserved for the rest of the run. |
| `Model() ReceptionModel` | Return the fixed effective model parameters used by both reception and coverage. |
| `EstimateCoverage(StationConfig, float64) (Coverage, error)` | Return the synthetic coverage radii of any valid station settings at an explicit reference altitude in feet, without needing a live engine. |
| `Advance`, `Elapse`, `SetCount` | Return `Batch` in place of `[]Transmission`. |
| `Snapshot()` | Gains `Stations []Station` in creation order. |

`StationConfig` contains `ID string`, `Enabled bool`, `LatitudeDegrees`,
`LongitudeDegrees`, `SiteElevationMetres`, `AntennaHeightMetres`,
`AntennaGainDBi`, `SensitivityDBm`, `SystemLossDB`, and
`FrameLossProbability`, all `float64` except the first two. No constructor
fills missing fields. A disabled station is expressed by `Enabled` false, so
disabling is an ordinary update and needs no separate method. An update
replaces every setting; there is no partial patch.

`Station` contains `Config StationConfig`, `Revision uint64`, and
`CreatedAt time.Time`.

`Batch` contains `Transmissions []Transmission` and `Receptions []Reception`.
Every successful mutation returns both slices allocated, empty when nothing was
emitted. A failed or canceled mutation returns the zero `Batch`.

`Reception` contains `TransmissionSequence uint64`, `StationID string`,
`StationRevision uint64`, `ICAO uint32`, `Kind MessageKind`,
`Timestamp time.Time`, `Frame [14]byte`, `SlantRangeNauticalMiles float64`,
and `ReceivedPowerDBm float64`. It is self contained, so a consumer never has
to join it against the transmission slice. The range and power fields are
synthetic model outputs.

`ReceptionModel` contains `TransmitPowerDBm`, `FrequencyMHz`,
`FreeSpacePathLossConstantDB`, `RefractionFactor`,
`HorizonMetresPerSqrtMetre`, and `EarthRadiusMetres`.

`Coverage` contains `ReferenceAltitudeFeet`, `HorizonRadiusNauticalMiles`,
`LinkBudgetRadiusNauticalMiles`, and `EffectiveRadiusNauticalMiles`. All three
radii are great-circle surface radii on the model sphere, and the effective
radius is the smaller of the other two.

## Fixed engine policy

These are documented implementation limits and scenario choices, not defaults
inserted into configuration.

| Policy | Decision |
| --- | --- |
| Active stations | 0-8; export `MaxStations = 8`. |
| Station identifier | Caller supplied, 1 to 64 bytes of ASCII letters, digits, hyphen, or underscore; case sensitive; preserved exactly; unique within a run; never reused after removal. |
| Station revision | Starts at 1 on creation and increments on every accepted update, including one that assigns identical settings. |
| Reception fan-out | At most `MaxStations` receptions per transmission; export `MaxBatchReceptions = MaxBatchFrames * MaxStations`. |
| Transmit power | Fixed 51 dBm effective radiated power for every aircraft, including its antenna. Scenario constant, not configuration. |
| Frequency | Fixed 1090 MHz. |
| Refraction | Fixed 4/3 Earth radius for the radio horizon. |
| Station location | Latitude [-90,90], longitude [-180,180] degrees. |
| Site elevation | [-500,9000] metres above the model sphere. |
| Antenna height | [0,500] metres above site elevation. |
| Antenna gain | [-10,40] dBi. |
| Receiver sensitivity | [-140,0] dBm. |
| System loss | [0,30] dB. |
| Frame loss probability | [0,1]; one draw per eligible transmission whatever the value. |
| Coverage reference altitude | [-1000,50175] feet, the aircraft altitude domain. |
| Propagation delay | Not modelled. Reception time equals transmission time. |
| Altitude reference | Pressure altitude is used directly as geometric height above the model sphere. |
| Negative heights | Heights below the sphere contribute zero to the radio horizon. |

Reject invalid input before changing any state. Station commands use the
existing engine mutex, staging, and cancellation rules.

## Determinism and model decisions

1. Station randomness reuses the existing SHA-256 seed derivation with a new
   domain tag 4 and the station creation ordinal in the ordinal field. Station
   ordinals and aircraft ordinals are independent counters, so the tag keeps the
   streams disjoint and no aircraft stream is disturbed.
2. Draw exactly one 53-bit fraction per enabled station per transmission that
   has already passed the deterministic checks. The configured probability never
   changes how many draws occur, so editing only that value cannot shift the
   stream.
3. A disabled station evaluates nothing and draws nothing. Its stream resumes
   where it stopped when the station is enabled again.
4. Evaluate stations in creation order. Receptions are ordered by transmission
   sequence, then by station creation order.
5. Station commands change station state only. They never touch aircraft
   records, deadlines, aircraft generators, the clock, the carry, identity
   allocation, sequences, or the transmission history.
6. Reception uses the aircraft truth evaluated at the transmission instant, not
   the quantized wire values and not a later snapshot.
7. Within one time call the station set is fixed by the staged state. A station
   created between calls receives only transmissions emitted at or after its
   creation instant; earlier transmissions are never redelivered.
8. Commit stations, revisions, reserved identifiers, ordinals, and station
   generator states together with the rest of the mutation. Cancellation is
   checked after acquiring the lock, during staged work, and immediately before
   commit.
9. Horizon and link budget are compared as a surface distance and a slant
   distance respectively. Chord length grows strictly with angular separation
   for fixed heights, so each slant limit maps to exactly one surface radius.
   Published coverage inverts both limits to surface radii, which makes the
   published boundary the exact reception boundary.

Identical bytes and identical reception decisions are promised for the same
engine implementation, Go toolchain, platform, configuration, and ordered
operations, including ordered station commands. Do not claim cross-platform
floating-point equivalence, RF calibration, or real receiver behavior.

## Implementation sequence

| Step | Deliverable |
| --- | --- |
| [01](01-station-contract-and-configuration.md) | Station value types, documented domains, validation, new error categories |
| [02](02-reception-model-and-coverage.md) | Geometry, link budget, horizon, published parameters and coverage |
| [03](03-station-registry-and-commands.md) | Registry, revisions, identifiers, and atomic station commands |
| [04](04-reception-batches-and-engine-integration.md) | `Batch` and `Reception` returns, reception evaluation during emission |
| [05](05-acceptance-and-failure-tests.md) | Backlog acceptance matrix, isolation and failure-path evidence |
| [06](06-documentation-and-acceptance.md) | Public examples, external consumer check, synchronized documentation |

Implement in that order. Each step lists its actual dependencies. Keep
unfinished public method bodies out of earlier steps.
Record step status in [progress.md](progress.md).

## Completion criteria

- Every backlog 03 acceptance condition is covered by a named test in step 05.
- Station commands leave generated transmissions and aircraft truth byte
  identical to a control run without stations.
- Published coverage and the reception decision agree at the published
  boundary for both horizon-limited and link-budget-limited stations.
- Reception decisions replay exactly across split time calls and mixed
  operations, and rejected or canceled commands change nothing.
- No library API exposes an `internal/adsb` or third-party type.
- Root and package documentation describe the implemented model honestly and
  label coverage and range estimates as synthetic model output.
- Implementation passes the commands in step 06, including `task all`; do not
  run them for this documentation-only planning task.
