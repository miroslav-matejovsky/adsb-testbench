---
title: "04 - Prove response capacity and correct example byte budgets"
dependencies: ["01-scenario-corpus.md", "02-station-state-bounds.md", "03-validation-before-settlement.md"]
effort: "L"
complexity: "high"
---

## Objective

Fix Q1. Establish which response shapes fit supported configurations, test the
actual HTTP boundaries, and keep all producer/consumer settings consistent.

## Target Artifacts

- New `simulator/capacity_test.go`, `display/capacity_test.go` and
  `internal/cli/capacity_test.go`.
- `simulator/acceptance_test.go`, `http_test.go`; `display/bounds_test.go`,
  `source_test.go`, `http_test.go`; `simulatorapi/contract_test.go`.
- `configs/combined.json`, `simulator.json`, `display.json`, `configs/README.md`.
- New capacity profile data in `testdata/quality/capacity.json`.

## Implementation Tasks

1. Preserve the old maximum-cardinality test as a baseline, but make new tests
   name the profile and configuration they prove. Use the corpus to create a
   stationary fleet, eight co-located enabled stations, loss probability zero,
   and sensitivity that receives every generated transmission. Add stations
   before increasing the count so creation reports are received. Assert all
   station rings actually reach 1000 retained records and evict at least one.
2. Add response-size cases for zero, one, two and eight selected stations. Test
   one aircraft and 100 aircraft separately. Include 64-character station IDs,
   nanosecond-resolution timestamps, long sequences/revisions, and an effective
   run ID of 128 UTF-8 bytes. Use valid DTO construction for representational
   boundaries that cannot practically be reached by running the engine.
3. Add a churn case that leaves more than 100 different aircraft addresses in
   retained receptions. Remove a fleet before adding its replacement. Do not
   use `MaxAircraft` as a bound on decoded tracks. In display tests also build
   a semantically valid 8000-record source with distinct addresses distributed
   across eight station rings, and assert its partial-track response fits.
   Synthetic encoded frames are appropriate for cardinality; label them as such.
   Give each station local sequences 1..1000, globally increasing transmission
   sequences 1..8000, unique valid ICAOs matching their encoded identification
   frames, revision 1 and identical creation/reception instants allowed by the
   validator. Set every retention entry to oldest 1, latest 1000, limit 1000,
   truncated false, and choose `Now` so the identities have not expired. Sort
   records by transmission sequence and station IDs lexicographically.
4. Measure `json.Marshal` bytes for raw reception snapshots, 1000-record pages,
   decoded simulator observations, display refresh/snapshot bodies, full truth,
   metadata and stations. Measure full HTTP bodies too, including errors and
   any wrapper fields. Record records, distinct aircraft, provenance copies,
   selected stations and bytes for every row.
5. Build a conservative size worksheet in `capacity.json` and its README:
   array/object punctuation, field names, fixed-width frame/ICAO text, 20-digit
   uint64 strings, up-to-30-byte timestamps, escaped string expansion, longest
   finite numeric representation, and repeated evidence records. Count evidence
   separately for identity, altitude, velocity and both position halves. Have a
   test recompute the bound when DTO JSON keys or response shapes change.
6. State the supported byte-budget profile explicitly: existing hard cardinality
   limits, effective run ID at most 128 UTF-8 bytes, all valid station-ID lengths,
   all legal field widths and values, and the declared consumer concurrency.
   Engine IDs are currently arbitrary nonblank strings; this work does not claim
   a universal finite response size for arbitrarily long IDs. Include JSON
   escaping in the bound, not only ASCII example IDs. Commands add a 21-byte
   suffix, so an example's input label must fit within 107 bytes to stay in this
   profile. Document this as a supported profile, not a new hidden parser rule.
7. Choose each example response budget with this rule: take the largest of its
   measured valid shape and conservative profile bound, multiply by 1.25, then
   round up to a whole MiB. Record inputs and resulting integer bytes. Do not
   simply assume the old test helper's 16 MiB is sufficient. Keep independent
   budgets when raw snapshots and browser observations have different bounds.
8. Update `simulatorApi.maxResponseBytes` and `managerUi.maxResponseBytes` in
   combined/simulator examples; update `source.maxResponseBytes` in display;
   update `display.maxResponseBytes` and `aircraftUi.maxResponseBytes` in combined
   and display examples. Respect existing host-validation inequalities. Keep
   every value explicit and explain the selected profile in each `$comment`
   and `configs/README.md`.
9. Add tests that parse the actual three configuration files with the existing
   CLI parser, not copied constants. Check producer/consumer budgets and source
   timeout relationships. For runtime fixtures, use their parsed budget settings
   with the existing fake-clock simulator harness. Fetch maximum snapshots via
   `display.HTTPSource`, refresh a display, and serialize its response. Compare
   observations with the in-process path, ignoring only the diagnostic local
   last-update timestamp when separate clocks differ.
10. Add exact byte boundary tests: a valid body of B bytes succeeds at limit B;
    limit B-1 returns `response_limit` without a partial success body. Repeat for
    simulator handler, display handler, and HTTPSource. Test declared and
    undeclared Content-Length, dishonest length, truncated body and body-close
    errors through existing transport fakes. Assert one close and preserved error
    causes. Reuse existing coverage where it already establishes a case.
11. Add a contract test comparing display's accepted structural boundary with
    the engine-published limits via the acceptance layer. This catches drift
    between `display/validate.go`'s mirrored constants and simulation policies
    without adding a forbidden display-to-simulation production import.

## Technical Details

Data path and settings to check:

| Path | Producer limit | Consumer limit |
| --- | --- | --- |
| Simulator HTTP to standalone display | `simulatorApi.maxResponseBytes` | `source.maxResponseBytes` |
| Simulator HTTP to manager browser | `simulatorApi.maxResponseBytes` | `managerUi.maxResponseBytes` |
| Display HTTP to aircraft browser | `display.maxResponseBytes` | `aircraftUi.maxResponseBytes` |
| Combined in-process source | Structural validation; no HTTP byte check | Display output still has its HTTP/browser limits |

Do not require local Go calls to fail just because an HTTP encoding budget is
small. Transport-specific byte failures are legitimate. Equivalence means the
same valid supported data decodes the same way when each path has sufficient
transport budget.

The existing `writeJSON` functions marshal before checking size. Their byte
limits constrain transmitted bodies, not peak allocation. Keep complete-error
behavior and measure temporary memory in step 05; do not switch to streaming a
success body that can be cut off after headers are sent.

An intentionally undersized custom configuration remains valid if it satisfies
existing minimum envelope rules. It must fail clearly at runtime. Do not silently
raise values or insert missing values. Report supported and custom configurations
separately.

## Verification

```text
go test ./simulator ./display ./internal/cli ./simulatorapi -run 'Test.*(Capacity|Response|Maximum|Bounds|Config)' -count=1 -v
go test ./simulator ./display ./internal/cli ./simulatorapi
```

Inspect test logs for the measured maximum rows and confirm the test actually
loaded the shipped JSON files. Verify the old 4 MiB setting fails the new
eight-station transport regression before accepting the corrected values.

## Acceptance Criteria

- The 4,362,387-byte baseline case and the wider supported cases pass through
  the real simulator handler and HTTPSource using corrected example budgets.
- More than 100 observed identities and an 8000-record conforming source are
  covered; no test incorrectly equates truth count with received track count.
- A documented bound accounts for all response wrappers, escaping and evidence.
- Exact-fit and one-byte-short tests return the specified success/failure shapes.
- All three example configurations and dependent browser/source budgets agree.

## Non-Goals

No compression protocol, paged replacement for atomic snapshots, arbitrary
increase of engine limits, silent configuration defaults, or universal capacity
claim for unconstrained host inputs.
