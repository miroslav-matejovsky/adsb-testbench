---
title: "01 - Public contract and explicit configuration"
dependencies: []
effort: "M"
complexity: "medium"
---

# 01 - Public contract and explicit configuration

## Objective

Define the engine's public value types, configuration rules, fixed limits, and error categories before adding stateful behavior.

## Target Artifacts

- Create `simulation/types.go`, `simulation/config.go`, and `simulation/config_test.go`.
- Extend `simulation/doc.go` with the configuration and unit contracts implemented in this step.

## Implementation Tasks

1. Define Config, SpawnConfig, Range, Aircraft, Transmission, MessageKind, Snapshot, and HistorySnapshot exactly as specified in the plan overview. Give the three message kinds values 1, 2, and 3 in identification/position/velocity order.

2. Define MaxAircraft, HistoryLimit, MaxAdvance, and MaxBatchFrames with the values in the overview. Document them as engine policy, not values filled into missing configuration.

3. Add ErrInvalid and ErrLimit sentinel errors. Invalid caller values wrap ErrInvalid; valid requests exceeding representable time, identity, sequence, or batch capacity wrap ErrLimit. Context failures will preserve context.Canceled or context.DeadlineExceeded without relabeling them.

4. Implement private configuration validation and normalization. Check nonblank ID; nonzero start time in years 1-9999; count 0-100; speed 0-10000; finite range endpoints; Min <= Max; and all six domain limits.

5. Copy the accepted configuration and normalize StartTime to UTC without a monotonic component. Keep Seed=0, count=0, speed=0, and valid zero range endpoints unchanged.

6. Add table-driven validation tests for every bound, meaningful zero, equal endpoints, NaN/infinity, inverted intervals, blank IDs, and date extremes.

## Technical Details

Use six occurrences of the same concrete Range type, with no generic configuration
framework. Sampling intervals are half-open when non-degenerate; equal endpoints
mean an exact value. A TrackDegrees upper bound of 360 is allowed only when
Min < Max, since no generated track may equal 360. Latitude endpoints must lie
in [-85,85]; longitude in [-180,180]; altitude in [-1000,50175]; speed in
[0,1000]; and vertical rate in [-10000,10000].

The effective snapshot Config retains original InitialAircraftCount while its
SpeedHundredths changes with SetSpeed. Its remaining fields are immutable.
All planned public values use standard-library or simulation-owned types.
Use [14]byte for raw frames rather than an alias to the internal codec's Frame.

Do not publish Engine constructors or methods with placeholder bodies. Their
implementation belongs to step 05 after the pure components exist.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestConfig|TestRange'`
- `go doc -all ./simulation` to inspect complete public field and error documentation.

## Acceptance Criteria

- Every out-of-domain value has a deterministic ErrInvalid result.
- Valid zeros survive normalization; no caller value is silently replaced.
- All proposed public record fields and numerical units are documented.
- No public type imports or aliases an internal or third-party type.

## Non-Goals

Engine mutation, random generation, scheduling, time advancement, and configuration-file parsing.
