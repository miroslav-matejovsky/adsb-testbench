# Backlog

Open implementation work for ADS-B TestBench, grouped by subsystem.
Each item is self-contained and records value, effort, complexity, dependencies,
scope, and acceptance criteria. Remove completed item files and index entries.

Front matter uses `value` (Low, Medium, High, Critical), `effort` (S, M, L, XL),
`complexity` (trivial, easy, medium, hard), and `dependencies` (a list of item
filenames). External prerequisites are described in the item.

## Top five next-work items

| Rank | Item | Effort | Value |
| ---: | --- | :---: | :---: |
| 1 | [Simulator contract and HTTP API](06-simulator-contract-and-api.md) | L | High |
| 2 | [Display sources and received-track decoding](07-display-backend.md) | L | High |
| 3 | [Manager controls and embedded assets](08-manager-ui.md) | L | High |
| 4 | [Aircraft map and reception inspector](09-aircraft-map-and-inspector.md) | L | High |
| 5 | [Combined commands and host embedding](10-commands-and-embedding.md) | L | High |

## Index

### Runtime and contracts

| Item | Effort | Value |
| --- | :---: | :---: |
| [Simulator contract and HTTP API](06-simulator-contract-and-api.md) | L | High |
| [Display sources and received-track decoding](07-display-backend.md) | L | High |

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
