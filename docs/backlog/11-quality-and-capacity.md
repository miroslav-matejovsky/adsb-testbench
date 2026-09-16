---
value: "High"
effort: "M"
complexity: "medium"
dependencies: ["10-commands-and-embedding.md"]
---

# Deterministic fixtures and capacity checks

## Area

simulation, simulator, display, ui, and repository tooling.

## Work

Build a reusable deterministic scenario corpus with independently verified ADS-B frames. Benchmark configured aircraft/station counts, accelerated virtual time, histories, and maximum observation responses. Define supported limits from measurements and document the environment. Run format, vet, architecture lint, code lint, tests, and command reachability in task all.

## Acceptance criteria

Require task all to pass. Record benchmark commands and results; prove response limits cover supported snapshots and retained-state limits remain bounded. Exercise errors and restarts without timing-dependent unit tests.

## Dependencies

- [10-commands-and-embedding](10-commands-and-embedding.md)
