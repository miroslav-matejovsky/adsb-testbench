---
value: "High"
effort: "L"
complexity: "hard"
dependencies: ["02-deterministic-engine.md"]
---

# Receiving stations and altitude-aware reception

## Area

simulation.

## Work

Add station creation, editing, disabling, and deletion with explicit location, elevation, antenna settings, sensitivity, and impairments. Define a deterministic synthetic reception model using aircraft altitude, station height, range, and documented loss assumptions. Keep reception randomness independent of aircraft generation. Publish effective model parameters and estimated coverage for an explicit reference altitude. Use revision checks for concurrent station edits.

## Acceptance criteria

Test station disablement, reproducible reception decisions, altitude and sensitivity effects, rejected revisions, and station changes preserving generated aircraft frames. Verify coverage and reception use the same model. Label estimates as synthetic model output.

## Dependencies

- [02-deterministic-engine](02-deterministic-engine.md)
