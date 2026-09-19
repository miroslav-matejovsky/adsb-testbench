# Run the browser module unit tests and the Playwright browser tests.
#
# Prerequisites are installed explicitly, never during a test run:
#   npm ci
#   npx playwright install chromium
# Results, traces and the HTML report of failing runs are kept below
# .test-results/.
$ErrorActionPreference = "Stop"

if (-not (Test-Path "node_modules/@playwright/test/package.json")) {
  Write-Host "browser test dependencies are missing; run: npm ci; npx playwright install chromium"
  exit 1
}

npm run --silent test:unit
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}

npx --no-install playwright test
exit $LASTEXITCODE
