# Backlog

Open implementation work for the ADS-B skeleton, grouped by subsystem.
Each item is self-contained and records value, effort, complexity, dependencies,
scope, and acceptance criteria. Remove completed items from this index.

Front matter uses `value` (Low, Medium, High, Critical), `effort` (S, M, L, XL),
`complexity` (trivial, easy, medium, hard), and `dependencies` (a list of item
filenames). External prerequisites are described in the item.

## Top five next-work items

| Rank | Item | Effort | Value |
| ---: | --- | :---: | :---: |
| 1 | [ADS-B frame codec](01-adsb-codec.md) | L | Critical |
| 2 | [Deterministic aircraft engine and virtual time](02-deterministic-engine.md) | L | Critical |
| 3 | [Real-time driver and simulator lifecycle](05-runtime-and-lifecycle.md) | M | High |
| 4 | [Receiving stations and altitude-aware reception](03-receiving-stations.md) | L | High |
| 5 | [Received observations and bounded history](04-observation-history.md) | L | High |

## Index

### Engine and reception

| Item | Effort | Value |
| --- | :---: | :---: |
| [ADS-B frame codec](01-adsb-codec.md) | L | Critical |
| [Deterministic aircraft engine and virtual time](02-deterministic-engine.md) | L | Critical |
| [Receiving stations and altitude-aware reception](03-receiving-stations.md) | L | High |
| [Received observations and bounded history](04-observation-history.md) | L | High |

### Runtime and contracts

| Item | Effort | Value |
| --- | :---: | :---: |
| [Real-time driver and simulator lifecycle](05-runtime-and-lifecycle.md) | M | High |
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
