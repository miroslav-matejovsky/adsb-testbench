# Remove build outputs and test results from the repository.
#
# Only the explicit output directories listed below are removed. Each target
# must resolve inside the repository root and must not be a symbolic link or
# junction, so cleanup can never follow a link out of the workspace. Nothing
# else is traversed: Go vendor, node_modules, browser caches, and third-party
# files are never touched.
param(
  # Repository root. Tests pass a temporary directory.
  [string]$Root = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = "Stop"

$outputs = @(".test-results", ".cache", "site", "bin", "playwright-report")

$rootPath = (Resolve-Path -LiteralPath $Root).Path.TrimEnd([System.IO.Path]::DirectorySeparatorChar)

foreach ($name in $outputs) {
  $target = Join-Path $rootPath $name
  if (-not (Test-Path -LiteralPath $target)) {
    continue
  }
  $item = Get-Item -LiteralPath $target -Force
  if ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) {
    throw "refusing to remove '$target': it is a symbolic link or junction"
  }
  $full = $item.FullName.TrimEnd([System.IO.Path]::DirectorySeparatorChar)
  $parent = Split-Path -Parent $full
  if ($parent -ne $rootPath) {
    throw "refusing to remove '$target': it resolves outside '$rootPath'"
  }
  Write-Host "removing folder: $name"
  Remove-Item -LiteralPath $full -Recurse -Force
}
# this script is called from Taskfile, so the final message is printed by Taskfile, not here
