---
value: "Critical"
effort: "L"
complexity: "hard"
dependencies: ["01-adsb-codec.md"]
---

# Deterministic aircraft engine and virtual time

## Area

simulation.

## Work

Create explicitly configured aircraft identities, initial positions, altitude, ground speed, track, and vertical rate. Define units and distinguish track from heading and altitude references. Advance three-dimensional motion and schedule each supported message family using virtual time. Provide seeded creation, aircraft count changes, pause, speed scaling, bounded frame history, run identity, and ordered transmission sequences. Keep wall-clock reads and pacing outside the engine. Document configuration limits and reject invalid input before mutation.

## Acceptance criteria

Test identical config and ordered calls producing identical frames; split and combined time advancement; pause/resume; speed scaling preserving reported speed; zero aircraft; history eviction; address uniqueness; and failed or canceled mutations leaving state unchanged.

## Dependencies

- [01-adsb-codec](01-adsb-codec.md)
