---
title: "07 - Aircraft map and reference-altitude coverage"
dependencies: ["06-received-aircraft-state.md"]
effort: "M"
complexity: "medium"
---

## Objective

Map received positions and station coverage without disrupting the observation
table or resetting the user's viewport on every poll.

## Target Artifacts

- New `ui/internal/assets/static/map.js` and map geometry unit tests.
- Pinned Leaflet bundle, notices, display template and scoped styles.
- `ui/browser/map.spec.mjs`, tile-success/error fixtures.
- UI asset and public configuration documentation.

## Implementation Tasks

1. Create one Leaflet map per mounted display from explicit center/zoom settings.
   Add tile layer only when the complete `tiles` object is supplied. Load all
   renderer JS/CSS/icons from the configured embedded asset base.
2. Key aircraft markers by run ID and ICAO, update their positions without
   recreating the map, and remove markers when received position becomes null or
   track is lost. Show stale-but-positioned aircraft with a distinct label/style.
   During transport outage freeze markers and show the stale-view banner.
3. Render track direction only when a valid received ground track exists. Link
   marker selection to table/inspector selection. Show altitude, speed, vertical
   rate, field age and provenance in an accessible details region shared with
   keyboard table navigation; do not require pointer hover.
4. Render station coordinates from discovery, distinguish disabled stations, and
   draw each selected enabled station's effective coverage radius. Convert
   nautical miles to metres exactly with factor 1852. Label reference pressure
   altitude, effective/horizon/link-budget radii and synthetic-model status.
5. Keep center and zoom unchanged across polls, selection changes, station edits
   and outage recovery. Add an explicit "Fit received aircraft" action; do not
   auto-fit empty data or fit on every response. Normalize date-line positions
   consistently. Web Mercator limits may clip polar points visually, so retain
   exact coordinates and a clipping indication in the table/details.
6. Catch tile errors separately, show a tile-unavailable message, and retain
   table/selector/inspector operation. Fully disabled tiles make zero tile
   requests. Remove event handlers/layers/map during component destruction.
7. Add browser tests for pan/zoom preservation, missing positions, track zero,
   disabled stations, coverage reference altitude changes, blocked tiles,
   dateline/polar fixtures and mounting two maps in one document.

## Technical Details

Coverage uses the server's published surface radius; the UI does not recompute
radio propagation. A Leaflet geographic circle is a visualization, and should
not be described as a navigation or RF guarantee. Renderer bounds must not
change the source coordinates or feed into decoder calculations.

No public tile provider is an implicit default. Example configs use `tiles:null`
and document the full object needed to enable a host-approved provider. Keep
configured attribution visible whenever tiles are enabled. Browser tests use
local deterministic tiles and block all unrelated network requests.

## Verification

```text
npm run test:unit
npx playwright test ui/browser/map.spec.mjs
go test ./ui ./ui/internal/assets
task all
```

## Acceptance Criteria

- Only valid received positions create aircraft markers.
- Viewport values remain equal before and after polling until a user action.
- Coverage labels include reference altitude and the synthetic-model qualifier.
- Observation tables and frame inspection work with every tile request blocked.
- Destroying one of two maps leaves the other map operational.

## Non-Goals

Offline tile download, geographic search, flight routes, terrain modeling,
calibrated propagation, or true-altitude conversion.
