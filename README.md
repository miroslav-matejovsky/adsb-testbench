# ADS-B TestBench

A planned local ADS-B simulator with a live aircraft map and a manager UI.
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

Application commands and the UI remain planned in
[docs/backlog](docs/backlog/README.md). Their
[implementation plan](docs/plans/06-simulator-api-and-display/README.md)
records how the API and display were built.

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

Command reachability checks require entry points, so `task all` omits deadcode
until backlog item 10 adds runnable commands. Codec tests and package checks
remain enabled.

Runnable commands are tracked in the backlog. Future behavior requires
deterministic tests and explicit, documented configuration without defaults.
See [docs](docs/README.md) and [taskfile](taskfile/README.md) for supporting documentation.
