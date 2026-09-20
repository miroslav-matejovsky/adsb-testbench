# Task scripts

PowerShell helpers invoked through the root `Taskfile.yml`.

| Script | Task | Purpose |
| --- | --- | --- |
| `clean.ps1` | `task clean` | Remove only the declared output directories (`.test-results`, `.cache`, `site`, `bin`, `playwright-report`) inside the repository; refuses links and junctions |
| `clean.tests.ps1` | `task clean-test` | Regression checks: outputs removed, vendor, `node_modules`, caches and other executables kept |
| `test.ps1` | `task test` | Go unit tests with output saved to `.test-results/` |
| `capacity.ps1` | `task capacity` | Capacity benchmarks and the opt-in measurement reports, with an environment manifest; every input is required |
| `capacity.tests.ps1` | `task capacity-test` | Regression checks of the capacity script: argument validation, output-path rules, child failure propagation |
| `ui-test.ps1` | `task ui-test` | Browser module unit tests and Playwright tests; needs `npm ci` and `npx playwright install chromium` |
| `deadcode.ps1` | `task deadcode` | Command reachability check with an explicit allowlist |
| `vendor.ps1` | `task vendor` | Re-sync a committed vendor tree after dependency changes |

`task all` runs tidy, vet, formatting, command reachability, architecture
lint, code lint, Go tests, cleanup checks, the capacity script checks, the
external embedding module and browser tests. Every script propagates child
failures. Repeated benchmarks stay out of `task all`.

`task capacity` needs every measurement input explicitly and writes inside
`.test-results/`:

```text
task capacity COUNT=5 BENCHTIME=1s CPU=1 OUT=.test-results/capacity-cpu1
task capacity COUNT=5 BENCHTIME=1s CPU=4 OUT=.test-results/capacity-cpu4
```

`COUNT` is the number of samples per benchmark case, `BENCHTIME` a Go
benchmark time such as `1s` or `50x`, `CPU` the value of both `GOMAXPROCS`
and the benchmark `-cpu` flag, and `OUT` a directory that must resolve inside
`.test-results` and must not be a link or junction. The script never deletes
an existing tree. It writes `benchmarks.txt`, `reports.txt`, `manifest.txt`,
`tracked.diff` and a `source/` copy of untracked first-party files with their
SHA-256 checksums, so an uncommitted implementation stays reproducible. While
the reports run it sets `ADSB_CAPACITY_MEASURE=1` and `GOMAXPROCS`, and
restores both afterwards. `task all` cleans `.test-results` first, so copy
evidence that must survive into `docs/evaluations/`.

Other tasks:

| Task | Purpose |
| --- | --- |
| `task build` | Build the three commands into `bin/` |
| `task run-combined`, `task run-simulator`, `task run-display` | Run a command; `CONFIG` and `CONFIG_MAX_BYTES` are required, for example `task run-combined CONFIG=configs/combined.json CONFIG_MAX_BYTES=65536` |
| `task examples` | Vet and test the external embedding module |
