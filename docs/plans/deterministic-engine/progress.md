# Implementation progress

Tracks execution of the [deterministic engine plan](README.md).
All steps are complete. `task all` passes: format, vet, arch-lint, lint, tests.

| Step | Status | Deliverables |
| --- | --- | --- |
| [01](01-contract-and-configuration.md) | done | `types.go`, `config.go`, `config_test.go`, and the configuration contract in `doc.go`. |
| [02](02-aircraft-identity-and-motion.md) | done | `random.go`, `aircraft.go`, `motion.go` plus their tests. |
| [03](03-frame-generation-and-scheduling.md) | done | `reports.go`, `schedule.go`; `reports_test.go` carries an independent bit, CRC, and CPR oracle and literal frames. |
| [04](04-virtual-clock-and-scaling.md) | done | `clock.go`, `clock_test.go` with a `big.Int` scaling oracle. |
| [05](05-engine-commands-and-history.md) | done | `engine.go`, `history.go`, `snapshot.go` and their tests. |
| [06](06-determinism-and-failure-tests.md) | done | `determinism_test.go`, `atomicity_test.go`; cross-component assertions added to the report, engine, history, and snapshot tests. |
| [07](07-public-documentation-and-acceptance.md) | done | Full `doc.go`, `example_test.go`, root `README.md`, backlog and plan index updates. |

## Acceptance matrix

Every named test from step 06 exists and passes.

| Test | File |
| --- | --- |
| `TestDeterminismReplay` | `determinism_test.go` |
| `TestDeterminismVirtualPartitions` | `determinism_test.go` |
| `TestDeterminismRealPartitions` | `determinism_test.go` |
| `TestDeterminismControls` | `determinism_test.go` |
| `TestDeterminismSurvivors` | `determinism_test.go` |
| `TestEngineZeroAircraft` | `determinism_test.go` |
| `TestEngineIdentityLifetime` | `determinism_test.go` |
| `TestEngineConcurrentAccess` | `determinism_test.go` |
| `TestHistoryCompleteBatch` | `history_test.go` |
| `TestAtomicityRejectedInput` | `atomicity_test.go` |
| `TestAtomicityCancellation` | `atomicity_test.go` |
| `TestAtomicityExhaustion` | `atomicity_test.go` |
| `TestAtomicityCodecFailure` | `atomicity_test.go` |
| `TestSnapshotOwnership` | `snapshot_test.go` |

## Validation performed

- `go test ./simulation ./internal/adsb` passes.
- `go test -race ./simulation` passes.
- `go test ./simulation -run 'TestDeterminism|TestAtomicity' -count=10` passes.
- A temporary external module outside the repository, using a local `replace`
  directive, constructs an engine, advances it, changes speed and count, and
  reads frame bytes using only the public `simulation` package. It compiles and
  its test passes, so no internal type is required by the public API.
- `task all` passes, with the existing deadcode exclusion still in place
  because the `cmd` packages have no entry points yet.

## Deviations from the plan

- The polar longitude convention is asserted with a position vector that lands
  exactly on a pole. A great-circle path that merely passes very close to a
  pole is checked for finite, in-range values instead of an exact longitude,
  because which side of the epsilon it falls on is not a meaningful contract.
- `TestDeterminismRealPartitions` uses a 500 ms total so that 100x scaling
  stays inside `MaxAdvance` in a single call.
