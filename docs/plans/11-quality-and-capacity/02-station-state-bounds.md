---
title: "02 - Bound station identity retention and reject revision overflow"
dependencies: []
effort: "M"
complexity: "medium"
---

## Objective

Fix Q2 and Q4 while preserving station ordering, deterministic streams,
historical provenance, and atomic failure behavior.

## Target Artifacts

- `simulation/types.go`, `registry.go`, `engine.go`, `doc.go`.
- `simulation/registry_test.go`, `atomicity_test.go`, `reception_determinism_test.go`.
- `simulatorapi/metadata.go`, `simulator/api.go`, relevant contract/API tests.
- `ui/browser/fixtures.mjs`, `configs/README.md`.

## Implementation Tasks

1. Add documented exported constant `MaxStationIDsPerRun = 1024` in
   `simulation/types.go`. This counts all successfully created station IDs,
   including removed ones. It is an engine policy, not a configuration default.
2. Extract a private registry addition check used by `stationRegistry.add`.
   Preserve error precedence: used ID returns `ErrInvalid`; active capacity,
   lifetime capacity, and ordinal exhaustion return `ErrLimit`. Check them
   before changing `nextOrdinal`, reserving an ID, or allocating a reception ring.
3. Keep removed IDs in `reserved`. Reject a new ID when its length reaches
   `MaxStationIDsPerRun`. Do not reject updates or removals because the lifetime
   budget is full. A fresh engine starts with an empty budget.
4. In `stationRegistry.update`, check `revision == math.MaxUint64` after ID and
   expected-revision checks but before assigning configuration. Return contextual
   `ErrLimit`; the current station must remain removable at its current revision.
5. In `stationRegistry.remove`, clear the vacated final slice element before
   reducing its length. Use an explicit `copy`, zero assignment and reslice,
   or `slices.Delete`, which clears obsolete elements. This prevents the unused
   tail from retaining a removed station's reception ring. Preserve active order.
6. Add `MaxStationIDsPerRun` / JSON `maxStationIdsPerRun` to
   `simulatorapi.EngineLimits`, populate it in `API.Metadata`, and update exact
   wire-shape tests and the browser fake metadata. Do not expose the complete
   reserved-ID set. Document both the active and lifetime limits and the restart
   requirement in engine docs and configuration documentation.
7. Add table-driven boundary tests. Arrange the reserved set near capacity in
   same-package tests to make exact-boundary tests fast. Also perform real
   add/remove cycles with zero aircraft to verify the public lifecycle. Keep
   only one station active so active capacity cannot mask lifetime exhaustion.
8. Arrange a station at `MaxUint64-1` in a same-package test. One matching update
   must reach `MaxUint64`; the next must return `ErrLimit`. A stale revision must
   still return `ErrConflict`; matching removal must succeed. Compare snapshots
   and run an identical valid suffix against a control engine after each rejection.

## Technical Details

Test the following states separately: 1023 used IDs accepting the 1024th,
1024 used IDs rejecting a distinct 1025th, reuse of a removed ID, eight active
stations with lifetime space left, and removal/update at lifetime capacity.
Failed additions must not consume an ordinal, RNG stream, or reserved ID.

Structural retained-state proof after this step:

- At most 100 active aircraft and 1,000 retained transmissions.
- At most eight live reception rings of 1,000 records each.
- At most 1024 reserved station IDs, each at most 64 bytes, plus bounded map
  overhead. Do not equate 65,536 identifier bytes with total heap consumption.
- Removed stations are absent from the live slice and its unused tail.

A narrow internal test may inspect the reserved set, ordinal and vacated tail
because public snapshots intentionally hide them. Most tests must assert public
errors, snapshots, ordering and future frame/reception output.

## Verification

```text
go test ./simulation -run 'TestStation|TestReceptionDeterminism|TestAtomic' -count=1
go test ./simulatorapi ./simulator
npm run test:unit
task arch-lint
```

Also run the whole simulation package so tests not matched by the focused
expression execute: `go test ./simulation`.

## Acceptance Criteria

- A run accepts at most 1024 distinct station IDs, including removed stations.
- Every overflow returns the documented category without changing future output.
- Revisions never wrap to zero; exhausted stations remain removable.
- Metadata reports the new lifetime bound and existing wire tests pass.
- Removal releases obsolete tail references while preserving station order.

## Non-Goals

No ID reuse, eviction of reserved IDs, persistence, configurable station-lifetime
default, or redesign of engine mutation staging.
