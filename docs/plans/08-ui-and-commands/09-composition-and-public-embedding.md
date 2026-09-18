---
title: "09 - Public application composition and host embedding"
dependencies: ["08-reception-inspector.md"]
effort: "L"
complexity: "high"
---

## Objective

Compose combined and standalone applications from the existing services and UI,
while hosts retain ownership of HTTP servers, logging, and lifecycle.

## Target Artifacts

- New `testbench/config.go`, `app.go`, `routes.go`, `app_test.go`, `example_test.go`.
- `testbench/doc.go`, `.go-arch-lint.yml` and UI public examples.
- New `examples/embedding/` external module, complete explicit fixtures and README.
- New browser multiple-bench/mount fixture using public APIs.

## Implementation Tasks

1. Add public constructors `testbench.New(Config)`,
   `NewSimulator(SimulatorConfig)` and `NewDisplay(DisplayConfig)`, all returning
   `(*App, error)`. Keep mode-specific configs concrete and complete. Reuse
   simulator/display/UI validation before starting any work.
2. Combined construction creates exactly one simulator/API, an in-process raw
   source, an in-process station source, and one display. Simulator-only exposes
   manager/API; display-only uses an explicitly supplied HTTP source config/client
   for observations and station discovery. Do not route combined reads via HTTP.
3. Expose `App.Handler() http.Handler` and `App.Run(context.Context) error`.
   Constructors start no listener, goroutine or signal registration. Combined
   and simulator modes supervise their one simulator; display-only Run waits for
   cancellation and does not poll. Document single-run lifecycle and clean stop.
4. Assemble relative routes listed below. Public paths are derived from the
   explicitly configured `PublicBasePath`; rendering never infers them from
   request Host, browser location or proxy headers. A host strips that public
   prefix once before dispatching to the application handler.
5. Add bounded status JSON with mode, configured/started/stopped state and known
   run ID. Display-only status identifies upstream availability independently of
   local readiness. Local readiness must not require contacting a remote source.
   No status request advances virtual time.
6. Adjust architecture edges explicitly: command packages may use `testbench`
   and `httpserver`; `testbench` may use existing UI/services; `cli` may use
   `simulatorapi` strict JSON primitives when needed. UI remains independent of
   simulation/runtime/display internals. Verify with architecture lint.
7. Create a separate external example module with a local `replace` directive
   that imports only public packages. Demonstrate a host-owned server, nested
   prefix, two independent applications, public UI-only components, cancellation
   and error handling. Use complete example settings and compile it separately.

## Technical Details

| Relative route | Combined | Simulator | Display |
| --- | --- | --- | --- |
| `/` | Links to enabled pages | Manager link | Aircraft link |
| `/manager/` | Manager page | Manager page | 404 |
| `/aircraft/` | Aircraft page | 404 | Aircraft page |
| `/api/simulator/` | Simulator API | Simulator API | 404 |
| `/api/display/` | Display API | 404 | Display API |
| `/assets/` | Embedded bundle | Embedded bundle | Embedded bundle |
| `/status` | Local status | Local status | Local/source status |

Mount `/bench/a/` and `/bench/b/` on one host with separate `App` instances and
configs. Assert that their URLs, station edits, run IDs and lifecycle never cross.
Standalone UI constructors also accept explicit API/asset URLs for a host that
builds its own page layout. No exported signature requires an internal type.

## Verification

```text
go test ./testbench ./ui ./simulator ./display
go -C examples/embedding test ./...
task arch-lint
task all
```

Use counting sources/transports to prove combined mode makes zero upstream HTTP
calls. Test all route/mode combinations, root and nested prefixes, invalid config,
constructor side-effect absence, repeated Run rejection and independent benches.

## Acceptance Criteria

- Public external consumer builds without importing repository internal packages.
- Host-supplied listeners/loggers/signals remain host-owned.
- Combined mode uses in-process evidence and standalone display uses HTTP evidence.
- All links/assets/API requests remain below their configured public prefix.
- Stopping or editing bench A does not affect bench B.

## Non-Goals

CLI flags, automatic server startup from constructors, authentication, or shared
state between independent benches.
