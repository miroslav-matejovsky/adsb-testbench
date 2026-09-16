---
title: "05 - Atomic engine API and bounded history"
dependencies: ["03-frame-generation-and-scheduling.md","04-virtual-clock-and-scaling.md"]
effort: "L"
complexity: "high"
---

# 05 - Atomic engine API and bounded history

## Objective

Compose the pure components into the public Engine with atomic mutations, complete report batches, stable count changes, and detached snapshots.

## Target Artifacts

- Create `simulation/engine.go`, `simulation/history.go`, and `simulation/snapshot.go`.
- Create `simulation/engine_test.go`, `simulation/history_test.go`, and `simulation/snapshot_test.go`.

## Implementation Tasks

1. Implement New and all Engine methods from the overview. New stages the initial fleet and creation reports before returning an engine; its at-most-300 initial reports all fit in history.

2. Store one mutex separately from a private state value. Clone the state for mutations: aircraft slice, birth/deadline values, all PCG values, history backing storage, time, carry, sequence, and identity counters. Never copy the mutex or retain a Rand wrapper pointing into committed state.

3. For Advance and Elapse, validate, compute the target, and drain every event in (current,target] in stable order. Update candidate time to the target even when there are no reports or aircraft.

4. Implement SetCount at committed time. Add fresh identities in creation order and return their three birth reports each; remove newest aircraft first. Removing targets does not remove retained transmissions.

5. Implement SetSpeed without settling real time or emitting reports. The future driver is responsible for calling Elapse before a speed/count change when needed.

6. Check context after lock acquisition, before expensive staging, between aircraft creation/event work, and immediately before commit. Return the original context error and a nil batch on failure.

7. Commit the entire candidate state only after encoding, bounds checks, and the final cancellation check pass. Return detached complete batches, including frames already evicted from history during the call.

8. Implement a 1000-element history ring ordered oldest to newest in snapshots. Make Snapshot copy all exposed slices and evaluate aircraft truth at the same committed time under the lock.

9. Add tests for creation, no-op commands, count changes, ordering, history eviction, snapshot ownership, and a batch larger than retained history.

## Technical Details

An accepted cancellation occurs when a context check observes cancellation.
Cancellation arriving after the final check/commit point may accompany a
successful return; document this standard race explicitly. Do not store
contexts in the Engine. Callers supply non-nil contexts, as with standard Go
context APIs. Err() checks provide cancellation here; no network waits occur.

A no-op SetCount/SetSpeed and zero-duration time call must still check context.
They leave random streams, identity allocation, schedules, and carry unchanged.
Return an allocated empty transmission slice on success with no reports;
return nil on an error. Failed New returns nil plus an error.

Use a last-sequence counter, not an unchecked next-sequence increment. The
last legal uint64 sequence can be emitted; a subsequent emission fails.
Likewise the final legal aircraft address can be allocated, but a further
allocation fails even after that aircraft has been removed.

At the end of each committed mutation, every retained aircraft has future
deadlines strictly greater than Now. A command issued after Advance reaches
an event's time therefore follows that event. SetCount does not duplicate due
reports for surviving aircraft.

History bounds refer to retained transmissions. OldestSequence and
LatestSequence are zero only when history is empty. Otherwise they match the
first and last retained records. A consumer detects lost retention when its
last processed sequence plus one is below OldestSequence, comparing within
the same run ID. Snapshot.Config and history bounds are captured together.

Atomic staging is intentionally direct for this bounded engine. Do not add
a transaction interface, callbacks, event bus, injectable production codec,
or externally configurable rollback system.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestEngine|TestHistory|TestSnapshot'`
- Decode complete emitted batches with internal/adsb in integration tests and assert monotonic sequence/timestamp ordering.

## Acceptance Criteria

- Public methods obey their proposed signatures and documented empty/error returns.
- No accepted mutation exposes partial aircraft, history, clock, or RNG changes.
- Reducing count preserves older identities and history; increasing never reuses removed identities.
- Complete returned batches remain complete even when history retains only their tail.
- Editing any returned batch or snapshot cannot alter engine state or subsequent output.
- Empty-fleet time advancement, count-zero transitions, and restart identities behave as documented.

## Non-Goals

Station state, HTTP data contracts, live observation history, wall-clock settlement, and public command implementations.
