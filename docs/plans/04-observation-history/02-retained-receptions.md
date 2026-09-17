---
title: "02 - Retained receptions"
dependencies: ["01-contracts-and-policies.md"]
effort: "M"
complexity: "high"
---

# Retained receptions

## Objective

Retain complete accepted receptions inside the engine's staged state.

## Target Artifacts

simulation/engine.go; simulation/registry.go; simulation/types.go;
new simulation/reception_history.go and simulation/reception_history_test.go;
simulation/reception_batch_test.go; simulation/atomicity_test.go.

## Implementation Tasks

1. Add a fixed-capacity reception ring and last reception sequence to each
   private station in registry.go. Initialize both on AddStation.
2. Implement append, clone, and chronological read helpers in
   reception_history.go following history.go's ring ownership pattern.
   Extend stationRegistry.clone to deep-copy every ring's backing array.
3. In state.deliver, after acceptance, check sequence overflow, build one
   Reception with the next station-local sequence and current.public(start),
   then append identical values to its ring and the returned batch.
4. Retain only successful reception decisions. Do not consume station sequences
   for disabled stations, coverage rejection, or configured frame loss.
5. Preserve rings through UpdateStation, including disable/re-enable. Remove
   the ring on successful RemoveStation. Failed commands preserve all records.
6. Extend tests for exact bytes/settings, wraparound, independent stations,
   complete batches larger than retention, revision edits, sequence overflow,
   and cancellation after staged reception work.

## Technical Details

All ring updates occur before the existing final commit. Errors return a zero
Batch and roll back history, sequence counters, station RNG, and aircraft state
together. Receiver is a value copy, not a reference to the current station.
New still has no stations and must never retroactively deliver birth frames.

## Verification

```text
go test ./simulation -run 'Reception|Station|Atomic|Cancel|Determin'
```

## Acceptance Criteria

Each active station retains at most 1000 records in order. Batches remain
complete. Failed mutations leave the next successful output identical to a
control engine. Old settings survive later station updates.

## Non-Goals

Changing transmission retention, scheduling, RF decisions, or station ID reuse.

