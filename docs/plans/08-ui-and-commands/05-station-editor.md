---
title: "05 - Station forms, revision conflicts, and draft preservation"
dependencies: ["04-manager-controls.md"]
effort: "M"
complexity: "high"
---

## Objective

Create, replace, enable/disable, and remove fully configured stations while
preserving unsaved work across polls, errors, and concurrent edits.

## Target Artifacts

- New `ui/internal/assets/static/stations.js` and corresponding unit tests.
- Manager template/module/styles from step 04.
- `ui/browser/stations.spec.mjs` and fixture station scenarios.
- UI package and asset documentation.

## Implementation Tasks

1. Render `GET stations` alongside manager metadata/truth without mixing runs.
   Build a complete station form for ID, enabled, latitude/longitude, site
   elevation, antenna height/gain, sensitivity, system loss and frame-loss
   probability. Display units and accepted domains from the existing contract.
   A new form has empty required numeric inputs, not hidden application defaults.
2. Store each draft separately from the latest server station, including the
   run ID and revision when editing began. Refresh clean forms only. If a dirty
   form's server revision changes, show both server changes and the local draft.
   Preserve focus and text even while other stations update.
3. Send complete create/replace commands using existing routes. Updates,
   enable/disable actions and deletion use the captured `expectedRevision`
   string; encode station IDs as exactly one route segment. Keep existing IDs
   immutable and show the returned revision only after acknowledgement.
4. On 409, preserve the original draft and fetch current station state. Provide
   explicit "Reload server values" and "Reapply draft to current revision"
   actions. Reapply first presents the current-versus-draft fields and requires
   a new submit. Never silently substitute the latest revision and resend.
5. Keep drafts on ordinary failure and distinguish station deletion, invalid ID,
   capacity, stale revision and changed run. A station removed by another tab
   cannot be recreated under its old ID in the same run. Require a new ID for a
   new station; display that engine rule next to the rejected creation.
6. Validate empty, non-finite and out-of-range inputs before sending, preserving
   valid zero and false values. Keep server-side validation authoritative and
   render field-path errors. Display reference-altitude coverage values as
   simulator reception diagnostics, not measured radio coverage.
7. Add two-tab tests for simultaneous replacement, disable versus edit, remove
   versus edit, polling while typing and restart with dirty forms. Add unit tests
   covering every station field and exact revision values above 2^53.

## Technical Details

`AddStationCommand` contains `runId` and complete `station`. Replacement adds
`expectedRevision`; removal contains `runId` and `expectedRevision` in its JSON
body. Respect HTTP 201 creation versus HTTP 200 mutation acknowledgements.
Disabled stations keep complete settings and can be re-enabled by a normal
revision-checked replacement. Do not introduce PATCH or an independent toggle API.

## Verification

```text
npm run test:unit
npx playwright test ui/browser/stations.spec.mjs
go test ./simulator -run 'Station|Conflict'
task all
```

## Acceptance Criteria

- Every station setting can be submitted and round-tripped with its unit.
- Conflicts never overwrite a dirty draft or trigger an automatic retry.
- Two tabs produce exactly one accepted update from the same revision.
- Zero probability/height/loss and disabled false values remain explicit.
- Removed-station and replacement-run drafts cannot mutate the wrong state.

## Non-Goals

Geographical station dragging, bulk import, partial setting updates, or changes
to the reception model.
