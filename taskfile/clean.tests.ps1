# Regression checks for clean.ps1. Run through `task clean-test`.
#
# Each case builds a temporary repository layout with sentinel files and
# asserts that cleanup removes only its declared output directories.
$ErrorActionPreference = "Stop"

$script = Join-Path $PSScriptRoot "clean.ps1"
$failures = 0

function New-Sentinel([string]$path) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $path) | Out-Null
  Set-Content -LiteralPath $path -Value "sentinel"
}

function Assert([bool]$condition, [string]$message) {
  if (-not $condition) {
    Write-Host "FAIL: $message"
    $script:failures++
  }
}

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("clean-test-" + [guid]::NewGuid())
$root = Join-Path $temp "repo"
$outside = Join-Path $temp "outside"
try {
  # Declared outputs are removed.
  New-Sentinel (Join-Path $root "bin/adsb-testbench.exe")
  New-Sentinel (Join-Path $root ".test-results/unit.log")
  New-Sentinel (Join-Path $root "playwright-report/index.html")
  # Everything else survives, including executables.
  $kept = @(
    "vendor/example/tool.exe",
    "node_modules/.bin/playwright.exe",
    "node_modules/pkg/index.js",
    ".cache-browsers/chromium.exe",
    "cmd/simulator/local.exe",
    "tool.exe"
  )
  foreach ($path in $kept) { New-Sentinel (Join-Path $root $path) }

  & pwsh -NoProfile -NonInteractive -File $script -Root $root | Out-Null
  Assert ($LASTEXITCODE -eq 0) "cleanup of a normal layout failed"
  Assert (-not (Test-Path (Join-Path $root "bin"))) "bin was not removed"
  Assert (-not (Test-Path (Join-Path $root ".test-results"))) ".test-results was not removed"
  Assert (-not (Test-Path (Join-Path $root "playwright-report"))) "playwright-report was not removed"
  foreach ($path in $kept) {
    Assert (Test-Path (Join-Path $root $path)) "$path was removed"
  }

  # A junction in place of an output directory is refused, and the directory
  # it points to survives.
  New-Sentinel (Join-Path $outside "keep.txt")
  New-Item -ItemType Junction -Path (Join-Path $root "site") -Target $outside | Out-Null
  & pwsh -NoProfile -NonInteractive -File $script -Root $root 2>&1 | Out-Null
  Assert ($LASTEXITCODE -ne 0) "cleanup followed or ignored a junction"
  Assert (Test-Path (Join-Path $outside "keep.txt")) "junction target content was removed"
}
finally {
  $link = Join-Path $root "site"
  if (Test-Path -LiteralPath $link) { (Get-Item -LiteralPath $link -Force).Delete() }
  Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}

if ($failures -gt 0) {
  Write-Host "clean tests: $failures failure(s)"
  exit 1
}
Write-Host "clean tests passed"
