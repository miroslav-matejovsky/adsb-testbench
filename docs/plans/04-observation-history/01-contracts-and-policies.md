---
title: "01 - Contracts and policies"
dependencies: []
effort: "M"
complexity: "medium"
---

# Contracts and policies

## Objective

Define engine query types and shared transport DTOs with explicit cursor,
availability, expiry, and provenance semantics.

## Target Artifacts

simulation/types.go; new simulation/observations.go;
new simulation/observation_config_test.go; simulatorapi/doc.go;
new simulatorapi/observations.go and simulatorapi/observations_test.go.

## Implementation Tasks

1. Add ReceptionHistoryLimit = 1000 and MaxHistoryPageSize = 1000 in
   simulation/types.go. Extend Reception with Sequence and Receiver Station.
   Keep StationID and StationRevision consistent with Receiver in construction.
2. Add ReceptionCursor {RunID, StationID, AfterSequence}, HistoryRequest
   {StationID, Cursor *ReceptionCursor, Limit}, and ReceptionPage with RunID,
   StationID, Now, Records, OldestSequence, LatestSequence, NextCursor, Gap,
   HasMore, and RetentionLimit. Bounds describe the retained ring, not the page.
3. Add ObservationRequest {StationIDs, Expiry} and ObservationExpiry with
   Identity, Position, Altitude, Velocity time.Duration fields. Require each TTL
   > 0, even for an empty selection. Reject more than MaxStations distinct IDs.
4. Define ObservationSnapshot with RunID, Now, canonical StationIDs, per-station
   retention bounds/truncation, effective expiry policy, and Aircraft.
   Define ObservedAircraft with ICAO, LastReceivedAt, and optional Identity,
   Position, BarometricAltitude, and Velocity field records.
5. Each present field record contains decoded values, ObservedAt, and source
   evidence. Source evidence contains exact frame, transmission sequence,
   virtual timestamp, and all selected retained Reception copies. Position
   references both CPR halves. Optional velocity scalars retain availability,
   over-range, and vertical-rate source; never use zero as unknown.
6. Add equivalent dependency-free DTOs in simulatorapi. Use decimal strings for
   uint64 sequences/revisions, 28 uppercase hex characters for frames, UTC
   RFC3339Nano timestamps, and explicitly named duration nanoseconds represented
   as decimal strings. Unknown fields serialize as null; collection fields
   serialize as arrays. JSON numbers must not carry uint64 values.
7. Document caller responsibility for fresh Config.ID on each restart. Add
   validation/serialization tests for zero values, absent optional fields,
   sequence values above 2^53, and complete reception-time settings.

## Technical Details

Keep engine types native Go values and transport types plain DTOs. Do not
import simulation into simulatorapi or reverse the dependency. HTTP parsing,
error status mapping, and engine-to-DTO conversion are backlog 06.
Use ErrInvalid for bad limits/TTLs and ErrConflict for stale run identity.
Limit must be in [1, MaxHistoryPageSize]; no implied page-size default.

## Verification

```text
go test ./simulation ./simulatorapi
go doc ./simulation.ObservationRequest
go doc ./simulatorapi
```

## Acceptance Criteria

All new exported types document units and availability. Invalid policies and
limits fail deterministically. JSON round trips preserve exact sequences,
frames, and known zero values. No new architecture dependency is introduced.

## Non-Goals

HTTP handlers, runtime ID generation, and configuration file parsing.

