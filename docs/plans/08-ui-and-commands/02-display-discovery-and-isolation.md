---
title: "02 - Display station discovery and failed-request isolation"
dependencies: ["01-url-and-server-contracts.md"]
effort: "M"
complexity: "high"
---

## Objective

Serve station choices and coverage through the display backend and ensure early
refresh failures never return another selection's fresh data. Fix F1 and F2.

## Target Artifacts

- `display/source.go`, `source_local.go`, `source_http.go`, `response_json.go`.
- `display/display.go`, `snapshot.go`, `http.go`, `validate.go`, `doc.go`.
- `display/source_test.go`, `display_test.go`, `http_test.go`, `validate_test.go`.
- New `display/stations.go`, `stations_test.go`; public example/test callers.

## Implementation Tasks

1. Add consumer-owned `StationSource` with
   `Stations(context.Context) (simulatorapi.StationsSnapshot, error)`. Keep
   `ObservationSource` unchanged. Extend `display.New` with an explicit station
   source argument, validate it, and update callers/fakes/examples together.
2. Add a separate `NewInProcessStationSource` adapter. Make `HTTPSource` also
   satisfy `StationSource` by issuing bounded `GET stations` requests through
   the existing transport rules. Preserve response limits, earlier deadlines,
   redirect refusal, response-body closure, and source error categories.
3. Add strict station-response parsing and one semantic validator shared by both
   adapters and `Display.Stations`. Validate run/time, station uniqueness/count,
   revisions, all numeric domains and finiteness, nonnegative coverage radii,
   reference altitude, and effective radius consistency. Deep-copy returned data.
4. Add `GET /stations` to `Display.Handler`, with no query/body, correct method
   errors, timeout, byte limits, no-store response, and existing error reporting.
   Station discovery participates in the same serialized source admission and
   validated run-change invalidation as refresh/history.
5. Fix early exits in `Refresh`: validate the explicit selection before source
   admission and return a detached `StatusUnavailable` response with the failure
   on invalid selection or canceled admission. Do not return `d.Snapshot()` or
   modify another admitted request's state on these paths.
6. Add regression tests that first publish A, then attempt canceled B and invalid
   B. Assert no A observations, no fresh status, and unchanged published A state.
   Add an HTTP failure-envelope test and a gated concurrent-refresh test.
7. Document `GET /snapshot` as shared diagnostic state. Preserve the existing
   single-snapshot bound; browsers must consume their own refresh result. Add
   run-change tests covering a discovery response from the replacement run before
   the next observation refresh.

## Technical Details

Reuse `simulatorapi.StationsSnapshot`, `StationState`, and `Coverage` instead of
inventing a new public station DTO. `simulator.API.Stations` already provides the
in-process operation and simulator `GET /stations` already provides the wire form.
Station discovery carries no aircraft truth and never calls `/truth`.

The client gets field lifetimes from `ObservationSnapshot.Expiry`, including an
explicit empty-selection refresh. `historyPageSize` is explicit UI configuration
within the existing 1..1000 history contract; no new general metadata proxy is
required. Station and observation snapshots are independent reads: browser code
must compare their run IDs and must not claim atomicity between them.

For early failures no safe fallback has been chosen, so return unavailable.
For admitted source failures keep the existing same-selection stale fallback
and validated restart rules. Step 06 supplies per-component outage fallback.

## Verification

```text
go test ./display ./simulator
go test ./display -run 'Stations|Refresh|Selection|Cancel|Restart'
task all
```

Exercise identical local/HTTP station data and malformed values, nil arrays,
unknown JSON keys, oversized bodies, wrong methods, canceled admission, and
station removal/restart. Tests use channel-controlled sources and fixed clocks.

## Acceptance Criteria

- A display host exposes station choices and coverage without browser access to
  the simulator host or truth endpoint.
- Local and remote station sources classify equivalent failures identically.
- Invalid/canceled B refreshes never return A observations or a fresh snapshot.
- Restart discovery clears the previous run's fallback before further rendering.

## Non-Goals

Server-side browser sessions, a multiple-selection cache, control proxying, or
changing raw ADS-B decoding and expiry rules.
