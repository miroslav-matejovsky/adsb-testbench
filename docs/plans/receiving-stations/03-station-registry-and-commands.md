---
title: "03 - Station registry, revisions, and atomic commands"
dependencies: ["01-station-contract-and-configuration.md","02-reception-model-and-coverage.md"]
effort: "L"
complexity: "high"
---

# 03 - Station registry, revisions, and atomic commands

## Objective

Store stations in the engine with reserved identifiers, monotonic revisions,
and independent random streams, and expose atomic add, update, and remove
commands plus detached station records in snapshots.

## Target Artifacts

- Create `simulation/registry.go` and `simulation/registry_test.go`.
- Extend `simulation/random.go` with the station domain tag.
- Extend `simulation/engine.go` with `AddStation`, `UpdateStation`, and
  `RemoveStation`, and with a `stations` field in the private `state`.
- Extend `simulation/snapshot.go` and `simulation/types.go` with
  `Snapshot.Stations`.
- Extend `simulation/engine_test.go` and `simulation/snapshot_test.go`.

## Implementation Tasks

1. Add `stationDomain` with value 4 to the existing domain tags in
   `simulation/random.go`. Document that station ordinals and aircraft ordinals
   are independent counters and that the tag keeps their streams disjoint.

2. Define a private `station` record holding the accepted `StationConfig`, the
   revision, the creation elapsed duration, the creation ordinal, and a
   `rand.PCG` value seeded with `newSource(cfg.Seed, ordinal, stationDomain)`.
   Store the generator by value, never a `rand.Rand` pointer into committed
   state.

3. Define a private `stationRegistry` holding an ordered slice of active
   stations in creation order, a set of reserved identifiers, and the next
   ordinal. Implement `clone` copying the slice and the set so a staged
   mutation shares nothing with committed state.

4. Implement registry operations: lookup by identifier through a linear scan of
   at most `MaxStations` entries; append preserving creation order; replace in
   place preserving position; and remove preserving the relative order of the
   remaining stations.

5. Implement checked ordinal allocation starting at 1, mirroring the existing
   aircraft allocator, and return an `ErrLimit` error if it would overflow.

6. Implement `Engine.AddStation`. Check the context, validate the settings,
   reject a reserved identifier, reject exceeding `MaxStations`, clone state,
   allocate the ordinal, create the station at the committed elapsed time with
   revision 1, check the context again, commit, and return the created record.

7. Implement `Engine.UpdateStation`. Check the context, validate the settings,
   locate the station named by `StationConfig.ID`, compare the supplied
   revision, replace every setting in a staged clone, increment the revision,
   check the context again, commit, and return the updated record. Leave the
   creation instant, ordinal, and generator state untouched.

8. Implement `Engine.RemoveStation`. Check the context, locate the station,
   compare the supplied revision, remove it from a staged clone while keeping
   its identifier reserved, check the context again, and commit.

9. Add `Stations []Station` to `Snapshot`, populated under the same lock as the
   rest of the snapshot, in creation order, with `CreatedAt` converted from the
   elapsed duration through the clock start instant.

10. Add tests for lifecycle, ordering, revisions, conflicts, reserved
    identifiers, limits, cancellation, snapshot detachment, and isolation from
    aircraft state.

## Technical Details

Command signatures are `AddStation(context.Context, StationConfig)
(Station, error)`, `UpdateStation(context.Context, uint64, StationConfig)
(Station, error)`, and `RemoveStation(context.Context, string, uint64) error`.
The revision argument is the revision the caller last observed.

An update replaces every setting except the identifier, so disabling a station
is an update with `Enabled` false and needs no separate method. Changing an
identifier is a removal followed by an addition, which allocates a new ordinal
and therefore a new random stream. An update that assigns identical settings
still increments the revision, so no value comparison is needed anywhere.

A revision mismatch returns `ErrConflict` and changes nothing. An unknown
identifier returns `ErrNotFound`. A duplicate or previously removed identifier
returns `ErrInvalid`, because the caller supplied a value outside the accepted
identifier domain for this run. Exceeding `MaxStations` returns `ErrLimit`.
Failed commands return the zero `Station` or the error alone.

Station commands emit no frames and settle no time, exactly like `SetSpeed`.
They must not read or write aircraft records, deadlines, aircraft generators,
the clock, the carry, the identity allocator, the sequence counter, or the
transmission history. The staged clone makes this testable: a control engine
that never receives a station command must produce byte-identical later output.

Reuse the existing state cloning pattern. Add the registry to the private
`state` struct and extend its `clone` method; do not introduce a second staging
mechanism, a transaction interface, callbacks, or an event bus.

`Snapshot.Stations` is a fresh slice of value records, so editing it cannot
reach engine state. `StationConfig` contains only a string and numbers, so a
value copy is already detached.

Use the same context-check placement as the existing mutations: after acquiring
the lock, before expensive staged work, and immediately before commit.

Do not evaluate receptions in this step and do not change any existing method
signature.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestStation|TestRegistry|TestSnapshot'`
- `go test -race ./simulation -run 'TestStation'`

## Acceptance Criteria

- Add, update, disable, enable, and remove behave as documented, and
  `Snapshot.Stations` lists active stations in creation order.
- Revisions start at 1 and increment on every accepted update, including an
  update that assigns identical settings.
- A stale revision on update or remove returns `ErrConflict`, changes nothing,
  and a retry with the current revision succeeds.
- An unknown identifier returns `ErrNotFound`; a duplicate or previously
  removed identifier returns `ErrInvalid`; a ninth active station returns
  `ErrLimit`.
- A removed identifier cannot be reused for the rest of the run, and a
  re-created station with a new identifier receives a new ordinal and a
  different random stream.
- Every rejected or canceled station command leaves stations, revisions,
  reserved identifiers, ordinals, and station generator states unchanged.
- An engine driven by an identical script with arbitrary station churn produces
  transmissions and aircraft snapshots identical to a station-free control.
- Editing a returned `Station` or `Snapshot.Stations` cannot change engine
  state.

## Non-Goals

Reception decisions, reception records, batch return changes, per-station
history, cursors, and observed aircraft state.
