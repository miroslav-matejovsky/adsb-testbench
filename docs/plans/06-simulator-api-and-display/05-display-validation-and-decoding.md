---
title: "05 - Common display validation and received decoding"
dependencies: ["01-contracts-and-explicit-configuration.md", "02-coherent-reception-snapshot.md"]
effort: "L"
complexity: "high"
---

# 05 - Common display validation and received decoding

## Objective

Create one pure semantic validator and decoder that turns bounded raw
reception evidence into partial aircraft observations for either source.

## Target Artifacts

- New `display/validate.go`, `decode.go`, `errors.go`, and associated tests.
- Existing `display/config.go`, `doc.go`, and shared observation DTOs.
- New `display/testdata/README.md` and small attributed fixture files.

## Implementation Tasks

1. Implement shared local request validators and snapshot/page validators.
   Parse canonical strings into temporary native values. Check run identity,
   normalized selected stations, complete retention membership, timestamp
   bounds, exact hex, numeric finiteness/domains, and record cardinality.
   Validate envelope identity/selection/time before records and distinguish
   an invalid envelope from an invalid payload. Add `SourceError` with
   operation, validated run ID when available, and an unwrap-compatible cause.
2. Validate station reception sequence ordering/continuity and matching
   oldest/latest bounds. Empty retention has zero bounds and empty records.
   Transmission sequences can skip; copies for one transmission must agree
   on frame, ICAO, kind, and timestamp. Reject duplicate station copies,
   contradictory receiver snapshots, unselected stations, and future records.
3. Validate historical provenance: station ID/revision equals embedded receiver
   ID/revision, creation time is no later than reception, receiver settings are
   within published domains, and records using the same station revision have
   identical settings. Preserve enabled state, RF data, and historical settings
   rather than replacing them with the current station configuration.
4. Validate history pages against the actual request: station/run/cursor,
   explicit limit, per-station ordered sequences, next cursor, retention bounds,
   hasMore, and gap. For an absent cursor, truncated retention is represented
   by bounds, with gap false as in the engine. Reject unexpected cursor advance
   on empty pages and impossible records outside retained bounds.
5. Group valid raw records by transmission, preserving all receiver copies in
   station-ID order. Decode each unique frame with `adsb.Decode`; compare the
   decoded address and message family to metadata. An invalid or unsupported
   frame fails the complete snapshot with its sequence and original cause.
6. Accumulate the latest identification, altitude, velocity, and CPR samples by
   ICAO in transmission order. Keep the last received timestamp independent of
   individual field timestamps. A newly received unavailable altitude clears
   altitude; unavailable velocity components remain null. An empty decoded
   callsign is a received empty value, not a missing identity.
7. Pair even/odd barometric CPR only for the same ICAO using `adsb.DecodeGlobal`
   at the current evidence timestamp. Honor its ten-second inclusive age bound
   and even-sample tie rule. An `ErrCPR` pair failure retains the prior accepted
   fix until its configured expiry; other decode errors fail the refresh. Keep
   lone halves available for subsequent records within this snapshot only.
8. Apply independent virtual-time field expiry at snapshot `now`, fresh at
   exactly lifetime and absent after it. Ground speed/track derive only from
   available, non-over-range east/north ground components. Zero speed has no
   track. Preserve magnetic heading, airspeed type, vertical-rate source,
   over-range thresholds, and GNSS-minus-baro availability independently.
9. Return `simulatorapi.ObservationSnapshot` with aircraft sorted by ICAO and
   detached field evidence. Drop an aircraft only when all four observed field
   groups are absent. Carry selection, expiry, now, and retention unchanged.
   Test evidence/provenance arrays and nested pointer ownership.
10. Build exact expected-value tests for independent published frames, lone
    CPR halves, ten-second and expiry boundaries, same-time samples, mismatched
    ICAOs, corrupt CRC, all supported velocity subtypes, null/zero/over-range,
    multi-station copies, unavailable replacement, and reception-time edits.

## Technical Details

The validator uses `simulatorapi` data and the codec, not simulation truth or
engine imports. Receive-path scalar values are checked even for local Go
providers, which can supply NaN, invalid pointers, or inconsistent metadata
without passing through JSON. HTTP syntax validation is additional to this
common semantic path, not a replacement.

Use global CPR only. Neither simulator coordinates, receiver coordinates,
nor stale fields become a local CPR reference. Do not extrapolate motion or
infer position from altitude. Pair validity and displayed field lifetime are
separate: a valid historical pair may still support a position until the
explicit position lifetime expires.

## Verification

```text
go test ./display ./internal/adsb
task arch-lint
```

Tests call pure functions with fixed virtual timestamps. Compare expected
values with codec-appropriate numeric tolerances, not exact truth coordinates.

## Acceptance Criteria

- Every displayed field is reproducible from its returned raw evidence.
- Invalid contracts or corrupt frames produce no partial decoded snapshot.
- Unknown, known zero, and over-range values remain distinct.
- CPR and field expiry boundary fixtures pass without real-clock dependencies.
- Production display imports conform to the existing architecture rules.

## Non-Goals

HTTP fetching, long-lived track storage, local CPR, extrapolation, or map UI.
