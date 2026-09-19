---
title: "03 - Reject invalid station commands before settling time"
dependencies: ["02-station-state-bounds.md"]
effort: "M"
complexity: "medium"
---

## Objective

Fix Q3 and ensure the new lifetime and revision checks also happen before the
runtime advances time. The engine remains the owner of all station rules.

## Target Artifacts

- `simulation/engine.go`, `registry.go`, new `simulation/station_validation_test.go`.
- `internal/simdriver/driver.go`, `driver_test.go`, `doc.go`.
- `simulator/api_test.go`, `acceptance_test.go`.

## Implementation Tasks

1. Add documented read-only methods
   `Engine.ValidateStationAddition(ctx context.Context, cfg StationConfig) error`
   and `Engine.ValidateStationUpdate(ctx context.Context, expectedRevision uint64,
   cfg StationConfig) error`. They lock the engine, check context and input, and
   call the same private registry checks as mutations. They copy no histories,
   emit no frames and reserve nothing.
2. Document that a successful validation does not reserve a future mutation.
   The actual engine command rechecks everything. Runtime correctness relies on
   the existing driver mutex and its sole-mutator contract, not on a cross-call
   transaction promised to arbitrary engine users.
3. In `Driver.AddStation`, under `d.mu`, check stopped state and call the new
   addition validator before `settleLocked`. Remove its incomplete active-ID
   scan through `Snapshot`. After settlement, retain the actual `AddStation`
   call and its error wrapping.
4. In `Driver.UpdateStation`, use the update validator before settlement. It
   must include unknown ID, mismatched revision and exhausted revision. Keep
   removal's distinct precheck: a maximum revision is legal for removal.
5. Add `TestDriverRejectedStationAdditionDoesNotSettle`. Use the existing
   fake clock: add/remove `primary` at time zero, move the clock five seconds,
   try the removed ID, assert `ErrInvalid`, unchanged elapsed time and histories.
   Then issue a valid command and compare its resulting state/output to a
   control run whose clock also advanced five seconds. This detects both early
   settlement and accidental loss of pending time.
6. Cover an active duplicate and active capacity similarly. For lifetime
   exhaustion, execute 1024 add/remove cycles with no pending time, then advance
   the fake clock and try a new ID. Assert `ErrLimit` without settlement.
   Test pre-canceled contexts and verify they consume neither time nor identity.
7. Test exhausted revision validation directly in same-package simulation
   tests with arranged boundary state. The driver must call that exact validator;
   do not add a production API allowing callers to set revision numbers merely
   to reproduce overflow through a black-box test.
8. Extend simulator acceptance tests to issue rejected removed-ID additions
   through direct API calls and the mounted HTTP handler on equivalent fresh
   fixtures. Assert the same category/run ID and unchanged truth and raw
   reception snapshots. Use `errors.Is` locally and the existing HTTP error
   envelope expectations remotely.

## Technical Details

Error precedence is deterministic: stopped driver, canceled context, invalid
configuration, registry eligibility, settlement, then mutation. Preserve
existing error wrapping with `%w`. A failure after successful settlement can
still occur for genuine runtime failures; the guarantee here is that known
invalid commands are rejected before settlement.

Use the existing `fakeClock`, `runtimeClock`, and acceptance mounts. Do not
start the real ticker to advance these tests. Do not use sleeps or compare
`time.Now()` against a tolerance. A full visible snapshot comparison is
necessary but insufficient; the valid follow-up comparison covers hidden RNG,
ordinal, sequence and pending-time state.

## Verification

```text
go test ./simulation ./internal/simdriver ./simulator -count=1
task arch-lint
```

Run the new regression against the original driver once while developing it;
it must expose the five-second advancement described in Q3. Restore the fix
and require it to pass. Do not leave a deliberate failure in the workspace.

## Acceptance Criteria

- Reuse of removed IDs, active/lifetime capacity violations and invalid updates
  return before advancing the runtime's virtual time.
- Pending valid time is applied exactly once by the next successful command.
- Validation and mutation use one rule implementation; production package
  dependency directions remain unchanged.
- Direct and HTTP command regressions pass without timing dependencies.

## Non-Goals

No new public clock injection, broad driver abstraction, scheduler change,
retry behavior, or relaxation of the sole-mutator rule.
