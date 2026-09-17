---
title: "06 - Acceptance regressions"
dependencies: ["05-selected-station-snapshots.md"]
effort: "M"
complexity: "high"
---

# Acceptance regressions

## Objective

Cover every backlog acceptance case through deterministic public engine APIs.

## Target Artifacts

new simulation/observation_acceptance_test.go;
simulation/determinism_test.go; simulation/atomicity_test.go;
simulation/reception_determinism_test.go; simulatorapi/observations_test.go.

## Implementation Tasks

1. Build fixtures from explicit existing engine/station configurations. Start
   with zero aircraft, add stations, then SetCount so birth receptions exist.
   Use loss probability 0 or 1 and ordered station edits for deterministic
   reception control; use codec-created inputs for isolated projection tests.
2. Implement the acceptance matrix below with assertions on values, timestamps,
   exact frames, provenance, cursor metadata, and original error categories.
3. Compare whole-duration and split-duration runs at the same control instants:
   complete batches, retained pages, and observations must match.
4. Extend atomicity comparisons to retained receptions and query results after
   rejected station revisions, sequence limits, and deterministic cancellation.
5. Exercise MaxStations simultaneous rings beyond capacity and repeated queries;
   assert retention bounds and unchanged later generated frames.
6. Run package tests and race tests where the installed toolchain supports them.
   Record results in progress.md.

## Technical Details

| Backlog case | Required assertion |
| --- | --- |
| Loss of one CPR half | Altitude may exist; no initial horizontal fix and no fabricated coordinates. |
| Independent expiry | Identity, position, altitude, and velocity expire independently at TTL + 1ns. |
| Selection/deduplication | Duplicate IDs and shared frames do not duplicate aircraft or refresh fields twice. |
| Complementary stations | Two selected stations can supply separate halves; selecting either alone cannot. |
| Retention gap | Stale reception cursor sets Gap; missing transmissions alone do not. |
| Station removal | Old returned records keep provenance; subsequent page/selection returns ErrNotFound. |
| Restart | Same scenario with fresh Config.ID rejects old cursors and exposes no old evidence. |
| Empty-scenario aging | SetCount(0), then Advance ages fields without receptions; eventual snapshot is empty. |
| Missed transmissions | Loss=1 or disable allows new transmissions but leaves observed times unchanged. |
| Pause | Elapse at speed 0 does not age fields; explicit Advance still does. |
| Eviction before TTL | Truncation is reported and lost evidence cannot supply a fresh field. |
| Receiver update | Old records keep settings/revision; later receptions contain the new values. |

No sleeps or probabilistic success criteria. An empty scenario may still expose
fresh historical observations until field expiry or evidence eviction.

## Verification

```text
go test ./simulation ./simulatorapi
go test -race ./simulation ./simulatorapi
```

## Acceptance Criteria

Every matrix row has a named passing regression test. Frame determinism and
atomic rollback remain intact. Transport fixtures preserve unknown versus zero
and exact sequence representations.

## Non-Goals

Live HTTP integration, browser tests, and hardware RF validation.

