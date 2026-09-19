---
title: "05 - Add reproducible capacity benchmarks and measurement tooling"
dependencies: ["01-scenario-corpus.md", "02-station-state-bounds.md", "03-validation-before-settlement.md", "04-response-capacity.md"]
effort: "L"
complexity: "high"
---

## Objective

Measure P1-P4 with reproducible workloads and separate retained memory,
allocations, processing speed, and transport size. Supply evidence for supported
profiles without making ordinary tests depend on machine speed.

## Target Artifacts

- New `simulation/benchmark_test.go`, `internal/simdriver/benchmark_test.go`,
  `simulator/benchmark_test.go`, `display/benchmark_test.go`.
- `testdata/quality/capacity.json` and README.
- New `taskfile/capacity.ps1`, `taskfile/capacity.tests.ps1`.
- `Taskfile.yml`, `taskfile/README.md`.
- New `docs/evaluations/quality-capacity/README.md` and `results/README.md`.

## Implementation Tasks

1. Give workloads stable names and explicit parameters in `capacity.json`:
   seed, start time, spawn ranges, aircraft/station counts, station settings,
   selection, speed, virtual duration, retention fill target, expiry settings,
   response budgets and concurrency. No ambient wall time chooses input.
2. Add `BenchmarkCapacityEngineAdvance` subcases from the matrix below. Report
   `ns/op`, `B/op`, `allocs/op`, `frames/op`, `receptions/op`, and `virtual-ns/op`.
   Derive `ns/frame` only when frames are nonzero. Construct and prewarm the
   engine outside the timed section; assert the intended retention state before
   timing. Keep encoding, scheduling, reception decisions, mutation cloning and
   batch creation inside the measured operation.
3. Add separately named cold-construction, zero-advance, same-count and station
   update cases. State what each measures. Do not mix constructor costs with
   advance costs or hide production work inside benchmark setup.
4. For fixed-window mutation benchmarks, prepare a fresh identical engine for
   each measured window with the timer stopped. For explicitly named steady
   workloads, use stationary aircraft, advance a bounded fixed virtual duration
   repeatedly, and report actual average work counts. Never retain every batch
   in a growing slice. Use `b.ReportAllocs()` and `b.ReportMetric`, consume results,
   and fail on errors. Keep fixture initialization outside allocation timing.
5. Add `BenchmarkCapacityDriverTick` using the existing fake-clock pattern.
   Move fake time by exactly one heartbeat, then call the driver's settlement
   path. Include paused, 1x, 10x and 100x. Add a separate catch-up benchmark with
   exact 60-second virtual work and one with exact `MaxCatchUp`; reset the clock
   and engine per operation. Label cancellation tests as correctness tests, not
   throughput measurements. Never start a system ticker for a benchmark.
6. Add `BenchmarkCapacityReceptionSnapshot`, `BenchmarkCapacitySimulatorHTTP`,
   `BenchmarkCapacityDisplayRefresh`, and `BenchmarkCapacityDisplayHTTP`.
   Separate raw engine capture, DTO conversion/JSON, local refresh, HTTP source
   parse/validation, decoding and browser-facing encoding. Use the real handler
   through an in-memory RoundTripper for stable transport processing costs; add
   a separately named loopback case for actual HTTP overhead. Never present the
   in-memory result as a network benchmark.
7. Benchmark maximum retained evidence, the normal fleet, churned addresses and
   the 8000-distinct-address source from step 04. Include errors at a response
   limit and stale fallback. A fake provider must return fresh detached data or
   immutable fixture data that the consumer cannot modify between iterations.
8. Add bounded concurrency cases with 1 and 4 callers for simulator reads and
   display refresh. A single display serializes refreshes through its gate;
   measure contention instead of bypassing the gate. Record request completion
   times and errors for the support report. Use explicit caller counts, not an
   uncontrolled `RunParallel` default as the definition of the workload. Add
   `TestCapacityLatencyReport` in `simulator/capacity_test.go`: reuse its fake-clock
   fixture, serve real handlers with `httptest.Server`, warm up ten requests,
   then complete exactly 100 measured requests per profile and caller count.
   Distribute work through a channel to the exact worker count, measure each
   request with Go's monotonic clock, and collect results into a fixed-size
   slice. Sort a copy for median, nearest-rank p95 and maximum; retain error
   counts and categories. Measurements may use real time; correctness assertions
   must not depend on their speed.
9. Add a memory measurement test, `TestCapacityMemoryReport`, runnable explicitly
   outside the normal suite's expensive measurement path. Use an explicit opt-in
   environment variable, `ADSB_CAPACITY_MEASURE=1`, for the memory and latency
   measurement tests only.
   Ordinary structural capacity regressions must never be skipped. Warm the
   fixture, release all response/batch references, run GC, sample `runtime.MemStats`,
   then repeat fixed-work epochs. Keep the engine/display alive during retained
   measurements using `runtime.KeepAlive`. Report HeapAlloc, HeapObjects,
   TotalAlloc deltas, NumGC and structural counts. Record runtime-owned caching
   and measurement noise; do not assert a fixed heap-byte tolerance in unit tests.
10. Add `task capacity` invoking `capacity.ps1`. Require all measurement inputs
    explicitly: `COUNT`, `BENCHTIME`, `CPU`, and `OUT` Task variables. The script
    runs the named Go benchmarks and opted-in memory/latency reports, captures stdout and
    stderr, checks every native exit code and emits a manifest plus raw files.
    Allow only an output path resolving inside repository `.test-results/`;
    reject traversal, links and junctions. Do not delete any existing tree.
11. Capture `git rev-parse HEAD`, dirty file list, `go version`, GOOS/GOARCH,
    `GOMAXPROCS`, `GOGC`/`GOMEMLIMIT` settings, CPU model/logical count, physical
    RAM, OS build, Node version, workload checksum, exact commands, start time
    and benchmark parameters. Record unset runtime settings as unset. Capture
    enough source-state information to reproduce an uncommitted implementation;
    a revision alone is insufficient when the workspace is dirty. Save the
    first-party tracked diff and a manifest of relevant untracked source files
    with SHA-256 checksums. Copy new benchmark/helper source files with that
    manifest into transient results; omit unrelated local files and secrets.
12. Add script regression tests for rejected output paths, argument validation
    and propagation of a child failure using a stub executable. Run these small
    tests in `task all`; keep repeated benchmarks out of it. Update task docs.
    Do not require a benchmark comparison dependency; Go's output and a small
    PowerShell summary are enough for medians/min/max over repeated samples.

## Technical Details

Use this minimum matrix, keeping dimensions explicit rather than running every
possible Cartesian product:

| Group | Aircraft | Stations | Speed hundredths | Work/state |
| --- | --- | --- | --- | --- |
| Empty overhead | 0 | 0, 8 | 0, 100 | Paused tick; advance 0 and 100ms; empty histories |
| Scheduler scaling | 1, 10, 100 | 0 | 100 | Advance 1s and 60s |
| Reception scaling | 100 | 1, 2, 8 | 100 | Advance 1s; all receive; full histories |
| Radio outcomes | 100 | 8 | 100 | All disabled, out of range, loss 0, loss 1, seeded loss 0.25 |
| Driver acceleration | 100 | 8 | 100, 1000, 10000 | One 100ms real heartbeat; 0.1s, 1s, 10s virtual |
| Catch-up | 100 | 8 | 10000 | 60s virtual and exact one-hour maximum, separate cases |
| Read/refresh | 1, 100, churned | 1, 2, 8 | Explicit paused source | 1000 records per selected station |
| Source cardinality | 8000 received addresses | 8 | Explicit snapshot | Synthetic source, not 8000 simultaneously active engine aircraft |
| Contention | 100 | 8 | Explicit paused source | 1 and 4 concurrent readers/refresher calls |

For full-ring setup, advance in chunks within `MaxAdvance` until the asserted
target is reached. The engine starts with no stations; configure receivers
before aircraft creation. Do not assume a warmup duration filled a ring without
checking its record count.

Use a fixed maximum of ten measurement epochs after warmup. Each epoch performs
100 normal heartbeat-equivalent advances and ten refreshes. Station churn uses
the 1024-ID policy and does not keep trying new IDs indefinitely. Report live
engine state separately from caller-retained snapshots and profiler overhead.

For the report tests, the script temporarily sets `ADSB_CAPACITY_MEASURE=1` and
sets `GOMAXPROCS` from the explicit `CPU` argument. It runs
`go test ./simulation ./simulator ./display -run '^TestCapacity(Memory|Latency)Report$' -count=1 -v`
and restores both environment variables in a `finally` block. Forward `CPU` to
the benchmark command's `-cpu` flag as well. Reject missing or nonpositive COUNT
and CPU, an invalid BENCHTIME, and an absent OUT rather than supplying defaults.

Collect a CPU profile and allocation/live-heap profiles for one clearly named
maximum case. Commands for those profiles must identify the exact sub-benchmark;
profiling every subcase together would hide where cost came from. Do not commit
binary test executables or profiles. Store selected `pprof -top` text with the
report and raw benchmark/manifest text in its results directory.

## Verification

Commands after adding the proposed tasks and benchmark names:

```text
go test ./simulation ./internal/simdriver ./simulator ./display -run '^$' -bench '^BenchmarkCapacity' -benchmem -benchtime=1x -count=1
pwsh -NoProfile -NonInteractive -File taskfile/capacity.tests.ps1
task capacity COUNT=5 BENCHTIME=1s CPU=1 OUT=.test-results/capacity-cpu1
task capacity COUNT=5 BENCHTIME=1s CPU=4 OUT=.test-results/capacity-cpu4
```

For profiles, select one actual full sub-benchmark name emitted by the first
command and record that exact command in the report. Run measurements on an idle
machine with the same power mode. Do not run `task all` concurrently with them.

## Acceptance Criteria

- All matrix rows have named executable cases and explicit work counts.
- At least five samples per CPU setting are saved with the environment manifest.
- Cold, warm, local, HTTP, normal and catch-up costs are distinguishable.
- Memory reports separate retained state from cumulative allocation and bound
  the number of epochs and all retained test results.
- The task exits nonzero for any failed child command and cannot write outside
  its declared output area.
- No unit test asserts wall-clock throughput or a noisy heap plateau.

## Non-Goals

No scheduler heap replacement, copy-on-write framework, runtime telemetry
service, production profiling endpoint, hardware-independent latency promise,
or automatic regression threshold chosen without baseline data.
