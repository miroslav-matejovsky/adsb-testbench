---
title: "06 - Backlog acceptance and failure-path evidence"
dependencies: ["05-engine-commands-and-history.md"]
effort: "L"
complexity: "high"
---

# 06 - Backlog acceptance and failure-path evidence

## Objective

Prove the backlog's reproducibility, atomicity, timing, identity, and retention guarantees through observable output and deterministic negative cases.

## Target Artifacts

- Create `simulation/determinism_test.go` and `simulation/atomicity_test.go`.
- Extend `simulation/engine_test.go`, `simulation/history_test.go`, and `simulation/reports_test.go` with cross-component assertions.

## Implementation Tasks

1. Create one shared test fixture builder that assigns every Config and Range field explicitly. Keep examples distinct enough to test zero, fixed-value, and varied ranges.

2. Compare independently constructed engines after fixed ordered scripts. Assert exact concatenated Transmission slices and complete final Snapshot equality.

3. Partition virtual and real durations using fixed tables and a separately seeded test generator. Keep operation totals within MaxAdvance per call and apply control changes at identical virtual instants.

4. Add canceled-before-work and canceled-during-work tests. Use a test-only counting context for late cancellation, with enough staged reports to ensure work preceded the observed cancellation; never use sleeps or wall time.

5. After every rejected or canceled operation, compare its snapshot with the pre-operation snapshot, then replay an identical valid suffix against an unaffected control engine. This verifies hidden RNG, parity, sequence, carry, and identity state too.

6. Use package-internal state fixtures only for counter exhaustion and deliberately invalid staged report inputs that cannot be reached through valid public configuration. Keep production code free of test hooks.

7. Test concurrent independent snapshot reads and serialized mutations under the race detector. Assert safety and snapshot invariants, not a particular concurrent command order.

8. Verify generated report payloads against absolute truth and fixed codec tolerances. Account for the codec's documented CPR-zone boundary rejection.

## Technical Details

Required named acceptance matrix:

| Test family | Required comparisons |
| --- | --- |
| TestDeterminismReplay | Same explicit config and script: equal birth frames, all later frames, and final snapshots; different non-degenerate seeds change at least one known fixture field. |
| TestDeterminismVirtualPartitions | One advance versus uniform, irregular, and exact-deadline partitions; both even and odd reports appear with no duplicates. |
| TestDeterminismRealPartitions | Integer scaling at 0.01x, 0.33x, 1x, and 100x, with nanosecond carry and complete frame equality. |
| TestDeterminismControls | Pause/resume, direct stepping while paused, carry preserved through speed changes, and no settling hidden in SetSpeed/SetCount. |
| TestDeterminismSurvivors | Add/remove newer aircraft; compare surviving aircraft's timestamps, kinds, and bytes while ignoring global sequence offsets. |
| TestEngineZeroAircraft | Time moves with no aircraft; no frames emit; later creation occurs at the then-current virtual time. |
| TestEngineIdentityLifetime | Count down/up does not reuse addresses or callsigns; zero and FFFFFF are never emitted. |
| TestHistoryCompleteBatch | A valid time call emits more than 1000 frames; returned batch is complete and snapshot history is exactly its retained tail. |
| TestAtomicityRejectedInput | Invalid count/speed/duration and scaled-duration limit do not change any visible or subsequent output. |
| TestAtomicityCancellation | Pre-canceled, deadline-exceeded, and late-observed cancellation return nil output; valid retry matches control. |
| TestAtomicityExhaustion | Near sequence, address, elapsed, and date limits: rejection preserves state and can occur after staged work without partial commit. |
| TestAtomicityCodecFailure | A deliberately malformed internal candidate produces a wrapped codec error after staged work; committed state is unaffected. |
| TestSnapshotOwnership | Mutating snapshot config copies, aircraft/history slices, and returned frame arrays has no engine effect. |
| TestEngineConcurrentAccess | Concurrent snapshot access does not race or expose inconsistent history/time/aircraft state. |

Use require assertions. A counting context must obey a monotonic Err result:
nil before its fixed cancellation point, then always context.Canceled or
context.DeadlineExceeded as configured. Do not assert an exact production
number of Err calls; test several cancellation depths and compare each failed
operation to its control. This is an internal algorithm test seam, not a public
context implementation recommendation.

Numerical checks: barometric wire altitude within 12.5 feet of truth; ordinary
ground components within 0.5 knot; vertical rate within 32 feet/minute. Decode
CPR globally only for pairs that satisfy the codec's zone requirements. Near a
boundary, test individual encoded fractions and local reconstruction from a
valid nearby reference instead of expecting every even/odd pair to succeed.

Use explicit creation and scheduled-event timestamps when computing expected
truth. Do not compare a historical frame to the aircraft's later snapshot.

## Verification

These commands apply to subsequent implementation, not this planning task.

- `go test ./simulation ./internal/adsb`
- `go test -race ./simulation` in an environment with the required Go race-detector toolchain.
- `go test ./simulation -run 'TestDeterminism|TestAtomicity' -count=10` to confirm the fixed scripts do not depend on test order or timing.

## Acceptance Criteria

- Every named matrix row has an executable assertion tied to the backlog.
- Each failed mutation is followed by a matching-control suffix, not just a snapshot comparison.
- Cancellation and timing tests contain no sleeps or reads of time.Now.
- Independent wire expectations supplement codec round trips.
- Concurrent tests assert safety without promising deterministic concurrent mutation ordering.

## Non-Goals

Throughput qualification, network/browser integration, receiver-model tests, and arbitrary correctness claims based solely on coverage percentage.
