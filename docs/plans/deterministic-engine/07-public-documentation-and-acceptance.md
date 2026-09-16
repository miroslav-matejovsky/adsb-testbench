---
title: "07 - Public examples, documentation, and implementation acceptance"
dependencies: ["06-determinism-and-failure-tests.md"]
effort: "M"
complexity: "medium"
---

# 07 - Public examples, documentation, and implementation acceptance

## Objective

Publish a usable engine contract and close backlog 02 only after implementation checks and external consumption succeed.

## Target Artifacts

- Update `simulation/doc.go` and root `README.md`.
- Create `simulation/example_test.go` in package simulation_test.
- Update `docs/backlog/README.md` and dependent backlog 03/05 links when backlog 02 is completed.
- Update this plan's README status to implemented only after all acceptance checks pass.

## Implementation Tasks

1. Replace simulation's skeleton documentation with the implemented configuration, API, time, motion, schedule, ownership, history, and error contracts. Keep the material self-contained and list every bound and unit.

2. Add deterministic public examples for construction, initial history, returned report batches, exact virtual stepping, real-time scaling, pause/resume, count down/up, and history-gap detection. Assign every configuration field explicitly.

3. Update the root overview and architecture to distinguish the implemented codec/engine from the still-planned simulator, display, UI, stations, and runtime. State that motion is synthetic and altitude is pressure altitude.

4. Compile a temporary external consumer module that imports only the public simulation package using a local replace directive. Construct and advance an engine and inspect frame bytes using standard-library types.

5. Run the future implementation verification commands below. Fix failures within implementation scope; record any genuine outstanding blocker in .todo instead of declaring completion.

6. Once all checks pass, remove the completed backlog 02 item and its index entries, update backlog 03/05 prerequisites to the implemented package documentation, and adjust next-work ranks without changing their feature scope.

7. Record completed implementation status and actual validation results in the plan overview. Do not mark incomplete steps complete merely because their documentation exists.

## Technical Details

Public examples must avoid imports from internal/adsb and go-adsb. They may
show raw frame length/hex, counts, timestamps, sequence order, and pause effects
through public Engine methods and snapshots. Examples use a supplied UTC time
and a fixed seed, not wall time or a hidden demo configuration.

The external consumer check should use a temporary directory outside the
repository and the local module path
github.com/miroslav-matejovsky/adsb-testbench in its replace mapping.
It verifies that the public API is usable without accessing internal types.
Do not add a permanent consumer task, dependency update, or new command package
solely for this check; broader public-consumer tooling belongs to backlog 10/11.

Documentation must explain that New retains all initial creation reports
because 3*MaxAircraft is less than HistoryLimit, while later Advance/Elapse
batches may exceed retention. It must describe the distinct meanings of
historical frame timestamps, current snapshot time, true ground track,
pressure altitude, and externally supplied real durations.

Keep task all's existing command-reachability exclusion while cmd packages
have no entry points. Do not claim executable application commands are available.
No vendor files are changed and no commits are created.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go doc -all ./simulation`
- `go test ./simulation ./internal/adsb`
- Compile and run the temporary public consumer with `go test ./...` from that temporary module.
- `task all` after the implementation and documentation are complete. This is a future acceptance command, explicitly not part of the current plan-only request.
- Check Markdown links and `git diff --check`; inspect changes for unrelated files.

## Acceptance Criteria

- Public examples compile, run deterministically, and document all required configuration.
- An external module can use the engine without importing internal packages.
- Root and package documentation match actual behavior and feature status.
- The future implementation's task all passes format, vet, architecture, lint, and test checks.
- Backlog links remain valid after completion, and only genuinely open work remains listed.
- No code implementation or check execution is performed as part of preparing this plan.

## Non-Goals

Implementing or launching commands, adding UI/server features, modifying vendor, committing changes, and running implementation checks during the present documentation-only task.
