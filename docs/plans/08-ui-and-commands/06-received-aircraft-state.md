---
title: "06 - Received aircraft, selection, age, and outage state"
dependencies: ["05-station-editor.md"]
effort: "L"
complexity: "high"
---

## Objective

Build the table and per-component state used by the map and inspector, including
partial tracks, explicit station selection, and distinct transport/track status.

## Target Artifacts

- New `ui/internal/assets/static/aircraft.js`, `tracks.js`, and unit tests.
- Aircraft page template, component configuration, common styles.
- `ui/browser/aircraft.spec.mjs` and deterministic reception fixtures.
- UI and display package documentation.

## Implementation Tasks

1. Fetch display `GET stations` and render choices with current enabled/revision
   state. Begin with the exact configured `stationIds`, including an empty array.
   On selection change, abort/obsolete earlier requests, clear old selection
   fallback/history, and `POST observations` with the complete new selection.
2. Validate response run ID and exact normalized selection before publishing.
   Render identity, ICAO, position, pressure altitude, ground speed, track,
   vertical rate, field ages and receiver provenance. Unknown is a labeled
   unavailable value, never zero. Preserve over-range flags and distinguish
   heading/airspeed from ground track/speed for partial velocity subtypes.
3. Compute field age using observation `now` and `observedAt` in exact virtual
   nanoseconds. Trust backend field expiry: a null position has no active map
   marker, even if a previous snapshot had coordinates. Identity, altitude and
   velocity remain independently useful when position is absent.
4. Apply explicit track thresholds to `now - lastReceivedAt`: fresh at age <=
   `freshForNanoseconds`; stale above fresh and <= `lostAfterNanoseconds`; lost
   above lost. Partial fields and track freshness are separate properties.
   Thresholds are view policy and do not extend any backend field lifetime.
5. When an aircraft disappears between successful same-selection snapshots,
   show an unpositioned lost row labeled "No retained evidence" for one
   successful refresh only, then remove it. Current lost tracks remain visible
   while the backend retains their evidence. Never carry forward expired fields.
6. Keep one last successful snapshot per component. A failed poll may display
   only that selection's same-run data, labeled transport stale with real update
   age and actionable error. Freeze virtual ages and track classes during outage.
   Validate error-envelope run IDs and clear old data on a confirmed replacement
   run, even when its first payload is invalid. Do not use shared `GET snapshot`.
7. Handle station removal by marking selection unavailable and refreshing the
   catalog. Require explicit user selection changes instead of silently switching
   to all stations. On run change, clear rows/cursors and reconcile configured
   station IDs against the new catalog before the next coherent refresh.
8. Test partial identity-only/altitude-only/velocity-only tracks, missing CPR
   halves, expiry boundaries, zero values, selected-empty state, two selections
   in two tabs, late responses, outage recovery and restart.

## Technical Details

Source `Snapshot.Status` describes the last backend refresh; the browser also
detects an HTTP/network outage that never reached that backend. Show transport
status in a banner and track status in rows. Do not let a wall-clock timeout age
ADS-B fields, classify a paused aircraft as lost, or manufacture motion.

At most the current and immediately previous successful aircraft lists are
retained. Tombstones carry identity/last-seen labels only, never an old coordinate
or field value presented as a measurement. Sort deterministically by ICAO while
preserving the selected inspection target where still present.

## Verification

```text
npm run test:unit
npx playwright test ui/browser/aircraft.spec.mjs
go test ./display
task all
```

Advance fixture virtual time to threshold, threshold+1 ns, and field expiry+1 ns.
Advance only browser real time during pause/outage. Assert exact row and status
changes and absence of previous-selection evidence.

## Acceptance Criteria

- Partial targets remain inspectable without an invented position or speed.
- Fresh/stale/lost thresholds and independent field expiry have exact boundaries.
- Pausing or losing transport never advances virtual field age.
- Selection and run changes clear incompatible cached data and history.
- Alternating tabs cannot display one another's selected aircraft as their own.

## Non-Goals

Track extrapolation, dead reckoning, server-side session caches, persistent
historical trajectories, or simulator-truth overlays.
