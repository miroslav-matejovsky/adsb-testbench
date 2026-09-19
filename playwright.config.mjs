// Playwright configuration for the UI browser tests in ui/browser.
//
// Setup is explicit and never happens during a test run:
//   npm ci
//   npx playwright install chromium
// The fixture host is started from Go source and must answer /ready first.
import { defineConfig, devices } from "@playwright/test";

// Loopback ports of the fixture host: presentation pages and nested
// integration benches, the root combined bench, the separate simulators,
// and the separate displays reading those simulators.
export const ports = Object.freeze({ main: 18431, combined: 18432, simulator: 18433, display: 18434 });
const port = ports.main;

export default defineConfig({
  testDir: "ui/browser",
  testMatch: "*.spec.mjs",
  outputDir: ".test-results/playwright",
  reporter: [["list"], ["html", { outputFolder: ".test-results/playwright-report", open: "never" }]],
  forbidOnly: true,
  retries: 0,
  fullyParallel: true,
  timeout: 30_000,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "retain-on-failure",
    serviceWorkers: "block",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
  webServer: {
    command: "go run ./ui/testdata/browserhost" +
      ` -listen 127.0.0.1:${ports.main} -listen-combined 127.0.0.1:${ports.combined}` +
      ` -listen-simulator 127.0.0.1:${ports.simulator} -listen-display 127.0.0.1:${ports.display}`,
    url: `http://127.0.0.1:${port}/ready`,
    reuseExistingServer: false,
    timeout: 180_000,
    stdout: "ignore",
    stderr: "pipe",
  },
});
