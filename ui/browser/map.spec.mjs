// Aircraft map and reference-altitude coverage.
// Layer: presentation with intercepted API payloads (fixtures.mjs) and
// local deterministic tiles from the fixture host.
import { aircraftConfig, expect, installPausedClock, openHarness, poll, test } from "./support.mjs";
import { aircraft, at, fakeDisplay, isoAt } from "./fixtures.mjs";

const mapOf = (scope) => scope.getByRole("region", { name: "Aircraft map" });
const markers = (scope) => mapOf(scope).locator(".tb-aircraft-marker");
const viewport = async (scope) => {
  const element = mapOf(scope);
  return [await element.getAttribute("data-tb-center"), await element.getAttribute("data-tb-zoom")];
};

function positioned(icao, seconds, latitude, longitude, values = null) {
  return aircraft({
    icao, lastReceivedAt: isoAt(at(seconds)),
    position: { latitudeDegrees: latitude, longitudeDegrees: longitude, observedAt: isoAt(at(seconds)) },
    velocity: values ? { observedAt: isoAt(at(seconds)), values } : null,
  });
}

// The fake clock is paused unless flowing is set: Leaflet zoom animations
// need timers that run with real time.
async function open(page, options, path = "/aircraft/", { flowing = false } = {}) {
  if (flowing) {
    await page.clock.install();
  } else {
    await installPausedClock(page);
  }
  const display = await fakeDisplay(page, options);
  await page.goto(path);
  await expect(mapOf(page)).toBeVisible();
  return display;
}

test("only valid received positions on live tracks create markers", async ({ page }) => {
  let list = [
    positioned("ABC001", 9, 50.1, 14.1),
    aircraft({ icao: "ABC002", lastReceivedAt: isoAt(at(9)), identity: { callsign: "NOPOS", observedAt: isoAt(at(9)) } }),
    positioned("ABC003", -60, 50.2, 14.2),
  ];
  await open(page, { aircraftFor: () => list });
  await expect(page.getByText("Lost", { exact: true })).toBeVisible();
  await expect(markers(page)).toHaveCount(1);
  await expect(markers(page).first()).toHaveAttribute("aria-label", "Aircraft ABC001");
  list = [aircraft({ icao: "ABC001", lastReceivedAt: isoAt(at(9)), identity: { callsign: "LOSTPOS", observedAt: isoAt(at(9)) } })];
  await poll(page);
  await expect(markers(page)).toHaveCount(0, { timeout: 5000 });
});

test("track direction is drawn only from a received ground track, including zero", async ({ page }) => {
  await open(page, {
    aircraftFor: () => [
      positioned("ABC001", 9, 50.1, 14.1, { groundSpeedKnots: 100, trackDegrees: 0 }),
      positioned("ABC002", 9, 50.2, 14.2),
    ],
  });
  await expect(markers(page)).toHaveCount(2);
  const withTrack = mapOf(page).locator('[data-icao="ABC001"] .tb-aircraft-icon');
  await expect(withTrack).toHaveText("\u2191");
  await expect(withTrack).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  await expect(mapOf(page).locator('[data-icao="ABC002"] .tb-aircraft-icon')).toHaveText("\u25cf");
});

test("stale positioned aircraft are marked distinctly and markers select aircraft", async ({ page }) => {
  await open(page, { aircraftFor: () => [positioned("ABC001", -5, 50.1, 14.1)] });
  await expect(mapOf(page).locator(".tb-stale-marker")).toHaveCount(1);
  await expect(markers(page).first()).toHaveAttribute("aria-label", "Aircraft ABC001 (stale)");
  await markers(page).first().click();
  await expect(page.getByRole("region", { name: "Selected aircraft" })).toContainText("ABC001");
  await markers(page).first().focus();
  await expect(markers(page).first()).toBeFocused();
});

test("the viewport stays fixed across polls and selection changes until the user acts", async ({ page }) => {
  const display = await open(page, { aircraftFor: () => [positioned("ABC001", 9, 10, 10)] }, "/aircraft/", { flowing: true });
  await expect(markers(page)).toHaveCount(1);
  const initial = await viewport(page);
  expect(initial).toEqual(["50.000000,14.000000", "7"]);
  await poll(page);
  display.state.stations = [...display.state.stations];
  await page.getByRole("checkbox", { name: /^bravo/ }).check();
  await poll(page);
  expect(await viewport(page)).toEqual(initial);

  await mapOf(page).getByRole("button", { name: "Zoom in" }).click();
  await expect.poll(() => viewport(page)).toEqual(["50.000000,14.000000", "8"]);
  await poll(page);
  expect(await viewport(page)).toEqual(["50.000000,14.000000", "8"]);

  await page.getByRole("button", { name: "Fit received aircraft" }).click();
  await expect.poll(async () => (await viewport(page))[0]).toBe("10.000000,10.000000");
});

test("coverage is drawn only for selected enabled stations with reference altitude labels", async ({ page }) => {
  const display = await open(page, {
    stations: [
      { id: "alpha", enabled: true, latitudeDegrees: 50, longitudeDegrees: 14, siteElevationMetres: 250, antennaHeightMetres: 10,
        antennaGainDBi: 3, sensitivityDBm: -95, systemLossDB: 2, frameLossProbability: 0 },
      { id: "bravo", enabled: false, latitudeDegrees: 51, longitudeDegrees: 15, siteElevationMetres: 250, antennaHeightMetres: 10,
        antennaGainDBi: 3, sensitivityDBm: -95, systemLossDB: 2, frameLossProbability: 0 },
    ],
  });
  const legend = page.locator(".tb-legend");
  await expect(legend.locator('[data-station="alpha"]')).toHaveText(
    "alpha: effective radius 78.0 NM (horizon 121.0 NM, link budget 78.0 NM) at reference pressure altitude 10000 ft. " +
    "Synthetic reception-model estimate, not measured coverage.");
  await expect(legend.locator('[data-station="bravo"]')).toHaveText("bravo: disabled; no coverage drawn.");
  await expect(mapOf(page).locator("path.tb-coverage")).toHaveCount(1);

  await page.getByRole("checkbox", { name: /^bravo/ }).check();
  await poll(page);
  await expect(mapOf(page).locator("path.tb-coverage")).toHaveCount(1);

  display.state.referenceAltitudeFeet = 35000;
  await poll(page);
  await expect(legend.locator('[data-station="alpha"]')).toContainText("reference pressure altitude 35000 ft");
});

test("date-line and polar positions keep exact coordinates with a clipping note", async ({ page }) => {
  await open(page, {
    aircraftFor: () => [positioned("ABC001", 9, 10, 180), positioned("ABC002", 9, 89.5, -179.5)],
  });
  await expect(markers(page)).toHaveCount(2);
  const table = page.getByRole("table", { name: "Received aircraft tracks" });
  await expect(table).toContainText("10.00000, 180.00000");
  await expect(table).toContainText("89.50000, -179.50000 (beyond the map projection limit, drawn clipped)");
  await expect(page.getByText("Drawn clipped at the map projection limit (exact coordinates in the table): ABC002.")).toBeVisible();
});

test("disabled tiles make zero tile requests", async ({ page }) => {
  await open(page, { aircraftFor: () => [positioned("ABC001", 9, 50, 14)] });
  await expect(markers(page)).toHaveCount(1);
  await expect(page.getByText("Map tiles are disabled")).toBeVisible();
  expect(page.requestsTo("/tiles/")).toEqual([]);
});

test("configured local tiles load with visible text attribution", async ({ page }) => {
  await open(page, { aircraftFor: () => [] }, "/aircraft-tiles/");
  await expect.poll(() => page.requestsTo("/tiles/").length).toBeGreaterThan(0);
  const attribution = page.locator(".tb-attribution");
  await expect(attribution).toHaveText("Map tiles: Local test tiles");
  await expect(attribution.getByRole("link", { name: "Local test tiles" })).toHaveAttribute("href", "https://example.test/tiles");
});

test("blocked tiles leave the table, selector and details usable", async ({ page }) => {
  page.allowedFailures.push("/tiles/");
  await page.route("**/tiles/**", (route) => route.abort("blockedbyclient"));
  await open(page, { aircraftFor: (selection) => selection.map((id) => positioned(id === "alpha" ? "AAAAAA" : "BBBBBB", 9, 50, 14)) },
    "/aircraft-tiles/");
  await expect(page.getByText("Map tiles are unavailable")).toBeVisible();
  await page.getByRole("rowheader", { name: "AAAAAA" }).click();
  await expect(page.getByRole("region", { name: "Selected aircraft" })).toContainText("AAAAAA");
  await page.getByRole("checkbox", { name: /^bravo/ }).check();
  await expect(page.getByRole("rowheader", { name: "BBBBBB" })).toBeVisible();
});

test("destroying one of two maps leaves the other operational", async ({ page }) => {
  await page.clock.install();
  let latitude = 50;
  await fakeDisplay(page, { aircraftFor: () => [positioned("ABC001", 9, latitude, 14)] });
  await openHarness(page);
  await page.evaluate(({ a, b }) => {
    window.tbHarness.mount("aircraft", "root-a", a);
    window.tbHarness.mount("aircraft", "root-b", b);
  }, { a: aircraftConfig(), b: aircraftConfig({ initialZoom: 5 }) });
  const rootA = page.locator("#root-a");
  const rootB = page.locator("#root-b");
  await expect(markers(rootA)).toHaveCount(1);
  await expect(markers(rootB)).toHaveCount(1);
  expect((await viewport(rootB))[1]).toBe("5");

  await page.evaluate(() => window.tbHarness.destroy("root-a"));
  await expect(rootA.locator(".leaflet-container")).toHaveCount(0);
  latitude = 10;
  await poll(page);
  await rootB.getByRole("button", { name: "Fit received aircraft" }).click();
  await expect.poll(async () => (await viewport(rootB))[0]).toBe("10.000000,14.000000");
  await rootB.getByRole("button", { name: "Zoom in" }).click();
  await expect(markers(rootB)).toHaveCount(1);
});
