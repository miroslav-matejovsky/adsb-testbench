---
value: "High"
effort: "L"
complexity: "medium"
dependencies: ["06-simulator-contract-and-api.md"]
---

# Manager controls and embedded assets

## Area

ui and ui/internal/assets.

## Work

Create embedded templates, styles, and browser modules for aircraft count, virtual time, speed/pause, station editing, and recent generated frames. Validate explicit API/asset URLs and show actionable errors and revision conflicts without losing drafts. Label simulator truth and reception diagnostics. Document and retain licenses for bundled assets.

## Acceptance criteria

Test rendering, URL configuration, malformed inputs, and API errors. Add browser checks for pause/resume, zero aircraft, station edits, and concurrent changes from multiple tabs. Confirm virtual time and real update time are distinguished.

## Dependencies

- [06-simulator-contract-and-api](06-simulator-contract-and-api.md)
