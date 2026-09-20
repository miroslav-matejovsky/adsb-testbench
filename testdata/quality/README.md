# Quality fixture corpus

Shared deterministic test inputs, loaded by the standard-library-only test
helper [internal/qualitytest](../../internal/qualitytest/doc.go). Codec,
engine, simulator transport, display and browser tests consume them. The
corpus is test input only: no production package reads it, and it is not a
scenario-loading or replay API.

## Classes of evidence

| Class | Meaning |
| --- | --- |
| `external` | Literal bytes and field values from a cited publication. Independent protocol evidence. Expected values are stored as data and never produced by this repository's encoder or decoder. |
| `synthetic` | Inputs constructed for a test. They pin behavior of this implementation and prove nothing about the protocol by themselves. |
| Reproducibility assertion | Two independently initialized executions of the same synthetic scenario compared with each other. Evidence of determinism, not of correctness against an outside reference. |

Agreement between this encoder and this decoder is not independent protocol
evidence. Protocol claims rest on the `external` frames.

## Files

### frames.json

An object with two arrays.

`frames` lists named literal frames. Each entry has `id`, `classification`,
`hex` (28 uppercase hexadecimal digits), `sourceUrl`, `sourceLocation`,
`verification` and `expected`. `expected` states `icao`, `family`,
`capability`, and exactly one payload object named after the family, holding
only fields that frame carries. Unavailable optional wire values are explicit
`null`.

`cprPairs` names an even and an odd position frame and the expected global
fix for each timing case (`even-newer`, `odd-newer`) with an explicit
tolerance. The reception instants are synthetic: the publication gives no
times, so the instants only select which half is newer.

Every current frame is `external`. The five frames and their expectations are
the literals of the codec provenance table in
[internal/adsb/testdata](../../internal/adsb/testdata/README.md), copied
unchanged on 2026-09-19. Each keeps its original ICAO address; the examples do
not describe one aircraft.

| Frame | Source | Fields |
| --- | --- | --- |
| `identification-4840d6` | mode-s.org identification example | 4840D6, TC 4, category 0, KLM1023 |
| `position-40621d-even` | mode-s.org airborne position example | 40621D, TC 11, 38000 ft, even CPR 93000/51372 |
| `position-40621d-odd` | Same example | 40621D, 38000 ft, odd CPR 74158/50194 |
| `velocity-485020-ground` | mode-s.org velocity example A | subtype 1, east -8 kt, north -159 kt, -832 ft/min, +550 ft |
| `velocity-a05f21-airspeed` | mode-s.org velocity example B | subtype 3, heading 243.984375, TAS 375 kt, -2304 ft/min |

### display-reference.json

One complete raw reception snapshot (`snapshot`, the exact
`simulatorapi.ReceptionSnapshot` wire JSON) carrying the five external frames
at one synthetic station, and the observations a display must derive from it
(`expected.aircraft`, in ICAO order). Frames and wire values are external;
the station, sequences and instants are synthetic. Derived values state their
derivation: the ground speed 159.20 kt and track 182.88 degrees of velocity
example A are the published results, compared with a 0.005 tolerance. The
position fix is the published even-newer solution. Transport tests serve the
`snapshot` bytes unchanged.

### scenarios.json

An array of named scenarios. `kind` is `engine` or `reference`.

Engine scenarios state every input explicitly: `engine` (decimal `seed`,
UTC `startTime`, `initialAircraftCount`, `speedHundredths`, `spawn` ranges),
`stations` added right after construction, the station `selection`, field
lifetimes in `expiry`, `runs`, and `assertions`. Each run has a `name`, its
`runId`, and ordered `operations`. Every run starts from a fresh engine.
Durations, seeds and revisions are decimal strings; durations are
nanoseconds and must be nonnegative.

| Operation | Fields |
| --- | --- |
| `advance` | `durationNanoseconds` of virtual time |
| `elapse` | `durationNanoseconds` of real time, scaled by the current speed |
| `setCount` | `count` |
| `setSpeed` | `speedHundredths` |
| `addStation` | complete `station` settings |
| `updateStation` | `expectedRevision`, complete `station` settings |
| `removeStation` | `stationId`, `expectedRevision` |

A reference scenario names a fixture file in this directory in `reference`.

The loader rejects missing or unknown fields, `null` where a value is
required, duplicate identifiers, unknown operations, unknown assertion names,
malformed frames, negative durations and trailing JSON.

### capacity.json

The supported byte-budget profile, the conservative response bounds derived
from it, and the example response budgets chosen from those bounds.

`profile` states every assumption: the structural limits (8 selected
stations, 1000 records each, 8000 raw records, 100 aircraft, 1000 retained
transmissions), the text limits of the profile (an effective run ID of at
most 128 UTF-8 bytes, station IDs of at most 64 bytes, error message and
field text of at most 512 bytes each), the encoding widths (20-digit
unsigned decimals, 30-byte timestamps, 25-byte finite numbers, and a JSON
escape factor of 6, since one byte may become a six-byte escape such as backslash-u003c), the budget rule
(bound times 1.25, rounded up to a whole MiB) and the declared consumer
concurrency.

Engine run identifiers are arbitrary nonblank strings, so no universal
finite response size exists. The profile bounds the effective run ID
instead. A command appends a 21-byte suffix to the configured
`simulation.id`, so a label must fit in 107 bytes to stay inside it.

`bounds` are computed, not estimated: `simulatorapi/capacity_test.go` builds
a worst-case DTO for every response shape, marshals it, and fails if the
recorded number no longer matches. `display/capacity_test.go` adds the
display refresh wrapper the same way. `observationSnapshotBytes` charges
every raw record one aircraft wrapper, the largest field object, and two
evidence objects with a reception copy each, which bounds the decoded
projection of any conforming snapshot.

`budgets` are the values the three example configuration files carry;
`internal/cli/capacity_test.go` parses those files and compares them.
Measured maximum shapes are far smaller than the bounds; the simulator and
display capacity tests log both.

| Response | Bound (bytes) | Budget |
| --- | --- | --- |
| Raw reception snapshot, 8000 records | 7,714,932 | `source.maxResponseBytes` 10,485,760 |
| Reception history page, 1000 records | 965,952 | same |
| Decoded observations, 8000 records | 25,643,125 | `simulatorApi.maxResponseBytes` and `managerUi.maxResponseBytes` 32,505,856 |
| Display refresh body | 25,657,229 | `display.maxResponseBytes` and `aircraftUi.maxResponseBytes` 32,505,856 |
| Truth snapshot | 197,580 | covered by the simulator budget |
| Stations, metadata, error envelope | 7,032 / 3,422 / 6,985 | covered by every budget |

These budgets bound transmitted bodies. Both HTTP layers marshal a response
before checking its size, so a budget does not bound peak allocation, and it
says nothing about concurrent requests.

## Scenarios and owning tests

Every engine run of every engine scenario also executes through the real
simulator runtime, its local service and its mounted HTTP handler in
`simulator/corpus_test.go` (`TestCorpusScenariosAgreeAcrossTransports`),
which requires identical raw evidence on both transports and display tracks
equal to the engine's own observation oracle.

| Scenario | Assertions | Owning tests |
| --- | --- | --- |
| `empty-selection` | `allocated-empty-arrays`, `no-fabricated-tracks` | `simulation` `TestCorpusEngineScenarios` |
| `reference-fields` | `known-fields`, `partial-tracks`, `cpr-coordinate-tolerance`, `distinct-original-icaos` | `display` `TestDecodeLiteralReferenceSnapshot`, `TestBothSourcesDecodeTheLiteralReferenceSnapshot`; codec `TestPublishedFrames`, `TestPublishedCPRPositions`; browser `unit/corpus.test.mjs` |
| `partitioned-time` | `identical-batches`, `identical-future` | `simulation` `TestCorpusEngineScenarios` (reproducibility) |
| `fractional-speed` | `identical-batches`, `identical-future` | `simulation` `TestCorpusEngineScenarios` (reproducibility) |
| `pause-and-accelerate` | `paused-time-discarded`, `physical-speed-unchanged`, `identical-batches` | `simulation` `TestCorpusEngineScenarios` |
| `receiver-revisions` | `historical-revisions`, `receiver-provenance`, `unchanged-transmissions` | `simulation` `TestCorpusEngineScenarios` |
| `loss-extremes` | `receive-all`, `drop-all`, `seeded-partial-loss`, `reproducible` | `simulation` `TestCorpusEngineScenarios` |
| `expiry-boundaries` | `inclusive-lifetime`, `expired-after-lifetime`, `cpr-pair-age-bound` | `simulation` `TestCorpusEngineScenarios` (field lifetimes); `display` `TestDecodeLiteralCPRPairAgeBound` (literal CPR pair at 10 s and 10 s + 1 ns) |
| `retention-and-churn` | `bounded-records`, `eviction-gap`, `removed-aircraft-observed`, `new-identities` | `simulation` `TestCorpusEngineScenarios` |
| `restart` | `same-generated-payloads`, `old-cursor-rejected`, `no-cross-run-fallback` | `simulation` `TestCorpusEngineScenarios`; `simulator` `TestCorpusRestartAcrossTransports` (HTTP cursor, display fallback) |

## Adding data

State every input explicitly and name the assertions a test executes. Add an
external frame only with a primary source location and literal expectations
read from that source, never with values produced by this codec. Do not
store generated histories or platform-wide hashes; compare two executions
instead.
