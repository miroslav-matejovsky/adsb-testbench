// Exact frame evidence and paged reception inspection.
// Layer: presentation with intercepted API payloads (fixtures.mjs).
import { expect, installPausedClock, poll, test } from "./support.mjs";
import { aircraft, at, fakeDisplay, frames, historyStore, isoAt } from "./fixtures.mjs";

const inspector = (page) => page.getByRole("region", { name: "Reception inspector" });
const evidence = (page) => inspector(page).getByRole("region", { name: "Field evidence" });
const history = (page) => inspector(page).getByRole("region", { name: "Station reception history" });
const historyRows = (page) => history(page).locator("tbody tr");

// The fake clock is paused: polls run only through poll(page), and request
// timeouts never fire because a loaded machine answered slowly.
async function open(page, options = {}, path = "/aircraft/") {
  await installPausedClock(page);
  const store = historyStore(options.store);
  const display = await fakeDisplay(page, { ...options, history: store.handler });
  await page.goto(path);
  await expect(inspector(page)).toBeVisible();
  return { display, store };
}

async function load(page, station = "alpha") {
  await history(page).getByLabel("Station").selectOption(station);
  await history(page).getByRole("button", { name: "Load history" }).click();
}

test("field evidence shows both CPR frames and every receiver copy exactly", async ({ page }) => {
  await open(page, {
    aircraftFor: () => [aircraft({
      icao: "4840D6", lastReceivedAt: isoAt(at(9)), stationIds: ["alpha", "bravo"],
      identity: { callsign: "TB0001", observedAt: isoAt(at(9)) },
      position: { latitudeDegrees: 50, longitudeDegrees: 14, observedAt: isoAt(at(9)) },
    })],
  });
  await page.getByRole("rowheader", { name: "4840D6" }).click();
  const items = evidence(page).locator(".tb-evidence-item");
  await expect(items).toHaveCount(3);
  await expect(items.nth(1).locator("h4")).toHaveText("Position CPR frame 1 of 2");
  await expect(items.nth(1).locator(".tb-frame")).toHaveText(frames.positionEven);
  await expect(items.nth(2).locator(".tb-frame")).toHaveText(frames.positionOdd);
  await expect(items.nth(0).locator(".tb-frame")).toHaveText(frames.identification);
  const copies = items.nth(1).locator("table.tb-receptions tbody tr");
  await expect(copies).toHaveCount(2);
  await expect(copies.nth(0)).toContainText("alpha");
  await expect(copies.nth(1)).toContainText("bravo");
  await expect(copies.nth(0)).toContainText("42.5");
  await expect(copies.nth(0)).toContainText("-71.25");
  await expect(evidence(page)).toContainText("Generated simulator frames are shown separately in the manager.");
});

test("receiver settings come from the reception, not the current catalog", async ({ page }) => {
  const { store } = await open(page, {
    stations: [{ id: "alpha", enabled: true, latitudeDegrees: 50, longitudeDegrees: 14, siteElevationMetres: 250,
      antennaHeightMetres: 10, antennaGainDBi: 30, sensitivityDBm: -95, systemLossDB: 2, frameLossProbability: 0 }],
  });
  store.revisionOf = (sequence) => (sequence > 2n ? "2" : "1");
  store.receiverOf = (sequence) => (sequence > 2n ? { antennaGainDBi: 7, revision: "2" } : {});
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  await expect(historyRows(page).nth(0)).toContainText("gain 3 dBi");
  await expect(historyRows(page).nth(2)).toContainText("gain 7 dBi");
  await expect(historyRows(page).nth(2).locator("td").nth(6)).toHaveText("2");
  await expect(history(page)).not.toContainText("gain 30 dBi");
});

test("paging sends the returned cursor unchanged and honors hasMore", async ({ page }) => {
  const { display } = await open(page, { store: { oldest: 1n, latest: 7n } });
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  const next = history(page).getByRole("button", { name: "Next page" });
  await next.click();
  await expect(historyRows(page)).toHaveCount(6);
  await next.click();
  await expect(history(page)).toContainText("No more records at the time of the last page");
  await expect(next).toBeDisabled();
  const bodies = display.bodies("POST", "receptions/history");
  expect(bodies).toEqual([
    { stationId: "alpha", cursor: null, limit: 3 },
    { stationId: "alpha", cursor: { runId: "run-1", stationId: "alpha", afterSequence: "3" }, limit: 3 },
    { stationId: "alpha", cursor: { runId: "run-1", stationId: "alpha", afterSequence: "6" }, limit: 3 },
  ]);
  await expect(history(page)).toContainText("Loaded 6 of at most 6 rows");
  await expect(history(page)).toContainText("Browser trimmed 1 older loaded row");
});

test("source gaps and browser trimming are labeled separately", async ({ page }) => {
  const { store } = await open(page, { store: { oldest: 1n, latest: 20n } });
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  store.oldest = 10n;
  await history(page).getByRole("button", { name: "Next page" }).click();
  await expect(history(page).locator(".tb-gap-row")).toHaveText(
    "Gap: older receptions were evicted by source retention before sequence 10. History is not contiguous here.");
  await history(page).getByRole("button", { name: "Next page" }).click();
  await expect(history(page)).toContainText("Browser trimmed 3 older loaded rows to stay within 6 rows. This is a display limit, not a source gap.");
  await expect(history(page).locator(".tb-gap-row")).toHaveCount(1);
  await expect(historyRows(page).filter({ hasNot: page.locator(".tb-gap-row") })).toHaveCount(7);
});

test("a gap page without records still shows a gap row", async ({ page }) => {
  const { display, store } = await open(page, { store: { oldest: 1n, latest: 5n } });
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  const original = display.state.history;
  display.state.history = (body, state) => {
    const [status, result] = original(body, state);
    return [status, { ...result, records: [], gap: true, oldestSequence: "50", latestSequence: "49", hasMore: false,
      nextCursor: body.cursor }];
  };
  store.oldest = 1n;
  await history(page).getByRole("button", { name: "Next page" }).click();
  await expect(history(page).locator(".tb-gap-row")).toContainText("before sequence 50");
});

test("history outside retention is labeled from the first retained sequence", async ({ page }) => {
  await open(page, { store: { oldest: 5n, latest: 6n } });
  await load(page);
  await expect(history(page)).toContainText("Receptions before sequence 5 of this run are outside source retention.");
});

test("sequences above 2^53 stay exact in rows and cursors", async ({ page }) => {
  const { display } = await open(page, { store: { oldest: 9007199254740993n, latest: 9007199254740999n } });
  await load(page);
  await expect(historyRows(page).first().locator("td").first()).toHaveText("9007199254740993");
  await history(page).getByRole("button", { name: "Next page" }).click();
  await expect(historyRows(page)).toHaveCount(6);
  expect(display.bodies("POST", "receptions/history")[1].cursor.afterSequence).toBe("9007199254740995");
});

test("a cursor conflict requires reset and a restart clears old history", async ({ page }) => {
  const { display } = await open(page);
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  display.state.runId = "run-2";
  await history(page).getByRole("button", { name: "Next page" }).click();
  await expect(history(page)).toContainText("Reset required: the cursor belongs to another run.");
  await expect(history(page).getByRole("button", { name: "Next page" })).toBeDisabled();
  await poll(page);
  await expect(history(page)).toContainText("History cleared: the simulator run changed.");
  await expect(historyRows(page)).toHaveCount(0);
});

test("a removed station keeps loaded rows as historical and blocks paging", async ({ page }) => {
  const { display } = await open(page);
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  display.state.stations = display.state.stations.filter((station) => station.id !== "alpha");
  await poll(page);
  await expect(history(page)).toContainText("Station alpha was removed. The loaded rows are historical");
  await expect(historyRows(page)).toHaveCount(3);
  await expect(history(page).getByRole("button", { name: "Next page" })).toBeDisabled();
});

test("frames copy exactly and clipboard failures are reported", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await open(page);
  await load(page);
  await history(page).getByRole("button", { name: `Copy frame ${frames.identification}` }).first().click();
  await expect(evidence(page).getByRole("status")).toHaveText(`Copied frame ${frames.identification}.`);
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(frames.identification);

  await page.evaluate(() => {
    navigator.clipboard.writeText = () => Promise.reject(new Error("permission denied"));
  });
  await history(page).getByRole("button", { name: `Copy frame ${frames.identification}` }).first().click();
  await expect(evidence(page).getByRole("alert")).toHaveText(`Copying frame ${frames.identification} failed: permission denied`);
});

test("inspection works with every tile request blocked", async ({ page }) => {
  page.allowedFailures.push("/tiles/");
  await page.route("**/tiles/**", (route) => route.abort("blockedbyclient"));
  await open(page, {}, "/aircraft-tiles/");
  await load(page);
  await expect(historyRows(page)).toHaveCount(3);
  await expect(historyRows(page).first().locator(".tb-frame")).toHaveText(frames.identification);
});
