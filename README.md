# ADS-B TestBench

A local ADS-B simulator with a live aircraft map and a manager UI.
Generate reproducible synthetic air traffic, model what receiving stations hear,
and inspect decoded observations for development and testing.

The raw ADS-B codec is implemented in [internal/adsb](internal/adsb/doc.go).
It encodes and decodes identification, barometric airborne position, and
velocity, and reconstructs airborne CPR positions with explicit age and
reference checks.

The deterministic traffic engine is implemented in
[simulation](simulation/doc.go). It creates synthetic aircraft from explicit
configuration, moves them in three dimensions, schedules each message family
independently in virtual time, and returns complete frame batches for
caller-supplied durations. Aircraft motion is a documented synthetic spherical
model, not WGS-84 navigation, and altitude is pressure altitude relative to
1013.25 hPa. The engine reads no wall clock and runs no background driver.

The same package owns receiving stations and decides which of them hears each
generated transmission, using a radio horizon driven by aircraft altitude and
antenna height together with a 1090 MHz link budget driven by range, gain,
system loss, and receiver sensitivity. Stations are created, edited, disabled,
and removed through revision-checked commands, and the model parameters and the
coverage they imply are published for an explicit reference altitude. Reception
is synthetic model output, not a calibrated RF prediction, and station changes
never alter the frames a run generates.

The [simulator](simulator/doc.go) owns that engine, its serialized real-time
driver, and one transport-neutral service over both. The service exposes
manager-facing truth, controls, explicit settings, stations with coverage,
decoded observations, atomic raw reception snapshots, and paged reception
history, using the transport-safe data of [simulatorapi](simulatorapi/doc.go).
`API.Handler` returns relative HTTP routes a host mounts wherever it likes
with `http.StripPrefix`; the host keeps ownership of listeners, logging, and
lifecycle.

The [display](display/doc.go) backend builds received-aircraft tracks from raw
evidence alone. It reads a simulator either in process or over HTTP through
one `ObservationSource` contract, runs the same semantic validation and codec
decoding on both paths, and publishes each refresh atomically with field
availability, age, and receiver provenance.

The [ui](ui/doc.go) package serves the manager page, the received-aircraft
page with its map and reception inspector, and the embedded browser bundle.
The [testbench](testbench/doc.go) package composes them into combined,
simulator-only, or display-only applications, and three
[commands](cmd/README.md) run those applications from complete
[configuration files](configs/README.md).

## Running

Each command requires a configuration file and its size bound. The shipped
examples are complete configurations, not defaults:

```text
task run-combined CONFIG=configs/combined.json CONFIG_MAX_BYTES=65536
task run-simulator CONFIG=configs/simulator.json CONFIG_MAX_BYTES=65536
task run-display CONFIG=configs/display.json CONFIG_MAX_BYTES=65536
```

The combined example serves `http://127.0.0.1:18480/` with links to
`manager/` and `aircraft/`. The separate examples run together: the simulator
serves its manager on `127.0.0.1:18481` and the display reads that
simulator's API and serves its aircraft page on `127.0.0.1:18482`. Every
application reports its mode, lifecycle state and effective run ID at
`status`.

## Inspiration and scope

The architecture follows [ais-testbench](https://github.com/miroslav-matejovsky/ais-testbench).
Its local sibling checkout supplied the reference structure: reusable engine,
simulator runtime, display backend, embeddable UI, and combined or separate
process composition.

The initial target is 1090 MHz ADS-B extended squitter with synthetic ICAO
aircraft addresses, identification, airborne position, altitude, and velocity.
Protocol details are documented in [the codec](internal/adsb/doc.go), with
[independent frame fixtures](internal/adsb/testdata/README.md).
Aircraft need three-dimensional motion and position reconstruction from
received frames. Coverage depends on aircraft altitude. AIS vessel identifiers,
channel rules, and reporting schedules must be replaced with ADS-B behavior.

## Package structure

| Package | Responsibility |
| --- | --- |
| `cmd/adsb-testbench` | Combined command |
| `cmd/simulator` | Standalone simulator command |
| `cmd/display` | Standalone display command |
| `simulation` | Deterministic traffic engine |
| `simulator` | Simulator runtime |
| `simulatorapi` | Shared contract |
| `display` | Received-aircraft backend |
| `testbench` | Composition API |
| `ui` | Manager and aircraft display |
| `ui/internal/assets` | Embedded asset bundle |
| `internal/adsb` | ADS-B codec |
| `internal/simdriver` | Real-time driver |
| `internal/cli` | Shared command input validation |
| `internal/httpserver` | HTTP lifecycle |
| `internal/urlpath` | Public URL validation |

Each package documents its purpose in `doc.go`. Allowed dependencies
are recorded in [.go-arch-lint.yml](.go-arch-lint.yml).

The manager controls aircraft, simulation speed, and stations through the
simulator service. Every mutation names the run it was written for, station
edits carry the revision the caller last observed, and the service validates
input before the driver settles any elapsed time. The engine owns aircraft
truth, generated frames, bounded transmission and per-station reception
histories, and received-aircraft snapshots for explicit station selections.
Reception cursors include run and station identities.

The display derives its tracks only from received frames. Combined mode passes
observations in process; separate mode uses the same contract over HTTP, with
browsers calling their own backend. Both paths share one validator and one
decoder, so they produce the same tracks and the same failure categories.
Identity, global CPR position, altitude, and velocity age independently in the
source's virtual time, never in wall time, and every refresh rebuilds that
state from the evidence the simulator still retains.

The engine accepts explicit configuration and caller-supplied elapsed time;
the runtime owns measured wall-clock pacing, settles elapsed time before
controls, and bounds catch-up after suspension. Time scaling changes virtual
time, not reported aircraft speed. No constructor or parser in these packages
inserts an application default: byte bounds, timeouts, field lifetimes, the
coverage reference altitude, the HTTP client, and the host error callback are
all supplied explicitly.

## Deployment and embedding

A combined application runs one simulator and one display in one process;
the display reads the simulator in process, with no HTTP hop. In separate
mode the display reads received evidence and station discovery from the
simulator API over HTTP. In both modes browsers contact only their own
application: the manager calls the simulator API, the aircraft page calls
the display API. Routes (`manager/`, `aircraft/`, `api/simulator/`,
`api/display/`, `assets/`, `status`) are relative and mounted once below the
configured public base path; the route table is in
[testbench](testbench/doc.go).

Two clocks are kept apart. Real time paces the driver and labels how old a
browser's last successful update is. Virtual time, scaled by the configured
speed, drives aircraft motion and every field age. The map uses the embedded
Leaflet 1.9.4 renderer; tiles are an explicit optional setting and `null`
requests none. Station coverage circles are synthetic estimates at the
configured reference altitude. The reception inspector pages bounded
per-station history and shows source retention gaps separately from its own
`maxHistoryRecords` trimming. Bundled third-party notices are served at
`assets/notices.html`.

Commands turn the configured simulation ID into a fresh effective run ID on
every launch, so requests written for an earlier run are rejected. On
interrupt they drain HTTP within the shutdown budget, then stop the
simulator. Hosts embed benches through public packages only; the
[embedding example](examples/embedding/README.md) mounts two independent
benches and a custom page with browser components.

## Development

The codec reuses [go-adsb](https://github.com/cjkreklow/go-adsb) v0.4.1 for
field access, parity, callsigns, and altitude. The
[dependency evaluation](docs/evaluations/2026-09-16-go-adsb.md) explains the
integration and local CPR implementation. Dependencies are pinned in
`go.mod` and `go.sum`.

```text
go doc -all ./internal/adsb
go doc -all ./simulation
go doc -all ./simulatorapi
go doc -all ./simulator
go doc -all ./display
go test ./internal/adsb ./simulation ./simulatorapi ./simulator ./display
```

Use the Go version in `go.mod`, Task, PowerShell, golangci-lint, deadcode,
go-arch-lint, and gotestsum. Run repository checks with:

```text
task all
```

`task all` runs tidy, vet, formatting, command reachability, architecture
lint, code lint, Go tests, cleanup checks, the external embedding module and
the browser tests. Browser tests need Node.js (version in `.node-version`)
and an explicit one-time setup; tests never download dependencies:

```text
npm ci
npx playwright install chromium
```

`task build` writes the three commands to `bin/`. Browser test results and
traces of failing runs are kept below `.test-results/`. See
[ui/browser](ui/browser/README.md) for the browser test layout.

New behavior requires deterministic tests and explicit, documented
configuration without defaults.
See [docs](docs/README.md) and [taskfile](taskfile/README.md) for supporting documentation.
