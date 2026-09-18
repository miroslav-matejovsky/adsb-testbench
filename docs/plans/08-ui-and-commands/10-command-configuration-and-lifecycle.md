---
title: "10 - Explicit command configuration and process lifecycle"
dependencies: ["09-composition-and-public-embedding.md"]
effort: "L"
complexity: "high"
---

## Objective

Implement all three commands with complete documented settings, early validation,
fresh startup identity, process logging, and deterministic shutdown tests.

## Target Artifacts

- `cmd/adsb-testbench/main.go`, `cmd/simulator/main.go`, `cmd/display/main.go`,
  command tests and existing `doc.go` files.
- `internal/cli/config.go`, `config_test.go`, logging/address input helpers.
- `testbench/config_json.go`, its tests, and public configuration documentation.
- New `configs/README.md`, `combined.json`, `simulator.json`, `display.json`.
- `cmd/README.md`, root README and `internal/httpserver` call sites.

## Implementation Tasks

1. Give each command required `-config` and positive `-config-max-bytes` flags;
   reject unknown flags and positional arguments. `-help` exits successfully.
   Keep `main` minimal and put executable work in a context-aware `run` function
   receiving arguments and output dependencies so tests do not call `os.Exit`.
2. Parse one complete bounded JSON object with duplicate/missing/null/unknown-key
   and trailing-value rejection. Reuse existing simulator/display parsers for
   nested settings and explicit UI/server parsers. Use typed section values and
   contextual errors; do not round integers or fill omitted values.
3. Define required sections in the table below. Each JSON file includes a required
   nonempty `$comment` string documenting its keys, chosen settings and units
   within the configuration file itself; consume this documentation field before
   binding operational sections. Document accepted ranges, valid zero/null values
   and configuration-file path behavior in `configs/README.md` as well.
   Configuration files contain all chosen values. No
   environment fallback, hard-coded listen address, poll interval, seed or expiry.
4. Validate complete configuration and cross-field constraints before creating
   the runtime or opening a listener: public prefix consistency, initial station
   ID uniqueness, selected station validity in local mode, UI byte budgets,
   positive timeouts, write/read budget versus API/source deadlines, map settings
   and full source URL. Source unavailability is an operational state, not a
   configuration parse failure.
5. For simulator modes, turn the configured simulation ID label into a fresh
   effective run ID with a crypto-random suffix before construction. Fail if
   entropy fails. Initialize the explicitly configured station list through the
   service while the runtime is paused, then restore configured speed immediately
   before Run. Do not allow construction delay to advance traffic while stations
   are being installed. Include empty stations as an explicit valid array.
6. Construct a command-owned `slog` logger, error callback and, for remote display,
   HTTP client/transport with explicit transport settings. Log startup addresses,
   effective run ID and fatal errors once. Join transport cleanup on exit; package
   callbacks report errors the browser cannot observe without duplicate logging.
7. Bind the validated address, register process cancellation using signals
   supported by the target OS, and call configured `httpserver.Run` with the
   application's handler/work. Drain HTTP before stopping the simulator, preserve
   bind/serve/work/shutdown causes and return a nonzero process status for failure.
8. Add tests for missing fields, null/duplicate/unknown input, explicit zero/false,
   oversized config, invalid source/prefix/address, station initialization failure,
   bind failure, entropy failure, clean cancellation and runtime failure. Inject
   run-ID generation and listener creation in command tests, not hidden globals.

## Technical Details

| Section | Combined | Simulator | Display |
| --- | --- | --- | --- |
| `$comment` (in-file settings documentation) | Required | Required | Required |
| `server` (listenAddress, publicBasePath, all step-01 limits) | Required | Required | Required |
| `logging` (level and format: text or JSON) | Required | Required | Required |
| `simulator` (existing complete simulator configuration) | Required | Required | Absent |
| `simulatorApi` (existing complete API configuration) | Required | Required | Absent |
| `stations` (array of complete station settings) | Required | Required | Absent |
| `display` (existing complete display configuration) | Required | Absent | Required |
| `managerUi` (step-03 manager settings except derived URL bases) | Required | Required | Absent |
| `aircraftUi` (step-03 display settings except derived URL bases) | Required | Absent | Required |
| `source` (existing complete HTTP source settings) | Absent | Absent | Required |
| `transport` (dial, keepalive, TLS handshake, response-header, idle timeouts and connection bounds) | Absent | Absent | Required |

Deriving browser base URLs from `publicBasePath` plus fixed route names is an
explicit composition rule, not a missing-value default. JSON run labels are not
silently used as lifetime identities. Log/document the generated identity, keep
seed and virtual start time unchanged, and test that identical settings preserve
generated traffic while producing distinct run IDs across launches.

For simulator initialization, pass a copied config with zero speed, create all
stations, then set the requested speed after successful listener binding and
immediately before starting work. Validate failures close the bound listener.
External Go hosts still supply a complete config with a fresh lifetime ID.

## Verification

```text
go test ./cmd/... ./internal/cli ./internal/httpserver ./testbench
go build ./cmd/adsb-testbench ./cmd/simulator ./cmd/display
task all
```

Parse every shipped configuration in a test. Command lifecycle tests use channels
and cancellation; OS signal smoke tests in step 11 wait for explicit readiness.

## Acceptance Criteria

- All commands build and start from their complete example configuration.
- Invalid configuration opens no listener and starts no worker.
- Reusing a configuration creates a new run identity and old mutations/cursors
  are rejected against it.
- Cancellation drains requests and joins work; failures keep original causes.
- No logging, signals, process exit, or listener creation occurs in constructors.

## Non-Goals

Environment-variable configuration, hot reload, service installation, daemon
management, TLS provisioning, or persisting manager edits across restarts.
