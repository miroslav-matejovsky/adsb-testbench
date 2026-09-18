# Progress

All nine steps are implemented and accepted. Backlog items 06 and 07 were
moved into this plan and are now delivered as software.

| Step | Status | Completed work | Pending work / completion evidence |
| --- | --- | --- | --- |
| Plan preparation | Complete | Scope, architecture, contracts, nine steps, risk assessment, dependency migration | None |
| 01 - Contracts and explicit configuration | Complete | `simulatorapi` wire helpers, strict JSON value tree, `APIError`/`Category`, raw snapshot/control/metadata DTOs, `simulator.ParseConfig`/`ParseAPIConfig`, `display.ParseConfig`/`ParseHTTPSourceConfig`, `urlpath.ParseSourceBase` | `go test ./simulatorapi ./simulator ./display ./internal/urlpath` pass; `go vet` pass; `go-arch-lint check` OK |
| 02 - Coherent reception snapshot | Complete | `Engine.ReceptionSnapshot`, shared `captureStations`/`normalizeStationSelection` reused by `Engine.Observations`, `Simulator.ReceptionSnapshot`, `MaxSnapshotReceptions`, doc update | `go test ./simulation ./simulator` and `go test -race ./simulation ./simulator` pass; ordering, retention, provenance, detachment, cancellation, concurrency tests added |
| 03 - Simulator service | Complete | `simulator.API` with `NewAPI`, `Metadata`/`Truth`/`Stations`/`Observations`/`ReceptionSnapshot`/`ReceptionHistory`, run-guarded `SetCount`/`SetSpeed`/`Add`/`Update`/`RemoveStation`, named converters, category mapping preserving causes | `go test ./simulator` and `go test -race ./simulator` pass; run-conflict, revision, zero-control, stopped-runtime, cancellation and detachment tests added |
| 04 - Simulator HTTP | Complete | `API.Handler` with all eleven routes, strict bounded request parsing, media-type/method/query checks, bounded response encoding, shared error envelope and status mapping, host error callback | `go test ./simulator` and `go test -race ./simulator` pass; prefix-mounted route matrix, method/Allow, media type, byte bound, malformed-body, response-limit, writer-failure and deadline tests added |
| 05 - Display validation and decoding | Complete | `display.SourceError`, semantic request/snapshot/page validators with provenance and retention checks, raw accumulation with global CPR and independent field expiry, `display/testdata/README.md` | `go test ./display ./internal/adsb` pass; published-frame, lone-half, ten-second bound, expiry boundary, unavailable-replacement, over-range, multi-receiver and corrupt-frame fixtures added |
| 06 - Local and HTTP sources | Complete | `display.ObservationSource`, `NewInProcessSource`, `NewHTTPSource` with owned client copy, refused redirects, bounded request/response bytes, strict response parsers, remote error recreation; `urlpath.ParseSourceBase` used for mount prefixes | `go test ./display ./internal/urlpath` and `go test -race` pass; equivalence, malformed-response, error-envelope, dishonest-length, body-closure, truncated-read, redirect, cancellation/deadline and prefix tests added |
| 07 - Display backend | Complete | `display.New`, cancellable one-token refresh admission, atomic publication with run-change invalidation and regression rejection, detached `Snapshot`/`Status`, history pass-through, `Display.Handler` routes with bounded strict JSON | `go test ./display` and `go test -race ./display` pass; initial state, stale fallback, selection change, new-run/corrupt-payload, regression, warm/cold equivalence, cancelled admission, concurrency and route/status tests added |
| 08 - Cross-transport acceptance | Complete | `simulator/acceptance_test.go` drives one paused runtime through the local service and its prefix-mounted HTTP handler into both display sources; `display/bounds_test.go` and `display/source_test.go` cover the shared corpus and bounds. Acceptance tests live in `simulator` because they need its fake runtime clock | `go test ./simulation ./simulator ./simulatorapi ./display ./internal/urlpath ./internal/adsb` pass; `go test -race` pass; `golangci-lint run ./...` 0 issues; `go-arch-lint check` OK. Recorded sizes: maximum reception snapshot 8000 records / 4362387 bytes, maximum history page 1000 records / 545452 bytes, both inside the declared 16777216 byte fixture budget |
| 09 - Documentation and final checks | Complete | Root `README.md`, `simulation`/`simulator`/`simulatorapi`/`display`/`urlpath` package docs, `display/testdata/README.md`, external `simulator/example_test.go` and `display/example_test.go`, plan index moved to completed | `go doc -all` for the three public packages, `go test -race ./simulation ./simulator ./simulatorapi ./display`, and `task all` pass |

## Notes

- Cross-transport acceptance tests live in `simulator/acceptance_test.go`
  rather than `display/`, because they need the simulator package's
  unexported fake runtime clock to run on explicit virtual time instead of
  elapsed wall time. The display-side corpus and bounds tests remain in
  `display/source_test.go` and `display/bounds_test.go`.
- The source base URL validator was implemented in `internal/urlpath` as step
  06 requires, and `display.HTTPSourceConfig.Validate` wraps its errors in the
  shared invalid category.
- No commit was created.
