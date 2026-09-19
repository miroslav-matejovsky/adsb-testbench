---
title: "01 - Create an attributed deterministic scenario corpus"
dependencies: []
effort: "M"
complexity: "medium"
---

## Objective

Provide shared data that distinguishes independent wire-format evidence from
synthetic engine scenarios, and make codec, display, transport and browser tests
consume it. Correct finding Q5.

## Target Artifacts

- New `testdata/README.md`, `testdata/quality/README.md`, `frames.json`,
  `scenarios.json`, and `display-reference.json` under `testdata/quality/`.
- New `internal/qualitytest/doc.go`, `corpus.go`, `corpus_test.go`.
- Existing codec tests and `internal/adsb/testdata/README.md`.
- `simulation/determinism_test.go`, `reception_determinism_test.go`.
- `display/fixtures_test.go`, `decode_test.go`, `source_test.go`,
  `display/testdata/README.md`, `simulator/acceptance_test.go`.
- `ui/browser/fixtures.mjs`, new `ui/browser/unit/corpus.test.mjs`.
- `.go-arch-lint.yml` and `internal/README.md` for the new test helper package.

## Implementation Tasks

1. Read the existing fixture provenance and helper files. Retain their exact
   expected values and tolerance explanations. Create the fixture directory
   documentation before moving any literal.
2. Define `frames.json` as an array of named entries. Each entry has `id`,
   `classification` (`external` or `synthetic`), `hex`, `sourceUrl`,
   `sourceLocation`, `verification`, and `expected`. Expected fields state ICAO,
   family and only fields actually carried by that frame. Optional wire values
   use explicit null. Decimal sequences and durations stay strings.
3. Copy the five attributed external frames from
   `internal/adsb/testdata/README.md`. Preserve all bytes and original addresses.
   Store the position pair's expected coordinates for even-newer and odd-newer
   cases, and synthetic reception times, separately from the individual frames.
   Use the existing literal expectations; inspect their referenced primary
   sources before adding any new protocol expectation. Record the verification
   date and calculation/reference used. Never recalculate the expected result
   by calling the encoder or decoder under test.
4. Add `scenarios.json` containing named, fully explicit engine inputs and
   ordered operations. Support only the operations needed here: `advance`,
   `elapse`, `setCount`, `setSpeed`, `addStation`, `updateStation`,
   `removeStation`. Each entry identifies its classification, seed, start time,
   spawn ranges, initial count, speed, stations, selection, expiry durations,
   operations, and assertion names. Store durations as decimal nanoseconds.
5. Implement a typed standard-library loader in `internal/qualitytest` accepting
   a path supplied by its caller. Validate missing required data, duplicate IDs,
   unknown operations and unknown JSON fields, malformed frames, negative
   durations and trailing JSON. Return errors with file and entry names. The
   loader only reads data; each package's tests execute their own operations.
   Tests pass explicit relative paths. No cwd changes, network reads, global
   mutable fixture state, simulation dependency, or production replay API.
6. Wire the codec's reference tests to the shared literals. Preserve exact-byte
   encode tests from independently stated fields. Keep deliberate CRC mutations
   and rational CPR grid tests next to the codec where their derivations belong.
7. Rename the misleading display test to identify it as synthetic. Add a new
   literal-reference test using original addresses: identity 4840D6, position
   40621D, velocity 485020 and A05F21. Do not assemble these into one fictitious
   aircraft by editing their ICAOs. Assert partial tracks and unknown fields.
8. Use `display-reference.json` for a complete valid raw reception snapshot and
   independently stated expected observations. In `display/source_test.go`,
   feed exactly that raw DTO through a fake local provider and an HTTP transport
   returning literal JSON; compare decoded observations with the stored
   expectations. Extend `simulator/acceptance_test.go` separately for engine
   scenarios through its actual local service and mounted handler. The production
   simulator does not need an arbitrary-frame injection endpoint.
9. Load the shared published frame values in `ui/browser/fixtures.mjs` using
   Node's filesystem APIs relative to `import.meta.url`. This file is test code,
   not the embedded browser bundle. Add a Node test checking frame IDs and
   selected reference response fields against the shared fixture. Preserve fake
   API behavior required by existing presentation specs.
10. Add an `internal/qualitytest` architecture component with standard-library
    dependencies only. `_test.go` imports already have an architecture exclusion;
    do not add production dependencies on the helper. Update fixture docs to
    state what is external, synthetic, or a deterministic replay assertion.

## Technical Details

Implement these scenario IDs and their assertions:

| ID | Explicit scenario | Assertions |
| --- | --- | --- |
| `empty-selection` | Zero aircraft; no selected station, and separately an explicit empty selection with configured stations | Allocated empty arrays and no fabricated tracks |
| `reference-fields` | Published literals, original ICAOs, explicit virtual times | Known fields, partial tracks, CPR coordinate tolerance |
| `partitioned-time` | Fixed seed; one 10s advance versus explicit partitions summing to 10s | Exact ordered frame/reception equality and final state |
| `fractional-speed` | Speed 33; real durations with nanosecond remainders | Same carry outcome and frames for split/unsplit elapsed time |
| `pause-and-accelerate` | Speeds 100, 0, 10000 at stated instants | Paused time discarded; aircraft physical speed unchanged |
| `receiver-revisions` | Two receivers; update, disable, re-enable, remove | Historical settings/revisions, receiver provenance, unchanged transmitted frames |
| `loss-extremes` | Same eligible geometry at probabilities 0, 1, and fixed 0.25 | Receive-all/drop-all boundaries and seeded reproducibility |
| `expiry-boundaries` | Four explicit lifetimes; exact boundary and one nanosecond later | Inclusive lifetime then expiry; CPR pair exactly 10s versus 10s+1ns |
| `retention-and-churn` | Fill/wrap rings; remove aircraft, add new identities | Eviction gaps, observed removed aircraft, bounded record counts |
| `restart` | Same seed/settings, different run IDs | Same generated payloads where applicable; old cursors and fallback rejected |

Use literal expectations for external frames. For synthetic determinism tests,
compare two independently initialized executions and explicitly label that
comparison as reproducibility evidence. Do not store giant generated histories
or fragile platform-wide hashes. Use `require` assertions in Go tests.

## Verification

```text
go test ./internal/qualitytest ./internal/adsb ./simulation ./display ./simulator
npm run test:unit
task arch-lint
```

Inspect the new fixture files: every expected protocol value must identify its
reference or derivation, and external fixtures must execute without encoding
their own input. Loader tests include corrupt JSON and an unknown scenario op.

## Acceptance Criteria

- All ten scenario IDs exist and execute in the owning layer's tests.
- The literal corpus is read by codec, display/source, and browser tests.
- Engine scenarios also execute through actual local/HTTP simulator sources.
- Four original ICAOs remain distinct in the reference display case.
- Existing tests retain their coverage; focused tests and architecture lint pass.

## Non-Goals

No production scenario schema, arbitrary-frame simulator endpoint, automatic
golden regeneration, new ADS-B family, or replacement for focused codec tests.
