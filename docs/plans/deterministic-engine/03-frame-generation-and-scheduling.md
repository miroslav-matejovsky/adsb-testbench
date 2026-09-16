---
title: "03 - ADS-B reports and independent schedules"
dependencies: ["02-aircraft-identity-and-motion.md"]
effort: "L"
complexity: "high"
---

# 03 - ADS-B reports and independent schedules

## Objective

Generate valid codec-backed frames at reproducible, independently randomized family deadlines with stable ordering and alternating CPR parity.

## Target Artifacts

- Create `simulation/reports.go` and `simulation/schedule.go`.
- Create `simulation/reports_test.go` and `simulation/schedule_test.go`.
- Reuse the existing `internal/adsb` API without changing its production implementation.

## Implementation Tasks

1. Map aircraft truth to identification, airborne position, and ground velocity codec inputs using the fixed profile below. Convert the codec Frame value into Transmission.Frame.

2. Prepare each aircraft's creation reports in identification/position/velocity order at its birth time, beginning with even CPR. Then draw its first deadline for each family.

3. Implement independent per-aircraft schedule generators and absolute elapsed deadlines. Draw identification intervals from 4800..5200 integer milliseconds and position/velocity from 400..600.

4. Implement next-event selection by deadline, then creation ordinal, then family order. Use a direct scan over at most 100 aircraft and three families instead of a generic scheduler or priority queue.

5. For each event, evaluate motion at that exact event time, encode the frame, and update only that family's deadline and random state. Toggle parity only after a successful position event in staged state.

6. Check sequence and batch bounds before appending. Wrap codec errors with aircraft, family, and virtual time while preserving their original causes.

7. Add literal engine wire fixtures derived independently from the field tables or published codec fixtures with independently computed CRC. Verify encoding through expected bytes as well as decoding.

## Technical Details

Fixed initial wire profile:

| Field group | Value |
| --- | --- |
| Header | DF17 through the codec, allocated ICAO, CA=5 (airborne). |
| Identification | TC4, category 0 (unspecified), generated callsign. |
| Position | TC11, surveillance status 0, supplement false, time-synchronized false, available pressure altitude, per-aircraft even/odd CPR. |
| Velocity | TC19 subtype 1, intent-change false, IFR capability false, NACv 0, signed current east/north ground components, available pressure vertical rate with BarometricVerticalRate true. |
| Other velocity fields | Heading and airspeed nil, TrueAirspeed false, GNSSMinusBaroFeet nil. |

These are deliberate synthetic profile constants, not defaults from a config
file. The engine does not claim calibrated position integrity or a complete
ADS-B operational-status profile. Do not invent GNSS-minus-baro altitude zero.

Project ground components from the absolute-time trajectory rather than from
the original bearing. With the chosen 1000-knot and 10000-ft/minute limits,
engine-generated components fit ordinary subtype-1 bins. Preserve genuine
zero as an available Measurement. Let the codec perform quantization and CRC.

The first odd position appears after the first randomized position interval,
not at birth alongside even. The birth reports therefore do not immediately
give a global CPR pair. Every later family deadline is based on its previous
deadline. Do not draw random intervals when a caller merely reads a snapshot.

Use stable event keys even when a deliberately constructed test makes all
deadlines equal. For a survivor comparison across count changes, compare
per-aircraft timestamps, kinds, and raw frames; global transmission sequences
naturally differ when other aircraft emit or are removed.

Generate one fixed stationary scenario fixture with explicit birth ranges,
and preserve literal expected frames in reports_test.go. State the derivation
next to those literals. Codec round trips alone are not an independent oracle.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestReport|TestSchedule'`
- `go test ./internal/adsb` to confirm the reused codec's independent fixtures remain green.

## Acceptance Criteria

- Every emitted frame decodes to the correct aircraft and expected message family.
- Creation emits exactly three reports per added aircraft and starts CPR with even.
- Successive deadlines stay on the one-millisecond grid and within family bounds.
- All three random streams are independent of reads and of each other's event counts.
- Simultaneous events have the specified total ordering.
- Generated truth, decoded navigation, and documented quantization tolerances agree.

## Non-Goals

Receiver reception, observed-track accumulation, transport output, surface frames, magnetic heading, airspeed, and new codec formats.
