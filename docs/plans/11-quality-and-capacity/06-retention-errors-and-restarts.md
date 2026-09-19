---
title: "06 - Prove retained-state bounds and deterministic failure recovery"
dependencies: ["01-scenario-corpus.md", "02-station-state-bounds.md", "03-validation-before-settlement.md", "04-response-capacity.md"]
effort: "M"
complexity: "medium"
---

## Objective

Prove that bounded histories and display/browser state stay bounded over repeated
operation, and that failures and replacement runs do not leave stale or partial
state. Extend existing tests instead of duplicating every current error case.

## Target Artifacts

- `simulation/history_test.go`, `reception_history_test.go`,
  `reception_snapshot_test.go`, `atomicity_test.go`.
- `internal/simdriver/driver_test.go`, `simulator/acceptance_test.go`.
- `display/bounds_test.go`, `display_test.go`, `source_test.go`.
- `ui/browser/unit/inspector.test.mjs`, `tracks.test.mjs`,
  `ui/browser/inspector.spec.mjs`, `aircraft.spec.mjs`, `restart.spec.mjs`.
- `ui/browser/README.md` and corpus coverage documentation.

## Implementation Tasks

1. Create a corpus-to-test coverage table. Mark existing tests that already
   prove a row, then add only missing combinations and boundary regressions.
   Keep timing, transport decoding and UI rendering assertions at their owners.
2. Advance an all-receive engine beyond three complete wraps of every selected
   station ring. At checkpoints assert transmission history <=1000, each station
   history <=1000, raw snapshot <=8000 and active stations <=8. Assert latest and
   oldest sequences, ordering and cursor gaps, not just slice lengths. Do not
   retain all generated batches in the test itself.
3. Exercise aircraft removal/replacement without time advance so retained
   addresses exceed the current fleet count. Compare warm and newly constructed
   displays over identical retained evidence. After eviction or expiry, verify
   both lose the same fields and tracks. Check returned snapshots are detached
   by mutating a response and fetching again.
4. Combine full-ring station updates, disable/enable and removal with revision
   provenance. Current settings must not rewrite historical records. Rejected
   revision or lifetime commands must preserve histories and the valid suffix's
   frame/reception stream. Reuse `requireIdenticalFuture` where applicable.
5. Extend fake-clock driver checks at maximum configured fleet/station counts:
   pause/resume, fractional speed, delayed wakeup, exact catch-up, over-limit
   catch-up, and cancellation between successfully committed chunks. Use small
   focused full-cardinality cases for ordinary tests; the expensive one-hour
   throughput run belongs in the measurement task. An exact one-hour correctness
   boundary can keep the existing zero-aircraft test.
6. Run the error/recovery matrix below through local and HTTP sources. For each
   failure, compare display publication and the next successful refresh. Reuse
   a barrier for source calls in flight and context cancellation; assert token
   release and that a subsequent call can proceed. Do not sleep until a deadline.
7. Feed more than 1000 deterministic pages into `mergePage` in the inspector
   unit test. Use a small explicit `maxRecords`, monotonically increasing
   sequences above 2^53, duplicates, and a gap on each page. Assert retained
   records <=maxRecords, exact latest cursor, trimming counts, and entries
   <=maxRecords plus the existing `MAX_GAP_ROWS` policy. Read its current value
   from the implementation documentation; if needed expose a testable constant
   without adding a configurable default. The current implementation already
   caps gap markers; protect that behavior rather than reporting it as a leak.
8. Add a track churn unit case proving disappeared aircraft survive only as the
   intended one-view tombstone. Repeated run replacement must clear that state.
9. Extend one presentation spec with a populated table/map, a response-limit
   failure, then recovery. Use `installPausedClock`, settled `poll`, and the
   fake API's barrier. Assert stale/error labeling and no partial new rows.
   Extend the existing restart integration only where needed for a history
   request held across replacement: release the old response after the new run
   is visible and assert it cannot restore old rows/cursors. A separate
   presentation test may own this ordering if the real host cannot hold it.
10. Update browser/test corpus docs with actual coverage and layer labels. Keep
    console errors, unhandled rejections and unexpected API access as failures.
    Browser tests must continue to require preinstalled dependencies.

## Technical Details

Required error/recovery matrix:

| Case | Publication and recovery assertion |
| --- | --- |
| Same-run transport outage | Last good matching selection is explicitly stale; recovery replaces it completely |
| Oversized HTTP payload | `response_limit`; body closed; no partially decoded publication |
| Truncated/malformed JSON | Source-invalid/unavailable category according to existing mapping; original read cause retained where applicable |
| Corrupt frame with otherwise valid envelope | Whole refresh fails; no mixing of valid prefix with last-good suffix |
| Invalid data carrying a validated new run ID | Old fallback is cleared before decode failure |
| Old-run history cursor or mutation | Conflict with current run identity; no mutation and no cursor reuse |
| Regressed time/sequence in same run | Source-invalid; subsequent forward valid snapshot succeeds |
| Changed selection during outage | No fallback from the previous selection |
| Cancellation while waiting at display gate | Source is not called for the rejected admission; gate remains usable |
| Station removal while history request is in flight | No new records appended to an invalidated selection/run |

Local and HTTP paths need equivalent semantic failures. Transport-only failures
such as malformed JSON exist only on HTTP and should be labeled accordingly.
Do not fabricate an HTTP error in a local provider simply to claim parity.

Use channel/barrier synchronization and supplied virtual instants. Existing
counting-context seams are acceptable for same-package atomicity checkpoints;
new service-level tests should prefer a source barrier and a normal cancelable
context so they do not depend on the number of `ctx.Err()` calls.

## Verification

```text
go test ./simulation ./internal/simdriver ./simulator ./display -count=1
npm run test:unit
npx playwright test ui/browser/inspector.spec.mjs ui/browser/aircraft.spec.mjs ui/browser/restart.spec.mjs
```

If the host has the Go race detector prerequisites, also run:

```text
go test -race ./simulation ./internal/simdriver ./simulator ./display
```

Record a missing race-toolchain prerequisite explicitly; it does not replace
the mandatory normal `task all` run. Do not install or update dependencies as a
side effect of ordinary tests.

## Acceptance Criteria

- Structural limits hold after at least three wraps, independent of heap noise.
- Churn, expiry, receiver revisions and warm/cold refresh yield expected data.
- Every matrix row maps to an executable test and a stated owning layer.
- Repeated history pages bound both records and gap markers with exact cursors.
- Old-run responses cannot resurrect cleared browser/display state.
- New tests use no sleeps, busy polling, or wall-clock-based field aging.

## Non-Goals

No browser heap-size gate, real-time unit-test assertions, redundant replacement
of existing restart suites, caching across refreshes, or production fault hooks.
