---
title: "07 - Publish measured support profiles and complete validation"
dependencies: ["05-benchmarks-and-measurements.md", "06-retention-errors-and-restarts.md"]
effort: "M"
complexity: "medium"
---

## Objective

Turn capacity evidence into specific, reproducible support statements. Correct
Q6 and documentation affected by the implementation. Complete the final checks.

## Target Artifacts

- `docs/evaluations/quality-capacity/README.md`, `results/README.md`, selected
  raw benchmark text, environment manifests and response-size reports.
- `docs/evaluations/README.md`, root `README.md`, `configs/README.md`.
- `simulation/doc.go`, `simulator/doc.go`, `display/doc.go`,
  `internal/simdriver/doc.go`, `taskfile/README.md`, `ui/browser/README.md`.
- This plan's `progress.md`, `docs/plans/README.md`, and the dependency link in
  `docs/backlog/12-replay-and-traffic-extensions.md` when the plan is retired.

## Implementation Tasks

1. Run the complete checks before final measurement. Copy selected benchmark
   logs and manifests into the documented results directory after `task capacity`
   finishes. Keep binary profiles and executable outputs under `.test-results`.
   Document every stored file, its generating command and the workload checksum.
2. Build separate tables for structural policies, response budgets, processing
   throughput, retained memory and end-to-end request latency. Include medians
   and min/max for five benchmark samples. For request latency, collect at least
   100 completed requests per declared concurrency level and report median,
   p95, maximum and errors. State whether the transport is in-memory or loopback.
3. Apply the support rule below to the measured candidate profiles. Record a
   row for every candidate, including failures. Publish only passing profiles
   as supported on the stated environment. No implementation constant is
   increased based solely on a successful low-cardinality result.
4. Record CPU/profile evidence for P1-P4. State the dominant functions, B/op,
   allocs/op, and the practical effect on tested profiles. This completes the
   observations even if the existing implementation meets the support target;
   no scheduler or staging rewrite is required just to close this plan.
5. Describe retained-state limits separately from process memory. Include the
   lifetime station budget, raw snapshot cardinality, caller-owned batch sizes,
   display last-good state, browser record/gap/tombstone limits, and the effect
   of concurrency. State that HTTP byte bounds are checked after marshaling
   and do not cap all transient allocations.
6. Explain measured retention coverage in virtual seconds. At roughly 4.2
   messages per aircraft per virtual second, 1000 receptions shared by a
   100-aircraft all-receive station cover only about 2.4 virtual seconds on
   average. Label this as an estimate; report measured oldest/newest times too.
   Field expiry is an upper age bound, not a promise that history retains an
   identity for its full lifetime. Acceleration shortens real-time coverage.
7. Update root README with a link to the capacity report, exact task commands,
   supported profile summary and distinction between accepted speed and measured
   sustained speed. Update configs documentation with final numeric budgets,
   workload assumptions and the effective run-ID profile. Do not call sample
   configuration values defaults.
8. Replace the stale manager-backlog sentence in `simulator/doc.go` with the
   implemented relationship: the simulator provides its service; `ui` provides
   manager pages; `testbench` composes them. Update engine/driver documentation
   for lifetime limits and pre-settlement validation. Update display fixture
   provenance and transport-specific capacity wording where it claims stronger
   equivalence than byte-limited HTTP can provide.
9. Update the evaluation index and progress table with actual results and paths.
   Verify every Q1-Q6 finding has the promised evidence. Retain precise
   limitations; do not describe a profile as supported when one stage failed.
10. Run `task all`. Because it cleans measurement outputs first, ensure permanent
    evidence is already copied, or run `task capacity` afterwards and copy it
    then. Run `git diff --check` and inspect the diff for accidental generated
    binaries, profiles, vendor changes or unrelated edits. Do not commit.
11. When every deliverable and acceptance criterion is implemented, follow the
    plans index convention: move lasting documentation into the capacity report,
    repoint backlog item 12 to it, and retire this completed plan directory and
    its active-index entry. If implementation is unfinished, keep the plan
    active and record the remaining work in `.todo` as well as `progress.md`.

## Technical Details

Candidate profiles must include:

| Profile | Configuration | Required evidence |
| --- | --- | --- |
| Shipped examples | Exact configured fleets/stations, 1x, declared poll cadence | All browser/source budgets; combined and separate paths |
| Maximum cardinality at 1x | 100 aircraft, 8 all-receive stations, full histories | Tick cost, local/HTTP refresh, one and four clients |
| Maximum cardinality at 10x | Same, speed 1000 | Sustained tick work, response latency and allocation rates |
| Maximum cardinality at 100x | Same, speed 10000 | Same metrics; support only if measured rules pass |
| Maximum source shape | 8000 conforming raw records/distinct partial tracks | Bytes, validation/decode/serialization; identify synthetic source |
| Suspended driver | Maximum fleet/stations, exact catch-up limit | Completion/error behavior, peak temporary memory; separate from sustained support |

Measurement decision rule for sustained support:

- For the normal 100ms heartbeat workload, the slowest of the five benchmark
  samples' mean operation times must be <=50ms. This reserves processing room
  for reads and scheduling; it is a support selection rule, not a unit test.
- For each declared client concurrency, p95 of loopback observation requests
  must fit within half the smallest applicable configured operation timeout,
  with no failed request in the fixed 100-request measurement. Also report the
  maximum and state if it exceeds a configured poll interval.
- All deterministic structural, response-size and failure tests must pass for
  that profile. A profile must not silently drop frames to meet throughput.
- Report the minimum passing candidate (the shipped 1x configuration) and every
  higher passing row. If the shipped profile fails, do not claim completion:
  identify the measured failing operation and add a concrete bounded fix step
  with a regression/benchmark before proceeding. Do not lower its acceptance
  threshold or quietly weaken its workload.

These are conservative project support criteria on the recorded host, not
guarantees for all hardware, simultaneous unrelated work, or remote networks.
Catch-up and maximum received-address cases need correct bounded behavior but
are reported separately from real-time pacing guarantees.

## Verification

```text
task all
task capacity COUNT=5 BENCHTIME=1s CPU=1 OUT=.test-results/capacity-final-cpu1
task capacity COUNT=5 BENCHTIME=1s CPU=4 OUT=.test-results/capacity-final-cpu4
git diff --check
git status --short
```

Review relative documentation links and ensure the persisted report includes
actual numbers, environment data, workload inputs and a pass/fail row for every
candidate. Confirm no pending implementation rows are marked complete.

## Acceptance Criteria

- Q1-Q6 have verified fixes and P1-P4 have recorded measurement/support outcomes.
- Reported support follows the stated rule with no invented or missing results.
- Permanent evidence survives `task all` cleanup and is linked from the indexes.
- Documentation matches implemented responsibilities, policies and settings.
- `task all` and the explicit measurement tasks pass; the working tree contains
  only intended first-party source, tests, configuration and documentation.

## Non-Goals

No automatic tuning, extrapolation to unmeasured hardware, hard CI timing gate,
production observability service, or completion claim while any fix is pending.
