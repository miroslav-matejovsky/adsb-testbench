---
value: "Critical"
effort: "L"
complexity: "hard"
dependencies: []
---

# ADS-B frame codec

## Area

internal/adsb.

## Work

Implement the initial 1090 MHz extended squitter subset for aircraft identification, airborne position, and velocity. Document supported type codes, address rules, bit layouts, parity validation, units, altitude reference, unavailable values, and emission cadence against authoritative specifications. Keep raw frame encoding separate from any receiver transport wrapper. Implement CPR encoding and decoding with explicit pairing, reference, and age rules.

## Acceptance criteria

Use independently sourced frames with documented provenance, expected fields, and exact encoded bytes. Test malformed lengths, parity failures, unsupported types, unavailable values, quantization boundaries, and CPR pairs near longitude wrap and latitude boundaries. A round trip through the same codec alone is insufficient.

## Dependencies

No preceding backlog item. Protocol work requires authoritative references and independently verified fixtures.
