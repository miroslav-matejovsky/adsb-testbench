---
title: "06 - Public examples, documentation, and implementation acceptance"
dependencies: ["05-acceptance-and-failure-tests.md"]
effort: "M"
complexity: "medium"
---

# 06 - Public examples, documentation, and implementation acceptance

## Objective

Publish the station and reception contract honestly and close backlog 03 only
after the implementation checks and external consumption succeed.

## Target Artifacts

- Update `simulation/doc.go` and root `README.md`.
- Extend `simulation/example_test.go` in package `simulation_test`.
- Update `docs/backlog/README.md` and `docs/backlog/04-observation-history.md`,
  and remove `docs/backlog/03-receiving-stations.md` when complete.
- Update this plan's `README.md` status and `progress.md`.
- Update the plan index row in `docs/plans/README.md`.

## Implementation Tasks

1. Extend `simulation/doc.go` with the implemented station configuration,
   commands, revision rules, identifier rules, reception model, published
   parameters, coverage, batch contract, ownership, and error categories. List
   every bound and unit. Keep the material self contained.

2. State plainly in the documentation that the reception model is synthetic:
   coverage radii, slant ranges, and received powers are model output, not
   calibrated RF predictions, and pressure altitude is used directly as
   geometric height above the model sphere with no propagation delay, terrain,
   antenna pattern, or interference modelled.

3. Add deterministic public examples for station creation, update and
   disablement, a revision conflict, a reception batch, published model
   parameters, and coverage at an explicit reference altitude. Assign every
   configuration field explicitly, use a supplied UTC start time and a fixed
   seed, and print stable values.

4. Update the root overview to record that the engine now owns stations and
   reception decisions, and that retained per-station history, observed
   aircraft, the runtime, the HTTP contract, and the UI remain planned. State
   that reception and coverage are synthetic model output.

5. Compile and run a temporary external consumer module outside the repository
   that imports only the public `simulation` package with a local `replace`
   directive for `github.com/miroslav-matejovsky/adsb-testbench`. It must
   construct an engine, add a station, advance, read receptions and frame
   bytes, read `Model()`, and call `EstimateCoverage`, using only
   standard-library types. Do not add a permanent consumer task, a dependency
   update, or a new command package for this check.

6. Run the verification commands below. Fix failures within implementation
   scope. Record any genuine outstanding blocker in `.todo` instead of
   declaring completion.

7. Once all checks pass, remove the completed backlog 03 item, drop it from the
   backlog index and the top-five table, promote the next items without
   changing their feature scope, and change
   `docs/backlog/04-observation-history.md` to depend on the implemented
   `simulation` package documentation instead of the removed item, matching how
   items 03 and 05 were updated when backlog 02 closed.

8. Record completed status and actual validation results in this plan's
   `README.md` and `progress.md`, including any deviation from the plan. Do not
   mark an incomplete step complete because its documentation exists.

## Technical Details

Public examples must not import `internal/adsb` or `go-adsb`. They may show
raw frame length and hexadecimal, reception counts, station identifiers and
revisions, coverage radii, and model parameters through public API only.
Coverage and model values printed in an example are fixed by the fixed
constants, so their expected output is stable.

The documentation must keep the distinctions explicit: a transmission is what
an aircraft emitted, a reception is what one station heard, the transmission
history is bounded and shared, and receptions in this increment are returned
but not retained. It must state that a station receives only transmissions
emitted at or after its creation instant, and that `New` therefore produces no
receptions.

Keep the existing `task all` deadcode exclusion while the `cmd` packages have
no entry points. Do not claim executable application commands are available.
No vendor files change and no commits are created.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go doc -all ./simulation`
- `go test ./simulation ./internal/adsb`
- `go test -race ./simulation`
- Compile and run the temporary public consumer with `go test ./...` from that
  temporary module.
- `task all` after the implementation and documentation are complete.
- Check Markdown links and `git diff --check`; inspect changes for unrelated
  files.

## Acceptance Criteria

- Public examples compile, run deterministically, and assign every
  configuration field explicitly.
- An external module uses stations, receptions, `Model`, and
  `EstimateCoverage` without importing internal packages.
- `go doc -all ./simulation` shows every station field, bound, unit, error
  category, and determinism rule, and labels coverage, slant range, and
  received power as synthetic model output.
- Root and package documentation match actual behavior and feature status.
- `task all` passes format, vet, architecture, lint, and test checks.
- `.go-arch-lint.yml` is unchanged and `simulation` still depends only on
  `adsb`.
- Backlog links remain valid after completion and only genuinely open work
  remains listed.
- This plan's status and `progress.md` record the executed checks and any
  deviation.

## Non-Goals

Implementing or launching commands, adding UI or server features, retained
reception history, modifying vendor, committing changes, and running
implementation checks during the present documentation-only task.
