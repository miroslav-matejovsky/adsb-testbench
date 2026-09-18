---
title: "02 - Coherent raw reception snapshot"
dependencies: ["01-contracts-and-explicit-configuration.md"]
effort: "M"
complexity: "medium"
---

# 02 - Coherent raw reception snapshot

## Objective

Expose bounded raw evidence for all selected stations at one committed
virtual instant, without changing generation, pacing, or reception decisions.

## Target Artifacts

- `simulation/observations.go`, `observation_snapshot.go`, and `doc.go`.
- New `simulation/reception_snapshot.go` and `reception_snapshot_test.go`.
- `simulator/simulator.go` and `simulator_test.go`.

## Implementation Tasks

1. Add native request/response types corresponding to the raw shared DTO.
   Include run ID, virtual now, selected IDs, per-station retention, and exact
   reception records. Keep native time/uint64/frame types inside `simulation`.
2. Implement `Engine.ReceptionSnapshot(ctx, request)` following the existing
   selected-station snapshot validation: validate IDs, deduplicate, sort,
   enforce `MaxStations`, and make empty selection deliberately empty.
3. Under one engine lock, resolve every selected active station and copy all
   records plus retention metadata, run ID, and now. If any station is absent,
   return `ErrNotFound` with no partial snapshot. Release the lock before
   sorting the detached records and doing any transport conversion.
4. Return records ordered by transmission sequence and station ID. Preserve
   historical receiver revisions and exact frame bytes. Bound the total by
   `MaxStations * ReceptionHistoryLimit` and check cancellation before/after
   capture and while processing the detached data.
5. Extract only the shared station-history capture logic actually needed by
   this method and existing `Engine.Observations`; retain the latter's decoding
   semantics. Do not implement the new method by looping over paged reads.
6. Add `Simulator.ReceptionSnapshot` as a read-through method. Extend runtime
   tests to prove it does not settle elapsed wall time or mutate engine state.
7. Test zero stations, unknown/removed/disabled stations, duplicate IDs, edited
   receiver provenance, capacity/eviction, cancellation, returned-slice
   mutation, and concurrent capture/mutation using barriers and invariants.

## Technical Details

Each retention entry describes exactly the records copied for that station.
An empty station has oldest/latest sequence zero. Nonempty records are
contiguous in station reception sequence even if transmission sequences skip.
`Truncated` follows the engine's existing `oldest > 1` rule. Snapshots cannot
claim a later clock instant or station revision fetched in a second read.

Tests assert timestamps do not exceed snapshot now, station record bounds
match retention, and no selection mixes a removed station with newly captured
state. Existing frame/determinism fixtures must remain byte-for-byte unchanged.

## Verification

```text
go test ./simulation ./simulator
go test -race ./simulation ./simulator
```

## Acceptance Criteria

- Every returned record and metadata field belongs to one atomic capture.
- Reads leave virtual time, randomness, histories, and frame output unchanged.
- Snapshots are detached and cancellation yields no partial result.
- Existing observation and history tests pass without changed expectations.

## Non-Goals

New retention policy, streaming, paging across stations, or engine HTTP types.
