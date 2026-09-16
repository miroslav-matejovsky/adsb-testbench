---
value: "High"
effort: "M"
complexity: "medium"
dependencies: []
---

# Real-time driver and simulator lifecycle

## Area

internal/simdriver, simulator, and internal/httpserver.

## Work

Serialize pacing and control commands. Measure elapsed real time, settle at the previous speed before edits, and bound catch-up after host suspension. Provide injected clock control and cancellation. Define runtime ownership, command behavior after stop, error propagation, and HTTP drain before pacing shutdown.

## Acceptance criteria

Use a fake clock without sleeps to test delayed ticks, pause, speed changes, catch-up limits, cancellation, and command ordering. Test startup failures and shutdown without leaked background work.

## Dependencies

Use the implemented [simulation engine](../../simulation/doc.go), including its
virtual clock, speed scaling, atomic mutations, and bounded transmission
history.
