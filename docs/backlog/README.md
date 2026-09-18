# Backlog

Open implementation work for ADS-B TestBench, grouped by subsystem.
Each item is self-contained and records value, effort, complexity, dependencies,
scope, and acceptance criteria. Remove completed item files and index entries.
When work moves into a detailed plan, remove its backlog file and index entry
and link dependent items to the active plan. Promotion is not implementation
completion.

Front matter uses `value` (Low, Medium, High, Critical), `effort` (S, M, L, XL),
`complexity` (trivial, easy, medium, hard), and `dependencies` (a list of item
filenames). External prerequisites are described in the item.

The simulator API and display backend are tracked together in the active
[Simulator API and received-aircraft display plan](../plans/06-simulator-api-and-display/README.md).

## Top five next-work items

| Rank | Item | Effort | Value |
| ---: | --- | :---: | :---: |
| 1 | [Manager controls and embedded assets](08-manager-ui.md) | L | High |
| 2 | [Aircraft map and reception inspector](09-aircraft-map-and-inspector.md) | L | High |
| 3 | [Combined commands and host embedding](10-commands-and-embedding.md) | L | High |
| 4 | [Deterministic fixtures and capacity checks](11-quality-and-capacity.md) | M | High |
| 5 | [Replay, transport output, and richer scenarios](12-replay-and-traffic-extensions.md) | XL | Medium |

## Index

### UI and composition

| Item | Effort | Value |
| --- | :---: | :---: |
| [Manager controls and embedded assets](08-manager-ui.md) | L | High |
| [Aircraft map and reception inspector](09-aircraft-map-and-inspector.md) | L | High |
| [Combined commands and host embedding](10-commands-and-embedding.md) | L | High |

### Quality and extensions

| Item | Effort | Value |
| --- | :---: | :---: |
| [Deterministic fixtures and capacity checks](11-quality-and-capacity.md) | M | High |
| [Replay, transport output, and richer scenarios](12-replay-and-traffic-extensions.md) | XL | Medium |
