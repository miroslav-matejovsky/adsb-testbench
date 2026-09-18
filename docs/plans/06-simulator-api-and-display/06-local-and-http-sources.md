---
title: "06 - In-process and HTTP observation sources"
dependencies: ["04-simulator-http.md", "05-display-validation-and-decoding.md"]
effort: "L"
complexity: "high"
---

# 06 - In-process and HTTP observation sources

## Objective

Provide equivalent source adapters with one semantic validation path and
bounded HTTP I/O that preserves error causes and mount prefixes.

## Target Artifacts

- New `display/source.go`, `source_local.go`, `source_http.go`, and tests.
- Existing `display/validate.go`, `errors.go`, `config.go`, and `config_json.go`.
- `internal/urlpath/doc.go` plus new source-URL validation code/tests.

## Implementation Tasks

1. Define the two-method `ObservationSource` interface in `display`, using
   shared raw-snapshot and history request/response DTOs and explicit contexts.
   Make `simulator.API` satisfy it structurally without importing display.
2. Implement `NewInProcessSource` with provider validation. Validate requests
   before calling the provider, check response semantics with step 05 helpers,
   return detached results, and wrap failures with operation context. Test
   canceled calls, invalid provider responses, and underlying error identity.
3. Implement `NewHTTPSource` with the explicit settings from step 01. Reject
   unsupported schemes, missing host, userinfo, query/fragment, malformed escapes,
   and dot-segment/encoded-separator paths. Accept an explicit root or nested
   base path and normalize only the boundary slash used to append route paths.
4. Add the small source-base URL validator to `internal/urlpath`, documenting
   this implemented responsibility in `doc.go`. Validate `/bench/a/simulator/`
   and ensure appending `observations/receptions` or `receptions/history` keeps
   the entire prefix. Leave API/asset UI URL configuration to its backlog work.
5. Copy the supplied HTTP client configuration into a source-owned value so
   request policy does not mutate the host client. Reject redirects rather than
   silently switching source identity. Keep ownership of the injected transport
   with the host; never close the host's idle connections from the source.
6. Encode bounded requests, set JSON headers, and use a derived timeout context
   canceled after every call. It covers response-body reading as well as
   headers. Respect earlier caller deadlines. Do not retry automatically.
7. Check status and media type, reject unsupported content encoding, and read
   at most `MaxResponseBytes + 1` bytes regardless of Content-Length. Reject
   oversized declared or actual length. Count bytes after any transport-provided
   decompression. Close bodies on success, decode error, status failure, and
   overflow; retain read/close errors using wrapping or `errors.Join`.
8. Decode exactly one strict JSON value. Validate required response keys and
   explicit nullability, including error envelopes. Use the same semantic
   validators as the local adapter. Map valid remote errors to shared categories;
   reject malformed or status-inconsistent error envelopes as source protocol
   errors. Retain remote HTTP status, code/message, and operation context.
   In both adapters, propagate a validated envelope run ID through `SourceError`
   if payload validation fails. Do not salvage identity from malformed JSON,
   an oversized body, or a semantically invalid envelope.
9. Test both adapters with identical valid/invalid DTO fixtures, and add HTTP
   cases for malformed JSON, missing keys, duplicate keys, redirects, status,
   media type, misleading length, oversize success/error bodies, truncated reads,
   trailing JSON, cancellation, timeout, prefix preservation, and body closure.

## Technical Details

Both adapters validate transport data. Display refresh also validates before
publication because hosts may supply their own `ObservationSource`. All paths
call the same semantic helper functions; there is no transport-specific
decoding algorithm.

Use a controlled `http.RoundTripper` and an instrumented `io.ReadCloser` in unit
tests. For deadline behavior, inspect the outgoing request's deadline and return
`context.DeadlineExceeded` deterministically. Channel-controlled cancellation
tests synchronize request entry and completion without sleeps.

## Verification

```text
go test ./display ./internal/urlpath ./simulator
task arch-lint
```

## Acceptance Criteria

- Both sources return equivalent DTOs and shared error categories for the corpus.
- Actual request/response bytes obey configured bounds even with false lengths.
- Every response body is closed and local read/context causes remain observable.
- Nested simulator mount paths survive URL construction unchanged.
- Source creation starts no server, polling loop, or background worker.

## Non-Goals

Retries, streaming, source discovery, source-owned processes, and UI URL settings.
