---
title: "01 - URL, address, and server lifecycle contracts"
dependencies: []
effort: "M"
complexity: "medium"
---

## Objective

Validate all future application mounts before serving and replace hidden HTTP
timeout settings with explicit caller configuration. Address findings F3 and F4.

## Target Artifacts

- Existing `internal/urlpath/source.go`, `source_test.go`, and `doc.go`.
- New `internal/urlpath/public.go` and `public_test.go`.
- Existing `internal/httpserver/server.go`, `server_test.go`, and `doc.go`.
- New `internal/cli/address.go`, `address_test.go`; update `internal/cli/doc.go`.

## Implementation Tasks

1. Add `ParseMountPrefix` for `/` or a canonical absolute path ending in `/`.
   Add `ParseBrowserBase` for root-relative bases and absolute HTTP(S) bases.
   Normalize one trailing slash, retain the entire mount path, and reject
   relative paths, protocol-relative URLs, userinfo, query/fragment delimiters,
   duplicate slashes, dot segments, control bytes, backslashes, encoded path
   separators and encoded dot segments. Test accepted and rejected vectors.
2. Keep `ParseSourceBase` absolute-only. Share path checks where semantics match;
   add regression cases for encoded backslashes, empty fragments, invalid ports,
   and nested source prefixes. Do not rely on `path.Clean` to repair bad input.
3. Add `cli.ValidateListenAddress`: require an explicit host and numeric port,
   allow bracketed IPv6 and explicitly configured wildcard hosts, reject URL
   syntax and missing ports. Permit port zero for caller-selected ephemeral
   listeners. Validation must not bind a socket or perform DNS lookup.
4. Add `httpserver.Config` with required positive `ReadHeaderTimeout`,
   `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `ShutdownTimeout`, and
   `MaxHeaderBytes`. Change `Serve` to return `(*Server, error)` and `Run` to
   accept the validated configuration. Reject nil logger/listener/handler before
   goroutines start. Update every existing test caller in this step.
5. Keep drain-before-work-cancellation behavior. Force-close connections after
   the configured drain deadline, join serving/work, and preserve all original
   errors with context. Expected cancellation is a clean shutdown; work failure
   or unexpected successful work termination before cancellation is not silently
   accepted. Document that work must honor cancellation.
6. Update package documentation for signatures, listener ownership, path forms,
   and error matching. Verify with the tests below and `go doc`.

## Technical Details

Relative handler routes remain unchanged. For a public base `/bench/a/api/`,
append `truth`, never `/truth`. A host registers the prefix and strips its
non-trailing-slash form once. A canonical prefix redirect must preserve the
configured mount and must not redirect an API mutation across origins.

Browser absolute API/asset URLs must resolve to the page's origin at mount time.
Different-origin tiles are a separate explicit setting. This permits explicit
absolute configuration without adding CORS or credential handling.

`Run` accepts context as an argument and creates the detached drain context only
when shutdown starts. Store no context in a server struct. Library constructors
do not own signals or call `os.Exit`.

## Verification

```text
go test ./internal/urlpath ./internal/cli ./internal/httpserver
go doc ./internal/httpserver
task all
```

Use channel barriers for in-flight drain tests. For forced shutdown, inject an
already-expired drain context into the shutdown path rather than sleeping.
Assert nil-dependency rejection, failed accept, concurrent work failure, and
error identity. Reuse URL vectors in step 03's browser checks.

## Acceptance Criteria

- Every accepted prefix resolves child routes beneath that prefix.
- Every ambiguous URL/address vector fails before listener or fetch creation.
- No HTTP timeout is silently supplied by a package constructor.
- Graceful drain preserves active work until requests finish; forced close and
  failures are observable and all cooperative workers are joined.

## Non-Goals

TLS certificate management, remote authentication, reverse-proxy configuration,
or changing simulator route names.
