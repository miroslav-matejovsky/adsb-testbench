---
title: "03 - Simulator service and guarded controls"
dependencies: ["01-contracts-and-explicit-configuration.md", "02-coherent-reception-snapshot.md"]
effort: "L"
complexity: "high"
---

# 03 - Simulator service and guarded controls

## Objective

Provide one transport-neutral service implementation for local calls and
HTTP, with coherent DTO reads and run/revision-safe mutation validation.

## Target Artifacts

- New `simulator/api.go`, `api_convert.go`, `api_errors.go`, and tests.
- `simulator/api_config.go`, `simulator.go`, and `doc.go`.
- `simulatorapi/control.go`, `metadata.go`, and contract tests as needed.

## Implementation Tasks

1. Implement `NewAPI(runtime, config)` with nil-runtime and configuration
   rejection. Construction starts no goroutine or server. Implement the read
   and mutation APIs enumerated in the README with explicit contexts.
2. Add named converters for configuration, truth aircraft, transmissions,
   stations, model/coverage, receptions, pages, and observation fields. Format
   exact strings, allocate empty arrays, and deep-copy optional nested data.
   Verify DTO mutation cannot affect runtime state or subsequent responses.
3. Build each truth/metadata/station response from one `Simulator.Snapshot`.
   Calculate coverage from that snapshot's station settings at the configured
   reference altitude. Publish `Heartbeat`, `MaxCatchUp`, engine bounds, and
   API settings as effective policy, and distinguish initial/live counts.
4. Convert observation, raw reception, and history requests before invoking
   their respective runtime read exactly once. Propagate the read's run ID and
   now unchanged. Test empty selection, unknown stations, stale cursors,
   future cursors, and gap/hasMore/nextCursor behavior.
5. Validate mutation run ID before calling the driver. An API is permanently
   attached to one runtime whose ID cannot change, so this guard cannot race
   a reset. Keep station revision checks inside existing serialized driver
   mutations. Require all fields and reject mismatched path/body IDs in HTTP.
6. Count and speed assignments call existing runtime methods. Station
   operations call existing Add/Update/Remove methods. Return the station
   actually returned by the command, or an acknowledgement without a later
   truth snapshot. Preserve settle-at-old-speed behavior and stopped errors.
7. Map engine invalid/not-found/conflict/limit and driver stopped errors into
   shared categories, preserving original causes. Preserve canceled/deadline
   causes. Unexpected errors remain internal with their causes retained locally.
8. Test run conflict causes no mutation or settlement, stale revision races,
   zero controls, deletion, reused station IDs, stopped runtime reads/controls,
   and fake-clock old-speed settlement. Document that a valid command failing
   during pacing may have committed earlier catch-up chunks, as today.

## Technical Details

Native runtime methods remain the owner of mutation serialization. Do not add
a second driver or call `Engine` mutations from the service. Field validation
occurs before the driver; authority checks that depend on mutable station
state stay within the driver's existing lock.

Service errors may expose current run ID on conflict for source restart
handling. A duplicate or previously removed station ID remains `invalid`,
matching engine semantics; it is not silently reassigned as a new station.

## Verification

```text
go test ./simulator ./simulatorapi
go test -race ./simulator
task arch-lint
```

## Acceptance Criteria

- Local service exposes all shared operations and detached responses.
- Every response's time-sensitive fields come from one underlying read.
- Stale run/revision controls fail and preserve the original error category.
- No control bypasses driver serialization or changes pacing semantics.
- Effective settings report explicit values and current speed accurately.

## Non-Goals

HTTP details, global control revisions, per-aircraft route editing, or restart.
