# Deterministic fixtures and measured capacity

Status: planned. This promotes backlog item 11; it does not mark its implementation complete.

## Goal

Create a reusable, independently attributed ADS-B fixture corpus. Measure the
engine, runtime, HTTP boundaries, display, and browser at explicit capacities.
Fix the defects recorded in [assessment.md](assessment.md), prove retained state
is bounded, and publish supported operating profiles with reproducible evidence.

Read the root [README](../../../README.md) first. Paths in the implementation
steps are relative to the repository root. Run commands from that root in
PowerShell unless a step says otherwise. Never commit changes.

## Scope and architecture

The engine remains deterministic and receives elapsed time from its caller.
The driver remains its only mutating owner in a runtime. Displays continue to
derive tracks from raw receptions. Both local and HTTP transports use the same
semantic rules. Browser tests retain controlled clocks and response barriers.

Add data files under `testdata/quality/`, test and benchmark files beside their
packages, and a small standard-library-only test fixture loader under
`internal/qualitytest/`. Production packages must not import that loader.
Keep protocol fixtures separate from engine-generated scenarios: agreement
between this encoder and this decoder is not independent protocol evidence.

Add one fixed engine policy, `MaxStationIDsPerRun = 1024`, to bound reserved
station identifiers. Preserve the existing rule that a removed ID is never
reused in its run. Add read-only station-command validation methods so the
driver can reject invalid commands before settling elapsed time. Guard revision
overflow. These changes preserve package dependency directions.

Response budgets remain explicit configuration. Choose example budgets from the
measured and conservatively bounded response shapes, then test the actual
configuration files. Distinguish hard structural limits from measured real-time
support. An accepted speed of 100x is not a throughput guarantee on every host.

## Deliverables and order

| Step | Deliverable | Dependencies |
| --- | --- | --- |
| [01](01-scenario-corpus.md) | Shared reference frames, scenario data, independent expected values | None |
| [02](02-station-state-bounds.md) | Bounded station-ID lifetime and revision-overflow rejection | None |
| [03](03-validation-before-settlement.md) | Engine-backed prechecks and no-settlement regressions | 02 |
| [04](04-response-capacity.md) | Response-size proof, boundary tests, corrected example budgets | 01, 02, 03 |
| [05](05-benchmarks-and-measurements.md) | Benchmarks, explicit workloads, reproducible measurement task | 01, 02, 03, 04 |
| [06](06-retention-errors-and-restarts.md) | Long-run bounds, failure recovery, transport and browser scenarios | 01, 02, 03, 04 |
| [07](07-supported-profiles-and-handoff.md) | Published results, supported profiles, synchronized docs, final checks | 05, 06 |

Steps 01 and 02 can be implemented independently. Step 05 captures performance
evidence; it does not authorize replacing the scheduler or weakening atomicity.
Use [progress.md](progress.md) to record completion with evidence.

## Success criteria

- Literal reference frames and expected decoded fields have provenance and
  explicit tolerances. Tests in the codec, display, and transport acceptance
  layer consume the corpus. Browser fixtures consume relevant shared data.
- Every discovered defect in assessment findings Q1-Q6 has its specified fix
  and regression or documentation check. Performance observations P1-P4 have
  measured costs and a documented support decision.
- Station churn cannot grow the reserved-ID set beyond 1024 entries. Revision
  exhaustion and rejected station commands preserve state and future output.
- Supported raw snapshots, decoded observations, history pages, and truth
  responses fit every applicable producer and consumer byte limit. Oversized
  responses fail with a complete error envelope, never partial success.
- Benchmarks report time, allocations, work counts, and response bytes, with
  CPU, memory, OS, tool versions, source revision, and exact commands recorded.
- Retention tests cover more than one ring wrap, aircraft churn, selection
  changes, failures, and restarts without sleeps or wall-clock aging assertions.
- `task all` passes. The separate measurement task passes and its results are
  stored before cleanup can remove transient artifacts.

## Boundaries

Replay products, TCP/UDP adapters, routes, surface traffic, new ADS-B families,
UAT, and cross-platform byte determinism belong to backlog item 12. This corpus
is test input, not a public scenario-loading or replay API. No dependency update
or scheduler rewrite is required by this plan.

## Review baseline

Analysis date: 2026-09-19. Source revision:
`d3b4c7b4a95b4a20d7c5ab3296a0d3e77ca6ad66`.
The planning review ran `task all` successfully: 1110 Go tests, 46 JavaScript
unit tests, 82 Chromium tests, external embedding checks, formatting and lint.
This is baseline evidence, not validation of the pending implementation.
