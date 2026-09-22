# Load .env into the process environment and run the API locally.
#   pwsh scripts/run-local.ps1               # runs: go run ./cmd/api
#   pwsh scripts/run-local.ps1 -Socks 1080   # also routes IDBI calls through a local SOCKS proxy
param(
    [int]$Socks = 0
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root ".env"

if (-not (Test-Path $envFile)) {
    Write-Error ".env not found at $envFile"
}

Get-Content $envFile | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }
    $i = $line.IndexOf("=")
    if ($i -lt 1) { return }
    $name = $line.Substring(0, $i).Trim()
    $value = $line.Substring($i + 1).Trim()
    # strip an inline "  # comment" and surrounding quotes
    $value = ($value -replace '\s+#.*$', '').Trim().Trim('"').Trim("'")
    [Environment]::SetEnvironmentVariable($name, $value, "Process")
    Write-Host "  set $name"
}

if ($Socks -gt 0) {
    $env:HTTPS_PROXY = "socks5://127.0.0.1:$Socks"
    $env:HTTP_PROXY = "socks5://127.0.0.1:$Socks"
    Write-Host "  routing outbound HTTP(S) via socks5://127.0.0.1:$Socks"
}

Push-Location $root
try {
    go run ./cmd/api
} finally {
    Pop-Location
}
