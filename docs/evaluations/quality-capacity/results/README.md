# Stored capacity measurement evidence

Raw output of `task capacity`, copied here after the run of 2026-09-20 so it
survives the cleanup `task all` performs first. The report that interprets
these files is [one directory up](../README.md).

| File | Generating command | Contents |
| --- | --- | --- |
| `benchmarks-cpu1.txt` | `go test ./simulation ./internal/simdriver ./simulator ./display -run ^$ -bench ^BenchmarkCapacity -benchmem -benchtime 1s -count 5 -cpu 1 -timeout 120m` | Five samples of every capacity benchmark at `GOMAXPROCS=1` |
| `benchmarks-cpu4.txt` | The same with `-cpu 4` | Five samples at `GOMAXPROCS=4` |
| `reports-cpu1.txt`, `reports-cpu4.txt` | `go test ./simulation ./simulator ./display -run ^TestCapacity(Memory\|Latency)Report$ -count=1 -v -timeout 60m` with `ADSB_CAPACITY_MEASURE=1` | Retained-memory epochs and the 100-request latency measurement |
| `manifest-cpu1.txt`, `manifest-cpu4.txt` | Written by `taskfile/capacity.ps1` | Environment, source revision, dirty files, untracked-source checksums, workload checksum and the exact commands |

Both runs used the workload file with SHA-256
`3198D66708CE76843E76A01094516B15E4BAC3BC81B365D2979C957D828799C2` and
finished with `failures 0`.

To refresh this directory:

```text
task capacity COUNT=5 BENCHTIME=1s CPU=1 OUT=.test-results/capacity-cpu1
task capacity COUNT=5 BENCHTIME=1s CPU=4 OUT=.test-results/capacity-cpu4
```

then copy `benchmarks.txt`, `reports.txt` and `manifest.txt` of each run here
with a `-cpu1` or `-cpu4` suffix and update the numbers in the report.

After these runs, one test-only change was made to satisfy the linter: the
contention benchmark now checks the error of `response.Body.Close()`, and two
capacity test files do the same. No measured production path changed, and no
number above was re-derived; a fresh run is the way to pick that change up.

Each run also writes `tracked.diff` and a `source/` copy of untracked
first-party files, so an uncommitted implementation stays reproducible; those
stay under `.test-results/` because they are large and transient. Binary
profiles and test executables are never committed. Selected `pprof -top` text
may be stored here together with the exact command that produced it.
