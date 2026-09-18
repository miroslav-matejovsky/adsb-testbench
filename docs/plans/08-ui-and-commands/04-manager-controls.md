---
title: "04 - Manager traffic, clock, and generated-frame controls"
dependencies: ["03-assets-components-and-browser-tests.md"]
effort: "M"
complexity: "medium"
---

## Objective

Render simulator truth and implement count, speed, pause/resume, and recent
generated-frame controls against the existing simulator API.

## Target Artifacts

- `ui/internal/assets/templates/manager.html`, `static/manager.js`, `ui.css`.
- Shared `static/client.js`, `time.js` and component helpers from step 03.
- `ui/browser/manager.spec.mjs`, browser fixtures and native-module tests.
- `ui/doc.go` and asset documentation.

## Implementation Tasks

1. Fetch `GET metadata` and `GET truth` beneath the configured simulator base.
   Require matching run IDs before enabling mutations. Read published count and
   speed bounds from metadata; display effective settings separately from drafts.
2. Show run ID, current count, configured initial count, exact virtual timestamp,
   elapsed virtual time, speed multiplier and pause status. Separately show the
   browser's last successful real update time. A failed poll labels the retained
   truth stale and disables submissions until a successful refresh.
3. Add integer count input, explicit speed input in hundredths, pause and resume.
   Send absolute `PUT aircraft/count` and `PUT time/speed` commands with the
   observed run ID. Count zero and speed zero are valid. Resume uses the last
   confirmed positive speed, or the explicitly configured resume speed if the
   page has never observed one. Do not round arbitrary input silently.
4. After an acknowledgement, fetch new truth. Do not treat `CommandAck` as a
   state snapshot or optimistically mark a rejected command successful. Keep
   drafts on validation/network failure and show the API code, message and field.
   Discard older poll responses after a command changes the request generation.
5. On detected restart, clear previous truth/history and require review of
   retained drafts before submitting them with the replacement run ID. A timed
   out mutation has an unknown outcome: refresh state and require a new user
   submission, without automatically replaying the command.
6. Render the bounded `TruthSnapshot.History` with sequence, virtual timestamp,
   ICAO, kind and exact 28-character frame. Label this panel "Generated frames"
   and the truth table "Simulator truth". Show retained sequence bounds and
   truncation implied by the oldest retained sequence. Keep reception diagnostics
   visually and semantically separate.
7. Add fixtures/tests for zero count, pause from positive speed, initially paused
   resume, all speed bounds, malformed input, non-2xx errors, concurrent tabs,
   restart and delayed responses. Document accepted controls and time units.

## Technical Details

Count and speed are last-accepted absolute assignments within one run. They do
not use station revisions. Two tabs observe the accepted value on refresh; a
dirty draft remains editable and clearly distinct from that value. Keep actions
pending per control, prevent duplicate submits, and preserve focused fields
during polling. Format multiplier from integer hundredths without float drift.

## Verification

```text
npm run test:unit
npx playwright test ui/browser/manager.spec.mjs
go test ./ui ./simulator
task all
```

Use controlled API responses to assert exact command bodies and run IDs. Advance
virtual snapshots explicitly to prove wall-clock polling does not advance the
displayed virtual time while paused.

## Acceptance Criteria

- Count zero removes every displayed truth row after an acknowledged refresh.
- Pause sends zero, resume sends the documented positive value, and aircraft
  speed labels are not multiplied by simulation time scaling.
- Rejected commands preserve drafts and show actionable errors.
- Real update age and virtual simulation time have separate labels.
- Displayed frame text matches the API byte-for-byte.

## Non-Goals

Station forms, aircraft creation forms beyond count, received-track rendering,
and changes to engine scheduling.
