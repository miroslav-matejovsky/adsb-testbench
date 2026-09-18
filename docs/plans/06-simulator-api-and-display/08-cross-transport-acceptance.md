---
title: "08 - Cross-transport acceptance and bounds"
dependencies: ["07-display-backend.md"]
effort: "L"
complexity: "high"
---

# 08 - Cross-transport acceptance and bounds

## Objective

Prove the combined original backlog acceptance criteria with shared fixtures,
deterministic integration tests, and measured response-size bounds.

## Target Artifacts

- New `display/source_contract_test.go`, `acceptance_test.go`, `bounds_test.go`.
- `display/testdata/README.md` and fixture corpus.
- `simulator/http_test.go`, `api_test.go`, and reception snapshot tests.
- This plan's `progress.md` with implementation evidence.

## Implementation Tasks

1. Create one fixture runner that supplies identical raw reception/history
   responses through a local provider and an `httptest` simulator endpoint.
   Compare DTOs, decoded observations, error categories, provenance, retention,
   and cursors. Compare local update timestamps separately using a fixed clock.
2. Exercise an actual paused runtime plus `simulator.API`, its HTTP handler,
   and both display sources. Use explicit virtual-time fixtures/fake clocks to
   avoid elapsed real-time dependencies. Verify root and prefixed deployments.
3. Add an expected-value scenario matrix: zero aircraft; no stations; disabled
   station; complete reception; frame loss; identity-only/velocity-only/altitude-
   only targets; valid/expired/missing CPR; unavailable fields replacing known
   values; zero speed; nonzero and over-range measurements; same-time reports.
4. Add provenance scenarios: one transmission heard at multiple receivers,
   different station revisions, station disable/remove, and selected subset
   changes. Check deduplication, all contributing receivers, stable ordering,
   and absence of evidence from unselected stations.
5. Add lifecycle scenarios: source failure after successful refresh, body read
   failure after valid bytes, canceled request, malformed later record, changed
   run with same ICAO, malformed new-run payload, old cursor conflict, and
   same-run time/sequence regression. Assert atomic results and cleared caches.
6. Fill/evict station history deterministically. Check first page without a
   cursor, last page, exact boundaries, explicit resume gap, empty page, future
   cursor, and restart. Compare warm/cold displays after eviction and verify no
   forgotten CPR sample is recovered from a warm cache.
7. Add truth-leak regressions: truth aircraft with no receptions yields no
   display aircraft; removed truth aircraft remains observable while received
   fields are retained/fresh; decoded positions use quantized received values.
   Have source tests fail if any truth endpoint is requested. Cross-check raw
   decoding against existing engine received observations as a secondary oracle.
8. Build valid maximum-cardinality snapshots with 8 stations and 1000 retained
   records each, bounded history pages, and count-churn historical targets.
   Encode complete requests/responses, record byte counts, and assert explicit
   fixture budgets cover them. Start fixture settings at 65536 request bytes
   and 16777216 response bytes; treat these as declared test choices, not
   defaults. Include valid extremes of station settings and timestamp/string
   representations. Separately test oversized identifiers/messages against
   configured body limits; do not claim all possible run-ID lengths fit.
9. Test exact-at-bound and one-byte-over requests/responses, large error bodies,
   dishonest Content-Length, and bound enforcement before publication. Assert
   state contains no more than one last-good snapshot and transient work is
   proportional to bounded raw evidence, including many historical ICAOs.
10. Use `require` assertions and channel synchronization. Document independent
    frame attribution and fixture semantics. Record actual commands/results
    in progress only after the corresponding tests pass.

## Technical Details

Parity alone is insufficient: at least one fixture for each message family
must have expected fields independent of this new decoder. Never use truth
coordinates as exact expected wire-decoded coordinates. HTTP/body failure
fixtures are synthetic transport tests, not real sleeping network outages.

Byte-size assertions establish that the declared fixture configuration supports
current cardinality limits. The API still fails explicitly for any payload
larger than a host's configured response limit. No test silently raises limits
or truncates successful responses to make the assertion pass.

## Verification

```text
go test ./simulation ./simulator ./simulatorapi ./display ./internal/urlpath
go test -race ./simulation ./simulator ./simulatorapi ./display
task arch-lint
```

## Acceptance Criteria

- One fixture corpus passes through both transports with identical observations.
- Independent expected values validate identity, altitude, velocity, and CPR.
- No source or display test depends on simulator truth to build tracks.
- Restart, cancellation, corruption, partial state, and bounded gaps are covered.
- Response byte counts and explicit supported fixture budgets are recorded.
- Concurrency checks pass without sleeps or probabilistic timeout assertions.

## Non-Goals

Browser automation, production load benchmarks, protocol expansion, and commands.
