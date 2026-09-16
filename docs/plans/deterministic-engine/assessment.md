# Feasibility and implementation assessment

## Feasibility

The engine fits the existing package boundary. The codec already handles
DF17 identification, barometric position, TC19 velocity, CRC, and CPR.
Only the standard library and the existing testify test dependency are needed.

The main work is preserving reproducibility through asynchronous message
deadlines, floating-point motion, count changes, and failed mutations.
This plan fixes those contracts before implementation rather than copying the
AIS engine's one-second reporting schedule.

## Dependencies and evidence

- Backlog item 02, now implemented and removed: intended behavior and acceptance.
- [Codec documentation](../../../internal/adsb/doc.go): actual APIs, representable
  ranges, timing guidance, reference rules, and error behavior.
- [Codec fixtures](../../../internal/adsb/testdata/README.md): externally sourced
  frames already used to verify the codec.
- [Architecture rules](../../../.go-arch-lint.yml): simulation may use adsb.
- Current module: Go 1.27.1, go-adsb v0.4.1, testify v1.12.1.
- `go doc math/rand/v2.PCG` confirms explicitly seeded generator state and
  binary state support. Use the pinned toolchain for deterministic expectations.

Protocol cadence is already documented and sourced in the codec. This plan
chooses a one-millisecond uniform grid within those published interval ranges.
It adds no independent claim of complete transponder or aviation-model fidelity.

## Risks and mitigations

| Risk | Required mitigation | Evidence |
| --- | --- | --- |
| Batch boundaries change floating-point paths | Evaluate each position from immutable birth state at an absolute elapsed instant. | Exact final snapshots and concatenated frames for many duration partitions. |
| Random consumption changes when calls are split or aircraft are removed | Per-aircraft, per-family PCG states; fixed birth draw order; stable event ordering. | Remaining aircraft's own frame bytes and timestamps match a control run, ignoring global sequence differences. |
| Shared RNG pointers survive a staged clone | Copy generator values with aircraft state; never retain a Rand wrapper pointing into committed state. | Cancel after staged creation/emission, retry, and compare all future output to a fresh control. |
| Pause or fractional scaling loses time | Integer hundredths and carried remainder; defined paused behavior. | Nanosecond-scale partitions at 0.01x, 0.33x, 1x, and 100x. |
| Signed arithmetic overflows before rejection | Check scaling, lifetime, sequence, and identity bounds before their arithmetic and allocations. | Duration extremes and near-overflow internal fixtures fail without state changes. |
| CPR pair crosses a latitude-zone boundary | Generate correct individual fractions; tolerate the codec's documented global-pair rejection at zone crossings. | Global decoding away from boundaries; trusted local decoding or raw fraction checks at boundaries. |
| Geodesic path reaches a pole | Vector trajectory, explicit polar longitude convention, local tangent projection. | Pole-crossing snapshots and frames remain finite and deterministic. |
| Climb exits encodable altitude | Clamp the absolute altitude trajectory and derive zero vertical rate at the boundary. | Position altitude and velocity agree before, at, and after each limit. |
| History or returned pointers mutate committed state | Use frame arrays and deep copies of all slices on public boundaries. | Mutate returned snapshots and batches, then compare subsequent output. |
| Cancellation tests become timing-dependent | Controlled counting context and direct internal fixtures, without sleeps or production hooks. | Deterministic late cancellation and retry tests. |
| Public package accidentally leaks internal types | Own public value types with raw [14]byte frames. | External consumer example compiles without internal imports. |

## Bounded work

An advance covers at most 60 virtual seconds and 100 aircraft. Using a
conservative extra event allowance, each aircraft contributes at most
151 position, 151 velocity, and 13 identification reports, or 31500 total
reports for the fleet. The 32000-frame batch cap covers that bound.
Creation returns at most 300 reports, below the 1000-report history limit.

Keep a sorted aircraft slice and scan at most three deadlines per aircraft to
find the next event. Use this direct approach before introducing a heap.
The history ring retains at most 1000 value records. Mutation staging copies
at most 100 aircraft with four small RNG states each and one bounded history.
Complete output batches are additional bounded transient storage.

These are resource bounds, not throughput promises. Capacity qualification at
maximum speed remains backlog 11; do not add performance claims or a benchmark
project to this increment. High speed changes how callers supply durations,
not the amount of virtual work allowed in one call.

## Verification strategy

Each implementation step defines focused tests. Step 06 adds the acceptance
matrix and failure-path evidence. Step 07 runs package tests, the public
consumer compile check, architecture validation, and the repository's
`task all` as future implementation checks.

Tests use testify/require, fixed configurations, explicit timestamps and
durations, and bounded deterministic operation sequences. They compare
observable frames and snapshots; internal fixtures are limited to otherwise
unreachable counter-exhaustion and late-failure states. No sleep, real clock,
network service, or statistically flaky distribution assertion is required.

The implementation ran these checks, including `task all`.

## Rollback

Implementation adds behavior to a currently unused package. If a step fails
its checks, keep the step incomplete and restore the affected first-party
changes using its reviewed diff, preserving unrelated user edits.
No database, persisted state, wire migration, deployment, or vendor change is
required. Do not use bulk repository resets and do not create commits.

If an implementation session stops incomplete, record concrete outstanding
actions in the repository `.todo` and keep backlog 02 open. Preparing this
complete plan alone is not an unfinished implementation session.
