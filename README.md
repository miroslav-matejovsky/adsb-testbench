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

The remaining packages are skeletons. Application commands, the simulator
runtime, stations, observation history, the display backend, and the UI are
planned in [docs/backlog](docs/backlog/README.md).

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

The manager will control aircraft, simulation speed, and stations through the
simulator. The engine already owns aircraft truth, generated frames, and a
bounded transmission history; stations and receptions remain planned. The display will derive tracks from received
frames, with independently aged identity, position, and velocity fields.

Combined mode will pass observations in process. Separate mode will use the
same contract over HTTP, with browsers calling their own backend. Both paths
will share validation and decoding. The engine accepts explicit configuration
and caller-supplied elapsed time; the runtime will own wall-clock pacing.
Time scaling changes virtual time, not reported aircraft speed.

## Development

The codec reuses [go-adsb](https://github.com/cjkreklow/go-adsb) v0.4.1 for
field access, parity, callsigns, and altitude. The
[dependency evaluation](docs/evaluations/2026-09-16-go-adsb.md) explains the
integration and local CPR implementation. Dependencies are pinned in
`go.mod` and `go.sum`.

```text
go doc -all ./internal/adsb
go doc -all ./simulation
go test ./internal/adsb ./simulation
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
