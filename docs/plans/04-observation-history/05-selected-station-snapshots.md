---
title: "05 - Selected-station snapshots"
dependencies: ["04-received-field-projection.md"]
effort: "M"
complexity: "high"
---

# Selected-station snapshots

## Objective

Provide coherent, detached observation snapshots for an explicit station set.

## Target Artifacts

new simulation/observation_snapshot.go and simulation/observation_snapshot_test.go;
simulation/observations.go; simulation/reception_history.go.

## Implementation Tasks

1. Implement Engine.Observations(ctx, ObservationRequest)
   (ObservationSnapshot, error). Validate and copy the request, deduplicate IDs,
   sort lexically, and resolve every station under the engine mutex.
2. Capture virtual Now and copy selected rings under the same lock. Release the
   lock before projection; the detached data describes that captured instant.
   Check cancellation during merge/replay and before returning.
3. Union records by transmission sequence within the captured run. Verify
   duplicate copies have matching timestamp/frame/ICAO/kind; retain each
   receiver copy once. Sort sources lexically by station ID and evidence by
   transmission sequence. Pairing across two selected stations is allowed.
4. Run the step 04 projection. Sort Aircraft numerically by ICAO. Attach explicit
   selection, effective TTLs, and per-station bounds and truncated flags.
   An empty selection returns allocated empty arrays and coherent Now.
5. Ensure unselected station evidence cannot appear in any field or provenance.
   Selection changes rebuild from that selection only; no cross-query cache.
6. Test duplicate and permuted IDs, overlapping receptions, complementary CPR
   halves, unknown and removed stations, disabled station history, empty
   selection, and mutation of all returned nested data followed by a reread.

## Technical Details

Projection is intentionally bounded by selected retained rings. Eviction
before field TTL must remove unreconstructible evidence and set truncation
metadata. Queries neither mutate RNG/clock nor cache state across selections.
No query may join historical receiver references with current station settings.

## Verification

```text
go test ./simulation -run 'ObservationSnapshot|ObservationSelection|ObservationOwnership'
```

## Acceptance Criteria

Equivalent station sets produce deeply equal snapshots. A single shared
transmission updates an aircraft once and retains all selected provenance.
Concurrent mutations cannot mix two instants within one result.

## Non-Goals

Cross-request accumulation, implicit select-all, truth-backed field filling.

