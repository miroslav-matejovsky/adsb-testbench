# Re-sync the committed vendor/ tree after changing dependencies.
#
# Runs `go mod tidy` + `go mod vendor`. If a future dependency pair ever introduces
# a case-only path collision (two modules whose paths differ only by the case of a
# segment), `go mod vendor` on the case-insensitive Windows filesystem collapses
# them into one directory and `task release:dryrun` surfaces it as a Linux build
# failure; normalize the physical directory name here when that happens.

$ErrorActionPreference = "Stop"

$root = git rev-parse --show-toplevel
if (-not $root) { throw "not in a git repository" }
Set-Location $root

Write-Host "vendor: syncing go.mod and vendor/"

go mod tidy
if ($LASTEXITCODE -ne 0) { throw "go mod tidy failed" }

go mod vendor
if ($LASTEXITCODE -ne 0) { throw "go mod vendor failed" }

Write-Host "vendor done"
