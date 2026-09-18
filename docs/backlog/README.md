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

Manager controls, the aircraft map and reception inspector, commands, and host
embedding are tracked together in the active
[UI and commands plan](../plans/08-ui-and-commands/README.md).
Former items 08, 09, and 10 are promoted, not implemented. The simulator API and
display backend prerequisites already exist and are documented in their packages.

## Remaining next-work items

| Rank | Item | Effort | Value |
| ---: | --- | :---: | :---: |
| 1 | [Deterministic fixtures and capacity checks](11-quality-and-capacity.md) | M | High |
| 2 | [Replay, transport output, and richer scenarios](12-replay-and-traffic-extensions.md) | XL | Medium |

## Index

### Quality and extensions

| Item | Effort | Value |
| --- | :---: | :---: |
| [Deterministic fixtures and capacity checks](11-quality-and-capacity.md) | M | High |
| [Replay, transport output, and richer scenarios](12-replay-and-traffic-extensions.md) | XL | Medium |
