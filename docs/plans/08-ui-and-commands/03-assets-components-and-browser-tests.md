---
title: "03 - Embedded assets, component lifecycle, and browser tests"
dependencies: ["02-display-discovery-and-isolation.md"]
effort: "L"
complexity: "high"
---

## Objective

Provide embeddable pages/components, explicit browser configuration, shared
request/time helpers, and deterministic browser tests before adding UI behavior.

## Target Artifacts

- New `ui/config.go`, `config_json.go`, `pages.go`, `assets.go`, and Go tests.
- New `ui/internal/assets/embed.go`, `templates/`, `static/`, `licenses/`.
- New `ui/internal/assets/static/components.js`, `client.js`, `time.js`, `ui.css`.
- New root `package.json`, `package-lock.json`, `playwright.config.mjs`.
- New `ui/browser/README.md`, `ui/browser/*.spec.mjs`, fixture support under
  `ui/testdata/browserhost/` with package documentation.
- `taskfile/clean.ps1`, new cleanup script tests, `Taskfile.yml`, `.gitignore`,
  architecture rules and relevant package/folder documentation.

## Implementation Tasks

1. Fix F6 before installing browser tooling: clean only resolved, validated
   repository output directories, reject reparse points/outside-workspace targets,
   and remove the recursive executable sweep. Test with temporary directories
   and sentinel files in third-party/browser cache locations.
2. Create `ui.NewManager(config)` and `ui.NewAircraftDisplay(config)` returning
   page handlers plus `ui.Assets()` returning a relative static handler. Validate
   settings before rendering. Embed allowlisted assets/templates/licenses;
   reject directory listing, traversal and unknown asset paths.
3. Render configuration as escaped JSON data, read by a same-origin external
   module. Use `html/template`; never interpolate config into executable JS or
   mark user input as trusted HTML. Give pages labeled component roots, loading,
   empty/error regions and keyboard-accessible controls.
4. Implement `mountManager` and `mountAircraftDisplay` with per-root state and
   idempotent `destroy()`. Scope styles to the component wrapper. Abort outstanding
   fetches, clear timers, disconnect observers/listeners, and remove the map on
   destroy. Suppress callbacks belonging to an obsolete request generation.
5. Build shared bounded fetch/error helpers. Parse JSON on non-2xx responses,
   including display `RefreshResponse`; handle invalid JSON, missing fields,
   network failure, deadline and byte limit errors. Do not retry mutations.
   Poll with one outstanding cycle per component and a timeout scheduled only
   after completion, not overlapping `setInterval` calls.
6. Implement canonical decimal-string and UTC RFC3339Nano parsing. Keep timestamps
   as integer nanoseconds using `BigInt`; test year 0001, year 0099, leap dates,
   year 9999, exact expiry boundaries, values beyond 2^53 and null versus zero.
7. Introduce a locked browser test dependency, declared Node version, Chromium
   setup command, and a fixture host using real embedded pages. Add `task ui-test`
   and native-module tests. Use controlled responses and clock advancement;
   dependencies are installed explicitly, never downloaded inside test execution.
8. Add pin/source/license/checksum records for Leaflet assets, a served notices
   document, and a notices link from both pages. Verify every template/static
   subfolder has suitable documentation; Go packages use `doc.go`.

## Technical Details

Common required fields: `apiBaseUrl`, `assetBaseUrl`, `pollIntervalMilliseconds`,
`requestTimeoutMilliseconds`, and `maxResponseBytes`. Manager additionally
requires `resumeSpeedHundredths`. Display additionally requires `stationIds`,
`freshForNanoseconds`, `lostAfterNanoseconds`, `historyPageSize`,
`maxHistoryRecords`, `initialLatitudeDegrees`, `initialLongitudeDegrees`,
`initialZoom`, and `tiles`. No parser supplies a missing field.

`tiles` is explicitly null to disable tiles, or a complete object containing
`urlTemplate`, `attributionText`, `attributionUrl`, `minZoom`, and `maxZoom`.
Require `{z}`, `{x}`, `{y}` in tile templates; validate the URL after substituting
safe numeric placeholders. Reject scripts, userinfo, fragments and control
characters. Attribution is text plus a validated link, not arbitrary HTML.

Require positive poll/request bounds, 0 < freshFor < lostAfter, 1 <= page size <=
1000, maxHistoryRecords >= page size, finite coordinates, and integral zoom.
Each mount validates resolved API/asset origin before sending any request.
Configuration also has strict duplicate/missing/unknown/null-key handling in Go;
only the explicitly nullable tiles field may be null.

## Verification

```text
go test ./ui ./ui/internal/assets
npm ci
npx playwright install chromium
npm run test:unit
task ui-test
task all
```

Test hostile config strings, root/nested mounts, content types, no missing assets,
cross-origin URL rejection, exact duration arithmetic, malformed error bodies,
out-of-order responses and repeated mount/destroy cycles. Run cleanup fixture
checks through the declared Task target as well.

## Acceptance Criteria

- Embedded pages and assets work without Node or CDN access at runtime.
- No request starts from invalid configuration; no late response updates a
  destroyed component; two roots have independent timers and state.
- All bundled third-party assets have retained notices and pinned provenance.
- Browser unit and fixture tests run deterministically with no sleeps.
- Cleanup preserves files outside its declared build-output directories.

## Non-Goals

Manager business controls, received-data rendering, application commands,
frontend frameworks, bundlers, and production dependency auto-installation.
