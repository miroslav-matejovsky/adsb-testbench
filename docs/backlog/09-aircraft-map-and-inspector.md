---
value: "High"
effort: "L"
complexity: "medium"
dependencies: ["07-display-backend.md","08-manager-ui.md"]
---

# Aircraft map and reception inspector

## Area

ui and ui/internal/assets.

## Work

Show received aircraft positions with altitude, track, speed, vertical rate, field age, and receiver provenance. Support station selection, coverage at a labeled reference altitude, and fresh/stale/lost states. Inspect exact raw frames and paged per-station receptions with visible gaps. Preserve map position between polls and label outage data as stale.

## Acceptance criteria

Add browser checks for partial targets, missed position reports, stale/lost tracks, station selection, history gaps, and simulator restart. Verify observation tables remain usable when map tiles are unavailable.

## Dependencies

- [07-display-backend](07-display-backend.md)
- [08-manager-ui](08-manager-ui.md)
