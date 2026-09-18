---
title: "09 - Documentation, examples, and final checks"
dependencies: ["08-cross-transport-acceptance.md"]
effort: "M"
complexity: "medium"
---

# 09 - Documentation, examples, and final checks

## Objective

Document the implemented APIs and mark this combined plan complete only after
all deliverables and repository checks pass.

## Target Artifacts

- Root `README.md`.
- `simulation/doc.go`, `simulator/doc.go`, `simulatorapi/doc.go`, `display/doc.go`.
- `internal/urlpath/doc.go` and public API comments.
- New `simulator/example_test.go` and `display/example_test.go`.
- `docs/plans/README.md`, this plan's `progress.md`, and dependent backlog docs.

## Implementation Tasks

1. Update root architecture text to describe implemented HTTP/local observation
   sources, raw reception capture, and received decoding. Keep commands and UI
   labeled as planned. Document the handler mount relationship and host-owned
   runtime/server lifecycle without adding executable commands.
2. Document public APIs, wire units, exact integer/time representations,
   nullability, required input keys, error/status mapping, explicit settings,
   body bounds, coverage reference altitude, and pagination semantics in package
   docs. Include the current decoded/raw observation distinction and provenance.
3. Document display rebuild/retention behavior, global-only CPR, independent
   virtual-time expiry, source outage status, real update time, selection cache
   limits, and fresh run-ID obligation. State which failures retain same-run
   data and which invalidate it, including failed new-run decoding.
4. Add external-package compileable examples for constructing a configured
   simulator API, local display source, HTTP source, and prefix-mounted handlers.
   Show all required settings/client/error callbacks explicitly. Example tests
   use local `httptest` only and must not contact a live simulator or internet.
5. Verify exports expose no internal codec/driver types and display has no
   production simulator/engine import. Review current downstream backlog
   dependencies: item 08 needs the completed API, items 09/10 need display.
   Retain links to this plan or implemented package docs as appropriate.
6. Check every acceptance criterion in steps 01-08 and record its evidence.
   Run the commands below, inspect failures, and fix them before marking the
   corresponding step complete. Inspect final diff for accidental vendor edits
   or unrelated changes and preserve existing user work.
7. Mark all nine steps Complete and move the plan index entry to Completed
   only after checks pass and documentation matches the code. If unfinished,
   leave the step Pending/In progress and write exact remaining work to root
   `.todo`. Do not recreate the removed backlog items or commit changes.

## Technical Details

Go packages use `doc.go`, not redundant README files. Fixture directories use
README for attribution and contents. Examples are executable tests or compile
checks with explicit construction, not undocumented default configuration.

## Verification

```text
go doc -all ./simulatorapi
go doc -all ./simulator
go doc -all ./display
go test -race ./simulation ./simulator ./simulatorapi ./display
git diff --check
task all
```

Run final race checks once after all behavior changes. If step 08 ran the same
checks against unchanged code, record that result rather than repeating it.
`task all` is mandatory after final documentation/example changes.

## Acceptance Criteria

- Exported APIs and examples match implemented behavior and explicit settings.
- All original requirements map to passing tests in steps 01-08.
- Documentation links resolve and no deleted backlog item is referenced.
- `task all` passes format, vet, architecture lint, code lint, and tests.
- Progress/index reflect actual implementation status; no commit is created.

## Non-Goals

Implementing downstream UI/commands, publishing a release, or changing capacity.
