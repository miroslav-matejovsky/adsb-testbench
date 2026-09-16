# Feasibility and implementation assessment

## Feasibility

Stations fit the existing package boundary. The engine already stages and
commits state under one mutex, already derives independent random streams from
an explicit seed, and already evaluates aircraft truth at an exact virtual
instant before encoding a frame. Reception is one more evaluation at that same
instant, over a small ordered collection.

The model needs only standard-library mathematics. The spherical Earth radius,
the nautical mile conversion, and the local unit-vector construction already
exist in `simulation/motion.go` and are reused rather than duplicated.

The main work is keeping the new randomness and the new commands strictly
isolated from aircraft generation, and making published coverage provably the
same rule as the reception decision.

## Dependencies and evidence

- [Backlog 03](../../backlog/03-receiving-stations.md): intended behavior and
  acceptance.
- [Engine documentation](../../../simulation/doc.go): implemented configuration,
  virtual time, determinism rules, ownership, and error categories.
- [Codec documentation](../../../internal/adsb/doc.go): frame contents, the
  180 NM local-decoding bound, and the caller responsibilities that receiving
  consumers inherit.
- [Architecture rules](../../../.go-arch-lint.yml): `simulation` may use
  `adsb`; no rule change is required.
- Current module: Go 1.27.1, go-adsb v0.4.1, testify v1.12.1.
- `go doc ./simulation` confirms the current public surface that step 04
  changes.

The model constants are ordinary published engineering values: free-space path
loss at 1090 MHz, the 4/3 Earth radio horizon, and a transponder power typical
of a Mode S class that transmits about 125 W. They are used as explicit
scenario constants. No claim of transponder, antenna, or receiver fidelity is
added.

A sanity check of the chosen constants, using 3 dBi gain, 2 dB system loss, and
-95 dBm sensitivity: the link budget allows about 147 dB of path loss, which is
about 264 NM, while the radio horizon at 35000 feet with a 30 m antenna is
about 242 NM. The horizon binds at cruise altitude and the link budget binds
when sensitivity is poor, so both acceptance criteria are reachable with
ordinary settings rather than contrived ones.

## Risks and mitigations

| Risk | Required mitigation | Evidence |
| --- | --- | --- |
| Reception randomness perturbs aircraft output | Separate per-station streams keyed by a distinct domain tag and an independent ordinal counter; station commands touch no aircraft state. | `TestStationsPreserveTransmissions` against a station-free control, comparing transmissions and aircraft snapshots. |
| Draw counts shift when settings change | Draw one fraction per eligible station and transmission whatever the configured probability. | Equal receptions for equal scripts after changing only the probability value to another value and back. |
| Published coverage drifts from the reception decision | Invert both documented limits to surface radii and prove equivalence numerically. | `TestCoverageMatchesReception` probing at 0.999 and 1.001 of the effective radius. |
| Split time calls change reception decisions | Evaluate reception from the same absolute-instant truth already used for encoding; the station set is fixed per staged mutation. | `TestReceptionDeterminismPartitions` over the same partitions the engine already uses. |
| Batch memory grows with station count | `MaxStations` bounds fan-out and receptions are transient in this increment. | Worst-case arithmetic below. |
| Square roots and logarithms of invalid inputs | Validate finiteness and domains before any arithmetic; clamp negative heights to zero for the horizon and clamp slant range at one metre for the logarithm. | `TestStationValidationRejects` and boundary cases in `TestCoverageEstimate`. |
| Pressure altitude treated as geometric height | Document the simplification in the package documentation and in the field comments. | Wording check in step 06 through `go doc -all ./simulation`. |
| Identifier reuse confuses later cursors | Identifiers are reserved for the whole run, mirroring the existing aircraft address rule. | `TestStationLifecycle` re-adding a removed identifier. |
| Stale edits overwrite concurrent changes | Every update and removal carries an expected revision; a mismatch changes nothing. | `TestStationRevisionConflict` with a stale value followed by a successful retry. |
| Changing the batch return type breaks callers silently | One coordinated change with every caller enumerated in step 04. | `task all` plus the external consumer check in step 06. |
| Receptions leak engine state | Value records with array frames and copied slices on every public boundary. | `TestReceptionOwnership` mutating a returned batch and comparing later output. |

## Bounded work

An advance covers at most 60 virtual seconds, 100 aircraft, and 8 stations.
The existing bound is at most 31500 transmissions per call, below the 32000
frame cap. Reception fan-out is at most 8 per transmission, so at most 252000
reception records per call, below the derived `MaxBatchReceptions` cap of
256000. That cap can therefore never be the first limit reached.

A reception record is about 80 bytes, so the worst case is roughly 20 MB of
transient reception storage per call, in addition to roughly 2.3 MB of
transmissions. Committed state grows by at most 8 station records, each with
one small generator value, plus the reserved identifier set.

Evaluate stations with a direct scan over at most 8 entries per transmission.
Use linear lookup by identifier over the same ordered slice. Do not introduce a
spatial index, cache, or worker pool.

These are resource bounds, not throughput promises. Capacity qualification at
maximum speed, aircraft, and station counts remains backlog 11. Do not add
performance claims or a benchmark project to this increment.

## Verification strategy

Each implementation step defines focused tests. Step 05 adds the acceptance
matrix, the isolation evidence, and the failure paths. Step 06 runs package
tests, the public consumer compile check, architecture validation, and the
repository `task all` as implementation checks.

Tests use testify/require, fixed configurations with every field assigned,
explicit coordinates and altitudes, and bounded deterministic operation
sequences. Coverage expectations are derived independently in the test from the
published formulas rather than by calling the production helper. Geometry tests
use analytic placements along a meridian and along the equator, where the
great-circle distance has a closed form. No sleep, real clock, network service,
or statistical distribution assertion is required.

This planning task performs only Markdown structure, link, and change-scope
checks. It does not run test, build, or lint commands, and it does not run
`task all`.

## Rollback

Implementation adds behavior to one package and changes one published return
type. If a step fails its checks, keep the step incomplete and restore the
affected first-party changes using its reviewed diff, preserving unrelated user
edits. No database, persisted state, wire migration, deployment, or vendor
change is required. Do not use bulk repository resets and do not create commits.

If an implementation session stops incomplete, record concrete outstanding
actions in the repository `.todo`, keep backlog 03 open, and leave the step
status in `progress.md` accurate. Preparing this complete plan alone is not an
unfinished implementation session.
