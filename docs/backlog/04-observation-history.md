---
value: "High"
effort: "L"
complexity: "hard"
dependencies: []
---

# Received observations and bounded history

## Area

simulation and simulatorapi.

## Work

Separate generated transmissions, per-station receptions, and observed aircraft. Retain exact received frames, reception-time receiver settings, virtual timestamps, and provenance. Define bounded station histories with cursors, gaps, and restart identities. Build selected-station observation snapshots with field-specific timestamps and expiry. Support partial aircraft state when identification, velocity, or a usable CPR pair has not been received.

## Acceptance criteria

Test loss of one CPR frame, independent field expiry, station selection and deduplication, retention gaps, station removal, restart cursors, and virtual-time aging during an empty scenario. Confirm missed transmissions never refresh received fields.

## Dependencies

Use the implemented [simulation engine](../../simulation/doc.go), including its
stations, revision-checked station commands, published reception model, and the
per-transmission reception records returned with every mutation batch.
