---
title: "01 - Station contract and explicit configuration"
dependencies: []
effort: "M"
complexity: "medium"
---

# 01 - Station contract and explicit configuration

## Objective

Define the station value types, their documented domains, their validation
rules, and the two new error categories, before adding any station state or
reception behavior.

## Target Artifacts

- Create `simulation/station.go` and `simulation/station_test.go`.
- Extend `simulation/types.go` with `MaxStations`, `ErrNotFound`, and
  `ErrConflict`.
- Extend `simulation/doc.go` with the station configuration and unit contracts
  implemented in this step.

## Implementation Tasks

1. Define `StationConfig` in `simulation/station.go` with exactly these fields:
   `ID string`, `Enabled bool`, `LatitudeDegrees float64`,
   `LongitudeDegrees float64`, `SiteElevationMetres float64`,
   `AntennaHeightMetres float64`, `AntennaGainDBi float64`,
   `SensitivityDBm float64`, `SystemLossDB float64`, and
   `FrameLossProbability float64`. Document every unit and domain on the field.

2. Define `Station` with `Config StationConfig`, `Revision uint64`, and
   `CreatedAt time.Time`. Document that `CreatedAt` is a virtual instant and
   that `Revision` starts at 1 and increments on every accepted update.

3. Add `MaxStations = 8` to the existing limit block in `simulation/types.go`,
   documented as engine policy rather than a configuration default.

4. Add `ErrNotFound` for an unknown station identifier and `ErrConflict` for a
   revision mismatch to the existing sentinel block in `simulation/types.go`.
   Document that `ErrInvalid` still covers out-of-domain values, that a
   duplicate or reserved identifier is an `ErrInvalid` case, and that exceeding
   `MaxStations` wraps `ErrLimit`.

5. Implement private identifier validation: 1 to 64 bytes, each byte an ASCII
   letter, digit, hyphen, or underscore. No trimming and no case folding; the
   identifier is preserved exactly as supplied.

6. Implement private settings validation covering every numeric domain in the
   table below, rejecting NaN and infinity first, and returning an `ErrInvalid`
   error that names the offending field.

7. Add table-driven tests for every bound, every rejected identifier shape,
   meaningful zero values, NaN and infinity, and the accepted extremes.

## Technical Details

| Field | Unit | Accepted domain |
| --- | --- | --- |
| `LatitudeDegrees` | degrees | [-90, 90] |
| `LongitudeDegrees` | degrees | [-180, 180] |
| `SiteElevationMetres` | metres above the model sphere | [-500, 9000] |
| `AntennaHeightMetres` | metres above site elevation | [0, 500] |
| `AntennaGainDBi` | dBi | [-10, 40] |
| `SensitivityDBm` | dBm | [-140, 0] |
| `SystemLossDB` | dB | [0, 30] |
| `FrameLossProbability` | fraction | [0, 1] |

Validation is a single private function over `StationConfig` so that
`AddStation`, `UpdateStation`, and `EstimateCoverage` all apply exactly the
same rules. It performs no normalization: unlike `Config.StartTime`, no station
field has a canonical form, so an accepted configuration is stored byte for
byte as supplied.

Zero values are meaningful and must survive: gain 0 dBi, system loss 0 dB,
antenna height 0 m, site elevation 0 m, frame loss probability 0, latitude 0,
and longitude 0 are all valid. `Enabled` false is a valid, fully configured
station that receives nothing.

Follow the existing package layout. Station value types and their validation
live in `station.go`; only the shared limit and sentinel blocks are extended in
`types.go`. Use the same error-wrapping style already used by
`normalizeConfig`, naming the field and the accepted range in the message.

Do not publish `Engine` station methods, a registry, a reception model, or
coverage in this step. Do not change any existing method signature.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation -run 'TestStationConfig|TestStationIdentifier'`
- `go doc -all ./simulation` to inspect the new field and error documentation.

## Acceptance Criteria

- Every out-of-domain numeric value and every malformed identifier has a
  deterministic `ErrInvalid` result naming the field.
- NaN and infinity are rejected for all eight numeric fields.
- All accepted domain endpoints validate successfully.
- Meaningful zero values are accepted and are not replaced.
- `ErrNotFound` and `ErrConflict` exist, are documented, and are distinct from
  `ErrInvalid` and `ErrLimit` under `errors.Is`.
- No new public type imports or aliases an internal or third-party type.

## Non-Goals

Station storage, revisions, engine commands, randomness, the reception model,
coverage, batch changes, and configuration-file parsing.
