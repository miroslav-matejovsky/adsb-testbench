---
value: "High"
effort: "L"
complexity: "hard"
dependencies: ["06-simulator-contract-and-api.md"]
---

# Display sources and received-track decoding

## Area

display.

## Work

Implement in-process and HTTP observation sources with one semantic validation and decoding path. Accumulate received identification, velocity, and CPR state using explicit pairing and expiry rules. Preserve unknown fields and receiver provenance. Bound HTTP bodies and deadlines, preserve source error causes, and reset caches on run changes.

## Acceptance criteria

Test both sources with the same fixtures; corrupt frames and contracts; timeouts; partial aircraft state; expired CPR pairs; source restarts; and bounded history gaps. Confirm no display state comes from simulator truth.

## Dependencies

- Implemented [ADS-B codec](../../internal/adsb/doc.go), including explicit
  unavailable/over-range values and CPR age/reference requirements.
- [06-simulator-contract-and-api](06-simulator-contract-and-api.md)
