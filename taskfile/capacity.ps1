# Run the capacity benchmarks and the opt-in measurement reports, and store
# their raw output with an environment manifest.
#
# Every measurement input is explicit: there is no default sample count,
# benchmark time, CPU count or output directory. The output directory must
# resolve inside the repository .test-results directory, must not be a link
# or junction, and an existing tree is never deleted.
param(
  # Number of benchmark samples per case.
  [Parameter(Mandatory = $true)][string]$Count,
  # Go benchmark time, for example 1s or 50x.
  [Parameter(Mandatory = $true)][string]$Benchtime,
  # GOMAXPROCS and the benchmark -cpu value.
  [Parameter(Mandatory = $true)][string]$Cpu,
  # Output directory, inside .test-results.
  [Parameter(Mandatory = $true)][string]$Out,
  # Repository root. Tests pass a temporary directory.
  [string]$Root = (Split-Path -Parent $PSScriptRoot),
  # Go command. Tests pass a stub executable.
  [string]$GoExecutable = "go"
)

$ErrorActionPreference = "Stop"

function Fail([string]$message) {
  Write-Host "capacity: $message"
  exit 1
}

# --- argument validation -----------------------------------------------------

[int]$sampleCount = 0
if (-not [int]::TryParse($Count, [ref]$sampleCount) -or $sampleCount -le 0) {
  Fail "COUNT must be a positive integer, got '$Count'"
}
[int]$cpuCount = 0
if (-not [int]::TryParse($Cpu, [ref]$cpuCount) -or $cpuCount -le 0) {
  Fail "CPU must be a positive integer, got '$Cpu'"
}
if ($Benchtime -notmatch '^\d+(x|ns|us|ms|s|m|h)$') {
  Fail "BENCHTIME must be a Go benchmark time such as 1s or 50x, got '$Benchtime'"
}

$rootPath = (Resolve-Path -LiteralPath $Root).Path.TrimEnd([System.IO.Path]::DirectorySeparatorChar)
$resultsRoot = Join-Path $rootPath ".test-results"
$outPath = $Out
if (-not [System.IO.Path]::IsPathRooted($outPath)) {
  $outPath = Join-Path $rootPath $outPath
}
$outPath = [System.IO.Path]::GetFullPath($outPath).TrimEnd([System.IO.Path]::DirectorySeparatorChar)
$resultsPrefix = $resultsRoot + [System.IO.Path]::DirectorySeparatorChar
if (-not $outPath.StartsWith($resultsPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
  Fail "OUT must resolve inside '$resultsRoot', got '$outPath'"
}

foreach ($candidate in @($resultsRoot, $outPath)) {
  if (Test-Path -LiteralPath $candidate) {
    $item = Get-Item -LiteralPath $candidate -Force
    if ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) {
      Fail "refusing to write through '$candidate': it is a symbolic link or junction"
    }
  }
}
# An existing tree is kept: results are added, never replaced.
New-Item -ItemType Directory -Force -Path $outPath | Out-Null

# --- environment manifest ----------------------------------------------------

function Invoke-Capture([string]$file, [string]$exe, [string[]]$exeArgs) {
  $path = Join-Path $outPath $file
  Write-Host "capacity: $exe $($exeArgs -join ' ') -> $file"
  & $exe @exeArgs 2>&1 | Tee-Object -FilePath $path | Out-Host
  return $LASTEXITCODE
}

function Optional([scriptblock]$block, [string]$fallback = "unavailable") {
  try {
    $value = & $block
    if ($null -eq $value -or "$value" -eq "") { return $fallback }
    return "$value"
  }
  catch {
    return $fallback
  }
}

function EnvOrUnset([string]$name) {
  $value = [Environment]::GetEnvironmentVariable($name)
  if ($null -eq $value -or $value -eq "") { return "unset" }
  return $value
}

$startedAt = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$workloadFile = Join-Path $rootPath "testdata/quality/capacity.json"
$manifestPath = Join-Path $outPath "manifest.txt"
$manifest = [System.Collections.Generic.List[string]]::new()
$manifest.Add("started $startedAt")
$manifest.Add("count $sampleCount")
$manifest.Add("benchtime $Benchtime")
$manifest.Add("cpu $cpuCount")
$manifest.Add("out $outPath")
$manifest.Add("revision " + (Optional { git -C $rootPath rev-parse HEAD 2>$null }))
$manifest.Add("go " + (Optional { & $GoExecutable version }))
$manifest.Add("goos " + (Optional { & $GoExecutable env GOOS }))
$manifest.Add("goarch " + (Optional { & $GoExecutable env GOARCH }))
$manifest.Add("gomaxprocs $cpuCount")
$manifest.Add("gogc " + (EnvOrUnset "GOGC"))
$manifest.Add("gomemlimit " + (EnvOrUnset "GOMEMLIMIT"))
$manifest.Add("node " + (Optional { node --version }))
$manifest.Add("cpu-model " + (Optional { (Get-CimInstance Win32_Processor | Select-Object -First 1).Name }))
$manifest.Add("cpu-logical " + (Optional { [Environment]::ProcessorCount }))
$manifest.Add("memory-bytes " + (Optional { (Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory }))
$manifest.Add("os " + (Optional { (Get-CimInstance Win32_OperatingSystem).Caption + " " + (Get-CimInstance Win32_OperatingSystem).Version }))
$manifest.Add("workload-file testdata/quality/capacity.json")
$manifest.Add("workload-sha256 " + (Optional { (Get-FileHash -LiteralPath $workloadFile -Algorithm SHA256).Hash }))

# A revision alone cannot reproduce an uncommitted implementation, so the
# dirty file list, the tracked diff and the checksums of untracked first-party
# source files are stored as well.
$dirty = Optional { (git -C $rootPath status --porcelain 2>$null) -join "`n" } ""
$manifest.Add("dirty-files")
foreach ($line in ($dirty -split "`n")) {
  if ($line.Trim() -ne "") { $manifest.Add("  $line") }
}
Optional { (git -C $rootPath diff HEAD 2>$null) -join "`n" } "" | Set-Content -LiteralPath (Join-Path $outPath "tracked.diff")

$sourceDir = Join-Path $outPath "source"
New-Item -ItemType Directory -Force -Path $sourceDir | Out-Null
$untracked = Optional { (git -C $rootPath ls-files --others --exclude-standard 2>$null) -join "`n" } ""
$manifest.Add("untracked-sources")
foreach ($relative in ($untracked -split "`n")) {
  $name = $relative.Trim()
  if ($name -eq "") { continue }
  if ($name -notmatch '\.(go|ps1|json|md|mjs|js)$') { continue }
  if ($name -like ".test-results/*" -or $name -like "node_modules/*" -or $name -like "vendor/*") { continue }
  $full = Join-Path $rootPath $name
  if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
  $hash = (Get-FileHash -LiteralPath $full -Algorithm SHA256).Hash
  $manifest.Add("  $hash  $name")
  $target = Join-Path $sourceDir ($name -replace '[\\/]', '_')
  Copy-Item -LiteralPath $full -Destination $target -Force
}

# --- measurements ------------------------------------------------------------

$commands = [System.Collections.Generic.List[string]]::new()
$failures = 0

$benchArgs = @(
  "test", "./simulation", "./internal/simdriver", "./simulator", "./display",
  "-run", "^$", "-bench", "^BenchmarkCapacity", "-benchmem",
  "-benchtime", $Benchtime, "-count", "$sampleCount", "-cpu", "$cpuCount", "-timeout", "120m"
)
$commands.Add("$GoExecutable $($benchArgs -join ' ')")
$previousMeasure = [Environment]::GetEnvironmentVariable("ADSB_CAPACITY_MEASURE")
$previousProcs = [Environment]::GetEnvironmentVariable("GOMAXPROCS")
try {
  $env:GOMAXPROCS = "$cpuCount"
  Push-Location $rootPath
  try {
    $code = Invoke-Capture "benchmarks.txt" $GoExecutable $benchArgs
    if ($code -ne 0) {
      Write-Host "capacity: benchmarks failed with exit code $code"
      $failures++
    }

    $env:ADSB_CAPACITY_MEASURE = "1"
    $reportArgs = @(
      "test", "./simulation", "./simulator", "./display",
      "-run", "^TestCapacity(Memory|Latency)Report$", "-count=1", "-v", "-timeout", "60m"
    )
    $commands.Add("$GoExecutable $($reportArgs -join ' ')")
    $code = Invoke-Capture "reports.txt" $GoExecutable $reportArgs
    if ($code -ne 0) {
      Write-Host "capacity: measurement reports failed with exit code $code"
      $failures++
    }
  }
  finally {
    Pop-Location
  }
}
finally {
  $env:ADSB_CAPACITY_MEASURE = $previousMeasure
  $env:GOMAXPROCS = $previousProcs
}

$manifest.Add("commands")
foreach ($command in $commands) { $manifest.Add("  $command") }
$manifest.Add("finished " + (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"))
$manifest.Add("failures $failures")
Set-Content -LiteralPath $manifestPath -Value $manifest

if ($failures -gt 0) {
  Fail "$failures measurement command(s) failed; see $outPath"
}
Write-Host "capacity: results in $outPath"
