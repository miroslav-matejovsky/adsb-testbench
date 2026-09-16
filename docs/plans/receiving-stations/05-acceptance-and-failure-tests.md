---
title: "05 - Backlog acceptance and failure-path evidence"
dependencies: ["04-reception-batches-and-engine-integration.md"]
effort: "L"
complexity: "high"
---

# 05 - Backlog acceptance and failure-path evidence

## Objective

Prove the backlog guarantees for disablement, reproducibility, altitude and
sensitivity effects, revision conflicts, aircraft isolation, and coverage
agreement, through observable output and deterministic negative cases.

## Target Artifacts

- Create `simulation/stations_test.go` and
  `simulation/reception_determinism_test.go`.
- Extend `simulation/fixtures_test.go` with station and fixed-geometry
  fixtures.
- Extend `simulation/determinism_test.go` and `simulation/atomicity_test.go`
  with station operations and station failure cases.

## Implementation Tasks

1. Add fixture builders that assign every station field explicitly: one
   ordinary enabled station, one deliberately insensitive station, and one
   disabled station. Keep them distinct enough to exercise zero values, both
   limiting regimes, and both frame-loss extremes.

2. Add a fixed-geometry fixture: an engine configuration whose spawn ranges are
   all degenerate, so every aircraft sits at one exact coordinate and altitude
   with zero ground speed and zero vertical rate, plus a helper that places a
   station at an exact great-circle distance along the same meridian.

3. Extend the existing `operation` script helpers with add, update, disable,
   and remove operations so station commands appear inside the same ordered
   scripts already used for replay and partition tests.

4. Implement the named acceptance matrix below. Use `require` assertions and
   fixed scripts.

5. Reuse the existing counting context for late cancellation inside station
   commands, at several depths, without sleeps or wall-clock reads.

6. After every rejected or canceled station command, compare the snapshot with
   the pre-operation snapshot and then replay an identical valid suffix against
   an unaffected control engine, so hidden revision, ordinal, reserved
   identifier, and generator state is also covered.

7. Derive every coverage expectation independently inside the test from the
   published formulas and constants, not by calling the production helper.

## Technical Details

Required named acceptance matrix:

| Test family | Required comparisons |
| --- | --- |
| `TestStationLifecycle` | Add, update, disable, enable, and remove; `Snapshot.Stations` ordering after a middle removal; revisions increment on every accepted update including an identical one; a ninth active station returns `ErrLimit`. |
| `TestStationRevisionConflict` | A stale revision on update and on remove returns `ErrConflict`, leaves the station and its settings unchanged, and a retry with the current revision succeeds. |
| `TestStationValidationRejects` | Every numeric bound, NaN and infinity, every malformed identifier shape, a duplicate identifier, a removed identifier, and an unknown identifier, each with the documented error category and no state change. |
| `TestStationDisabled` | An enabled and a disabled station with identical settings over the same script: the enabled one receives, the disabled one produces no receptions; re-enabling resumes the stream at the position it stopped, matching a control that was never disabled but evaluated the same transmissions. |
| `TestStationsPreserveTransmissions` | A script with heavy station churn against a station-free control: identical `Batch.Transmissions`, identical final `Snapshot.Aircraft`, identical `Snapshot.History`, and identical clock and carry. |
| `TestReceptionDeterminismReplay` | Two independently constructed engines running one fixed script that includes station commands: identical transmissions, identical receptions, and identical final snapshots including stations. |
| `TestReceptionDeterminismPartitions` | One advance against uniform, irregular, and exact-deadline partitions, and one scaled elapse at 0.01x, 1x, and 100x against split real durations: identical concatenated receptions in every case. |
| `TestReceptionAltitudeEffect` | Fixed station and fixed range at the boundary of the horizon rule: a higher aircraft is received and a lower one is not, with the crossover range matching an independently computed horizon within 0.1 NM. |
| `TestReceptionSensitivityEffect` | Fixed geometry inside the horizon: a station at one sensitivity receives and an otherwise identical station at a degraded sensitivity does not, with the crossover matching an independently computed link budget within 0.1 NM. |
| `TestReceptionFrameLoss` | Probability 0 accepts every geometrically eligible transmission; probability 1 accepts none; an intermediate probability reproduces exactly across two engines and across split calls, and changes no transmission. |
| `TestCoverageMatchesReception` | For a horizon-limited station and a link-budget-limited station, aircraft placed at 0.999 and 1.001 of the published effective radius at the reference altitude are received and not received respectively, with frame loss probability 0. |
| `TestCoverageEstimate` | Independently computed horizon, link budget, and effective radii for both regimes, the unreachable case where the budget chord is shorter than the height difference, and rejection of invalid settings and out-of-domain reference altitudes. |
| `TestStationAtomicity` | Pre-canceled, deadline-exceeded, and late-observed cancellation at several depths for add, update, and remove; plus every rejected input case: nil or zero results, unchanged snapshots, and an identical valid suffix against a control. |
| `TestReceptionOwnership` | Mutating returned `Batch.Receptions`, a reception frame array, and `Snapshot.Stations` has no effect on engine state or on subsequent output. |
| `TestStationConcurrentAccess` | Concurrent snapshot reads alongside serialized station commands and time calls do not race and never expose a station list inconsistent with the snapshot time. |

Geometry tests place stations and aircraft on one meridian so the great-circle
distance is the difference in latitude times the Earth radius, which the test
computes directly. Use degenerate spawn ranges so aircraft truth is exact and
no tolerance is needed for position.

Numerical tolerances: crossover ranges within 0.1 NM, published radii within
0.1 NM of the independently computed value, received power within 0.01 dB, and
reception slant range within 0.01 NM of the independently computed chord.

Probe coverage boundaries strictly inside and strictly outside, never exactly
at the boundary, so no assertion depends on floating-point tie behavior.

Package-internal fixtures are limited to states that valid public configuration
cannot reach, such as ordinal exhaustion. Keep production code free of test
hooks.

Cancellation and timing tests contain no sleeps and never read `time.Now`.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation ./internal/adsb`
- `go test -race ./simulation`
- `go test ./simulation -run 'TestStation|TestReception|TestCoverage' -count=10`

## Acceptance Criteria

- Every named matrix row has an executable assertion tied to a backlog 03
  acceptance condition.
- Each failed or canceled station command is followed by a matching-control
  suffix comparison, not only a snapshot comparison.
- Altitude and sensitivity crossovers are verified against independently
  computed limits, not against the production helper.
- Coverage agreement is verified for both limiting regimes.
- Station churn provably leaves generated transmissions and aircraft truth
  byte identical.
- Concurrent tests assert safety without promising a concurrent command order.
- The whole suite passes ten consecutive runs and under the race detector.

## Non-Goals

Throughput qualification, reception history retention, observed aircraft state,
HTTP integration, browser tests, and coverage-percentage targets.
