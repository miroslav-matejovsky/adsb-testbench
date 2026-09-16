# ADS-B TestBench

A planned local ADS-B simulator with a live aircraft map and a manager UI.
Generate reproducible synthetic air traffic, model what receiving stations hear,
and inspect decoded observations for development and testing.

This repository is a skeleton. Go packages contain only purpose documentation
and package declarations. Commands and APIs are planned in
[docs/backlog](docs/backlog/README.md).

## Inspiration and scope

The architecture follows [ais-testbench](https://github.com/miroslav-matejovsky/ais-testbench).
Its local sibling checkout supplied the reference structure: reusable engine,
simulator runtime, display backend, embeddable UI, and combined or separate
process composition.

The initial target is 1090 MHz ADS-B extended squitter with synthetic ICAO
aircraft addresses, identification, airborne position, altitude, and velocity.
Protocol details and independent fixtures belong to the codec backlog.
Aircraft need three-dimensional motion and position reconstruction from
received frames. Coverage depends on aircraft altitude. AIS vessel identifiers,
channel rules, and reporting schedules must be replaced with ADS-B behavior.

## Planned architecture

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

Each package documents its purpose in `doc.go`. Allowed future dependencies
are recorded in [.go-arch-lint.yml](.go-arch-lint.yml).

The manager will control aircraft, simulation speed, and stations through the
simulator. The simulator will own aircraft truth and produce frames, station
receptions, and bounded histories. The display will derive tracks from received
frames, with independently aged identity, position, and velocity fields.

Combined mode will pass observations in process. Separate mode will use the
same contract over HTTP, with browsers calling their own backend. Both paths
will share validation and decoding. The engine will accept explicit configuration
and caller-supplied elapsed time; the runtime will own wall-clock pacing.
Time scaling will change virtual time, not reported aircraft speed.

## Development

Use the Go version in `go.mod`, Task, PowerShell, golangci-lint, deadcode,
go-arch-lint, and gotestsum. Run repository checks with:

```text
task all
```

Command reachability checks require entry points, so `task all` omits deadcode
until backlog item 10 adds runnable commands. Package checks remain enabled.

Runnable commands are tracked in the backlog. Future behavior requires
deterministic tests and explicit, documented configuration without defaults.
See [docs](docs/README.md) and [taskfile](taskfile/README.md) for supporting documentation.
