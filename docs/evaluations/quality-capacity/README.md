# Measured capacity and supported profiles

What this bench was measured to do, on one recorded host, with the exact
workloads and commands that produced the numbers. These are conservative
project support statements, not guarantees for other hardware, for a machine
doing other work, or for remote networks.

Structural limits come from the engine and are enforced everywhere. Response
budgets are example configuration values derived from a conservative
worksheet. Throughput, retained memory and request latency are measurements
of the host recorded below, repeated on a fresh run whenever the
implementation or the host changes.

- Workload inputs: [testdata/quality/capacity.json](../../../testdata/quality/capacity.json)
- Raw output and environment manifests: [results](results/README.md)
- Response budget derivation: [testdata/quality](../../../testdata/quality/README.md)
- Configuration values: [configs](../../../configs/README.md)

## Measurement environment

| Item | Value |
| --- | --- |
| Commands | `task capacity COUNT=5 BENCHTIME=1s CPU=1 OUT=.test-results/capacity-cpu1` and the same with `CPU=4` |
| Date | 2026-09-20, 02:06 to 02:37 UTC |
| Source revision | `00fd78efc6dd1e9d527d4d6cea1b9c5043c9fa20` plus the uncommitted changes of this work; each run stored `tracked.diff` and SHA-256 checksums of every untracked first-party file |
| Workload checksum | `3198D66708CE76843E76A01094516B15E4BAC3BC81B365D2979C957D828799C2` (`testdata/quality/capacity.json`) |
| Go / OS | go1.27.1 windows/amd64, Microsoft Windows 11 Business 10.0.26200 |
| CPU / memory | Intel Core Ultra 7 265H, 16 logical processors, 68,111,437,824 bytes |
| GOMAXPROCS | 1 and 4, set by the task; `GOGC` and `GOMEMLIMIT` unset |
| Samples | 5 per case per CPU setting, `-benchtime=1s` |
| Node | v26.9.0 (browser tests only) |

Every number below is a median of the five samples unless a minimum and
maximum are given. `CPU=1` is the pessimistic setting: the whole runtime
shares one processor.

## Structural policies

| Policy | Value | Enforced in |
| --- | --- | --- |
| Active aircraft | 100 | `simulation.MaxAircraft` |
| Active stations | 8 | `simulation.MaxStations` |
| Station IDs per run | 1024, including removed ones | `simulation.MaxStationIDsPerRun` |
| Retained transmissions | 1000 | `simulation.HistoryLimit` |
| Retained receptions per station | 1000 | `simulation.ReceptionHistoryLimit` |
| Raw snapshot records | 8000 | a selection of 8 full rings |
| Virtual time per advance | 60 s | `simulation.MaxAdvance` |
| Driver heartbeat / catch-up | 100 ms / 1 h | `simdriver.Heartbeat`, `simdriver.MaxCatchUp` |
| Speed | 0 to 10000 hundredths | `simulation.MaxSpeedHundredths` |

## Response budgets

The budgets are derived, not measured: each is the largest conservative bound
of its path times 1.25, rounded up to a whole MiB. The derivation and the
bound of every response shape are in
[testdata/quality](../../../testdata/quality/README.md); the shipped values
are in [configs](../../../configs/README.md).

Largest shapes the engine actually produced, through the real handler, with a
128-byte effective run ID and 64-byte station identifiers:

| Response | Records | Distinct aircraft | Provenance copies | Bytes | Bound |
| --- | --- | --- | --- | --- | --- |
| Raw snapshot, 8 stations, 1 aircraft | 8000 | 1 | 40 | 5,359,994 | 7,714,932 |
| Raw snapshot, 8 stations, 100 aircraft | 8000 | 100 | 3,560 | 5,371,714 | 7,714,932 |
| Simulator observations, 8 stations, 100 aircraft | 8000 | 100 | 3,560 | 2,540,675 | 25,643,125 |
| Display refresh body, same | 8000 | 100 | 3,560 | 2,540,795 | 25,657,229 |
| History page, 1000 records | 1000 | 1 | - | 672,426 | 965,952 |
| Truth, 100 aircraft with full history | - | 100 | - | 154,562 | 197,580 |
| Stations / metadata | - | - | - | 4,646 / 2,074 | 7,032 / 3,422 |

The former 4 MiB example budget cannot carry an eight-station snapshot. A
regression test asserts that, so the corrected budgets cannot be reverted by
accident.

## Throughput

Engine work per operation in milliseconds, with the work counts the engine
reported:

| Case | CPU=1 | CPU=4 | Work per operation |
| --- | --- | --- | --- |
| Advance 1 s, 1 aircraft, no station | 0.019 | 0.023 | 4.2 frames, 4.7 us/frame |
| Advance 1 s, 10 aircraft, no station | 0.070 | 0.068 | 42 frames, 1.5 us/frame |
| Advance 1 s, 100 aircraft, no station | 1.046 | 0.967 | 420 frames, 2.4 us/frame |
| Advance 1 s, 100 aircraft, 8 all-receive stations | 5.137 | 3.169 | 420 frames, 3,360 receptions |
| Advance 1 s, 100 aircraft, 8 stations, loss 0.25 | 4.785 | 2.989 | 420 frames, 2,517 receptions |
| Advance 1 s, 100 aircraft, 8 stations, loss 1 | 3.217 | 2.252 | 420 frames, 0 receptions |
| Advance 100 ms, no aircraft, 8 stations | 0.579 | 0.470 | no work; staging only |
| Zero advance, full histories | 0.699 | 0.639 | no work; staging only |
| No-op fleet command, full histories | 0.707 | 0.618 | no work; staging only |
| One station edit, full histories | 0.661 | 0.698 | no work; staging only |
| Cold construction of 100 aircraft | 0.540 | 0.514 | 300 creation frames |

An aircraft emits 4.2 frames per virtual second, so a 100-aircraft fleet
emits 420 frames per virtual second and, with eight all-receiving stations,
3,360 receptions.

One real heartbeat of driver settlement with the maximum fleet and station
count, the workload that must keep up with real time:

| Speed | CPU=1 median (min-max) | CPU=4 median (min-max) | Virtual time per tick |
| --- | --- | --- | --- |
| Paused | 0.66 (0.63-0.70) | 0.54 (0.52-0.58) | 0 |
| 1x | 1.70 (1.60-1.80) | 1.40 (1.29-1.50) | 100 ms |
| 10x | 4.84 (4.61-5.13) | 4.27 (4.01-4.37) | 1 s |
| 100x | 47.65 (43.68-49.90) | 32.00 (31.53-34.00) | 10 s |

Reading and serving the maximum shapes, in milliseconds:

| Path | CPU=1 | CPU=4 | Notes |
| --- | --- | --- | --- |
| Raw capture and DTO conversion, 8000 records | 14.2 | 12.4 | 3.6 MB per call in 15 allocations |
| JSON serialization of that snapshot | 18.6 | 16.7 | 4,506,410 bytes |
| Simulator request, in-memory transport | 402 | 318 | request build, handler, parse and validate |
| Simulator request, loopback socket | 388 | 323 | the same over a real connection |
| Display refresh from an 8000-record source | 75.3 | 57.5 | validation, decoding and publication |
| Display browser request, 8000 records | 160 | 137 | including response encoding |
| Display source parse, 8000 records | 308 | 144 | reading and parsing one 4.3 MB body |
| Refresh that fails at the response limit | 146 | 70.9 | the work is done, then the body is refused |
| Stale republication after a source failure | 8.2 | 4.7 | no decode; last-good state is kept |

Suspended-driver catch-up, reported separately from real-time pacing:

| Case | CPU=1 | CPU=4 | Allocation |
| --- | --- | --- | --- |
| 60 s of virtual work at 100x | 290 ms | 192 ms | 276 MB cumulative |
| Exactly the 1 h catch-up limit at 100x | 16.5 s | 11.4 s | 16.5 GB cumulative in 1.5 M allocations |

## Retained memory

Measured by `TestCapacityMemoryReport` with the maximum fleet, eight full
rings and ten epochs of fixed work (100 heartbeat advances and ten
observation snapshots each), sampling `runtime.MemStats` after a forced
collection and keeping the engine alive:

| Sample | CPU=1 | CPU=4 |
| --- | --- | --- |
| Retained heap after warmup | 2,098,120 bytes, 1,137 objects | 2,160,144 bytes, 1,160 objects |
| Retained heap after ten epochs | 2,098,472 bytes, 1,141 objects | 2,170,568 bytes, 1,216 objects |
| Cumulative allocation over ten epochs | 2,842,733,192 bytes | 2,842,751,216 bytes |
| Collections | 355 | 643 |

Retained state is flat while cumulative allocation grows: the engine stages a
copy per mutation and returns detached batches and snapshots, so that cost is
temporary, not retained. Structural counts are asserted; heap numbers are
reported, never gated, because they depend on the collector and on whatever
else the machine is doing.

Retained state is bounded by the structural policies, not by process memory:
at most 100 aircraft, 1000 transmissions, eight reception rings of 1000
records, at most 1024 reserved station identifiers of at most 64 bytes each,
one last-good display snapshot, and browser records bounded by
`aircraftUi.maxHistoryRecords` with gap markers bounded separately. Batches
and snapshots handed to a caller are extra and live as long as that caller
keeps them. Response budgets bound transmitted bodies only: both HTTP layers
marshal a response before checking its size, so a budget does not bound peak
allocation, and concurrent requests multiply the temporary cost.

## Request latency

`TestCapacityLatencyReport` serves the real handler over a loopback
`httptest.Server`, warms up with ten requests, then completes exactly 100
measured requests per caller count. Each request reads the full 8000-record
raw snapshot of eight stations with a 128-byte run ID.

| Callers | CPU=1 median / p95 / max | CPU=4 median / p95 / max | Errors |
| --- | --- | --- | --- |
| 1 | 89.3 / 443.4 / 734.5 ms | 99.5 / 179.1 / 305.7 ms | 0 |
| 4 | 247.3 / 469.5 / 622.3 ms | 136.0 / 242.1 / 273.9 ms | 0 |

The transport is loopback, not a network. The smallest configured operation
timeout on this path is the display source timeout of 4 s, so the support
rule allows 2 s at p95. The largest observed request, 734.5 ms at CPU=1, also
stays below the 1 s browser poll interval.

## Supported profiles

Support rule, applied to the measurements above:

1. For the 100 ms heartbeat workload, the slowest of the five samples must
   average at most 50 ms per tick, leaving room for reads and scheduling.
   This is a support selection rule, not a unit test.
2. For each declared caller count, p95 of loopback observation requests must
   fit in half the smallest applicable configured timeout, 2 s here, with no
   failed request in the fixed 100-request measurement.
3. Every deterministic structural, response-size and failure test must pass.
   No profile may drop frames to meet throughput.

| Profile | Tick, slowest sample | Request p95 | Deterministic tests | Supported |
| --- | --- | --- | --- | --- |
| Shipped examples: 8 aircraft, 2 stations, 1x, 1 s poll | Covered by the strictly larger maximum-cardinality 1x row | 443 ms (CPU=1), 179 ms (CPU=4) | Pass | Yes |
| Maximum cardinality at 1x | 1.80 ms (CPU=1), 1.50 ms (CPU=4) | as above | Pass | Yes |
| Maximum cardinality at 10x | 5.13 ms (CPU=1), 4.37 ms (CPU=4) | as above | Pass | Yes |
| Maximum cardinality at 100x | 49.90 ms (CPU=1), 34.00 ms (CPU=4) | as above | Pass | Yes, with no margin at CPU=1 |
| Maximum source shape: 8000 conforming records with 8000 distinct addresses | Not applicable; synthetic source | 144-308 ms to parse, 57-75 ms to decode | Pass | Yes, as a source shape |
| Suspended driver at the exact 1 h catch-up limit | 11.4-16.5 s for one catch-up | Not applicable | Pass | Bounded and correct; reported separately from real-time pacing |

The shipped example configuration is supported a fortiori: its fleet and
station count are strictly smaller than the maximum-cardinality profile that
passed on the same code path, and its budgets are checked by
`internal/cli/capacity_test.go` and the simulator capacity tests. No separate
throughput number is invented for it.

The 100x row passes on this host and is published with an explicit warning:
at `GOMAXPROCS=1` its slowest sample was 49.90 ms against the 50 ms rule. A
slower machine, a busier machine or a larger per-tick workload will not keep
up at 100x. An accepted speed is an input the engine accepts, never a
throughput promise.

Retention coverage in virtual time is short at high cardinality. A station
hearing a 100-aircraft fleet receives about 420 records per virtual second,
so its 1000-record ring covers about 2.4 virtual seconds; with one aircraft
the same ring covers about 238 virtual seconds. The capacity fixtures show
this directly: filling every ring took 300 virtual seconds with one aircraft
and 20 with a hundred. Field expiry is an upper age bound, not a promise that
history keeps an identity for its whole lifetime, and acceleration shortens
real-time coverage by the speed factor.

## P1-P4 observations

| Observation | Measured cost | Effect on the tested profiles |
| --- | --- | --- |
| P1: state, history and registry cloning copies full rings even for a no-op mutation | Zero advance, a no-op count command and one station edit each cost 0.62-0.71 ms and 1,918,720 bytes in 13 allocations with full histories | Real but not decisive: a 100x tick costs 32-48 ms, of which staging is under 1 ms. The supported profiles need no copy-on-write change |
| P2: the scheduler scans all aircraft and all three families for every event | 4.7 us per frame at 1 aircraft, 1.5 us at 10 and 2.4 us at 100; a 60 s advance of 100 aircraft costs 48.8-63.7 ms for 25,200 frames | Scales with emitted work at these counts and is not the dominant cost of a tick once stations are attached |
| P3: driver settlement reads a full snapshot for speed and returns complete batches | 1.4-1.7 ms per tick at 1x, 32-48 ms at 100x; one catch-up of the 1 h limit costs 11.4-16.5 s and 16.5 GB cumulative allocation | Acceptable for pacing. Catch-up is bounded and correct but is a burst of temporary allocation, so it is reported separately from sustained support |
| P4: capture copies and sorts records, adapters validate, handlers marshal before checking bytes | Capture 12.4-14.2 ms, serialization 16.7-18.6 ms, source parse 144-308 ms, display decode 57-75 ms, full browser request 137-160 ms for 8000 records | Parsing and decoding dominate a maximum read, not capture. A refused oversized response still pays for the whole body, which is why budgets are derived from a bound instead of tuned down |

No scheduler or staging rewrite was required to close these observations.

## Failure and recovery coverage

Every row is executed at the layer that owns it. Transport-only failures,
such as a malformed body, exist on HTTP alone and are labeled as such; a
local provider is never made to fabricate one.

| Case | Publication and recovery | Owning tests |
| --- | --- | --- |
| Same-run transport outage | Last good data for the same selection is explicitly stale; the next success replaces it completely | `display` `TestDisplayFailureAndRecoveryMatrix` (local, HTTP), `TestDisplayReturnsStaleDataWithItsError` |
| Oversized HTTP payload | `response_limit`, body closed, nothing partially decoded | `display` `TestDisplayFailureAndRecoveryMatrix` (HTTP), `TestHTTPSourceResponseLimitIsExact`, `TestCapacityMaximumSourceFitsTheShippedBudgets` |
| Truncated or malformed JSON | Source-invalid; the original read cause is kept | `display` `TestDisplayFailureAndRecoveryMatrix` (HTTP), `TestHTTPSourceTruncatedAndCloseFailuresKeepTheirCause` |
| Corrupt frame in a valid envelope | The whole refresh fails; no valid prefix is mixed with a last-good suffix | `display` `TestDisplayFailureAndRecoveryMatrix` (local, HTTP), `TestDecodeFailsTheWholeSnapshotOnCorruptFrames` |
| Invalid data carrying a new run ID | The old fallback is cleared before the decode failure | `display` `TestDisplayNewRunClearsPreviousStateEvenWhenItsPayloadFails` |
| Old-run cursor or mutation | Conflict with the current run identity; no mutation, no cursor reuse | `simulation` `TestCorpusEngineScenarios/restart`, `simulator` `TestCorpusRestartAcrossTransports`, `inspector.spec.mjs` |
| Regressed time or sequence in one run | Source-invalid; the next forward snapshot succeeds | `display` `TestDisplayFailureAndRecoveryMatrix`, `TestDisplayRejectsSameRunRegression` |
| Changed selection during an outage | No fallback from the previous selection | `display` `TestDisplayFailedSelectionChangeShowsNoOtherSelection` |
| Cancellation while waiting at the display gate | The source is not called; the gate stays usable | `display` `TestDisplayRefreshHonorsCancellationDuringAdmission` |
| Station removal while a history request is in flight | The invalidated page fails; no records are appended | `display` `TestDisplayHistoryInFlightDuringStationRemoval`, `inspector.spec.mjs` |
| Replacement run while a history response is held | Old rows and cursors are not restored | `inspector.spec.mjs`, `restart.spec.mjs` |

Browser-side bounds and their tests are listed in
[ui/browser](../../../ui/browser/README.md).

## Coverage of the assessment findings

The findings and observations come from the review recorded in the
implementation plan of this work.

| Finding | Fix | Evidence |
| --- | --- | --- |
| Q1: shipped response budgets did not cover every allowed station selection | Budgets derived from a conservative worksheet and applied to all three example files | `simulator` `TestCapacityShippedBudgetsCarryMaximumEngineShapes` (asserts the former 4 MiB value cannot carry an eight-station snapshot), `internal/cli` `TestShippedResponseBudgetsFollowTheCapacityProfile` |
| Q2: removed station identifiers accumulated for the whole run | `simulation.MaxStationIDsPerRun = 1024`, checked before anything is reserved, published in metadata | `simulation` `TestStationLifetimeIDBudget`, `TestStationLifetimeBudgetThroughPublicCycles` |
| Q3: a rejected reuse of a removed ID settled time first | The driver asks the engine to validate before it settles | `internal/simdriver` `TestDriverRejectedStationAdditionDoesNotSettle`, `TestDriverRejectedStationCommandsDoNotSettle`, `simulator` `TestRejectedStationCommandsDoNotSettleOnEitherTransport` |
| Q4: a station revision could wrap from MaxUint64 to zero | An exhausted revision returns `ErrLimit` before settings change; removal still works | `simulation` `TestStationRevisionNeverWraps`, `TestStationValidatorsRejectExhaustedRevisions` |
| Q5: a display test overstated the independence of its frames | The synthetic test is named as synthetic; literal published frames are read from the shared corpus | `display` `TestDecodeBuildsFieldsFromSyntheticEncodedFrames`, `TestDecodeLiteralReferenceSnapshot`, `TestBothSourcesDecodeTheLiteralReferenceSnapshot` |
| Q6: simulator documentation called the manager UI backlog work | `simulator/doc.go` states the implemented ownership | `simulator/doc.go` |

## Limitations

- One host, one operating system, two `GOMAXPROCS` settings, five samples per
  case. Nothing here extrapolates to other hardware.
- The transport is loopback. A real network adds its own latency and failure
  modes, and a display source has its own timeout and byte budget.
- Response budgets bound transmitted bodies, not peak memory and not
  concurrent requests.
- The 8000-record source shape is synthetic. The engine cannot hold 8000
  simultaneously active aircraft; it can retain 8000 records naming that many
  distinct received addresses.
- `simulatorapi.MinResponseBytes` (2048) assumes error text does not expand
  under JSON escaping. The measured worst-case envelope is 6,985 bytes: far
  below every shipped budget, but above that documented minimum. It is
  recorded in the repository `.todo`.
