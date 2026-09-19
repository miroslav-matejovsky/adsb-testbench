# Progress

Implementation is complete. All 12 steps pass their acceptance gates.

## Completed planning work

| Work | Status | Evidence |
| --- | --- | --- |
| Read backlog 08, 09 and 10 and inspect the current implementation | Complete | Scope and findings in README and assessment |
| Define one ordered implementation plan and acceptance mapping | Complete | Steps 01-12 |
| Move backlog references to the plan | Complete | Backlog, command, tooling and root documentation |

## Implementation status

| Step | Status | Completion evidence |
| --- | --- | --- |
| 01 - URL and server contracts | Complete | `go test ./internal/urlpath ./internal/cli ./internal/httpserver` pass; `task all` pass (959 tests) |
| 02 - Display discovery and isolation | Complete | `go test ./display ./simulator` pass (stations, refresh isolation, local/HTTP parity); `task all` pass (1005 tests) |
| 03 - Assets, components and browser tests | Complete | `go test ./ui/...`, `npm run test:unit` (23), `npx playwright test` (10), `task clean-test` pass; `task all` pass with `clean-test` and `ui-test`. Playwright 1.63.0 locked, Chromium headless shell v1243, Leaflet 1.9.4 checksums verified |
| 04 - Manager controls | Complete | `ui/browser/manager.spec.mjs` (15 tests: zero count, pause/resume, paused virtual time, speed bounds, rejected/unknown-outcome commands, stale poll, late responses, restart review, two tabs) pass, stable over `--repeat-each=4`; `npm run test:unit` pass; `task all` pass |
| 05 - Station editor | Complete | `ui/browser/stations.spec.mjs` (13 tests: create/update/enable/disable/remove, zero and false values, >2^53 revisions, focus during polling, reapply/reload, two-tab replacement/disable/remove, restart, capacity) and `unit/stations.test.mjs` pass, stable over `--repeat-each=3`; `task all` pass |
| 06 - Received aircraft state | Complete | `ui/browser/aircraft.spec.mjs` (11 tests: partial targets, exact thresholds, tombstone, empty selection, late responses, outage freeze, restart, error-envelope run change, removed station, keyboard details, two components) and `unit/tracks.test.mjs` pass; all browser tests stable over `--repeat-each=3`; `task all` pass |
| 07 - Map and coverage | Complete | `ui/browser/map.spec.mjs` (10 tests: positions only, track zero, stale markers, viewport preservation and fit, coverage/reference altitude, date line and polar clipping, zero tile requests, local tiles, blocked tiles, two maps) and `unit/geo.test.mjs` pass; all browser tests stable over `--repeat-each=3`; `task all` pass |
| 08 - Reception inspector | Complete | `ui/browser/inspector.spec.mjs` (11 tests: both CPR halves, duplicate receiver copies, historical provenance, unchanged cursors and hasMore, gaps vs trimming, empty gap page, retention label, >2^53 sequences, cursor conflict and restart, removed station, clipboard success/failure, blocked tiles) and `unit/inspector.test.mjs` pass; suite stable over `--repeat-each=3`; `task all` pass |
| 09 - Composition and public embedding | Complete | `go test ./testbench` (routes per mode, nested prefix and redirects, host-header independence, validation before construction, status, single-use Run, display-only no polling, in-process vs HTTP evidence, independent benches, example) pass; `go -C examples/embedding test ./...` pass (public packages only); `task arch-lint` and `task all` pass |
| 10 - Command configuration and lifecycle | Complete | `go test ./internal/cli ./testbench` (shipped configs parse; missing/null/unknown/duplicate/trailing input; explicit zero/false; oversized file; invalid address/prefix/source; cross-section rules; flags and -help; entropy, station, bind and serve failures; clean cancellation; fresh run IDs with old-run conflict; separate simulator/display) pass; `go build ./cmd/...` pass; `task all` pass |
| 11 - Deployment and browser acceptance | Complete | `deployment.spec.mjs` (4 deployments incl. separate-origin isolation, narrow viewport, accessibility), `embedding.spec.mjs`, `restart.spec.mjs` pass against real handlers; `go test ./testbench` in-process vs HTTP payload and failure parity; `go test ./cmd/...` process smoke tests (prefix, fresh identity, occupied address, invalid input, -help; interrupt test on non-Windows) pass; `task all` pass (81 browser tests). Found and fixed: command did not mount below `publicBasePath`; manager cleared a draft typed while its command was pending; components overflowed at 375 px |
| 12 - Tooling, documentation and completion | Complete | `task build`; `task deadcode` pass (only allowlisted `DecodeLocal`, `distanceNM`); `task all` runs deadcode, `examples` and `ui-test` and passes (1119 Go tests, 46 module tests, 82 browser tests); three example configs run and serve their documented `status`, `manager/` and `aircraft/` paths; local Markdown links resolve; `git diff --check` pass. Found and fixed: browser test races under load (background polls, request timeouts, early handler swaps); fake-driven specs now use a paused clock and a settled `poll` helper based on a new component `aria-busy` flag; full suite stable over `--repeat-each=6` and `--repeat-each=4 --workers=12` |

For each completed step, replace its evidence cell with the actual test commands,
results, and relevant artifacts. Record any remaining implementation work in the
repository-root `.todo` when an implementation session stops incomplete.

## Planning validation

On 2026-09-18, `task all` passed against the existing implementation: 833 tests,
formatting, vet, architecture lint and code lint. Go and lint caches were directed
to writable temporary directories for this restricted workspace session.
Plan structure checks found all 12 steps with the required front matter and
sections. Local Markdown link checks passed, and `git diff --check` passed.
These results validate the planning change and baseline only.
