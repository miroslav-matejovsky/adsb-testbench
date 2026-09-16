---
title: "04 - Reception batches and engine integration"
dependencies: ["02-reception-model-and-coverage.md","03-station-registry-and-commands.md"]
effort: "L"
complexity: "high"
---

# 04 - Reception batches and engine integration

## Objective

Evaluate every station against every emitted transmission inside the existing
atomic mutations, and return transmissions and receptions together from the
engine commands that emit frames.

## Target Artifacts

- Extend `simulation/types.go` with `Batch`, `Reception`, and
  `MaxBatchReceptions`.
- Extend `simulation/engine.go` with reception evaluation and the new return
  type on `Advance`, `Elapse`, and `SetCount`.
- Extend `simulation/reception.go` with the random impairment draw.
- Update every existing caller: `simulation/engine_test.go`,
  `simulation/determinism_test.go`, `simulation/atomicity_test.go`,
  `simulation/history_test.go`, `simulation/snapshot_test.go`,
  `simulation/reports_test.go`, and `simulation/example_test.go`.
- Create `simulation/reception_batch_test.go`.

## Implementation Tasks

1. Define `Batch` with `Transmissions []Transmission` and
   `Receptions []Reception`. Document that a successful mutation returns both
   slices allocated and possibly empty, and that a failed or canceled mutation
   returns the zero value.

2. Define `Reception` with `TransmissionSequence uint64`, `StationID string`,
   `StationRevision uint64`, `ICAO uint32`, `Kind MessageKind`,
   `Timestamp time.Time`, `Frame [14]byte`, `SlantRangeNauticalMiles float64`,
   and `ReceivedPowerDBm float64`. Document it as self contained, and document
   the last two fields as synthetic model output.

3. Add `MaxBatchReceptions = MaxBatchFrames * MaxStations`. Document that
   reception fan-out per transmission is at most `MaxStations`, so this bound
   is a guard that the frame bound always reaches first. Update the
   `MaxBatchFrames` comment to mention the derived reception bound.

4. Change `Advance`, `Elapse`, and `SetCount` to return `(Batch, error)`.
   Leave `New`, `SetSpeed`, and `Snapshot` signatures unchanged.

5. Change the private `record` helper to take `*Batch`, append the transmission
   to `Batch.Transmissions`, and then call a new private `deliver` helper with
   the emitted transmission and the same evaluated `navState`.

6. Implement `deliver` on the staged state. Iterate stations in creation order.
   Skip disabled stations. Apply the step 02 decision helper. When it accepts,
   draw one 53-bit fraction from that station's generator and record a
   reception when the fraction is greater than or equal to
   `FrameLossProbability`. Append accepted receptions to `Batch.Receptions`.

7. Check the reception bound before appending, wrapping `ErrLimit` with the
   station identifier and the transmission sequence, using the same style as
   the existing frame and sequence bound checks.

8. Update the existing tests and examples for the new return type, including
   the failed-operation assertions that currently compare against a nil slice.

9. Add tests for fan-out, ordering, disabled stations, stream independence,
   frame-loss extremes, mid-run station changes, and batch ownership.

## Technical Details

Only these three methods emit frames, so only these three change shape.
`New` creates the initial fleet before any station can exist, so it produces no
receptions and keeps its signature. A station therefore never receives the
initial creation reports of a run, and only receives transmissions emitted at
or after its own creation instant. Identification repeats every 4.8 to 5.2
seconds and position and velocity every 0.4 to 0.6 seconds, so a newly created
station observes every active aircraft within a few seconds of virtual time.
Document this explicitly rather than redelivering retained transmissions.

Reception evaluation uses the aircraft truth `navState` already computed for
encoding at the exact event instant. Do not re-evaluate motion, do not use the
quantized wire values, and do not use a later snapshot.

`Reception.Timestamp` equals the transmission timestamp because no propagation
delay is modelled. `Reception.StationRevision` is the revision in effect when
the transmission was evaluated, which is the provenance a later consumer needs
in order to know which settings produced the decision.

Draw exactly one fraction per enabled station per transmission that passed the
deterministic checks, whatever the configured probability. A probability of 0
still consumes its draw and always accepts; a probability of 1 consumes its
draw and always rejects. Use the same `rand.New` over the stored `rand.PCG`
value already used for schedule intervals, so a staged clone never shares
generator state with the committed engine.

Ordering inside one batch is transmission sequence ascending, and within one
transmission, station creation order. Receptions carry no sequence of their
own; a consumer keys on the transmission sequence and the station identifier.

Failed or canceled mutations return the zero `Batch`, so the existing
assertions that a rejected command returns nil become assertions that it
returns `Batch{}`. Successful mutations that emit nothing still return two
allocated empty slices, preserving the existing contract.

Keep the commit boundary unchanged: receptions are staged with everything else
and become visible only when the mutation commits.

Do not retain receptions in engine state, do not add per-station counters, and
do not derive observed aircraft. Those belong to backlog 04.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation`
- `go test -race ./simulation`
- `go doc -all ./simulation` to confirm the published batch contract.

## Acceptance Criteria

- `Advance`, `Elapse`, and `SetCount` return `Batch`, and every existing test
  and example compiles and passes against the new shape.
- An engine with no stations returns an allocated empty reception slice and
  transmissions identical to the pre-change behavior.
- Each emitted transmission produces at most one reception per active station,
  ordered by transmission sequence then station creation order.
- A disabled station produces no receptions and consumes no draws, and
  re-enabling it resumes its stream where it stopped.
- Frame loss probability 0 accepts every geometrically eligible transmission
  and probability 1 accepts none, while transmissions are unchanged in both
  cases.
- Changing only the frame loss probability to another value and back reproduces
  the original receptions for the same script.
- Editing a returned `Batch`, including a reception frame array, cannot change
  engine state or later output.
- A station created between two time calls receives no transmission emitted
  before its creation instant.

## Non-Goals

Retained reception history, cursors, gaps, observed aircraft state, field
expiry, HTTP contracts, and the real-time driver.
