// Source restart across the manager, the aircraft display and history.
// Layer: integration with real Go handlers; the fixture host's test-only
// POST /fixture/restart replaces bench E with a new run ID. Production
// handlers expose no such action.
import { ports } from "../../playwright.config.mjs";
import { expect, test } from "./support.mjs";

const main = `http://127.0.0.1:${ports.main}`;
const wait = { timeout: 15_000 };
const fact = (scope, term) => scope.getByRole("region", { name: "Run and clock" }).getByText(term, { exact: true })
  .locator("xpath=following-sibling::dd[1]");

test.describe.configure({ mode: "serial" });

test("a restart clears old tracks and cursors and rejects old-run mutations", async ({ context }) => {
  const manager = await context.newPage();
  await manager.goto(`${main}/int/e/manager/`);
  await expect(fact(manager, "Run ID")).toHaveText(/^e-run-\d+$/, wait);
  const oldRun = await fact(manager, "Run ID").textContent();
  const count = manager.getByLabel("Aircraft count");
  await count.fill("0");
  await manager.getByRole("button", { name: "Set count" }).click();
  await expect(manager.getByText("Aircraft count 0 accepted")).toBeVisible(wait);
  await count.fill("3");
  await manager.getByRole("button", { name: "Set count" }).click();
  await expect(fact(manager, "Aircraft count")).toHaveText("3", wait);

  const display = await context.newPage();
  await display.goto(`${main}/int/e/aircraft/`);
  const rows = display.getByRole("table", { name: "Received aircraft tracks" }).locator("tbody tr");
  await expect.poll(() => rows.count(), wait).toBeGreaterThan(0);
  const oldIcaos = await rows.locator("th").allTextContents();
  const history = display.getByRole("region", { name: "Station reception history" });
  await history.getByLabel("Station").selectOption("alpha");
  await history.getByRole("button", { name: "Load history" }).click();
  await expect(history.locator("tbody tr").first()).toBeVisible(wait);

  // A dirty manager draft must be reviewed after the restart.
  await count.fill("2");
  const restart = await manager.request.post(`${main}/fixture/restart?bench=e`);
  expect(restart.status()).toBe(204);

  await expect(fact(manager, "Run ID")).not.toHaveText(oldRun, wait);
  const newRun = await fact(manager, "Run ID").textContent();
  await expect(manager.getByText(`Drafts written for run ${oldRun} are kept.`, { exact: false })).toBeVisible(wait);
  await expect(manager.getByRole("button", { name: "Set count" })).toBeDisabled();

  await expect(display.getByText(`Run ${newRun}.`, { exact: false })).toBeVisible(wait);
  await expect(history).toContainText("History cleared: the simulator run changed.", wait);
  await expect(history.locator("tbody tr")).toHaveCount(0);
  for (const icao of oldIcaos) {
    // The new run has no aircraft yet, so no old-run row may remain.
    await expect(display.getByRole("rowheader", { name: icao })).toHaveCount(0);
  }

  const stale = await manager.request.put(`${main}/int/e/api/simulator/aircraft/count`, {
    data: { runId: oldRun, count: 1 },
  });
  expect(stale.status()).toBe(409);
  expect((await stale.json()).error.runId).toBe(newRun);

  await manager.getByRole("button", { name: "Drafts reviewed" }).click();
  await manager.getByRole("button", { name: "Set count" }).click();
  await expect(fact(manager, "Aircraft count")).toHaveText("2", wait);
});
