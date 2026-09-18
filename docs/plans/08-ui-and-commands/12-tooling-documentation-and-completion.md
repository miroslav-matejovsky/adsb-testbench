---
title: "12 - Runnable tasks, documentation, and completion checks"
dependencies: ["11-deployment-and-browser-acceptance.md"]
effort: "M"
complexity: "medium"
---

## Objective

Make the implemented workflow reproducible, enforce command/browser checks, and
replace planned-feature documentation with tested usage and embedding examples.

## Target Artifacts

- `Taskfile.yml`, `taskfile/deadcode.ps1`, test scripts and `taskfile/README.md`.
- Root `README.md`, `cmd/README.md`, `configs/README.md`, browser/embedding docs.
- `ui/doc.go`, `ui/internal/assets/doc.go`, `testbench/doc.go`, command `doc.go`
  and updated display/HTTP/URL package documentation.
- This plan's `progress.md`, `docs/plans/README.md`, remaining backlog links.

## Implementation Tasks

1. Add `task build`, `task run-combined`, `task run-simulator`, and
   `task run-display`. Run tasks require explicit `CONFIG` and
   `CONFIG_MAX_BYTES` variables rather than choosing hidden example files or
   values. Document concrete invocations against each shipped example config.
2. Enable `task deadcode` in `task all` now that command entry points exist.
   Resolve real unreachable internal functions. For intentional exported host APIs
   that command roots do not call, use only individually named, explained entries
   backed by executable external-consumer examples. Do not blanket-allow packages.
3. Include native-module tests, browser acceptance, cleanup regression checks and
   external-module compilation in `task all`. Keep explicit setup separate:
   `npm ci` and installed Chromium are prerequisites, not test-time downloads.
   Make missing prerequisites fail with their setup command and preserve failing
   browser traces/results under `.test-results`.
4. Update root architecture/development docs for actual combined versus separate
   deployment, route mounts, both clocks, source data flow, map/tiles, inspector
   retention, explicit settings, effective run IDs and shutdown behavior. Link
   configuration examples, public APIs, external embedding and bundled notices.
5. Replace skeleton package docs with implemented responsibilities and exported
   API contracts. Keep JavaScript/folder documentation close to its assets/tests.
   Include public error/lifecycle expectations and component destroy ownership.
6. Validate all local Markdown links and remove stale backlog/absent-plan links.
   Keep backlog 11 focused on broader capacity measurements and backlog 12 on
   extensions. Do not claim this plan delivers their remaining scope.
7. Run final commands, record actual results per step in progress, and mark all
   implementation steps complete only when every acceptance gate passes. If any
   step is incomplete, leave it pending and record remaining implementation work
   in the root `.todo`. Never create a commit.

## Technical Details

The final `task all` keeps existing tidy, vet, format, architecture lint, code
lint and Go tests, and adds the new checks once their prerequisites exist.
Commands must propagate child failures, including PowerShell pipelines. The
safe cleanup policy from step 03 must preserve browser/dependency caches and
all third-party source. Go `vendor` stays read-only throughout this work.

Document example tasks with full variable values, for example:

```text
task run-combined CONFIG=configs/combined.json CONFIG_MAX_BYTES=65536
task run-simulator CONFIG=configs/simulator.json CONFIG_MAX_BYTES=65536
task run-display CONFIG=configs/display.json CONFIG_MAX_BYTES=65536
```

These invocations are explicit examples, not Task defaults. Run tasks forward
arguments without shell interpolation of configuration-file contents.

## Verification

```text
task build
task deadcode
go -C examples/embedding test ./...
npm run test:unit
task ui-test
task all
git diff --check
```

Run the three documented examples and verify their configured status/page paths.
Check that every step's required files/tests exist and its recorded result is
from the final implementation. Review changes for accidental modifications to
Go `vendor`, generated browser downloads or dependency caches.

## Acceptance Criteria

- `task all` passes with command reachability and browser checks enabled.
- All three run tasks require and honor explicit configuration inputs.
- External public examples compile and their lifecycle examples run.
- No skeleton or active backlog text claims delivered features are still absent.
- Every numbered step is complete with verification evidence, or the overall
  implementation remains explicitly incomplete with work recorded in `.todo`.

## Non-Goals

Committing changes, deployment/publishing, benchmark capacity claims, replay,
new transport outputs or automatic dependency upgrades.
