// Station forms, revision conflicts and draft preservation.
// Layer: presentation with intercepted API payloads (fixtures.mjs).
import { expect, installPausedClock, poll, test } from "./support.mjs";
import { fakeSimulator } from "./fixtures.mjs";

const card = (page, id) => page.getByRole("article", { name: id === null ? "New station" : `Station ${id}` });
const field = (scope, label) => scope.getByLabel(label, { exact: false });

const newStation = {
  "Station ID": "bravo", "Latitude": "0", "Longitude": "-0.5", "Site elevation": "0", "Antenna height": "0",
  "Antenna gain": "0", "Sensitivity": "-140", "System loss": "0", "Frame loss probability": "0",
};

async function open(page, options, target = page) {
  await installPausedClock(page);
  const simulator = await fakeSimulator(target, options);
  await page.goto("/manager/");
  await expect(card(page, "alpha")).toBeVisible();
  return simulator;
}

test("lists stations with units, revision and synthetic coverage labels", async ({ page }) => {
  await open(page);
  const alpha = card(page, "alpha");
  await expect(alpha).toContainText("Enabled; revision 1");
  await expect(alpha).toContainText("Synthetic coverage at reference pressure altitude 10000 ft");
  await expect(alpha).toContainText("not measured radio coverage");
  await expect(alpha.getByLabel("Antenna gain")).toHaveValue("3");
  await expect(alpha.getByText("dBm", { exact: true })).toBeVisible();
  await expect(alpha.getByLabel("Station ID")).toHaveJSProperty("readOnly", true);
});

test("creates a disabled station with explicit zero values", async ({ page }) => {
  const simulator = await open(page);
  await page.getByRole("button", { name: "Add station" }).click();
  const form = card(page, null);
  for (const [label, value] of Object.entries(newStation)) {
    await field(form, label).fill(value);
  }
  await form.getByLabel("Reception").selectOption("false");
  await form.getByRole("button", { name: "Create station" }).click();
  await expect(page.getByText("Station bravo created at revision 1.")).toBeVisible();
  await expect(card(page, "bravo")).toContainText("Disabled; revision 1");
  expect(simulator.bodies("POST", "stations")).toEqual([{
    runId: "run-1",
    station: {
      id: "bravo", enabled: false, latitudeDegrees: 0, longitudeDegrees: -0.5, siteElevationMetres: 0,
      antennaHeightMetres: 0, antennaGainDBi: 0, sensitivityDBm: -140, systemLossDB: 0, frameLossProbability: 0,
    },
  }]);
});

test("a new form starts empty and invalid drafts are never sent", async ({ page }) => {
  const simulator = await open(page);
  await page.getByRole("button", { name: "Add station" }).click();
  const form = card(page, null);
  await expect(field(form, "Latitude")).toHaveValue("");
  await expect(form.getByLabel("Reception")).toHaveValue("");
  await form.getByRole("button", { name: "Create station" }).click();
  await expect(form).toContainText("Latitude is required");
  await expect(form).toContainText("Choose whether the station is enabled");
  await field(form, "Station ID").fill("bad id");
  await field(form, "Latitude").fill("91");
  await field(form, "Frame loss probability").fill("Infinity");
  await form.getByRole("button", { name: "Create station" }).click();
  await expect(form).toContainText("Latitude must be within -90..90 degrees");
  await expect(form).toContainText("Frame loss probability must be a finite decimal number");
  expect(simulator.bodies("POST", "stations")).toEqual([]);
});

test("updates, disables, enables and removes with the captured revision", async ({ page }) => {
  const simulator = await open(page);
  const alpha = card(page, "alpha");
  await alpha.getByLabel("System loss").fill("0");
  await alpha.getByRole("button", { name: "Save changes" }).click();
  await expect(alpha).toContainText("Accepted at revision 2.");
  await alpha.getByRole("button", { name: "Disable" }).click();
  await expect(alpha).toContainText("Disabled; revision 3");
  await alpha.getByRole("button", { name: "Enable" }).click();
  await expect(alpha).toContainText("Enabled; revision 4");
  await alpha.getByRole("button", { name: "Remove" }).click();
  await expect(page.getByText("Station alpha removed.")).toBeVisible();
  await expect(alpha).toHaveCount(0);

  const puts = simulator.bodies("PUT", "stations/alpha");
  expect(puts.map((body) => [body.expectedRevision, body.station.enabled, body.station.systemLossDB])).toEqual([
    ["1", true, 0], ["2", false, 0], ["3", true, 0],
  ]);
  expect(simulator.bodies("DELETE", "stations/alpha")).toEqual([{ runId: "run-1", expectedRevision: "4" }]);
});

test("revisions above 2^53 are sent exactly", async ({ page }) => {
  await page.clock.install();
  const simulator = await fakeSimulator(page);
  simulator.state.stations.get("alpha").revision = 9007199254740993n;
  await page.goto("/manager/");
  const alpha = card(page, "alpha");
  await expect(alpha).toContainText("revision 9007199254740993");
  await alpha.getByLabel("Antenna height").fill("12");
  await alpha.getByRole("button", { name: "Save changes" }).click();
  await expect(alpha).toContainText("Accepted at revision 9007199254740994.");
  expect(simulator.bodies("PUT", "stations/alpha")[0].expectedRevision).toBe("9007199254740993");
});

test("polling keeps focus and typed text while other stations update", async ({ page }) => {
  const simulator = await open(page, { stations: [
    { id: "alpha", enabled: true, latitudeDegrees: 50, longitudeDegrees: 14, siteElevationMetres: 250, antennaHeightMetres: 10,
      antennaGainDBi: 3, sensitivityDBm: -95, systemLossDB: 2, frameLossProbability: 0 },
    { id: "charlie", enabled: true, latitudeDegrees: 49, longitudeDegrees: 13, siteElevationMetres: 250, antennaHeightMetres: 10,
      antennaGainDBi: 3, sensitivityDBm: -95, systemLossDB: 2, frameLossProbability: 0 },
  ] });
  const input = card(page, "alpha").getByLabel("Sensitivity");
  await input.fill("-10");
  await input.press("Backspace");
  simulator.editStation("charlie", { sensitivityDBm: -80 });
  await poll(page);
  await poll(page);
  await expect(card(page, "charlie").getByLabel("Sensitivity")).toHaveValue("-80");
  await expect(input).toHaveValue("-1");
  await expect(input).toBeFocused();
  await expect(card(page, "alpha").getByText("Unsaved draft")).toBeVisible();
});

test("a changed server station keeps the draft until it is reapplied", async ({ page }) => {
  const simulator = await open(page);
  const alpha = card(page, "alpha");
  await alpha.getByLabel("Antenna gain").fill("7");
  simulator.editStation("alpha", { sensitivityDBm: -90 });
  await poll(page);
  await expect(alpha).toContainText("The station changed on the server while you were editing.");
  await expect(alpha.getByRole("table")).toContainText("Server (revision 2)");
  await expect(alpha.getByRole("button", { name: "Save changes" })).toBeDisabled();
  await expect(alpha.getByLabel("Antenna gain")).toHaveValue("7");
  await alpha.getByRole("button", { name: "Reapply draft to current revision" }).click();
  await expect(alpha).toContainText("Draft now targets revision 2.");
  expect(simulator.bodies("PUT", "stations/alpha")).toEqual([]);
  await alpha.getByRole("button", { name: "Save changes" }).click();
  await expect(alpha).toContainText("Accepted at revision 3.");
  const [body] = simulator.bodies("PUT", "stations/alpha");
  expect(body.expectedRevision).toBe("2");
  expect(body.station.antennaGainDBi).toBe(7);
  expect(body.station.sensitivityDBm).toBe(-95);
});

test("reloading server values discards the draft explicitly", async ({ page }) => {
  const simulator = await open(page);
  const alpha = card(page, "alpha");
  await alpha.getByLabel("Antenna gain").fill("7");
  simulator.editStation("alpha", { antennaGainDBi: 5 });
  await poll(page);
  await alpha.getByRole("button", { name: "Reload server values" }).click();
  await expect(alpha.getByLabel("Antenna gain")).toHaveValue("5");
  await expect(alpha.getByText("Unsaved draft")).toBeHidden();
  await expect(alpha.getByText("changed on the server")).toBeHidden();
});

test.describe("two tabs", () => {
  async function twoTabs(context) {
    const a = await context.newPage();
    const b = await context.newPage();
    // A tab polls only when the test runs its clock, so a background poll
    // cannot flag a draft before the test submits it.
    await installPausedClock(a);
    await installPausedClock(b);
    const simulator = await fakeSimulator(context);
    await a.goto("/manager/");
    await b.goto("/manager/");
    await expect(card(a, "alpha")).toBeVisible();
    await expect(card(b, "alpha")).toBeVisible();
    return { a, b, simulator };
  }

  test("simultaneous replacement accepts exactly one update", async ({ context }) => {
    const { a, b, simulator } = await twoTabs(context);
    await card(a, "alpha").getByLabel("Antenna gain").fill("10");
    await card(b, "alpha").getByLabel("Antenna gain").fill("20");
    await card(a, "alpha").getByRole("button", { name: "Save changes" }).click();
    await expect(card(a, "alpha")).toContainText("Accepted at revision 2.");
    await card(b, "alpha").getByRole("button", { name: "Save changes" }).click();
    await expect(card(b, "alpha")).toContainText("revision conflict");
    await expect(card(b, "alpha").getByLabel("Antenna gain")).toHaveValue("20");
    await expect(card(b, "alpha").getByRole("table")).toContainText("Server (revision 2)");
    await b.clock.runFor(3000);
    const puts = simulator.bodies("PUT", "stations/alpha");
    expect(puts.map((body) => [body.expectedRevision, body.station.antennaGainDBi])).toEqual([["1", 10], ["1", 20]]);
    expect(simulator.state.stations.get("alpha").settings.antennaGainDBi).toBe(10);
  });

  test("disable in one tab flags an edit in the other", async ({ context }) => {
    const { a, b } = await twoTabs(context);
    await card(b, "alpha").getByLabel("System loss").fill("5");
    await card(a, "alpha").getByRole("button", { name: "Disable" }).click();
    await expect(card(a, "alpha")).toContainText("Disabled; revision 2");
    await b.clock.runFor(1000);
    await expect(card(b, "alpha")).toContainText("changed on the server");
    await expect(card(b, "alpha").getByLabel("System loss")).toHaveValue("5");
  });

  test("remove in one tab blocks the other tab's edit and its ID", async ({ context }) => {
    const { a, b, simulator } = await twoTabs(context);
    await card(b, "alpha").getByLabel("System loss").fill("5");
    await card(a, "alpha").getByRole("button", { name: "Remove" }).click();
    await expect(card(a, "alpha")).toHaveCount(0);
    await b.clock.runFor(1000);
    await expect(card(b, "alpha")).toContainText("cannot be recreated under a used ID");
    await expect(card(b, "alpha").getByRole("button", { name: "Save changes" })).toBeDisabled();

    await b.getByRole("button", { name: "Add station" }).click();
    const form = card(b, null);
    for (const [label, value] of Object.entries({ ...newStation, "Station ID": "alpha" })) {
      await field(form, label).fill(value);
    }
    await form.getByLabel("Reception").selectOption("true");
    await form.getByRole("button", { name: "Create station" }).click();
    await expect(form).toContainText("Station IDs cannot be reused within one run");
    await expect(field(form, "Station ID")).toHaveValue("alpha");
    expect(simulator.bodies("PUT", "stations/alpha")).toEqual([]);
  });
});

test("a restart keeps dirty drafts flagged and blocks them for the new run", async ({ page }) => {
  const simulator = await open(page);
  const alpha = card(page, "alpha");
  await alpha.getByLabel("Antenna gain").fill("9");
  simulator.restart("run-2");
  await poll(page);
  await expect(page.getByText("Drafts written for run run-1 are kept.", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Drafts reviewed" }).click();
  await expect(alpha).toContainText("The simulator run changed.");
  await expect(alpha.getByRole("button", { name: "Save changes" })).toBeDisabled();
  await alpha.getByRole("button", { name: "Reapply draft to current revision" }).click();
  await alpha.getByRole("button", { name: "Save changes" }).click();
  await expect(alpha).toContainText("Accepted at revision 2.");
  expect(simulator.bodies("PUT", "stations/alpha")).toEqual([expect.objectContaining({ runId: "run-2", expectedRevision: "1" })]);
});

test("capacity failures are reported with the station limit", async ({ page }) => {
  const simulator = await open(page);
  simulator.state.limits.maxStations = 1;
  await page.getByRole("button", { name: "Add station" }).click();
  const form = card(page, null);
  for (const [label, value] of Object.entries(newStation)) {
    await field(form, label).fill(value);
  }
  await form.getByLabel("Reception").selectOption("true");
  await form.getByRole("button", { name: "Create station" }).click();
  await expect(form).toContainText("The station limit was reached");
  await expect(field(form, "Station ID")).toHaveValue("bravo");
});
