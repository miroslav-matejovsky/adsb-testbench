# Simulator API and received-aircraft display

## Goal and status

Implement one transport-safe simulator API and a received-aircraft backend
that produces equivalent results from local calls and HTTP. This active plan
replaces backlog items 06 (simulator contract and HTTP API) and 07 (display
sources and received-track decoding). Moving those items here does not mean
their implementation is complete. All implementation steps are pending.

## Existing implementation

- `simulation` owns deterministic truth, stations, bounded reception history,
  and received-aircraft projection. `Engine.Observations` captures selected
  histories under one lock before decoding. `Engine.ReceptionHistory` returns
  a coherent page for one station.
- `simulator.Simulator` owns the engine and serialized `simdriver.Driver`.
  Reads do not advance time. Controls settle elapsed time before mutation.
  Engine run IDs are immutable and must differ between engine lifetimes.
- `simulatorapi/observations.go` already defines received DTOs, decimal-string
  cursors, historical receiver provenance, optional measurements, and expiry.
  Conversion, presence validation, service errors, and HTTP are pending.
- `display/doc.go` is a skeleton. `internal/adsb` already decodes supported
  frames and implements global CPR with a ten-second pairing bound.
- `internal/httpserver` supplies host lifecycle support. This plan adds
  mountable handlers; hosts continue to own listeners and background work.

## Scope and architecture

Deliver manager-facing truth, controls, time, explicit settings, stations,
coverage metadata, decoded observations, atomic raw reception snapshots, and
paged reception history. Deliver display sources, common semantic validation
and decoding, bounded refresh state, and a display HTTP handler.

The production dependency graph remains the one in `.go-arch-lint.yml`:

```text
simulation -> internal/adsb
simulator -> simulation + internal/simdriver + simulatorapi
display -> simulatorapi + internal/adsb
host -> simulator + display
```

The host passes `simulator.NewAPI(...)` to `display.NewInProcessSource(...)`
through an interface defined by `display`. Display never imports `simulation`
or `simulator`. `simulatorapi` contains DTOs and transport-neutral error
categories, with no engine, codec, HTTP client, or service implementation.

### Coherent evidence and display state

Add `Engine.ReceptionSnapshot` to capture all retained receptions for an
explicit station selection under one engine lock. Its transport equivalent,
`simulatorapi.ReceptionSnapshot`, contains `runId`, `now`, `stationIds`,
`retention`, and `records`. Records carry existing `Reception` provenance.
Empty selections return empty arrays. Station IDs are deduplicated and sorted.
Records are ordered by transmission sequence, then station ID.

This endpoint is needed because independently fetched history pages do not
share a read instant, and the existing decoded snapshot omits raw evidence
that has not produced a field, including an unpaired CPR sample. Keep paged
history for inspection and the existing decoded observation API for received
diagnostics. Display track construction consumes only raw reception snapshots.

Each display refresh validates a complete bounded snapshot, groups receiver
copies of each transmission, and accumulates identification, altitude,
velocity, and even/odd CPR state in sequence order. It rebuilds that state
from the retained evidence on every refresh. This deliberately gives late
joiners and continuously connected displays the same retention semantics.
No evicted CPR half or expired field survives through an old display cache.
With current engine policy, input is at most 8 stations * 1000 receptions.
Track count is bounded by distinct received ICAOs in those records, not by
the current truth count of 100 aircraft.

Publish only a completely validated refresh. Retain at most one last-good
snapshot with its exact selection and expiry settings for outage reporting.
Source failure returns an error and explicitly stale last-good data for the
same selection; it never reports a successful fresh refresh. A new run clears
previous tracks, pairs, history cursors, and last-good fallback before data
from that run can be published. Same-run virtual-time regression is invalid.

### Wire rules

- Preserve existing lower-camel JSON names and units: degrees, knots, pressure
  altitude feet, vertical feet/minute, metres, dBi, dBm, dB, and nautical miles.
- Unsigned 64-bit values, including seed, sequences, and revisions, are
  canonical decimal strings: `0` or digits without leading zeros. Durations
  are decimal nanosecond strings within signed `time.Duration` bounds. Require
  positive expiry/timeouts and nonnegative elapsed time.
- Times are UTC RFC3339Nano within years 1-9999. ICAO is six uppercase hex
  characters in the codec domain; a frame is exactly 28 uppercase hex
  characters. Reject non-finite floating-point input, including local input.
- Optional received values are explicit `null` when unavailable. Known zero
  remains present. Collections serialize as arrays, including when empty.
- JSON input rejects missing required fields, `null` required fields, unknown
  fields, duplicate object keys, wrong types, and trailing JSON values.
  Presence is checked recursively before conversion to ordinary Go values.
  An explicit empty station array, `false`, or numeric zero is not omission.
- Shared DTO Go values are validated semantically on the local path as well.
  Plain Go scalar values cannot express omission; JSON presence checking is
  the responsibility of parsers, not a claim about Go struct literals.

### Proposed service and source APIs

Names below are implementation targets; add exported API documentation and
keep the established runtime methods available to direct Go callers.

| Owner | API | Responsibility |
| --- | --- | --- |
| `simulation` | `ReceptionSnapshot(ctx, ReceptionSnapshotRequest)` | Atomic copy of selected retained receptions |
| `simulator` | `NewAPI(*Simulator, APIConfig) (*API, error)` | Explicit transport service configuration |
| `simulator.API` | `Metadata`, `Truth`, `Stations`, `Observations`, `ReceptionSnapshot`, `ReceptionHistory` | DTO reads with contexts and detached results |
| `simulator.API` | `SetCount`, `SetSpeed`, `AddStation`, `UpdateStation`, `RemoveStation` | Validated run-guarded controls |
| `simulator.API` | `Handler() http.Handler` | Relative simulator routes |
| `display` | `ObservationSource` | `ReceptionSnapshot(ctx, request)` and `ReceptionHistory(ctx, request)` using shared DTOs |
| `display` | `NewInProcessSource(provider)` | Local provider adapter |
| `display` | `NewHTTPSource(HTTPSourceConfig)` | Bounded HTTP adapter |
| `display` | `New(Config, ObservationSource) (*Display, error)` | Explicit expiry and request bounds; no background work |
| `display.Display` | `Refresh(ctx, stationIDs)`, `Snapshot()`, `ReceptionHistory(ctx, request)`, `Handler()` | Serialized refresh, detached publication, inspection, backend routes |

### Simulator HTTP routes

Paths are relative to the handler mount. Hosts can use `http.StripPrefix`.
Read queries with structured bodies use POST to preserve the existing request
DTO shape and required-field rules; they are still read-only operations.

| Method and path | Request | Successful response |
| --- | --- | --- |
| `GET /metadata` | None | Effective settings, units, limits, model, run ID, virtual time |
| `GET /truth` | None | One engine truth snapshot, current count/speed, time, bounded generated history |
| `PUT /aircraft/count` | `runId`, `count` | 200 command acknowledgement |
| `PUT /time/speed` | `runId`, `speedHundredths` | 200 command acknowledgement; zero pauses |
| `GET /stations` | None | Stations and coverage at configured reference altitude, one read instant |
| `POST /stations` | `runId`, complete station settings | 201 accepted station and revision |
| `PUT /stations/{id}` | `runId`, `expectedRevision`, complete station settings | 200 accepted station and revision |
| `DELETE /stations/{id}` | `runId`, `expectedRevision` | 200 command acknowledgement |
| `POST /observations` | Existing `ObservationRequest` | Existing decoded `ObservationSnapshot` |
| `POST /observations/receptions` | `stationIds` | New raw `ReceptionSnapshot` |
| `POST /receptions/history` | Existing `HistoryRequest` | Existing `ReceptionPage` |

All controls require the current run ID. Updates and deletion require a
positive expected station revision; route and body station IDs must agree.
Count and speed are absolute assignments, with last accepted command winning
within one run. There is no invented global revision or reset endpoint.
Acknowledgements identify the accepted operation and run, without combining a
later independently read snapshot with the command result.

### Errors, limits, and configuration

Use shared `APIError` with stable `code`, actionable `message`, optional
`field`, and `runId` when known. Local errors wrap the original cause through
`Unwrap`. HTTP cannot retain a remote Go error object; its client recreates
the shared category and preserves the returned code/message/status. Transport
and codec failures retain their actual local causes.
Display source errors also carry a validated source run ID when the envelope
is valid but payload validation fails, so restart invalidation survives an
adapter error. An unvalidated run ID is never used to reset state.

| HTTP status | Code/category | Meaning |
| --- | --- | --- |
| 400 | `invalid` | Malformed or semantically invalid input |
| 404 | `not_found` | Unknown route or station |
| 405 | `method_not_allowed` | Wrong method, with `Allow` header |
| 409 | `conflict` | Stale run or station revision |
| 413 | `body_too_large` | Request body exceeds explicit byte limit |
| 415 | `unsupported_media_type` | Body is not JSON |
| 422 | `limit` | Engine capacity or representability limit |
| 503 | `unavailable` | Driver stopped or upstream unavailable |
| 503 | `response_limit` | Complete response cannot fit configured bound |
| 504 | `deadline_exceeded` | Service/source deadline reached |
| 500 | `internal` | Unexpected server failure |

Cancellation is propagated as `context.Canceled`; a disconnected HTTP client
has no promised response status. All successful JSON responses use 200 unless
the route table specifies 201. Responses use `Cache-Control: no-store`.

`APIConfig` requires `MaxRequestBytes`, `MaxResponseBytes`, `RequestTimeout`,
and `CoverageReferenceAltitudeFeet`. `display.Config` requires expiry for
each field and the same three handler limits. `HTTPSourceConfig` requires an
absolute HTTP(S) base URL, positive timeout, positive request/response byte
limits, and an explicit non-nil HTTP client. Constructors insert no application
defaults. API/display construction also requires a host-owned error callback
for actionable HTTP write/internal failures. The callback and HTTP client are
Go dependencies, supplied separately from serializable configuration. Validate
a minimum response budget sufficient for the fixed error
envelope; cap externally derived error text to that envelope budget. Document
example values as explicit fixture/host choices, never implicit defaults.

Add presence-aware `simulator.ParseConfig` for a complete JSON simulation
configuration and `ParseAPIConfig`, `display.ParseConfig`, and
`display.ParseHTTPSourceConfig` for serializable settings. The HTTP parser
receives the host-owned client as a separate Go argument; API/display parsers
receive their error callback separately. Parse settings before
constructing runtime/source objects. Publish effective settings and fixed
policy limits; distinguish initial aircraft count from current count and use
current speed from the committed engine state.

## Implementation order

| Step | Deliverable | Depends on |
| --- | --- | --- |
| [01](01-contracts-and-explicit-configuration.md) | DTOs, errors, explicit parsing | Existing packages |
| [02](02-coherent-reception-snapshot.md) | Atomic raw evidence read | 01 |
| [03](03-simulator-service.md) | Conversions, metadata, guarded controls | 01, 02 |
| [04](04-simulator-http.md) | Simulator routes and bounded JSON | 03 |
| [05](05-display-validation-and-decoding.md) | Common semantic validator and decoder | 01, 02 |
| [06](06-local-and-http-sources.md) | Equivalent bounded source adapters | 04, 05 |
| [07](07-display-backend.md) | Refresh state, outage behavior, display routes | 05, 06 |
| [08](08-cross-transport-acceptance.md) | Parity, restart, failure, and bounds tests | 07 |
| [09](09-documentation-and-final-checks.md) | Public docs, examples, all checks | 08 |

Execute in this order. Update [progress](progress.md) with evidence when each
step completes. See [assessment](assessment.md) for risks and validation.

## Success criteria and coverage

| Original requirement | Implementation and proof |
| --- | --- |
| Controls, time, metadata, stations, settings | 01, 03, 04: DTO, service, HTTP tests |
| Truth/received separation and consistent reads | 02, 03, 05, 08: atomicity and truth-leak regression tests |
| Units, optional fields, safe integer sequences | 01, 05, 08: strict fixtures, zero/null and large-number tests |
| Presence, body/method/status validation | 01, 04, 06, 07: parser and handler matrices |
| Stale identities, revisions, cursor gaps | 03, 04, 07, 08: conflict and retention tests |
| Equivalent in-process/HTTP observations | 06, 08: one fixture corpus through both adapters |
| Identification, velocity, CPR, field expiry | 05, 08: raw-frame expected-value tests |
| Receiver provenance and partial state | 02, 05, 08: edited receivers, multi-station copies, unknown measurements |
| Corrupt frames/contracts and preserved causes | 05, 06, 08: validator and `errors.Is`/`errors.As` tests |
| Deadlines, restarts, bounded caches/history | 06, 07, 08: deterministic cancellation, replacement-run, bounds tests |

Completion requires all nine steps accepted and `task all` passing. UI,
commands, asset bundles, process composition, replay, additional ADS-B message
families, and broad capacity benchmarking remain in backlog items 08-12.
