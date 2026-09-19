# Browser tests

Deterministic tests of the embedded UI. They are development tooling only;
Go binaries never need Node at run time.

## Setup

Install the exact locked tools once; tests never download anything:

```text
npm ci
npx playwright install chromium
```

`package.json` pins `@playwright/test` exactly and `package-lock.json` locks
it. `.node-version` declares the Node version used to produce the lock
(`engines` states the minimum). Run everything with `task ui-test`, which fails
with the setup command when dependencies are missing.

## Layout

| Path | Contents |
| --- | --- |
| `unit/*.test.mjs` | `node --test` unit tests of DOM-free modules (`npm run test:unit`) |
| `*.spec.mjs` | Playwright tests against real embedded pages (`npx playwright test`) |
| `deployment.spec.mjs` | Integration: combined and separate deployments at root and nested prefixes, network isolation, narrow viewport, accessibility |
| `embedding.spec.mjs` | Integration: independent benches, host-built component layout; presentation: repeated mount/destroy with requests in flight |
| `restart.spec.mjs` | Integration: source restart clears tracks, cursors and review-gated drafts; old-run mutations are rejected |
| `support.mjs` | Shared fixture: fails on console errors, records requests; `installPausedClock`, settled `poll` |
| `fixtures.mjs` | Stateful fake simulator and display APIs with barriers and failures |

Playwright starts `ui/testdata/browserhost` (see its `doc.go`), which serves
the real pages and assets at `/` and `/bench/a/` for presentation tests, whose
API routes are fulfilled through the fakes in `fixtures.mjs`, and real paused
benches on four loopback origins for integration tests. Integration specs
say so in their header; they use real browser time and wait for conditions,
and each spec owns its benches so specs never share mutable state.

## Determinism

- Browser timers are controlled with the Playwright clock; virtual simulation
  time only moves when a test changes the fake's state.
- Specs with intercepted payloads install a paused fake clock
  (`installPausedClock`), so no background poll or request timeout can fire
  between test steps on a loaded machine. `poll(page)` advances one interval
  after the previous cycle settled and waits for the new cycle, using the
  component's `aria-busy` flag. Tests that need Leaflet animations or a
  moving real-time age keep a flowing clock. Wait for a request's visible
  result before changing the fake state it reads.
- Response ordering uses barriers (`hold`), never sleeps or `waitForTimeout`.
- Every test fails on browser console errors, uncaught exceptions and
  unhandled rejections. Expected API error statuses are the only exempt
  resource failures.
- Failing runs keep traces and the HTML report below `.test-results/`.

Each spec header states the layer it covers: presentation with intercepted
payloads, or integration with real handlers.
