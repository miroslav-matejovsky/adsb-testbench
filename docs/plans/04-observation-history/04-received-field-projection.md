---
title: "04 - Received-field projection"
dependencies: ["03-cursor-paging.md"]
effort: "L"
complexity: "high"
---

# Received-field projection

## Objective

Build a bounded aircraft projection exclusively from received frame evidence.

## Target Artifacts

new simulation/observation_decode.go and simulation/observation_decode_test.go;
simulation/observations.go; internal/adsb APIs used without changing the codec.

## Implementation Tasks

1. Implement a private projection function accepting deduplicated received
   evidence, snapshot Now, and validated expiry policy. It must receive no fleet,
   navState, transmission-history, or current receiver configuration parameter.
2. Replay evidence in transmission sequence order. Decode exact Frame bytes
   with adsb.Decode. Verify decoded ICAO and message family against metadata.
   Wrap unexpected decode/metadata failures with transmission context and
   preserve causes. Do not silently omit corrupt evidence.
3. Maintain identity, altitude, velocity, and reconstructed horizontal position
   independently per ICAO. Altitude can be known from one position frame even
   when horizontal coordinates remain unknown. A newer payload with unavailable
   components replaces those components with unknown; unrelated fields persist.
4. Retain latest even and odd samples per aircraft, age them at each new
   reception instant, and call DecodeGlobal only for a valid opposite pair.
   ErrCPR means a documented unusable pair; keep the previous fix and its time.
   Propagate other unexpected errors. Use Fix.At, including even timestamp ties,
   as horizontal position ObservedAt.
5. Preserve both samples' evidence for a successful fix. Missing, same-parity,
   or too-old pairs produce no new fix. Use no local CPR fallback.
6. At snapshot Now remove fields whose age exceeds their own TTL. Keep known
   empty callsigns and genuine zero measurements distinct from unknown.
   Velocity derived ground speed/track require available non-over-range east
   and north components; at zero speed leave track unknown. Preserve raw
   component availability and over-range values.
7. Compute LastReceivedAt from latest retained evidence for each aircraft;
   never use it to refresh another field. Omit aircraft with no fresh observed
   fields. Test each field independently and test snapshots at exact TTL and
   TTL + 1ns boundaries.

## Technical Details

Decode CPR at event time, then expire accepted fixes at query time. A valid fix
can remain fresh under Position TTL even after its pair exceeds MaxCPRAge.
Expected unusable CPR is partial state, not an engine mutation failure.
All field timestamps are virtual reception times, never snapshot query times.

## Verification

```text
go test ./simulation -run 'ObservationDecode|ObservationExpiry|ObservationCPR'
```

## Acceptance Criteria

Identity-only, velocity-only, altitude-only, and full states are represented.
A missed message never advances the corresponding timestamp. Corrupt evidence
returns an error with its original cause. Position uses only selected evidence.

## Non-Goals

External frame ingest, local-reference CPR, display track continuity policy.

