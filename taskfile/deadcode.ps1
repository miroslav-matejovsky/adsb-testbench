# Check for unreachable functions in cmd entry points.
#
# Use the allowlist for three cases:
#  - functions staged for future use that are not reachable yet (temporary);
#  - unexported marker methods that seal an event interface. A seal exists to stop
#    another package from implementing the interface; calling it would defeat the
#    point, so it is unreachable by construction and always will be;
#  - test-support API: a function that exists so a test can assert something about
#    the production code itself. It is unreachable from a cmd entry point by nature,
#    and the alternative (dropping it) means dropping the assertion it enables.
$allow = @(
)

$out = deadcode ./cmd/... 2>&1
$exit = $LASTEXITCODE

$lines = $out | Out-String -Stream | Where-Object { $_.Trim() -ne '' }

# A non-zero exit is a tool/build error, not a findings report: surface it verbatim.
if ($exit -ne 0) {
    $lines | ForEach-Object { Write-Host $_ }
    exit 1
}

# Drop allowlisted staged symbols; fail on any other unreachable function.
$remaining = $lines | Where-Object {
    if ($_ -match 'unreachable func:\s*(.+)$') {
        $allow -notcontains $matches[1].Trim()
    } else {
        $true
    }
}

if ($remaining) {
    Write-Host "dead code found:"
    $remaining | ForEach-Object { Write-Host $_ }
    exit 1
}

Write-Host "deadcode: no issues found (allowlisted symbols skipped)"
