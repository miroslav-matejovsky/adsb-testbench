# Assessment

## Feasibility and current coverage

The work fits the current architecture. Reuse the fake clocks and existing
acceptance harnesses instead of adding a new runtime or public clock injection.
Expect more work than the original M estimate: eight subsystems, configuration
regressions, tooling, and evidence documentation are involved. The steps split
the work into separately reviewable changes.

Existing evidence to retain:

| Area | Current implementation and tests |
| --- | --- |
| Codec | `internal/adsb/frame_test.go`, `velocity_test.go`, `cpr_test.go`; literal external frames attributed in `internal/adsb/testdata/README.md` |
| Engine determinism | `simulation/determinism_test.go`, `reception_determinism_test.go`; replay, partition equivalence, controls and independent random streams |
| Atomicity | `simulation/atomicity_test.go`, `reception_batch_test.go`; state and identical-future comparisons after failures |
| Driver | `internal/simdriver/driver_test.go`; measured elapsed time, pause, catch-up, cancellation between chunks, terminal lifecycle |
| Transport equivalence | `simulator/acceptance_test.go`; local/HTTP observations, provenance, paging, restarts and eviction |
| Display | `display/bounds_test.go`, `source_test.go`, `decode_test.go`; one last-good snapshot, response bounds and virtual-time expiry |
| Browser | `ui/browser/restart.spec.mjs`, `inspector.spec.mjs`, `embedding.spec.mjs`; real-handler restart, bounded history, lifecycle and barriers |

Do not replace this coverage with a single large end-to-end test. Extend the
tests at the layer owning each invariant.

## Findings and required fixes

### Q1: shipped response budgets do not cover all allowed station selections

`configs/combined.json`, `simulator.json`, and `display.json` use 4,194,304-byte
response budgets. `simulator/acceptance_test.go` uses 16,777,216 bytes in its
helpers, and `TestMaximumCardinalitySnapshotFitsTheFixtureBudgets` measures a
local snapshot with one aircraft and eight full station rings. It does not
exercise that maximum through its HTTP source or the shipped configuration.

Reproduced with:

```text
go test ./simulator -run '^TestMaximumCardinalitySnapshotFitsTheFixtureBudgets$' -count=1 -v
```

Result on Go 1.27.1, Windows/amd64: 8,000 records, **4,362,387 bytes**.
The 1,000-record history page was **545,452 bytes**. The snapshot exceeds the
shipped limit by 168,083 bytes even with one-character station IDs. The samples
start with two stations; the failure concerns a permitted later eight-station
selection, not a claim that the samples fail immediately after launch.

Fix in step 04: cover maximum-width records and aircraft churn, exercise both
transports, derive sufficient budgets, and update all dependent example settings.

### Q2: removed station identifiers accumulate for the entire run

`simulation/registry.go` retains every ID in `stationRegistry.reserved`.
`remove` never deletes it, and `add` bounds only active stations. Repeated
add/remove commands with new IDs grow this map until the practically unreachable
uint64 ordinal limit. `state.clone` copies the entire set on every staged mutation.
Eight active stations therefore do not establish a useful retained-state bound.

Fix in step 02: limit accepted station creations to 1024 distinct IDs per run,
publish the policy, reject overflow atomically, and retain no-reuse semantics.

### Q3: rejected reuse of a removed ID settles time first

`internal/simdriver/driver.go:Driver.AddStation` checks active IDs through
`Engine.Snapshot`, then calls `settleLocked`, then `Engine.AddStation`.
Only the last call checks the reserved-ID set. This contradicts the documented
validation-before-settlement behavior.

A temporary public-API probe used one aircraft, speed 100, a fake clock, and a
station added and removed at time zero. After moving the clock five seconds,
adding the same ID returned `ErrInvalid`, but elapsed time changed from 0s to
5s and the latest transmission sequence changed from 3 to 21. The probe was
removed after the review.

Fix in step 03: engine-owned prechecks before settlement, including Q2's new
lifetime bound. Check future output as well as visible time and sequences.

### Q4: a station revision can wrap from MaxUint64 to zero

`simulation/registry.go:stationRegistry.update` checks revision equality and
then performs an unchecked `revision++`. Revisions start at one and API update
requests require a positive expected revision. Wrapping breaks that invariant.
This is established by code inspection; normal use cannot practically execute
2^64 updates. Test the boundary by arranging internal state directly.

Fix in steps 02 and 03: return `ErrLimit` before changing settings, and reject
exhausted updates before runtime settlement. Removing an exhausted station must
remain possible because removal does not increment its revision.

### Q5: a display test overstates the independence of its frames

`display/decode_test.go:TestDecodeBuildsFieldsFromIndependentPublishedFrames`
calls `identificationFrame`, `positionFrame`, and `velocityFrame` from
`display/fixtures_test.go`. These encode synthetic values through this project's
codec. They test useful behavior, but their bytes are not independent published
inputs. The codec already has genuine external literals. Several browser frame
literals are also copies of those references.

Fix in step 01: name the existing synthetic test accurately, add a literal
reference-frame path through display and both sources, and document fixture
classes accurately. Preserve original ICAOs; the published examples do not all
belong to one aircraft.

### Q6: simulator package documentation describes completed work as backlog

The final comment in `simulator/doc.go` says the manager UI remains backlog
work. It is implemented in `ui` and composed by `testbench`.

Fix in step 07: document the implemented ownership and remove the stale claim.

## Capacity observations requiring measurements

These are measured-work targets, not claims that a particular host is too slow.

| ID | Evidence | Required measurement |
| --- | --- | --- |
| P1 | `simulation/engine.go:state.clone`, `history.clone`, `stationRegistry.clone` copy full rings even for several no-op mutations | Empty/full histories, zero advance, same count, station edits, and normal advances; B/op and allocs/op |
| P2 | `simulation/schedule.go:nextEvent` scans all aircraft and all three families for every event | Aircraft counts 1, 10, 100 at fixed emitted work; ns/frame and full operation time |
| P3 | `Driver.settleLocked` reads the full truth/history snapshot to obtain speed and returns/discards complete batches; catch-up can cover one virtual hour | Paused tick, normal tick, accelerated tick, maximum advance and exact catch-up; live memory versus cumulative allocations |
| P4 | Snapshot capture copies and sorts raw records; local/HTTP adapters and display validate; HTTP handlers fully marshal before checking bytes | Raw snapshot, DTO conversion, serialization, HTTP read/parse, refresh and browser response as separate benchmark names |

The structural policies are 100 active aircraft, 8 active stations, 1,000
transmissions retained, 1,000 receptions retained per station, 8,000 raw snapshot
records, 60 seconds per advance, 32,000 transmissions and 256,000 receptions per
batch. Speed ranges from paused to 100x. Driver heartbeat is 100ms and maximum
virtual catch-up is one hour.

Do not cap received tracks at 100: retained evidence can contain removed
aircraft. The raw-record bound permits up to 8,000 distinct received addresses
for a conforming source. Response proof must include that shape. Identity is
not currently length-bounded by the engine; the supported byte-budget profile
in step 04 explicitly bounds effective run IDs instead of asserting a universal
maximum for all arbitrary embedded configurations.

## Dependencies, risks and mitigation

- Use the repository Go version and existing Node/Playwright lock. No new
  third-party dependency is needed. Task, PowerShell and current lint tools are
  prerequisites. Use `gopls` for references and `go doc` for API exploration.
- Reference frames stay attributed and literal. Record source locations,
  verification method, and any synthetic timestamps. Do not download fixtures
  during tests or use this project's decoder to generate expected results.
- A 1024-ID lifetime policy changes behavior intentionally. Document that
  restart starts a fresh budget and a fresh run identity. Do not silently reuse
  an old ID to reduce memory.
- Larger response budgets allow larger allocations; they do not bound total
  process memory or concurrent requests. Measure the complete response path and
  distinguish retained data, temporary allocations, and host concurrency.
- Maximum-cardinality cases can be costly. Run ordinary regression cases in
  `task all`; keep repeated timing/profile collection in `task capacity`.
- Benchmarks are host-specific. Never gate unit tests on elapsed milliseconds,
  GC heap noise, or an arbitrary performance percentage.
- `task all` starts with cleanup of `.test-results`. Store report evidence in
  documentation before a later cleanup, or rerun the measurement task after it.

## Validation and rollback

Each numbered step supplies focused commands. Finish with `task all`, then
`task capacity` and review the resulting report. Baseline `task all` passed
during planning. No implementation benchmarks were present, and no performance
numbers are invented in this plan.

Keep implementation changes grouped by step. If a regression appears, restore
only that step's changes, keep its new failing regression test, and mark it
pending in `progress.md`. Never remove error checks, shrink test workloads, or
increase timeouts solely to hide a failed support criterion. There is no data
migration because engine and display state are in memory.
