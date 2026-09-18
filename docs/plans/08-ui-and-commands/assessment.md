# Assessment

## Feasibility and baseline

Assessment date: 2026-09-18. The repository already has a deterministic engine,
revision-checked simulator controls, strict transport types, relative API
handlers, bounded reception history, and equivalent local/HTTP display sources.
`ui`, `ui/internal/assets`, `testbench`, `internal/cli`, and all three command
packages are documented skeletons. No browser implementation or browser test
runner exists. Implementation is feasible without replacing the engine or codec.

The delivery is L overall, with most effort in browser behavior, composition,
and deterministic acceptance fixtures. Numbered step sizes describe relative
implementation effort, not calendar promises. Steps are intentionally one
sequence so commands are integrated with completed reusable components.

## Findings and required fixes

These observations come from inspected code. They distinguish existing defects
from functionality still required by the backlog.

| ID | Evidence and impact | Required change | Verification |
| --- | --- | --- | --- |
| F1 | `display/display.go`, `Refresh`, returns `d.Snapshot()` when admission or request validation fails. That snapshot can be fresh and belong to another station selection. | Step 02 returns unavailable request-scoped state on these early failures, without mutating another request's published state. | Publish A, cancel or invalidate B, and assert B has no A observations and no fresh status. |
| F2 | `display/http.go` exposes only snapshot, observations, and history. `ObservationSource` has no station discovery. A separate display browser cannot obtain station choices or coverage through its backend. | Step 02 adds a separate station source and `GET /stations`, preserving local/HTTP validation parity. | A display-only host lists stations and coverage while browser access to the simulator origin is blocked. |
| F3 | `internal/httpserver/server.go` hard-codes five-second header and shutdown timeouts, and exported helpers do not validate nil dependencies. These cannot honor explicit command settings. | Step 01 adds validated server settings and errors before starting work; step 10 supplies every setting. | Invalid/nil inputs start no server; drain and forced-close paths preserve causes. |
| F4 | `internal/urlpath/source.go` only validates absolute source URLs. There is no prefix, browser API/asset base, or listen-address validator. `validBasePath` rejects `%2F` but does not cover browser backslash normalization. | Step 01 adds browser-specific path rules and regression cases, including literal/encoded backslashes and control characters. | Go and browser vectors preserve a nested prefix or reject it without request dispatch. |
| F5 | `display.Display` intentionally retains only the last successful selection. Multiple tabs can replace that shared fallback. | Steps 03 and 06 keep bounded fallback per component, verify response selection, and avoid using `GET /snapshot` as a tab's data source. | A/B selections remain isolated across alternating success, errors, cancellation and destruction. |
| F6 | `taskfile/clean.ps1` recursively removes every `.exe` outside `.venv`, including possible future browser tooling or third-party files. | Step 03 restricts cleanup to validated, explicit output directories before introducing browser dependencies. | A fixture executable outside those outputs survives; cleanup never traverses Go `vendor`, `node_modules`, or browser caches. |
| F7 | Root/backlog documentation links to absent `docs/plans/06-simulator-api-and-display`; command and tooling docs point at backlog files being promoted. | Correct the documentation links during this planning change. Step 12 updates feature status only after implementation. | Local Markdown links resolve; no live link targets a removed backlog item. |
| F8 | `simulation.Config.ID` requires a fresh identity per engine lifetime, but replaying one future example configuration verbatim would reuse it. | Step 10 treats the command's configured ID as a label and appends a fresh random suffix before constructing the runtime; document effective identity. | Two launches from identical configuration have different run IDs; old commands/cursors conflict against the new run. |

## Risks and constraints

- A restart can happen between station discovery and observation/history calls.
  Treat independently fetched payloads as separate instants. Compare run IDs;
  clear previous-run data and require a coherent refresh instead of merging runs.
- Transport status and aircraft age are different. Outages freeze the last
  virtual instant and mark the view stale. Only a successful new virtual snapshot
  can move a track across fresh/stale/lost thresholds.
- A failed response can contain valid stale data. Decode `RefreshResponse` even
  on non-2xx status, retain the error, and validate selection and run identity
  before showing observations.
- Browser timestamps lose submillisecond precision and can mishandle years
  below 100. Step 03 provides tested RFC3339Nano-to-BigInt arithmetic covering
  the complete source year range. Wall-clock formatting may be rounded; expiry
  and threshold comparisons may not be.
- Lost targets and history pages can grow if retained indefinitely. Step 06
  retains only the current successful snapshot and one bounded previous view;
  step 08 retains a configured maximum number of records and displays trimming.
- Tiles can fail independently of observations. The table, selector and inspector
  remain usable; tile errors never invalidate received data.
- Enabling deadcode will report public library entry points used only by hosts.
  Step 12 covers them through external-consumer analysis, and allows only named,
  justified library APIs in the command-only check. Do not delete useful public
  embedding APIs to satisfy command reachability or allowlist whole packages.
- Local network listeners and browser installation require available development
  tools. Install exact test dependencies in setup, never implicitly during tests.

## Dependencies and validation strategy

Use the Go version and tools already declared by the repository. Browser tests use
an exact locked `@playwright/test` release, a declared Node version, and installed
Chromium. Pin versions and record them when step 03 creates the lockfile; no
floating version is accepted in the committed manifest. Native modules have no
production build pipeline.

Go tests use `require`, fake clocks, channels, and explicit engine advancement.
Browser tests use controlled routes, response barriers, and the
[Playwright clock](https://playwright.dev/docs/clock) for polling. Start the fixture
server using [Playwright webServer](https://playwright.dev/docs/test-webserver).
No sleeps or `waitForTimeout` calls establish correctness. Real-process smoke
tests wait for an explicit ready endpoint and terminate through controlled
cancellation. Keep deterministic unit tests separate from OS signal smoke tests.

Run each step's targeted commands, then `task all` before marking it complete.
The final gate includes Go tests, formatting, vet, architecture lint, code lint,
command reachability, browser tests, and external consumer compilation. Record
actual commands and outcomes in progress; passing this documentation change's
baseline checks is not evidence that planned functionality exists.

## Rollback and delivery boundaries

There is no database or persisted-state migration. Implement in numbered slices
and keep each slice's tests passing. If a slice fails its acceptance gates,
retain the previous working behavior and keep that step pending. Remove only
that slice's unshipped implementation when reversing it; never edit Go `vendor`.
Hosts can continue using existing relative API handlers while UI and commands
are built. API signature changes update all in-repository consumers and public
examples together; compatibility shims are unnecessary at this stage.

The planning change creates documentation and removes promoted backlog files.
It makes no application, Go dependency, configuration, or tooling changes.
