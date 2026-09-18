---
title: "07 - Display refresh state and HTTP backend"
dependencies: ["05-display-validation-and-decoding.md", "06-local-and-http-sources.md"]
effort: "L"
complexity: "high"
---

# 07 - Display refresh state and HTTP backend

## Objective

Publish received tracks atomically, expose outage/restart state, and provide
relative browser-facing routes backed by either observation source.

## Target Artifacts

- New `display/display.go`, `snapshot.go`, `http.go`, `http_json.go`, and tests.
- Existing `display/config.go`, `config_json.go`, `errors.go`, and `doc.go`.
- Public display response DTOs in `display/snapshot.go`.

## Implementation Tasks

1. Implement `New(Config, ObservationSource)` with complete validation and no
   background work. Add an explicit host error callback for HTTP writer/internal
   failures, supplied separately from serialized settings as in simulator.
2. Implement `Refresh(ctx, stationIDs)` with context-aware serialized admission
   using a one-token gate and `select` on cancellation. Perform source fetching,
   common semantic validation, and decoding into temporary state. The gate
   serializes refresh/history operations that can detect a source run change;
   a separate short lock protects detached snapshot publication.
3. Normalize station selection for comparison. Store only one successful
   snapshot plus its exact selection and configured expiry. Do not create a
   map of per-selection caches. Rebuild all fields and CPR state on each refresh
   from the current full raw snapshot; never merge prior decoded fields.
4. For a valid same-run snapshot, reject virtual-time regression and per-station
   latest-sequence regression for overlapping selections. Publish atomically
   only after validation and decoding succeed. Selection changes replace data
   only after success, and cannot borrow last-good data from another selection.
5. On a validated new-run envelope, invalidate prior-run tracks/cursors/fallback
   before attempting the new run's payload decode. If the payload then fails,
   publish unavailable state, not old-run data. A valid shared conflict error
   carrying a different current run ID also invalidates old-run state. Handle
   `SourceError`'s validated run ID when an adapter rejected the payload before
   returning a DTO in the same way. Unknown
   or malformed source identity is an error and never a successful refresh.
6. Return errors on all failed refreshes. For failure with matching selection
   and no detected run change, retain last-good data explicitly marked stale.
   Store last successful local update time separately from source virtual now.
   Inject a private clock in tests; production can use `time.Now` for this
   diagnostic timestamp. Never age aircraft fields using wall time.
7. Define detached `Snapshot` output containing `status` (`unavailable`, `fresh`,
   or `stale`), optional last successful real update time, optional received
   observation snapshot, and structured last source error. Before first success,
   status is unavailable and observations are null. `fresh` describes the last
   successful refresh, not a promise about time elapsed since polling stopped.
8. Pass `ReceptionHistory` requests through the same source and validators.
   Preserve explicit page size, run/station identity, gaps, and raw records.
   Keep cursor ownership with the caller; maintain no unbounded inspector cache.
   Gap is a successful incomplete-history response, not a failed source read.
9. Implement relative routes: `GET /snapshot` returns detached cached status,
   `POST /observations` requires explicit `stationIds` and refreshes using
   configured expiry, `POST /receptions/history` proxies validated history.
   The browser calls this backend, never the upstream simulator directly.
10. Reuse the bounded strict parsing/encoding pattern from step 04 within the
    display package. Return 200 for successful refresh, the shared error status
    with optional matching stale observations on refresh failure, and explicit
    history gap metadata. Map malformed upstream contracts/frames to 502
    `source_invalid`, source unavailable to 503, and deadlines to 504. Invalid
    browser input remains 400 and is rejected before source access.
11. Test initial state, selection changes/empty selection, same/different runs
    with reused ICAOs, partial failures, regressions, repeated snapshots, caller
    mutation, bounded cache replacement, canceled admission, overlapping reads,
    nested route mounts, and warm/cold equivalence after retention eviction.

## Technical Details

Track-field expiry comes only from raw snapshot virtual time and explicit
lifetimes. A source outage freezes the last received evidence but exposes
stale status and local update time so the UI can label it correctly. Failed
selection changes return no previous selection's aircraft in the response.

`Refresh` may return stale data together with an error; callers must not drop
the error. A decoded snapshot is never partially committed. A reset caused by
a new run is a separate deliberate invalidation, preventing fallback to a
known obsolete run even when the new payload is corrupt.

## Verification

```text
go test ./display
go test -race ./display
task arch-lint
```

## Acceptance Criteria

- Refresh results are atomic and contain only received-frame-derived fields.
- Errors are returned with explicitly stale same-selection fallback or no data.
- Detected run changes clear prior state, even if new-run decoding fails.
- One last-good snapshot bounds retained display state across selections.
- Browser-facing routes preserve limits, context, provenance, and history gaps.
- Concurrent refreshes, reads, and cancellation pass deterministic/race tests.

## Non-Goals

Map rendering, fresh/stale/lost UI styling, automatic polling, listener ownership,
and combined/standalone command wiring.
