# Task scripts

PowerShell helpers invoked through the root `Taskfile.yml`.

| Script | Task | Purpose |
| --- | --- | --- |
| `clean.ps1` | `task clean` | Remove only the declared output directories (`.test-results`, `.cache`, `site`, `bin`, `playwright-report`) inside the repository; refuses links and junctions |
| `clean.tests.ps1` | `task clean-test` | Regression checks: outputs removed, vendor, `node_modules`, caches and other executables kept |
| `test.ps1` | `task test` | Go unit tests with output saved to `.test-results/` |
| `ui-test.ps1` | `task ui-test` | Browser module unit tests and Playwright tests; needs `npm ci` and `npx playwright install chromium` |
| `deadcode.ps1` | `task deadcode` | Command reachability check with an explicit allowlist |
| `vendor.ps1` | `task vendor` | Re-sync a committed vendor tree after dependency changes |

`task all` runs tidy, vet, formatting, command reachability, architecture
lint, code lint, Go tests, cleanup checks, the external embedding module and
browser tests. Every script propagates child failures.

Other tasks:

| Task | Purpose |
| --- | --- |
| `task build` | Build the three commands into `bin/` |
| `task run-combined`, `task run-simulator`, `task run-display` | Run a command; `CONFIG` and `CONFIG_MAX_BYTES` are required, for example `task run-combined CONFIG=configs/combined.json CONFIG_MAX_BYTES=65536` |
| `task examples` | Vet and test the external embedding module |
