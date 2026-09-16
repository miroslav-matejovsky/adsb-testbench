---
value: "High"
effort: "L"
complexity: "medium"
dependencies: ["04-observation-history.md","05-runtime-and-lifecycle.md"]
---

# Simulator contract and HTTP API

## Area

simulatorapi and simulator.

## Work

Define shared request/response data and errors for aircraft controls, time, metadata, stations, observations, and paged reception history. Keep truth endpoints for manager diagnostics distinct from received-data endpoints used by display. Specify units, optional fields, JSON-safe sequence representation, run and revision conflicts, body limits, and status codes. Publish effective explicit settings. Provide the same observation contract in process.

## Acceptance criteria

Test HTTP validation, missing configuration fields including meaningful zero values, body bounds, methods, error categories, stale identities, pagination gaps, and consistent snapshots. Verify the in-process and HTTP paths expose equivalent observations.

## Dependencies

- [04-observation-history](04-observation-history.md)
- [05-runtime-and-lifecycle](05-runtime-and-lifecycle.md)
