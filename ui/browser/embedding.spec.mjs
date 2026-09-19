// Independent benches, host-built layouts and repeated mount/destroy.
// Layer: integration with real Go handlers for benches C and D and the host
// components page; presentation with intercepted payloads for the
// mount/destroy cycles, which need response barriers.
import { ports } from "../../playwright.config.mjs";
import { aircraftConfig, expect, openHarness, test } from "./support.mjs";
import { aircraft, at, fakeDisplay, isoAt } from "./fixtures.mjs";

const main = `http://127.0.0.1:${ports.main}`;
const wait = { timeout: 15_000 };
const fact = (scope, term) => scope.getByRole("region", { name: "Run and clock" }).getByText(term, { exact: true })
  .locator("xpath=following-sibling::dd[1]");

test.describe.configure({ mode: "serial" });

test("two benches on one host never share state, URLs or runs", async ({ context }) => {
  const benchC = await context.newPage();
  const benchD = await context.newPage();
  const requestsC = [];
  benchC.on("request", (request) => requestsC.push(new URL(request.url()).pathname));
  await benchC.goto(`${main}/int/c/manager/`);
  await benchD.goto(`${main}/int/d/manager/`);
  await expect(fact(benchC, "Run ID")).toHaveText(/^c-run-\d+$/, wait);
  await expect(fact(benchD, "Run ID")).toHaveText(/^d-run-\d+$/, wait);
  const countD = await fact(benchD, "Aircraft count").textContent();
  const bravoD = await benchD.getByRole("article", { name: "Station bravo" }).locator("p").first().textContent();

  const next = String((Number(await fact(benchC, "Aircraft count").textContent()) + 1) % 5);
  await benchC.getByLabel("Aircraft count").fill(next);
  await benchC.getByRole("button", { name: "Set count" }).click();
  await expect(fact(benchC, "Aircraft count")).toHaveText(next, wait);
  const bravoC = benchC.getByRole("article", { name: "Station bravo" });
  await bravoC.getByRole("button", { name: /^(Disable|Enable)$/ }).click();
  await expect(bravoC.getByText("Accepted at revision")).toBeVisible(wait);

  await benchD.reload();
  await expect(fact(benchD, "Aircraft count")).toHaveText(countD, wait);
  await expect(benchD.getByRole("article", { name: "Station bravo" }).locator("p").first()).toHaveText(bravoD, wait);
  const outside = requestsC.filter((path) => !path.startsWith("/int/c/"));
  expect(outside).toEqual([]);
});

test("a host layout mounts components with different selections on one backend", async ({ page }) => {
  await page.goto(`${main}/int/components/`);
  await page.waitForFunction(() => window.hostReady === true);
  const alpha = page.locator("#host-alpha");
  const bravo = page.locator("#host-bravo");
  await expect(alpha.getByRole("checkbox", { name: /^alpha/ })).toBeChecked(wait);
  await expect(alpha.getByRole("checkbox", { name: /^bravo/ })).not.toBeChecked();
  await expect(bravo.getByRole("checkbox", { name: /^bravo/ })).toBeChecked(wait);
  await expect(bravo.getByRole("checkbox", { name: /^alpha/ })).not.toBeChecked();
  await expect(page.locator("#host-manager").getByRole("region", { name: "Run and clock" })).toContainText("c-run-", wait);

  const ids = await page.evaluate(() => [...document.querySelectorAll("[id]")].map((element) => element.id));
  expect(ids.length).toBe(new Set(ids).size);

  // Record only after destroy returns: a poll alpha sent before destroy is
  // allowed, any alpha request after it is a leak.
  await page.evaluate(() => window.hostComponents.alpha.destroy());
  const bodies = [];
  page.on("request", (request) => {
    if (request.url().endsWith("/int/c/api/display/observations")) {
      bodies.push(request.postData());
    }
  });
  await expect(alpha).toBeEmpty();
  await expect.poll(() => bodies.length, wait).toBeGreaterThan(1);
  expect(bodies.every((body) => body === '{"stationIds":["bravo"]}')).toBe(true);
  await expect(bravo.getByRole("checkbox", { name: /^bravo/ })).toBeChecked();
});

test("repeated mount and destroy with requests in flight leaves one clean instance", async ({ page }) => {
  await page.clock.install();
  const display = await fakeDisplay(page, {
    aircraftFor: () => [aircraft({
      icao: "ABC001", lastReceivedAt: isoAt(at(9)),
      position: { latitudeDegrees: 50, longitudeDegrees: 14, observedAt: isoAt(at(9)) },
    })],
  });
  await openHarness(page);
  const cycles = 8;
  for (let cycle = 0; cycle < cycles; cycle++) {
    const held = display.hold("POST", "observations");
    await page.evaluate((config) => window.tbHarness.mount("aircraft", "root-a", config), aircraftConfig());
    await held.arrived;
    const mutations = await page.evaluate(() => {
      const root = document.getElementById("root-a");
      window.tbHarness.destroy("root-a");
      window.tbMutations = 0;
      window.tbObserver = new MutationObserver((records) => {
        window.tbMutations += records.length;
      });
      window.tbObserver.observe(root, { childList: true, subtree: true, attributes: true, characterData: true });
      return window.tbHarness.handles["root-a"].pendingRequests;
    });
    expect(mutations).toBe(0);
    held.release();
    const before = display.calls.length;
    await page.clock.runFor(3000);
    expect(display.calls.length, `cycle ${cycle}: a destroyed component sends nothing`).toBe(before);
    expect(await page.evaluate(() => {
      window.tbObserver.disconnect();
      return window.tbMutations;
    }), `cycle ${cycle}: a late response changes no DOM`).toBe(0);
  }

  await page.evaluate((config) => window.tbHarness.mount("aircraft", "root-a", config), aircraftConfig());
  await expect(page.locator("#root-a .leaflet-container")).toHaveCount(1);
  await expect(page.locator("#root-a .tb-aircraft-marker")).toHaveCount(1);
  const start = display.count("POST", "observations");
  await page.clock.runFor(5000);
  await expect.poll(() => display.count("POST", "observations") - start).toBeGreaterThanOrEqual(4);
  expect(display.count("POST", "observations") - start, "exactly one polling loop remains").toBeLessThanOrEqual(5);
  await expect(page.locator(".leaflet-container")).toHaveCount(1);
});
