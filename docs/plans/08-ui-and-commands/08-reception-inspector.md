---
title: "08 - Exact frame evidence and paged reception inspection"
dependencies: ["07-map-and-coverage.md"]
effort: "M"
complexity: "high"
---

## Objective

Let users inspect the exact frames behind received fields and browse bounded
per-station reception history with visible retention gaps.

## Target Artifacts

- New `ui/internal/assets/static/inspector.js` and unit tests.
- Aircraft template, map/table selection integration and styles.
- `ui/browser/inspector.spec.mjs` and cursor/history fixtures.
- UI package and inspector behavior documentation.

## Implementation Tasks

1. Render selected field `Evidence` without reconstructing frames. Position
   inspection shows both CPR frames. List transmission sequence, ICAO, kind,
   virtual timestamp, exact frame text and each receiving station's copy.
2. For each reception show station ID/revision, reception sequence, transmission
   sequence, reception-time station settings, slant range and received power.
   Never replace historical receiver settings with current catalog settings.
   Label generated manager frames separately from received evidence.
3. Add a per-station history selector and explicit load/next/reset actions using
   display `POST receptions/history`. The initial request contains
   `cursor:null`; subsequent requests use the returned `nextCursor` unchanged.
   Honor `hasMore`, show oldest/latest sequence and retention limit, and keep
   sequence comparisons exact above 2^53.
4. Show a persistent gap row whenever `gap:true`, even when no records accompany
   it. Explain that older receptions were evicted; do not classify a gap as a
   transport failure or silently imply contiguous history. If the first retained
   sequence exceeds one, label that earlier run history is outside retention.
5. Bound retained inspection records by configured `maxHistoryRecords`. Trim
   oldest loaded rows explicitly with a visible browser-trimming notice. Dedup
   overlapping rows by run/station/reception sequence without hiding gap markers.
   Keep one active history request and invalidate its generation on selector or
   run change. Stop paging when `hasMore:false`; refresh is an explicit action.
6. Clear cursors and records on replacement run. On cursor conflict, show reset
   required; never silently restart and append a new run's records. Keep a
   removed station's already loaded history clearly historical, reject further
   paging, and require a valid station selection for new requests.
7. Keep the inspector usable with no map/tiles and no valid aircraft position.
   Render all text through safe text nodes. Add an accessible frame-copy action
   with an error state if browser clipboard access fails.

## Technical Details

Request shape is `{stationId, cursor, limit}`. A cursor contains `runId`,
`stationId`, and `afterSequence`, all kept exactly as returned. The same
transmission can appear at multiple receivers; do not collapse those receptions
into one provenance record. Viewer bounds apply to loaded rows, not source
retention, and both kinds of loss must be labeled separately.

## Verification

```text
npm run test:unit
npx playwright test ui/browser/inspector.spec.mjs
go test ./display ./simulator -run 'History|Reception|Cursor|Gap'
task all
```

Fixtures cover both CPR halves, duplicate receiver copies, edited station
provenance, history wraparound, empty pages, gap with no records, overlapping
pages, station deletion, cursor mismatch, >2^53 sequences and restart.

## Acceptance Criteria

- Every displayed/copied frame equals retained evidence exactly.
- Pagination preserves station/run cursor identity and respects `hasMore`.
- Source retention gaps and browser trimming remain visibly distinct.
- Loaded record count never exceeds the explicit browser bound.
- A restart cannot append new-run records to old history.

## Non-Goals

Raw-frame editing, packet transmission, recording export, or replay support.
