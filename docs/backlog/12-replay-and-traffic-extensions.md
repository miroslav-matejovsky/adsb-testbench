---
value: "Medium"
effort: "XL"
complexity: "hard"
dependencies: ["11-quality-and-capacity.md"]
---

# Replay, transport output, and richer scenarios

## Area

simulation, simulator, and internal/adsb.

## Work

Add deterministic scenario loading and raw-frame record/replay with explicit timing and provenance. Introduce bounded TCP/UDP output adapters after selecting and documenting the receiver framing required by target consumers. Add waypoint routes and surface traffic as separately testable increments. Evaluate additional ADS-B message families and 978 MHz UAT only with explicit scope, codec fixtures, and configuration.

## Acceptance criteria

Test exact replay order and timing, malformed recordings, pause and acceleration, slow/disconnected consumers, and bounded output queues. Verify transport failures preserve engine determinism and new message families decode against independent fixtures.

## Dependencies

- [11-quality-and-capacity](11-quality-and-capacity.md)
