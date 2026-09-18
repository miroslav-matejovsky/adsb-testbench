---
title: "04 - Simulator HTTP handlers"
dependencies: ["03-simulator-service.md"]
effort: "L"
complexity: "high"
---

# 04 - Simulator HTTP handlers

## Objective

Expose the simulator service through the specified relative HTTP routes with
strict input validation, bounded bodies, deadlines, and stable error responses.

## Target Artifacts

- New `simulator/http.go`, `http_json.go`, `http_test.go`, `http_json_test.go`.
- Existing `simulator/api.go`, `api_config.go`, and `doc.go`.

## Implementation Tasks

1. Implement `API.Handler()` and every route in the plan README. Route handlers
   parse requests and call the corresponding API method; no handler directly
   accesses engine state. Test each success route and exact status/DTO shape.
2. Check route and method before parsing bodies. Return JSON 404 for unknown
   paths and 405 with the explicit `Allow` value for unsupported methods,
   including unsupported HEAD/OPTIONS. Reject bodies on GET routes and unknown
   query parameters; request DTO routes use only JSON bodies.
3. Apply required JSON content type, the configured request byte bound, strict
   recursive key/presence validation from step 01, and exactly one JSON value.
   Support `application/json` with a valid charset parameter. Reject compressed
   request bodies and unsupported media types explicitly. Validate untrusted
   integers before conversion to count, speed, revisions, and durations.
4. Validate station path identifiers without silently normalizing them. Require
   body station ID to match the path for update. Reject missing/zero revisions,
   missing run IDs, and stale run/revision identities through shared errors.
5. Derive a context deadline from the earlier of the caller deadline and
   configured request timeout; pass it through the service. Pre-canceled work
   stops before mutation. Do not spawn a detached goroutine to enforce timeout.
6. Encode into a bounded buffer before writing status or headers. If output
   exceeds `MaxResponseBytes`, return the fixed bounded `response_limit` error,
   never truncated successful JSON. Apply the same rule to error output and
   preserve causes from encoding failures for the configured error-reporting
   path. Return writer failures to a package-local serving helper so tests can
   assert them; route wrappers report actionable unexpected failures via an
   explicit host-supplied error callback, without logging routine 4xx errors.
7. Set JSON content type and `Cache-Control: no-store` consistently. Map shared
   categories to the README status table. Keep internal implementation details
   out of internal-error wire messages while retaining causes locally.
8. Add table-driven handler tests for missing/null/unknown/duplicate fields,
   malformed/trailing JSON, byte limits at/over boundary, methods, media types,
   stale identities, engine limits, page gaps, and response limits. Mount below
   `/bench/a/simulator` using `http.StripPrefix` and verify all paths still work.

## Technical Details

Add an explicit error-reporting callback to `APIConfig` as a Go dependency
when implementing HTTP write/error handling; it is not a serializable setting.
Require it at construction, and inject it separately into the JSON config
parser. Tests capture errors through the callback. The handler does not own
logs, signals, listeners, or `httpserver.Run`.

Reject invalid input before invoking a control. HTTP cancellation during
already-running catch-up retains the runtime's documented chunk-commit
semantics; transport failure does not imply command rollback. No mutation
is automatically retried.

## Verification

```text
go test ./simulator -run 'TestHTTP|TestAPI'
go test ./simulator ./simulatorapi
```

Use `httptest.NewRecorder`, an erroring writer, canceled contexts, and the
existing fake runtime clock. Test deadline cause mapping with a pre-expired
context or controlled service error instead of elapsed-time assertions.

## Acceptance Criteria

- All listed routes implement the documented method, status, and DTO contract.
- Rejected request syntax and stale identities never invoke a mutation.
- Request and response bodies obey explicit bounds, including error output.
- Context and unexpected writer/encoding failures remain observable.
- Prefix mounting works without changing the route definitions.

## Non-Goals

Servers, CORS policy, authentication, browser assets, and runnable commands.
