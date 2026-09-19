// Embedded pages, configuration and component lifetime.
// Layer: presentation with intercepted API payloads.
import {
  aircraftConfig, barrier, expect, json, managerConfig, openHarness, test,
} from "./support.mjs";
import { fakeDisplay, fakeSimulator } from "./fixtures.mjs";

test.describe("embedded pages", () => {
  for (const prefix of ["/", "/bench/a/"]) {
    test(`manager page loads every asset below ${prefix}`, async ({ page }) => {
      await fakeSimulator(page, { prefix: `${prefix}api/simulator/` });
      await page.goto(`${prefix}manager/`);
      await expect(page.getByRole("region", { name: "Simulator manager" })).toBeVisible();
      await expect(page.getByRole("region", { name: "Run and clock" })).toContainText("run-1");
      const outside = page.recorded.filter((request) => !new URL(request.url).pathname.startsWith(prefix));
      expect(outside).toEqual([]);
      expect(page.requestsTo(`${prefix}assets/ui.css`).length).toBeGreaterThan(0);
    });

    test(`aircraft page loads every asset below ${prefix}`, async ({ page }) => {
      await fakeDisplay(page, { prefix: `${prefix}api/display/` });
      await page.goto(`${prefix}aircraft/`);
      await expect(page.getByRole("region", { name: "Received aircraft", exact: true })).toBeVisible();
      expect(page.requestsTo(`${prefix}assets/leaflet/leaflet.css`).length).toBeGreaterThan(0);
      const outside = page.recorded.filter((request) => !new URL(request.url).pathname.startsWith(prefix));
      expect(outside).toEqual([]);
    });
  }

  test("both pages link the bundled notices", async ({ page }) => {
    await fakeSimulator(page);
    await page.goto("/manager/");
    await page.getByRole("link", { name: "Third-party notices" }).click();
    await expect(page.getByRole("heading", { name: "Third-party notices" })).toBeVisible();
    await page.getByRole("link", { name: "BSD 2-Clause" }).click();
    await expect(page.locator("body")).toContainText("Volodymyr Agafonkin");
  });
});

test.describe("component configuration", () => {
  test("invalid configuration throws before any request", async ({ page }) => {
    await openHarness(page);
    const before = page.recorded.length;
    const errors = await page.evaluate(({ manager, aircraft }) => {
      const attempts = [
        ["manager", { ...manager, pollIntervalMilliseconds: 0 }],
        ["manager", { ...manager, apiBaseUrl: "http://evil.test/api/" }],
        ["manager", { ...manager, apiBaseUrl: "//evil.test/api/" }],
        ["manager", { ...manager, assetBaseUrl: "/assets/../x/" }],
        ["aircraft", { ...aircraft, stationIds: null }],
        ["aircraft", { ...aircraft, extra: true }],
      ];
      return attempts.map(([kind, config]) => {
        try {
          window.tbHarness.mount(kind, "root-a", config);
          return "mounted";
        } catch (error) {
          return error.name;
        }
      });
    }, { manager: managerConfig(), aircraft: aircraftConfig() });
    expect(errors).toEqual(Array(6).fill("ConfigError"));
    expect(page.recorded.length).toBe(before);
    await expect(page.locator("#root-a")).toBeEmpty();
  });

  test("a root cannot host two components at once", async ({ page }) => {
    await fakeSimulator(page);
    await openHarness(page);
    const result = await page.evaluate((config) => {
      window.tbHarness.mount("manager", "root-a", config);
      try {
        window.tbHarness.mount("manager", "root-a", config);
        return "mounted twice";
      } catch (error) {
        return error.message;
      }
    }, managerConfig());
    expect(result).toContain("already hosts");
  });
});

test.describe("component lifetime", () => {
  test("a destroyed component sends no request and changes no DOM", async ({ page }) => {
    await page.clock.install();
    const gate = barrier();
    let requests = 0;
    await page.route("**/api/simulator/**", async (route) => {
      requests += 1;
      await gate.promise;
      await route.fulfill({ status: 200, contentType: "application/json", body: '{"runId":"late"}' }).catch(() => {});
    });
    await openHarness(page);
    await page.evaluate((config) => window.tbHarness.mount("manager", "root-a", config), managerConfig());
    await expect.poll(() => requests).toBeGreaterThan(0);
    const sent = requests;
    await page.evaluate(() => {
      window.tbHarness.destroy("root-a");
      window.tbHarness.destroy("root-a");
    });
    gate.release();
    await page.clock.runFor(10_000);
    await expect(page.locator("#root-a")).toBeEmpty();
    expect(requests).toBe(sent);
    expect(await page.evaluate(() => window.tbHarness.handles["root-a"].pendingRequests)).toBe(0);
  });

  test("a component is aria-busy only while its poll cycle runs", async ({ page }) => {
    await page.clock.install();
    const simulator = await fakeSimulator(page);
    await openHarness(page);
    await page.evaluate((config) => window.tbHarness.mount("manager", "root-a", config), managerConfig());
    const component = page.locator("#root-a .tb-component");
    await expect(component.locator("> .tb-status")).toHaveText(/current/);
    await expect(component).not.toHaveAttribute("aria-busy");

    const held = simulator.hold("GET", "truth");
    await page.clock.runFor(1000);
    await held.arrived;
    await expect(component).toHaveAttribute("aria-busy", "true");
    held.release();
    await expect(component).not.toHaveAttribute("aria-busy");
  });

  test("two roots poll independently and remount cleanly", async ({ page }) => {
    await page.clock.install();
    const simulator = await fakeSimulator(page);
    await openHarness(page);
    await page.evaluate(({ a, b }) => {
      window.tbHarness.mount("manager", "root-a", a);
      window.tbHarness.mount("manager", "root-b", b);
    }, { a: managerConfig(), b: managerConfig({ pollIntervalMilliseconds: 5000 }) });
    await expect(page.locator("#root-a .tb-component > .tb-status")).toHaveText(/current/);
    await expect(page.locator("#root-b .tb-component > .tb-status")).toHaveText(/current/);

    for (let cycle = 0; cycle < 5; cycle++) {
      await page.evaluate((config) => {
        window.tbHarness.destroy("root-a");
        window.tbHarness.mount("manager", "root-a", config);
      }, managerConfig());
    }
    await expect(page.locator("#root-a .tb-component")).toHaveCount(1);
    await page.evaluate(() => window.tbHarness.destroy("root-b"));

    const before = simulator.count("GET", "metadata");
    await page.clock.runFor(1000);
    await expect.poll(() => simulator.count("GET", "metadata") - before).toBeGreaterThan(0);
    const afterOne = simulator.count("GET", "metadata");
    await page.clock.runFor(1000);
    await expect.poll(() => simulator.count("GET", "metadata")).toBeGreaterThan(afterOne);
    // Only the remounted root A polls: one refresh per interval, not five.
    expect(simulator.count("GET", "metadata") - afterOne).toBeLessThanOrEqual(2);
    await expect(page.locator("#root-b")).toBeEmpty();
  });

  test("malformed error bodies surface an actionable error", async ({ page }) => {
    await page.route("**/api/simulator/**", (route) => json(route, "{broken", 500));
    await openHarness(page);
    await page.evaluate((config) => window.tbHarness.mount("manager", "root-a", config), managerConfig());
    await expect(page.locator("#root-a .tb-component > [role=alert]")).toContainText("not valid JSON");
  });
});
