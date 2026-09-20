# Regression checks for capacity.ps1. Run through `task capacity-test`.
#
# The measurement commands themselves are not run here: a stub executable
# stands in for Go, so these checks stay fast enough for `task all` and still
# cover argument validation, output-path rules and child failure propagation.
$ErrorActionPreference = "Stop"

$script = Join-Path $PSScriptRoot "capacity.ps1"
$failures = 0

function Assert([bool]$condition, [string]$message) {
  if (-not $condition) {
    Write-Host "FAIL: $message"
    $script:failures++
  }
}

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("capacity-test-" + [guid]::NewGuid())
$root = Join-Path $temp "repo"
$outside = Join-Path $temp "outside"
New-Item -ItemType Directory -Force -Path (Join-Path $root "testdata/quality") | Out-Null
New-Item -ItemType Directory -Force -Path $outside | Out-Null
Set-Content -LiteralPath (Join-Path $root "testdata/quality/capacity.json") -Value "{}"

# Stub executables standing in for Go: one succeeds, one fails.
$okStub = Join-Path $temp "go-ok.ps1"
Set-Content -LiteralPath $okStub -Value 'param([Parameter(ValueFromRemainingArguments = $true)]$rest) Write-Output "stub ok $rest"; exit 0'
$failStub = Join-Path $temp "go-fail.ps1"
Set-Content -LiteralPath $failStub -Value 'param([Parameter(ValueFromRemainingArguments = $true)]$rest) Write-Output "stub failure"; exit 3'

function Invoke-Capacity([string]$out, [string]$stub, [string]$count = "1", [string]$benchtime = "1x", [string]$cpu = "1") {
  & pwsh -NoProfile -NonInteractive -File $script -Count $count -Benchtime $benchtime -Cpu $cpu `
    -Out $out -Root $root -GoExecutable $stub 2>&1 | Out-String
}

try {
  # A complete run with a succeeding stub writes its raw output and manifest.
  $out = Join-Path $root ".test-results/capacity-ok"
  $output = Invoke-Capacity $out $okStub
  Assert ($LASTEXITCODE -eq 0) "a successful run exited with $LASTEXITCODE : $output"
  Assert (Test-Path (Join-Path $out "manifest.txt")) "the manifest was not written"
  Assert (Test-Path (Join-Path $out "benchmarks.txt")) "benchmark output was not written"
  Assert (Test-Path (Join-Path $out "reports.txt")) "report output was not written"
  $manifest = Get-Content -LiteralPath (Join-Path $out "manifest.txt") -Raw
  foreach ($key in @("started ", "count 1", "benchtime 1x", "cpu 1", "workload-sha256 ", "commands", "failures 0")) {
    Assert ($manifest.Contains($key)) "the manifest is missing '$key'"
  }

  # Existing content of the output tree survives a second run.
  Set-Content -LiteralPath (Join-Path $out "keep.txt") -Value "sentinel"
  Invoke-Capacity $out $okStub | Out-Null
  Assert ($LASTEXITCODE -eq 0) "a repeated run failed"
  Assert (Test-Path (Join-Path $out "keep.txt")) "an existing result file was deleted"

  # A failing child command fails the task.
  $output = Invoke-Capacity (Join-Path $root ".test-results/capacity-fail") $failStub
  Assert ($LASTEXITCODE -ne 0) "a failing measurement command did not fail the task"
  Assert ($output -match "exit code 3") "the child exit code was not reported: $output"

  # Rejected arguments.
  foreach ($case in @(
      @{ name = "zero count"; args = @{ count = "0" } },
      @{ name = "negative count"; args = @{ count = "-1" } },
      @{ name = "non-numeric count"; args = @{ count = "many" } },
      @{ name = "zero cpu"; args = @{ cpu = "0" } },
      @{ name = "non-numeric cpu"; args = @{ cpu = "all" } },
      @{ name = "invalid benchtime"; args = @{ benchtime = "fast" } }
    )) {
    $arguments = @{ out = (Join-Path $root ".test-results/capacity-args"); stub = $okStub }
    foreach ($key in $case.args.Keys) { $arguments[$key] = $case.args[$key] }
    Invoke-Capacity @arguments | Out-Null
    Assert ($LASTEXITCODE -ne 0) "$($case.name) was accepted"
  }

  # Rejected output paths.
  foreach ($candidate in @(
      (Join-Path $root "capacity-results"),
      (Join-Path $root ".test-results/../../escape"),
      $outside,
      ".test-results-sibling"
    )) {
    Invoke-Capacity $candidate $okStub | Out-Null
    Assert ($LASTEXITCODE -ne 0) "output path '$candidate' was accepted"
  }
  Assert (-not (Test-Path (Join-Path $root "capacity-results"))) "a rejected output path was created"

  # A junction in place of the results directory is refused.
  $linked = Join-Path $root ".test-results-linked"
  New-Item -ItemType Junction -Path $linked -Target $outside | Out-Null
  Invoke-Capacity (Join-Path $linked "run") $okStub | Out-Null
  Assert ($LASTEXITCODE -ne 0) "a junction output path was accepted"
}
finally {
  $link = Join-Path $root ".test-results-linked"
  if (Test-Path -LiteralPath $link) { (Get-Item -LiteralPath $link -Force).Delete() }
  Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}

if ($failures -gt 0) {
  Write-Host "capacity script checks failed: $failures"
  exit 1
}
Write-Host "capacity script checks passed"
