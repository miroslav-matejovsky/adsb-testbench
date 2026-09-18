---
title: "11 - Deployment, restart, and embedding acceptance"
dependencies: ["10-command-configuration-and-lifecycle.md"]
effort: "L"
complexity: "high"
---

## Objective

Prove the complete browser experience works in combined, separate, prefixed, and
embedded deployments, including failures and repeated component lifecycles.

## Target Artifacts

- New `ui/browser/deployment.spec.mjs`, `embedding.spec.mjs`, `restart.spec.mjs`.
- `ui/testdata/browserhost/` fixtures and `playwright.config.mjs` projects.
- `testbench/acceptance_test.go`, command process smoke tests.
- `examples/embedding/` tests and browser fixture documentation.

## Implementation Tasks

1. Run the same manager/display assertions against combined mode and separate
   simulator/display mode, both at root and nested prefixes. Separate-mode browser
   requests may contact only their own app backend and configured local tiles.
   Record requests and fail if the display browser contacts the simulator origin.
2. Compare deterministic received payloads from the same paused source through
   in-process and HTTP adapters. Ignore only explicitly real update timestamps;
   compare frames, provenance, null availability, virtual age and failure codes.
   Reuse existing acceptance fixtures instead of adding a second decoder.
3. Exercise the original acceptance matrix: pause/resume, zero count, station
   creation/edit/removal, concurrent revisions, partial targets, missed position
   reports, field expiry, stale/lost tracks, selection changes, history gaps,
   source outage, malformed replacement-run response and source restart.
4. Serve two independent benches under `/bench/a/` and `/bench/b/`, plus two
   display components with different selections against one backend. Change
   controls and selected stations independently. Assert no DOM IDs, global
   handlers, network requests, drafts, revisions or fallback cross instances.
5. Mount and destroy each component repeatedly while requests are in flight.
   Resolve delayed responses after destroy, advance the test clock through more
   poll periods, and assert no requests/DOM mutations occur. Remount into the same
   root and verify one polling loop and one functioning map, with no old listeners.
6. Block tiles and verify table navigation, station selection, reception paging
   and copyable frame text still work. Check labels, keyboard activation, focus
   preservation, error announcements and rendering at narrow viewport widths.
7. Add real-process smoke tests for all three commands, occupied listen address,
   invalid configuration, configured prefix, fresh startup identity and shutdown.
   Wait for the explicit status response, never a fixed delay. Send supported
   process signals in platform-specific smoke tests and keep cancellation unit
   tests portable. Ensure subprocesses/listeners close on assertion failure.
8. Document fixture controls and trust boundaries. A test-only fixture may expose
   deterministic scenario actions, but production handlers must not expose
   time-advance, fault-injection or process-exit endpoints.

## Technical Details

Browser failure scenarios use route barriers and explicit fixture response
sequences, not wall-clock-sensitive RF simulations. Integration tests exercise
real API and UI handlers; intercepted payload tests isolate presentation edge
cases. Both are required, and their test names identify which layer they cover.

Bind test listeners to loopback ephemeral ports, publish actual addresses to the
harness, and supervise them with cleanup hooks. Report browser console exceptions
and unhandled promise rejections as failures. Use Playwright clock control only
for browser timers; advance source virtual timestamps separately.

## Verification

```text
go test ./testbench ./cmd/... ./simulator ./display
go -C examples/embedding test ./...
task ui-test
task all
```

Run the focused mount/destroy test with a declared repeated cycle count and assert
the same bounded pending-request/listener counts after each cycle. Use test
instrumentation, not subjective visual or heap-size judgments.

## Acceptance Criteria

- The requirement matrix in the plan README has a named automated test for every
  row, passing against the applicable deployment modes.
- Separate display browsers send no request to the upstream simulator origin.
- Restart clears old tracks/cursors; stale mutation run IDs are rejected.
- Both independent benches and different selections on one backend remain isolated.
- Destroyed components produce zero further polling requests or DOM updates.
- All command processes terminate with their expected status and no leaked test
  listeners/processes remain.

## Non-Goals

Load benchmarking, production monitoring, broader ADS-B protocol coverage,
cross-browser support beyond the declared Chromium test target, or visual redesign.
