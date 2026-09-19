// Manager traffic, clock and generated-frame controls.
// Layer: presentation with intercepted API payloads (fixtures.mjs).
import { expect, installPausedClock, poll, test } from "./support.mjs";
import { fakeSimulator, frames } from "./fixtures.mjs";

const facts = (page) => page.getByRole("region", { name: "Run and clock" });
const fact = (page, term) => facts(page).getByText(term, { exact: true }).locator("xpath=following-sibling::dd[1]");
const truthRows = (page) => page.getByRole("region", { name: "Simulator truth" }).locator("tbody tr");

// The fake clock is paused unless flowing is set, for tests that read the
// real update age, which advances with the clock between polls.
async function openManager(page, options, { flowing = false } = {}) {
  if (flowing) {
    await page.clock.install();
  } else {
    await installPausedClock(page);
  }
  const simulator = await fakeSimulator(page, options);
  await page.goto("/manager/");
  await expect(fact(page, "Run ID")).toHaveText(options?.runId ?? "run-1");
  return simulator;
}

test("renders run, clocks, truth and exact generated frames", async ({ page }) => {
  await page.clock.install();
  const simulator = await fakeSimulator(page, { speedHundredths: 125 });
  simulator.advance(1.5, frames.identification);
  simulator.advance(0.25, frames.velocity, "velocity");
  await page.goto("/manager/");

  await expect(fact(page, "Virtual time")).toHaveText("2024-03-05T12:00:01.75Z");
  await expect(fact(page, "Elapsed virtual time")).toHaveText("1.75 s");
  await expect(fact(page, "Clock")).toHaveText("Running at 1.25x real time");
  await expect(fact(page, "Aircraft count")).toHaveText("2");
  await expect(fact(page, "Initial aircraft count")).toHaveText("2");
  await expect(fact(page, "Last successful update (real time)")).toContainText("s ago");
  await expect(truthRows(page)).toHaveCount(2);
  await expect(truthRows(page).first()).toContainText("420.0");

  const framesPanel = page.getByRole("region", { name: "Generated frames" });
  await expect(framesPanel.locator("tbody tr")).toHaveCount(2);
  await expect(framesPanel.locator("td.tb-frame").nth(0)).toHaveText(frames.identification);
  await expect(framesPanel.locator("td.tb-frame").nth(1)).toHaveText(frames.velocity);
  await expect(framesPanel).toContainText("Retained sequences 1..2 (history limit 4).");
});

test("shows retention truncation from the oldest retained sequence", async ({ page }) => {
  await page.clock.install();
  const simulator = await fakeSimulator(page);
  for (let i = 0; i < 6; i++) {
    simulator.advance(1);
  }
  await page.goto("/manager/");
  await expect(page.getByRole("region", { name: "Generated frames" }))
    .toContainText("Frames before sequence 3 are no longer retained.");
});

test("count zero removes every truth row after an acknowledged refresh", async ({ page }) => {
  const simulator = await openManager(page);
  await expect(truthRows(page)).toHaveCount(2);
  await page.getByLabel("Aircraft count").fill("0");
  await page.getByRole("button", { name: "Set count" }).click();
  await expect(page.getByText("No simulated aircraft.")).toBeVisible();
  await expect(truthRows(page)).toHaveCount(0);
  expect(simulator.bodies("PUT", "aircraft/count")).toEqual([{ runId: "run-1", count: 0 }]);
  await expect(page.getByLabel("Aircraft count")).toHaveValue("");
});

test("pause sends zero and resume sends the last confirmed positive speed", async ({ page }) => {
  const simulator = await openManager(page, { speedHundredths: 250 });
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(fact(page, "Clock")).toHaveText("Paused");
  await page.getByRole("button", { name: "Resume at 2.5x" }).click();
  await expect(fact(page, "Clock")).toHaveText("Running at 2.5x real time");
  expect(simulator.bodies("PUT", "time/speed")).toEqual([
    { runId: "run-1", speedHundredths: 0 },
    { runId: "run-1", speedHundredths: 250 },
  ]);
});

test("an initially paused run resumes at the configured speed", async ({ page }) => {
  const simulator = await openManager(page, { speedHundredths: 0 }, { flowing: true });
  await expect(page.getByRole("button", { name: "Pause" })).toBeDisabled();
  await page.getByRole("button", { name: "Resume at 1x" }).click();
  await expect(fact(page, "Clock")).toHaveText("Running at 1x real time");
  expect(simulator.bodies("PUT", "time/speed")).toEqual([{ runId: "run-1", speedHundredths: 100 }]);
});

test("paused virtual time does not advance with browser time", async ({ page }) => {
  const simulator = await openManager(page, { speedHundredths: 0 }, { flowing: true });
  simulator.advance(30);
  await poll(page);
  await poll(page);
  await expect(fact(page, "Virtual time")).toHaveText("2024-03-05T12:00:00Z");
  await expect(fact(page, "Last successful update (real time)")).toContainText("0 s ago");
});

test("accepts every speed bound and rejects malformed drafts without sending", async ({ page }) => {
  const simulator = await openManager(page);
  const speed = page.getByLabel("Speed");
  const submit = page.getByRole("button", { name: "Set speed" });
  for (const value of ["0", "1", "100000"]) {
    await speed.fill(value);
    await submit.click();
    await expect(page.getByText(`Speed ${value} accepted`, { exact: false })).toBeVisible();
  }
  for (const value of ["100001", "-1", "1.5", "01", "abc", " ", "1e3"]) {
    await speed.fill(value);
    await submit.click();
    await expect(page.locator(".tb-field-error").nth(1)).not.toBeEmpty();
    await expect(speed).toHaveValue(value);
  }
  expect(simulator.bodies("PUT", "time/speed").map((body) => body.speedHundredths)).toEqual([0, 1, 100000]);
});

test("rejected commands keep the draft and show code, message and field", async ({ page }) => {
  const simulator = await openManager(page);
  simulator.failNext("PUT", "aircraft/count", 400, {
    error: { code: "invalid", message: "count 7 exceeds the configured spawn budget", field: "$.count", runId: "run-1" },
  });
  await page.getByLabel("Aircraft count").fill("7");
  await page.getByRole("button", { name: "Set count" }).click();
  const alert = page.getByRole("region", { name: "Controls" }).locator(".tb-error");
  await expect(alert).toContainText("count 7 exceeds the configured spawn budget");
  await expect(alert).toContainText("code invalid, field $.count");
  await expect(page.getByLabel("Aircraft count")).toHaveValue("7");
  await expect(fact(page, "Aircraft count")).toHaveText("2");
});

test("a network failure has an unknown outcome and is never replayed", async ({ page }) => {
  const simulator = await openManager(page);
  simulator.abortNext("PUT", "aircraft/count");
  await page.getByLabel("Aircraft count").fill("5");
  await page.getByRole("button", { name: "Set count" }).click();
  await expect(page.getByRole("region", { name: "Controls" }).locator(".tb-error")).toContainText("is unknown");
  await poll(page);
  await poll(page);
  expect(simulator.bodies("PUT", "aircraft/count")).toHaveLength(1);
  await expect(page.getByLabel("Aircraft count")).toHaveValue("5");
});

test("a failed poll marks truth stale and disables submissions", async ({ page }) => {
  const simulator = await openManager(page);
  simulator.failNext("GET", "truth", 503, { error: { code: "unavailable", message: "driver stopped", field: "", runId: "run-1" } });
  await poll(page);
  await expect(page.getByText("Showing the last successful simulator state.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Set count" })).toBeDisabled();
  await expect(truthRows(page)).toHaveCount(2);
  await poll(page);
  await expect(page.getByText("Showing the last successful simulator state.")).toBeHidden();
  await expect(page.getByRole("button", { name: "Set count" })).toBeEnabled();
});

test("an older poll response cannot overwrite state after a command", async ({ page }) => {
  const simulator = await openManager(page);
  const held = simulator.hold("GET", "truth");
  await poll(page, { settle: false });
  await held.arrived;
  await page.getByLabel("Aircraft count").fill("0");
  await page.getByRole("button", { name: "Set count" }).click();
  await expect.poll(() => simulator.bodies("PUT", "aircraft/count").length).toBe(1);
  held.release();
  await expect(page.getByText("No simulated aircraft.")).toBeVisible();
  await poll(page);
  await expect(truthRows(page)).toHaveCount(0);
});

test("a restart clears old truth and requires draft review", async ({ page }) => {
  const simulator = await openManager(page);
  await page.getByLabel("Aircraft count").fill("3");
  simulator.restart("run-2");
  simulator.state.aircraft = [];
  await poll(page);
  await expect(fact(page, "Run ID")).toHaveText("run-2");
  await expect(page.getByText("Drafts written for run run-1 are kept.", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "Set count" })).toBeDisabled();
  await expect(page.getByLabel("Aircraft count")).toHaveValue("3");
  await page.getByRole("button", { name: "Drafts reviewed" }).click();
  await page.getByRole("button", { name: "Set count" }).click();
  await expect.poll(() => simulator.bodies("PUT", "aircraft/count")).toEqual([{ runId: "run-2", count: 3 }]);
});

test("a command for a replaced run is rejected and the new run is loaded", async ({ page }) => {
  const simulator = await openManager(page);
  simulator.restart("run-2");
  await page.getByLabel("Aircraft count").fill("1");
  await page.getByRole("button", { name: "Set count" }).click();
  await expect(page.getByRole("region", { name: "Controls" }).locator(".tb-error")).toContainText("code conflict");
  await expect(fact(page, "Run ID")).toHaveText("run-2");
  expect(simulator.bodies("PUT", "aircraft/count")).toEqual([{ runId: "run-1", count: 1 }]);
});

test("two tabs observe accepted values while keeping their own drafts", async ({ context }) => {
  const tabA = await context.newPage();
  const tabB = await context.newPage();
  await tabA.clock.install();
  await tabB.clock.install();
  const simulator = await fakeSimulator(context);
  await tabA.goto("/manager/");
  await tabB.goto("/manager/");
  await expect(fact(tabB, "Aircraft count")).toHaveText("2");
  await tabB.getByLabel("Aircraft count").fill("9");

  await tabA.getByLabel("Aircraft count").fill("4");
  await tabA.getByRole("button", { name: "Set count" }).click();
  await expect(fact(tabA, "Aircraft count")).toHaveText("4");
  await tabB.clock.runFor(1000);
  await expect(fact(tabB, "Aircraft count")).toHaveText("4");
  await expect(tabB.getByLabel("Aircraft count")).toHaveValue("9");
  await expect(tabB.getByText("Unsaved draft; current value is 4")).toBeVisible();
  expect(simulator.bodies("PUT", "aircraft/count")).toEqual([{ runId: "run-1", count: 4 }]);
});

test("virtual time and real update time are labeled separately", async ({ page }) => {
  const simulator = await openManager(page);
  simulator.advance(5);
  await poll(page);
  await expect(fact(page, "Virtual time")).toHaveText("2024-03-05T12:00:05Z");
  await expect(facts(page).locator("dt", { hasText: "Last successful update (real time)" })).toBeVisible();
});

test("a draft typed while a command is pending is not cleared by its acknowledgement", async ({ page }) => {
  const simulator = await openManager(page);
  const held = simulator.hold("PUT", "aircraft/count");
  await page.getByLabel("Aircraft count").fill("0");
  await page.getByRole("button", { name: "Set count" }).click();
  await held.arrived;
  await page.getByLabel("Aircraft count").fill("4");
  held.release();
  await expect(page.getByText("Aircraft count 0 accepted")).toBeVisible();
  await expect(page.getByLabel("Aircraft count")).toHaveValue("4");
});
