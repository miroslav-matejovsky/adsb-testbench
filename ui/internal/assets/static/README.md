# Static browser bundle

Native JavaScript modules and styles served at the root of the asset base by
`ui.Assets()`. There is no bundler or build step; browsers load these files
directly, and `ui/browser/unit/` tests the DOM-free modules with Node.

| File | Responsibility |
| --- | --- |
| `components.js` | Public entry: `mountManager(root, config)` and `mountAircraftDisplay(root, config)` |
| `manager-page.js`, `aircraft-page.js` | Page bootstraps reading the template's JSON configuration |
| `component.js` | Root ownership, scoped wrapper, status/error regions, busy flag during poll cycles, stylesheet loading |
| `lifecycle.js` | Tracks requests, listeners and cleanups; idempotent `destroy()` |
| `client.js` | Bounded JSON requests and the non-overlapping poll loop |
| `config.js` | Configuration validation mirroring `ui/config.go`, same-origin base resolution |
| `time.js` | Exact BigInt decimals, RFC3339Nano instants and presentation formatting |
| `dom.js` | Safe element construction; never uses `innerHTML` |
| `manager.js` | Manager component: truth, count, speed, pause/resume, generated frames |
| `stations.js` | Station editor with revision-checked drafts |
| `aircraft.js` | Aircraft display component: selection, polling, outage and restart handling |
| `tracks.js` | DOM-free received-track model: ages, thresholds, tombstones |
| `map.js` | Leaflet map, received markers and synthetic coverage circles |
| `inspector.js` | Exact frame evidence and paged per-station reception history |
| `ui.css` | Styles scoped below `.tb-component` and `.tb-page` |
| `notices.html` | Third-party notices linking `licenses/` |
| `leaflet/` | Pinned Leaflet 1.9.4; provenance and checksums in `leaflet/README.md` |

Every component validates its complete configuration and resolves its URL
bases against the page origin before sending a request. Values that exceed
JavaScript number precision (revisions, sequences, nanoseconds) stay strings
or `BigInt`.
