# Assessment

## Feasibility and dependencies

The codec, deterministic engine, receiver histories, runtime serialization,
and server lifecycle already exist. The work is concentrated in DTO parsing,
conversion, handlers, and the currently empty display package. The required
engine addition is a read-only reception snapshot, following the lock/copy
pattern in `simulation/observation_snapshot.go`.

No new external dependency is required. Use the standard JSON, HTTP, URL,
context, and synchronization packages, the existing codec, and `testify/require`
in tests. Tests may import simulator fixtures while production `display`
remains independent of simulator/engine types. Keep `.go-arch-lint.yml` rules.

Total effort is XL across nine reviewable steps. Steps 01-04 deliver the
simulator contract/API prerequisite for manager work. Steps 05-08 deliver and
prove display behavior. No unimplemented UI or command package is a dependency.

## Decisions and risks

| Risk | Concrete decision | Verification |
| --- | --- | --- |
| Separate pages mix virtual instants | Capture raw selected histories once under the engine lock | Step 02 coherent-read and mutation tests |
| Decoded snapshots omit unpaired CPR | Display consumes complete retained raw evidence | Step 05 lone-half then valid-pair fixture |
| Warm display remembers evicted evidence | Rebuild every refresh; retain only one last-good publication | Step 07 warm/cold equivalence after eviction |
| Multi-receiver duplicates refresh ages | Group by run/transmission sequence and union provenance before decoding | Step 05 duplicate/disagreement fixtures |
| HTTP and local validation diverge | Both feed the same request/response semantic validators and decoder | Step 08 shared corpus |
| JSON erases missing-versus-zero distinction | Strict recursive presence parsing, then concrete semantic validation | Step 01 remove each required leaf and compare explicit zero |
| Restart mixes reused ICAOs and cursors | Fresh run ID per runtime; clear old caches on detected run change | Step 07 same ICAO in different runs |
| Sources reuse a run ID after restart | Document host run-ID obligation; reject same-run time/sequence regression | Step 07 regression fixtures; same-ID indistinguishable resets cannot be inferred |
| HTTP bound rejects a supported snapshot | Encode representative maximum-cardinality fixtures and compare exact bytes to explicit test settings | Step 08, including one-byte-over failures |
| Retained aircraft exceeds active aircraft limit | Bound tracks by evidence cardinality, not current fleet count | Step 08 count churn with historical targets |
| Source fails after partial read | Validate into temporary state; publish atomically; expose stale fallback with error | Steps 06-07 late read and decode failures |
| CPR reference creates false position | Global CPR only with codec age checks; no truth/receiver/local-reference fallback | Step 05 invalid and absent-pair fixtures |
| HTTP mounting drops a path prefix | Relative handlers and prefix-preserving URL construction | Steps 04, 06, 07 nested-prefix tests |

Full retained snapshots trade repeated decoding for bounded, consistent
results. This is acceptable for the current eight-station, thousand-record
limits. This plan validates those bounds and failure behavior. Workload
benchmarking and changes to supported capacity belong to backlog item 11.

## Validation strategy

Run the commands specified by each step after its implementation. Unit tests
use caller-supplied virtual instants, the existing runtime fake clock, channel
barriers, and controllable HTTP round trippers. No unit test waits for a sleep
or a race against elapsed real time. Use `httptest` for HTTP integration.

Shared fixtures assert independently specified decoded values, rather than
only comparing two implementations that might share the same bug. Reuse
published codec fixtures; keep their attribution with the fixture data.
Use simulation observations as an additional received-only parity check.

Final validation is `go test -race ./simulation ./simulator ./simulatorapi
./display`, `git diff --check`, and mandatory `task all`. Record results and
any platform limitation in progress; do not mark an unavailable check passed.

## Rollback and incremental delivery

There is no persisted schema or data migration. Each step keeps existing
package tests passing. New service constructors and handlers can be omitted
from host composition until accepted. A failed refresh keeps only the valid
same-selection last-good snapshot; a detected new run invalidates old data.

If implementation must stop, record the exact pending step and failure in
root `.todo` and `progress.md`. Do not remove existing user changes or restore
the retired backlog files. No commit, reset, or automated source rollback is
part of executing this plan.
