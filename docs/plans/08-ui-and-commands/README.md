# Manager, aircraft display, commands, and embedding

Status: complete. This is one implementation sequence for former backlog items
08 (manager UI), 09 (aircraft map and inspector), and 10 (commands and embedding).
Promoting the backlog items does not mean their implementation is complete.

## Goal and scope

Deliver a usable local ADS-B test bench: manage synthetic traffic and stations,
view received aircraft and exact reception evidence, run combined or separate
processes, and embed independent benches or UI components in a host application.
All configuration is explicit. Libraries expose handlers and lifecycle methods;
applications own listeners, logging, signals, and shutdown.

The existing codec, engine, simulator service, raw observation sources, and display
decoder are the foundation. Read their current `doc.go` files and root
[README](../../../README.md) before implementation. The previously referenced
`06-simulator-api-and-display` plan is absent from this checkout; it is not an
implementation dependency. Current code and tests define the baseline.

## Architecture decisions

- Embed Go templates, CSS, native JavaScript modules, and map dependencies in
  `ui/internal/assets`. Use `html/template` and `embed.FS`. Go binaries need no
  Node runtime, asset build service, CDN, or external font service.
- Use Leaflet 1.9.4 as the pinned map renderer, with its license and required
  bundled files. The official [download page](https://leafletjs.com/download.html)
  provides this distribution. Keep copies under the UI asset tree, never the
  existing Go `vendor` directory. Tiles are an explicit optional host setting.
- Expose `ui.NewManager`, `ui.NewAircraftDisplay`, and `ui.Assets`, plus browser
  exports `mountManager(root, config)` and `mountAircraftDisplay(root, config)`.
  Each mount returns an idempotent `destroy()` method. A component owns only its
  root, requests, timers, listeners, and map instance.
- Manager requests go to the simulator API. Aircraft display requests go to the
  display backend, including station discovery and coverage. Preserve the narrow
  raw `ObservationSource`; add a separate `StationSource` for `Stations`.
  Neither aircraft rendering nor decoding uses `/truth`.
- Use each `POST /observations` response directly. The shared backend
  `GET /snapshot` is a diagnostic view of the last request, not browser session
  state. Each component retains at most one successful observation snapshot for
  its own selection and marks it stale on an outage. No server-side session store
  or unbounded selection cache is introduced.
- Preserve run IDs, revisions, sequences, and nanosecond durations as strings.
  Use JavaScript `BigInt` for comparisons and exact time arithmetic. Render zero
  as a value and null as unavailable. Do not derive field freshness from wall time.
- Compose the combined bench through `display.NewInProcessSource(api)` and the
  station adapter. Separate display mode uses `display.NewHTTPSource` for both
  interfaces. Browser requests remain on the application's own origin.
- Keep APIs and UI handlers relative. A host strips its prefix once. Commands
  derive explicit public API and asset paths from their validated mount prefix;
  embedding callers supply those URLs explicitly.
- Commands load complete JSON configuration through required `-config` and
  `-config-max-bytes` flags. Examples are complete configurations, not defaults.
  Commands generate a fresh run identity on each simulator startup using the
  configured simulation ID as a label plus a cryptographically random suffix.
  Library hosts remain responsible for assigning fresh run IDs.

## Unified implementation order

Execute the numbered steps in order. Front matter also names direct dependencies.
Each step includes implementation locations, checks, and measurable completion gates.

| Step | Deliverable | Original scope |
| --- | --- | --- |
| [01](01-url-and-server-contracts.md) | URL/address validation and explicit server lifecycle settings | 08, 10; existing issues |
| [02](02-display-discovery-and-isolation.md) | Display station discovery and safe failed-refresh responses | 09; existing issues |
| [03](03-assets-components-and-browser-tests.md) | Embedded pages, component lifetime, and browser test harness | 08, 09, 10 |
| [04](04-manager-controls.md) | Aircraft count, speed/pause, virtual clock, generated frames | 08 |
| [05](05-station-editor.md) | Full station editing, revision conflicts, draft preservation | 08 |
| [06](06-received-aircraft-state.md) | Partial observations, station selection, age, stale/lost states | 09 |
| [07](07-map-and-coverage.md) | Persistent map, received markers, synthetic station coverage | 09 |
| [08](08-reception-inspector.md) | Exact frame evidence, bounded history pages, visible gaps | 09 |
| [09](09-composition-and-public-embedding.md) | Public combined/standalone handlers and external host examples | 10 |
| [10](10-command-configuration-and-lifecycle.md) | Three executable commands, configuration, startup and shutdown | 10 |
| [11](11-deployment-and-browser-acceptance.md) | Combined/separate browser acceptance and multiple benches | 08, 09, 10 |
| [12](12-tooling-documentation-and-completion.md) | Runnable tasks, reachability, complete documentation and checks | 10 |

## Requirements coverage

| Requirement | Implementation | Evidence |
| --- | --- | --- |
| Explicit API/asset URLs, templates and licenses | 01, 03 | URL tables, escaping tests, embedded asset checks |
| Count, virtual time, pause/resume, generated frames | 04 | Zero count and pause/resume browser tests |
| Complete station edits, API errors and concurrent drafts | 05 | Two-tab revision conflict browser tests |
| Received altitude, track, speed, vertical rate, age and provenance | 06, 08 | Partial-target and exact-evidence fixtures |
| Station selection, reference-altitude coverage and persistent map | 06, 07 | Selection, coverage, viewport and blocked-tile tests |
| Missed positions, fresh/stale/lost, outage and restart | 02, 06, 11 | Deterministic boundary, cancellation and restart tests |
| Paged per-station history and retention gaps | 08 | Cursor, wraparound, gap and station-removal tests |
| Commands, explicit configuration and graceful shutdown | 01, 10 | Invalid-config, bind-failure and cancellation tests |
| Combined in-process and separate HTTP deployment | 09, 11 | Equivalent received data and browser network assertions |
| Prefixes, public examples, multiple independent benches | 03, 09, 11 | External-module compile and mount/destroy tests |
| Run tasks and command reachability | 12 | `task all` including browser and deadcode checks |

## Success criteria

All 12 steps are complete in [progress](progress.md), their specified tests pass,
and `task all` passes with reachability and browser checks enabled. Every original
backlog acceptance criterion has evidence in the table above. The three commands
run from checked-in example configurations. A separate consumer module compiles
using only public packages. No host must import an `internal` package, and no
component sends requests or changes its DOM after destruction.

The wider benchmark/capacity corpus remains in
[backlog 11](../../backlog/11-quality-and-capacity.md). Replay and traffic
extensions remain in [backlog 12](../../backlog/12-replay-and-traffic-extensions.md).
This plan includes bounded-state checks needed for these UI features, not a new
capacity study or new ADS-B message families.
