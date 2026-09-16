---
title: "02 - Reproducible aircraft creation and absolute-time motion"
dependencies: ["01-contract-and-configuration.md"]
effort: "L"
complexity: "high"
---

# 02 - Reproducible aircraft creation and absolute-time motion

## Objective

Create reproducible aircraft and evaluate their truth state at any permitted virtual instant without accumulating caller-batch rounding error.

## Target Artifacts

- Create `simulation/aircraft.go`, `simulation/random.go`, and `simulation/motion.go`.
- Create `simulation/aircraft_test.go`, `simulation/random_test.go`, and `simulation/motion_test.go`.

## Implementation Tasks

1. Implement checked monotonic identity allocation beginning at 1. Format callsigns as TB plus six uppercase hexadecimal address digits. Keep a creation ordinal even after an aircraft is removed, and reject allocation past FFFFFE.

2. Implement the documented SHA-256 seed derivation with explicit byte order and domain tags. Store PCG values, not shared pointers or a package-global generator.

3. Sample six birth fields in the documented fixed order from the aircraft's birth generator. Take one 53-bit fractional draw for every field, including equal ranges. Correct any floating rounding to a non-degenerate upper endpoint with math.Nextafter toward Min.

4. Create an internal aircraft record holding immutable birth navigation, birth elapsed duration, initial tangent vectors, creation identity, and generator states. Avoid exposing these scheduling details in public Aircraft snapshots.

5. Implement absolute-time great-circle horizontal motion and a clamped linear pressure-altitude trajectory. Project the current tangent onto local east/north axes for velocity generation and true ground track.

6. Add analytic motion and identity tests, including zero elapsed, zero ground speed, exact bounds, date-line crossing, a pole crossing, climb/descent level-off, and deterministic creation from identical seeds.

## Technical Details

Use a spherical Earth radius of 6371000 metres and the exact unit conversions
1852 metres per nautical mile, 3600 seconds per hour, and 60 seconds per minute.
This is a documented synthetic motion model, not WGS-84 geodesic navigation.

Construct an orthonormal basis at the birth point: a unit position vector,
local north, and local east. The initial tangent is north*cos(track) +
east*sin(track). At absolute age t, rotate the position and tangent through
groundSpeedMetresPerSecond*t/radius in that plane. Derive latitude with atan2
of vertical and horizontal components and normalize longitude to [-180,180).

When horizontal vector length is at most 1e-12, use longitude 0 as an explicit
polar convention. Project the tangent onto north/east at the resulting
coordinate; use atan2(east,north) normalized to [0,360) for current true track.
At zero ground speed retain the birth track as a display convention and emit
zero east/north components. Do not claim that ground track is magnetic heading.

Pressure altitude is birth altitude plus birth vertical rate times absolute
age in minutes, clamped to [-1000,50175]. Current vertical rate becomes zero
only when motion is directed out of the reached bound. At a boundary, an
inward-pointing initial rate still moves inward. Snapshot altitude is not
rounded to the ADS-B wire increment. No geometric altitude is inferred.

Tests must compare two evaluations at the same absolute age exactly, regardless
of previously evaluated intermediate ages. Use analytic equator and meridian
paths with explicit tolerances for physical coordinates. Identity tests verify
uniqueness, uppercase eight-character callsigns, and no reuse after removals
once the public engine is integrated.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestAircraft|TestRandom|TestMotion'`
- Inspect fixed-seed expectations against the documented binary seed tuple and fixed draw order.

## Acceptance Criteria

- Matching config and ordinal produce the same birth record and random streams.
- The same absolute time yields identical navigation irrespective of intermediate evaluations.
- Positions and projected velocities remain finite across the date line and poles.
- Pressure altitude and current vertical rate agree at both altitude limits.
- Each allocated address and callsign is unique within its run.

## Non-Goals

Waypoint routes, turns commanded after birth, wind, magnetic heading, terrain, receiver geometry, and per-aircraft editing.
