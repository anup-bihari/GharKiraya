param([switch]$Demo, [string]$ListenAddress = '127.0.0.1:8080')
$ErrorActionPreference = 'Stop'
$previousEnvironment = @{}
foreach ($setting in @('GOCACHE', 'GOPATH', 'DEMO_MODE', 'ADDR', 'DATA_FILE')) {
    $previousEnvironment[$setting] = [Environment]::GetEnvironmentVariable($setting, 'Process')
}
Push-Location $PSScriptRoot
try {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    $goExecutable = if ($goCommand) { $goCommand.Source } else { Join-Path $PSScriptRoot '.tools/go/bin/go.exe' }
    if (-not (Test-Path -LiteralPath $goExecutable)) { throw 'Install Go 1.25 or later from https://go.dev/dl/, then run this script again.' }
    $env:GOCACHE = Join-Path $PSScriptRoot '.tools/cache'
    $env:GOPATH = Join-Path $PSScriptRoot '.tools/gopath'
    $env:DEMO_MODE = if ($Demo) { 'true' } else { 'false' }
    $env:ADDR = $ListenAddress
    # Keep demo accounts and advertisements separate from real user data.
    if (-not $env:DATA_FILE) { $env:DATA_FILE = if ($Demo) { 'data/demo.json' } else { 'data/state.json' } }
    & $goExecutable run .
    if ($LASTEXITCODE -ne 0) { throw 'The Go server exited with an error.' }
} finally {
    foreach ($setting in $previousEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($setting, $previousEnvironment[$setting], 'Process')
    }
    Pop-Location
}
