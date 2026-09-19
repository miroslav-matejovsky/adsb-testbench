// Received aircraft, selection, age and outage state.
// Layer: presentation with intercepted API payloads (fixtures.mjs).
import { aircraftConfig, expect, installPausedClock, openHarness, poll, test } from "./support.mjs";
import { aircraft, at, fakeDisplay, isoAt } from "./fixtures.mjs";

const rows = (scope) => scope.getByRole("table", { name: "Received aircraft tracks" }).locator("tbody tr");
const pageOf = (scope) => (typeof scope.page === "function" ? scope.page() : scope);
const row = (scope, icao) => rows(scope).filter({ has: pageOf(scope).getByRole("rowheader", { name: icao, exact: true }) });
const banner = (scope) => scope.locator(".tb-banner");

function identityOnly(icao, seconds) {
  return aircraft({ icao, lastReceivedAt: isoAt(at(seconds)), identity: { callsign: `CS${icao.slice(-4)}`, observedAt: isoAt(at(seconds)) } });
}

async function open(page, options) {
  await installPausedClock(page);
  const display = await fakeDisplay(page, options);
  await page.goto("/aircraft/");
  return display;
}

test("starts with the exact configured selection and renders partial targets", async ({ page }) => {
  const display = await open(page, {
    aircraftFor: () => [
      identityOnly("ABC001", 9),
      aircraft({ icao: "ABC002", lastReceivedAt: isoAt(at(4.5)), altitude: { feet: 0, observedAt: isoAt(at(4.5)) } }),
      aircraft({
        icao: "ABC003", lastReceivedAt: isoAt(at(10)),
        velocity: { observedAt: isoAt(at(10)), values: { groundSpeedKnots: 0, trackDegrees: 0 } },
      }),
    ],
  });
  await expect(rows(page)).toHaveCount(3);
  expect(display.bodies("POST", "observations")[0]).toEqual({ stationIds: ["alpha"] });
  await expect(page.getByRole("checkbox", { name: "alpha (enabled, revision 1)" })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "bravo (enabled, revision 1)" })).not.toBeChecked();

  const identity = row(page, "ABC001");
  await expect(identity).toContainText("CSC001 (age 1 s)");
  await expect(identity.locator(".tb-unavailable")).toHaveCount(5);
  const altitude = row(page, "ABC002");
  await expect(altitude).toContainText("0 ft pressure altitude (age 5.5 s)");
  const velocity = row(page, "ABC003");
  await expect(velocity).toContainText("0.0 kt, track 0.0 deg (age 0 s)");
  await expect(velocity).toContainText("0 ft/min (barometric)");
  await expect(page.getByText("Virtual time of this snapshot: 2024-03-05T12:00:10Z. Run run-1.")).toBeVisible();
});

test("track classes switch exactly at the configured thresholds", async ({ page }) => {
  const display = await open(page, {
    aircraftFor: () => [identityOnly("ABC001", 0)],
  });
  display.state.nowNs = at(10);
  await poll(page);
  await expect(row(page, "ABC001")).toContainText("Fresh");
  display.state.nowNs = at(10) + 1n;
  await poll(page);
  await expect(row(page, "ABC001")).toContainText("Stale");
  display.state.nowNs = at(60);
  await poll(page);
  await expect(row(page, "ABC001")).toContainText("Stale");
  display.state.nowNs = at(60) + 1n;
  await poll(page);
  await expect(row(page, "ABC001")).toContainText("Lost");
  await expect(row(page, "ABC001")).toContainText("age 60.000000001 s");
});

test("a disappearing aircraft shows no retained evidence for one refresh", async ({ page }) => {
  let present = true;
  await open(page, {
    aircraftFor: () => (present ? [aircraft({
      icao: "ABC001", lastReceivedAt: isoAt(at(9)), identity: { callsign: "GONE01", observedAt: isoAt(at(9)) },
      position: { latitudeDegrees: 50, longitudeDegrees: 14, observedAt: isoAt(at(9)) },
    })] : []),
  });
  await expect(row(page, "ABC001")).toContainText("Fresh");
  present = false;
  await poll(page);
  await expect(row(page, "ABC001")).toContainText("Lost - No retained evidence");
  await expect(row(page, "ABC001")).toContainText("GONE01");
  await expect(row(page, "ABC001")).not.toContainText("50.00000");
  await poll(page);
  await expect(rows(page)).toHaveCount(0);
});

test("an empty selection is explicit and requests no station", async ({ page }) => {
  const display = await open(page, { aircraftFor: (selection) => (selection.length ? [identityOnly("ABC001", 9)] : []) });
  await expect(rows(page)).toHaveCount(1);
  await page.getByRole("checkbox", { name: /^alpha/ }).uncheck();
  await expect(page.getByText("No stations selected, so no aircraft are received.")).toBeVisible();
  await expect(rows(page)).toHaveCount(0);
  expect(display.bodies("POST", "observations").at(-1)).toEqual({ stationIds: [] });
});

test("a selection change discards a late response for the old selection", async ({ page }) => {
  const display = await open(page, {
    aircraftFor: (selection) => (selection.includes("alpha") ? [identityOnly("AAAAAA", 9)] : [identityOnly("BBBBBB", 9)]),
  });
  await expect(row(page, "AAAAAA")).toBeVisible();
  const held = display.hold("POST", "observations");
  await poll(page, { settle: false });
  await held.arrived;
  await page.getByRole("checkbox", { name: /^alpha/ }).uncheck();
  await page.getByRole("checkbox", { name: /^bravo/ }).check();
  held.release();
  await expect(row(page, "BBBBBB")).toBeVisible();
  await expect(row(page, "AAAAAA")).toHaveCount(0);
  expect(display.bodies("POST", "observations").at(-1)).toEqual({ stationIds: ["bravo"] });
});

test("an outage freezes virtual ages and labels the retained view stale", async ({ page }) => {
  const display = await open(page, { aircraftFor: () => [identityOnly("ABC001", 9)] });
  await expect(row(page, "ABC001")).toContainText("age 1 s");
  display.failNext("POST", "observations", 503, {
    snapshot: { status: "unavailable", lastUpdatedAt: null, observations: null, error: null },
    error: { code: "unavailable", message: "the source is unavailable", field: "read reception snapshot", runId: "run-1" },
  });
  display.abortNext("POST", "observations");
  display.state.nowNs = at(100);
  await poll(page);
  await expect(banner(page)).toContainText("Transport stale: showing the last successful data for this selection");
  await expect(page.locator(".tb-component > .tb-error")).toContainText("the source is unavailable");
  await expect(row(page, "ABC001")).toContainText("age 1 s");
  await expect(row(page, "ABC001")).toContainText("Fresh");
  await poll(page);
  await expect(banner(page)).toContainText("Transport stale");
  await expect(row(page, "ABC001")).toContainText("Fresh");
  await poll(page);
  await expect(banner(page)).toContainText("Received data is current.");
  await expect(row(page, "ABC001")).toContainText("Lost");
});

test("a replacement run clears previous-run rows before rendering", async ({ page }) => {
  const display = await open(page, {
    aircraftFor: (selection, state) => (state.runId === "run-1" ? [identityOnly("0DD001", 9)] : [identityOnly("0EE001", 9)]),
  });
  await expect(row(page, "0DD001")).toBeVisible();
  await row(page, "0DD001").click();
  display.state.runId = "run-2";
  await poll(page);
  await expect(row(page, "0EE001")).toBeVisible();
  await expect(row(page, "0DD001")).toHaveCount(0);
  await expect(page.getByText("Virtual time of this snapshot: 2024-03-05T12:00:10Z. Run run-2.")).toBeVisible();
  await expect(page.getByRole("region", { name: "Selected aircraft" })).toContainText("Select an aircraft");
});

test("an error envelope from a replacement run clears old data even without a payload", async ({ page }) => {
  const display = await open(page, { aircraftFor: () => [identityOnly("0DD001", 9)] });
  await expect(row(page, "0DD001")).toBeVisible();
  const invalid = {
    snapshot: { status: "unavailable", lastUpdatedAt: null, observations: null, error: null },
    error: { code: "source_invalid", message: "invalid frame", field: "read reception snapshot", runId: "run-2" },
  };
  display.state.runId = "run-2";
  display.failNext("POST", "observations", 502, invalid);
  display.failNext("POST", "observations", 502, invalid);
  await poll(page);
  await expect(row(page, "0DD001")).toHaveCount(0);
  await expect(page.locator(".tb-component > .tb-error")).toContainText("invalid frame");
});

test("a removed selected station is flagged and never replaced silently", async ({ page }) => {
  const display = await open(page, { aircraftFor: () => [identityOnly("ABC001", 9)] });
  await expect(rows(page)).toHaveCount(1);
  display.state.stations = display.state.stations.filter((station) => station.id !== "alpha");
  await poll(page);
  await expect(page.getByText("Selected station alpha is not in the current catalog.")).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "alpha (unavailable)" })).toBeChecked();
  await expect(page.locator(".tb-component > .tb-error")).toContainText('station "alpha" not found');
  expect(display.bodies("POST", "observations").at(-1)).toEqual({ stationIds: ["alpha"] });
});

test("keyboard selection shows accessible details", async ({ page }) => {
  await open(page, { aircraftFor: () => [identityOnly("ABC001", 9), identityOnly("ABC002", 8)] });
  await row(page, "ABC002").focus();
  await page.keyboard.press("Enter");
  const details = page.getByRole("region", { name: "Selected aircraft" });
  await expect(details).toContainText("ABC002");
  await expect(details).toContainText("CSC002 (age 2 s)");
  await expect(row(page, "ABC002")).toHaveAttribute("aria-selected", "true");
  await poll(page);
  await expect(row(page, "ABC002")).toBeFocused();
});

test("two components with different selections on one backend stay isolated", async ({ page }) => {
  await page.clock.install();
  await fakeDisplay(page, {
    aircraftFor: (selection) => selection.map((id) => identityOnly(id === "alpha" ? "AAAAAA" : "BBBBBB", 9)),
  });
  await openHarness(page);
  await page.evaluate(({ a, b }) => {
    window.tbHarness.mount("aircraft", "root-a", a);
    window.tbHarness.mount("aircraft", "root-b", b);
  }, { a: aircraftConfig({ stationIds: ["alpha"] }), b: aircraftConfig({ stationIds: ["bravo"] }) });
  const rootA = page.locator("#root-a");
  const rootB = page.locator("#root-b");
  for (let i = 0; i < 3; i++) {
    await expect(row(rootA, "AAAAAA")).toBeVisible();
    await expect(row(rootB, "BBBBBB")).toBeVisible();
    await expect(row(rootA, "BBBBBB")).toHaveCount(0);
    await expect(row(rootB, "AAAAAA")).toHaveCount(0);
    await poll(page);
  }
  await page.evaluate(() => window.tbHarness.destroy("root-a"));
  await poll(page);
  await expect(row(rootB, "BBBBBB")).toBeVisible();
});
