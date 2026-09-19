// Combined and separate deployments at root and nested prefixes.
// Layer: integration with real Go handlers served by the fixture host; no
// API response is intercepted. Browser timers run in real time here, so
// assertions wait for conditions, never for fixed delays.
import { ports } from "../../playwright.config.mjs";
import { expect, test } from "./support.mjs";

const origin = (port) => `http://127.0.0.1:${port}`;

const deployments = [
  { name: "combined at root", manager: `${origin(ports.combined)}/manager/`, aircraft: `${origin(ports.combined)}/aircraft/`,
    apiPrefix: "/", simulatorOrigin: null },
  { name: "combined nested", manager: `${origin(ports.main)}/int/a/manager/`, aircraft: `${origin(ports.main)}/int/a/aircraft/`,
    apiPrefix: "/int/a/", simulatorOrigin: null },
  { name: "separate at root", manager: `${origin(ports.simulator)}/manager/`, aircraft: `${origin(ports.display)}/aircraft/`,
    apiPrefix: "/", simulatorOrigin: origin(ports.simulator) },
  { name: "separate nested", manager: `${origin(ports.simulator)}/nested/sim/manager/`,
    aircraft: `${origin(ports.display)}/nested/display/aircraft/`, apiPrefix: "/nested/display/", simulatorOrigin: origin(ports.simulator) },
];

const wait = { timeout: 15_000 };
const fact = (page, term) => page.getByRole("region", { name: "Run and clock" }).getByText(term, { exact: true })
  .locator("xpath=following-sibling::dd[1]");

for (const deployment of deployments) {
  test(`${deployment.name}: manager changes reach the received display`, async ({ page, context }) => {
    await page.goto(deployment.manager);
    await expect(fact(page, "Run ID")).toHaveText(/-run-\d+$/, wait);
    const runId = await fact(page, "Run ID").textContent();

    // Toggle bravo and remember the resulting state for the display side.
    const bravo = page.getByRole("article", { name: "Station bravo" });
    await expect(bravo).toContainText("revision", wait);
    const toggle = bravo.getByRole("button", { name: /^(Disable|Enable)$/ });
    const action = await toggle.textContent();
    await toggle.click();
    await expect(bravo.getByText("Accepted at revision")).toBeVisible(wait);
    const accepted = (await bravo.getByText("Accepted at revision").textContent()).match(/revision (\d+)/)[1];

    // New aircraft emit creation reports that the paused stations receive.
    const count = page.getByLabel("Aircraft count");
    await count.fill("0");
    await page.getByRole("button", { name: "Set count" }).click();
    await expect(page.getByText("Aircraft count 0 accepted")).toBeVisible(wait);
    await expect(fact(page, "Aircraft count")).toHaveText("0", wait);
    await count.fill("4");
    await page.getByRole("button", { name: "Set count" }).click();
    await expect(page.getByRole("region", { name: "Simulator truth" }).locator("tbody tr")).toHaveCount(4, wait);

    const display = await context.newPage();
    const requests = [];
    display.on("request", (request) => requests.push(request.url()));
    await display.goto(deployment.aircraft);
    const state = action === "Disable" ? "disabled" : "enabled";
    await expect(display.getByRole("checkbox", { name: `bravo (${state}, revision ${accepted})` })).toBeVisible(wait);
    await expect(display.getByRole("checkbox", { name: /^alpha/ })).toBeChecked();
    const rows = display.getByRole("table", { name: "Received aircraft tracks" }).locator("tbody tr");
    await expect.poll(() => rows.count(), wait).toBeGreaterThan(0);
    await expect(display.getByText(`Run ${runId}.`, { exact: false })).toBeVisible(wait);

    const history = display.getByRole("region", { name: "Station reception history" });
    await history.getByLabel("Station").selectOption("alpha");
    await history.getByRole("button", { name: "Load history" }).click();
    await expect(history.locator("tbody tr").first().locator(".tb-frame")).toHaveText(/^[0-9A-F]{28}$/, wait);

    const apiRequests = requests.filter((url) => new URL(url).pathname.includes("/api/"));
    expect(apiRequests.length).toBeGreaterThan(0);
    for (const url of apiRequests) {
      expect(new URL(url).pathname.startsWith(`${deployment.apiPrefix}api/display/`), url).toBe(true);
    }
    if (deployment.simulatorOrigin) {
      expect(requests.filter((url) => url.startsWith(deployment.simulatorOrigin)),
        "a separate display browser never contacts the simulator origin").toEqual([]);
    }
    await display.close();
  });
}

test("pages render within a narrow viewport without horizontal page scrolling", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 800 });
  for (const url of [deployments[1].manager, deployments[1].aircraft]) {
    await page.goto(url);
    await expect(page.locator(".tb-component > .tb-status")).not.toHaveText("Loading...", wait);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, url).toBeLessThanOrEqual(0);
  }
});

test("labels, keyboard activation and error announcements are exposed", async ({ page }) => {
  await page.goto(deployments[1].manager);
  await expect(page.getByRole("status").first()).toBeVisible();
  // Submissions are ignored until the manager has confirmed the run.
  await expect(page.getByRole("button", { name: "Set count" })).toBeEnabled();
  const count = page.getByRole("textbox", { name: /Aircraft count/ });
  await count.focus();
  await count.fill("1000");
  await page.keyboard.press("Enter");
  await expect(page.locator(".tb-field-error").first()).toContainText("Aircraft count must be within 0..");
  await expect(page.locator(".tb-field-error").first()).toHaveAttribute("role", "alert");
  await expect(count).toBeFocused();
});
