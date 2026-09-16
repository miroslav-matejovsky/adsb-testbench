Set-StrictMode -Version Latest # Enforces strict language rules (no uninitialized variables, no invalid property access)
$ErrorActionPreference = 'Stop' # Converts all non-terminating errors into terminating ones (fail immediately)
$PSNativeCommandUseErrorActionPreference = $true # Treats non-zero exit codes from native executables as errors
trap { exit 1 } # Ensures script exits immediately with non-zero code on any unhandled error
# $ProgressPreference = 'SilentlyContinue' # Suppresses progress output (avoids noise and side effects in automation)

function Enable-LocalEnv {
    $env:SOME_KEY = 'some-value'
}

Enable-LocalEnv
