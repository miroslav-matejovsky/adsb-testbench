// Shared Playwright fixtures for the UI browser tests.
//
// Every test fails on a browser console error, an uncaught exception or an
// unhandled promise rejection. Requests are recorded so tests can assert
// exactly what each component sent. API routes are fulfilled by the test;
// anything the fixture host does not serve and no test fulfils is a 404.
import { test as base, expect } from "@playwright/test";

import { barrier, json } from "./fixtures.mjs";

export { barrier, expect, json };

export const test = base.extend({
  page: async ({ page }, use) => {
    const problems = [];
    page.on("console", (message) => {
      // Chromium logs every failed or non-2xx fetch. API error statuses and
      // aborted API requests are expected test scenarios; any other failed
      // resource, such as a missing asset, still fails the test.
      const resource = message.text().startsWith("Failed to load resource:")
        ? new URL(message.location().url, "http://x").pathname : null;
      const expected = resource !== null &&
        (resource.includes("/api/") || page.allowedFailures.some((fragment) => resource.includes(fragment)));
      if (message.type() === "error" && !expected) {
        problems.push(`console error: ${message.text()}`);
      }
    });
    page.on("pageerror", (error) => problems.push(`page error: ${error.message}`));    // Tests that deliberately block other resources, such as map tiles,
    // list the path fragments whose load failures are expected.
    page.allowedFailures = [];
    page.requestsTo = (fragment) => page.recorded.filter((request) => request.url.includes(fragment));
    page.recorded = [];
    page.on("request", (request) => {
      page.recorded.push({ url: request.url(), method: request.method(), body: request.postData() });
    });
    await use(page);
    expect(problems, "browser console errors and exceptions").toEqual([]);
  },
});

/**
 * Installs a paused fake clock before the page navigates. Polls, request
 * timeouts and other page timers then fire only when the test advances time
 * with runFor, so no background poll can race the test's own steps. The
 * clock is paused one minute ahead of its install time: real time keeps
 * flowing until pauseAt runs, and pausing in the past is an error. No page
 * timer exists yet, so the jump fires nothing.
 */
export async function installPausedClock(page) {
  const time = Date.now();
  await page.clock.install({ time });
  await page.clock.pauseAt(time + 60_000);
}

/**
 * Runs one poll interval of every component on the page. It first waits
 * until no poll cycle is in flight: advancing a paused clock during a cycle
 * would schedule nothing and lose the poll. It then waits for the cycle the
 * interval started to settle, unless settle is false because the test holds
 * that cycle's response. Components mark a running cycle with
 * aria-busy="true".
 */
export async function poll(page, { settle = true } = {}) {
  const busy = page.locator('.tb-component[aria-busy="true"]');
  await expect(busy).toHaveCount(0);
  await page.clock.runFor(1000);
  if (settle) {
    await expect(busy).toHaveCount(0);
  }
}

/** Complete manager configuration used by harness tests. */
export function managerConfig(overrides = {}) {
  return {
    apiBaseUrl: "/api/simulator/", assetBaseUrl: "/assets/",
    pollIntervalMilliseconds: 1000, requestTimeoutMilliseconds: 5000,
    maxResponseBytes: 1048576, resumeSpeedHundredths: 100, ...overrides,
  };
}

/** Complete aircraft display configuration used by harness tests. */
export function aircraftConfig(overrides = {}) {
  return {
    apiBaseUrl: "/api/display/", assetBaseUrl: "/assets/",
    pollIntervalMilliseconds: 1000, requestTimeoutMilliseconds: 5000, maxResponseBytes: 1048576,
    stationIds: ["alpha"], freshForNanoseconds: "10000000000", lostAfterNanoseconds: "60000000000",
    historyPageSize: 3, maxHistoryRecords: 6, initialLatitudeDegrees: 50, initialLongitudeDegrees: 14,
    initialZoom: 7, tiles: null, ...overrides,
  };
}

/** Opens the harness page and waits until its module loaded. */
export async function openHarness(page) {
  await page.goto("/harness/");
  await page.waitForFunction(() => window.tbHarnessReady === true);
}
