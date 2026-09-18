---
title: "01 - Contracts and explicit configuration"
dependencies: []
effort: "L"
complexity: "high"
---

# 01 - Contracts and explicit configuration

## Objective

Define the shared service data and errors, and ensure parsers distinguish
missing configuration from explicit zero without introducing defaults.

## Target Artifacts

- Existing `simulatorapi/observations.go`, `observations_test.go`, and `doc.go`.
- New `simulatorapi/control.go`, `metadata.go`, `errors.go`, and associated tests.
- New `simulator/config_json.go`, `api_config.go`, and associated tests.
- New `display/config.go`, `config_json.go`, and associated tests.

## Implementation Tasks

1. Extend shared DTOs with raw `ReceptionSnapshotRequest`/`ReceptionSnapshot`,
   keeping existing observation/history DTO names and field representations.
   Add JSON round-trip tests for empty arrays, null fields, and exact strings.
2. Add truth-only aircraft/transmission DTOs and metadata DTOs that cover every
   current `simulation.Config`, `SpawnConfig`, `StationConfig`, model, coverage,
   and history-limit field. Keep truth types separate from `simulatorapi.Aircraft`.
   Test JSON field names and units against explicit expected fixtures.
3. Add command DTOs for count, speed, station create/update/delete, with run ID
   on every mutation and positive expected revision on update/delete. Use
   complete station settings rather than a partial-update merge. Test zero
   count/speed and disabled stations as valid fully specified inputs.
4. Add shared error categories and `APIError` implementing `Error`, `Unwrap`,
   and category matching. Keep an unexported/local cause out of JSON. Test
   category matching after wrapping and exact wire fields without cause data.
5. Add explicit API/display/HTTP source configuration types and constructors'
   validation functions. Validate positive limits/timeouts and all four expiry
   durations. Validate coverage reference altitude through engine coverage
   rules. Explicit zero reference altitude remains accepted where valid.
   Include required host error callbacks for API/display handlers as Go-only
   dependencies, alongside the HTTP source's explicit client. Reject nil
   dependencies; parsers receive them separately from JSON.
6. Implement `simulator.ParseConfig(io.Reader, maxBytes)` and transport/display
   configuration parsers using bounded reads and strict recursive presence
   checks. List all required keys, including every spawn range min/max and
   station property when parsed in later requests. Reject duplicate keys,
   unknown keys, null required values, trailing JSON, and overflow before
   narrowing integers. Use package-local parsing helpers; do not create a
   cross-package generic configuration framework.
7. Table-test removal of every required leaf independently. Pair each
   meaningful zero/false/empty-array success case with missing/null failures.
   Test uint64 values above 2^53 and at MaxUint64, signed duration overflow,
   malformed hex/time strings, and locally supplied NaN/Inf.

## Technical Details

Use the route/representation/error tables in the plan README as the contract.
`DisallowUnknownFields` alone does not reject duplicate keys or detect all
missing scalar fields; token-level key checking and presence-aware wire types
must cover those cases before constructing concrete values.

Simulator JSON settings contain a required `simulation` object with `id`,
`startTime`, `seed`, `initialAircraftCount`, `speedHundredths`, and `spawn`.
All six spawn range objects require `min` and `max`. API/display configuration
parsers require every field listed in the README. A client is supplied to the
HTTP source parser as a Go dependency, not serialized as JSON.

Truth responses distinguish configured initial count from live count. Seed,
run IDs, and original configuration are diagnostic data, never track inputs.
Station reception provenance keeps the historical station revision/settings.

## Verification

```text
go test ./simulatorapi ./simulator ./display
go vet ./simulatorapi ./simulator ./display
task arch-lint
```

## Acceptance Criteria

- Every wire field has documented units, presence, and range semantics.
- Missing/null required leaves fail; all documented explicit zero values pass.
- Large sequences/revisions/seeds round-trip without precision loss.
- Shared contract package has no engine or HTTP dependency.
- Error wrapping preserves local cause and shared category independently.

## Non-Goals

HTTP routing, display decoding, CLI configuration files, and runtime reset.
