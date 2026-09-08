<#
.SYNOPSIS
  Open an SSH SOCKS proxy through the IP-whitelisted EC2 box so you can reach
  the IDBI Atlas portal / APIs (https://innobox.idbi.bank.in).

.DESCRIPTION
  The portal only allows the EC2 IP 43.205.52.148. This script opens a SOCKS5
  proxy on localhost that exits from that IP, optionally verifies access, and
  optionally launches an isolated Chrome pointed at it.

.PARAMETER Test
  After the tunnel is up, curl the portal and print the status. Then exit
  (closes the tunnel).

.PARAMETER Browser
  Launch an isolated Chrome routed through the proxy once the tunnel is up.

.PARAMETER Port
  Local SOCKS port. Default 1080.

.PARAMETER Key
  Path to the .pem. Default: %USERPROFILE%\.ssh\zeyro-idbi.pem

.EXAMPLE
  .\scripts\idbi-proxy.ps1
  # just open the tunnel, leave it running (Ctrl+C to stop)

.EXAMPLE
  .\scripts\idbi-proxy.ps1 -Test
  # open tunnel, check the portal returns 200, close

.EXAMPLE
  .\scripts\idbi-proxy.ps1 -Browser
  # open tunnel + launch proxied Chrome, tunnel stays up until you Ctrl+C
#>
param(
  [switch]$Test,
  [switch]$Browser,
  [int]$Port = 1080,
  [string]$Key = "$env:USERPROFILE\.ssh\zeyro-idbi.pem",
  [string]$Ec2User = "ec2-user",
  [string]$Ec2Ip = "43.205.52.148",
  [string]$PortalUrl = "https://innobox.idbi.bank.in"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Key)) {
  Write-Error "SSH key not found at: $Key`nPass -Key <path> or move the .pem there."
  exit 1
}

# SSH refuses a world-readable key; fix perms if they're too open.
$acl = (icacls $Key) -join "`n"
if ($acl -match "Authenticated Users|BUILTIN\\Users|Everyone") {
  Write-Host "Locking down key permissions..." -ForegroundColor Yellow
  icacls $Key /inheritance:r | Out-Null
  icacls $Key /grant:r "$($env:USERNAME):R" | Out-Null
}

# Bail early if the port is already taken.
if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
  Write-Error "Local port $Port is already in use. Pass -Port <n> to pick another."
  exit 1
}

Write-Host "Opening SOCKS proxy on localhost:$Port via $Ec2User@$Ec2Ip ..." -ForegroundColor Cyan

# Start ssh in the background so we can test / launch the browser, then wait on it.
$sshArgs = @(
  "-i", $Key,
  "-D", $Port,
  "-N", "-C",
  "-o", "ExitOnForwardFailure=yes",
  "-o", "ServerAliveInterval=30",
  "-o", "StrictHostKeyChecking=accept-new",
  "$Ec2User@$Ec2Ip"
)
$ssh = Start-Process ssh -ArgumentList $sshArgs -PassThru -NoNewWindow

# Give the tunnel a moment to establish.
Start-Sleep -Seconds 3
if ($ssh.HasExited) {
  Write-Error "SSH tunnel failed to start (exit $($ssh.ExitCode)). Try:  ssh -v -i `"$Key`" $Ec2User@$Ec2Ip"
  exit 1
}

function Stop-Tunnel {
  if (-not $ssh.HasExited) { Stop-Process -Id $ssh.Id -Force -ErrorAction SilentlyContinue }
}

try {
  # Sanity: confirm traffic actually exits the whitelisted IP.
  $egress = (& curl.exe -s --max-time 15 --proxy "socks5h://localhost:$Port" https://checkip.amazonaws.com).Trim()
  if ($egress -eq $Ec2Ip) {
    Write-Host "Egress IP via proxy: $egress  (matches whitelist)" -ForegroundColor Green
  } else {
    Write-Host "Egress IP via proxy: '$egress'  (expected $Ec2Ip - proxy may not be routing correctly)" -ForegroundColor Yellow
  }

  $status = (& curl.exe -s -o NUL -w "%{http_code}" --max-time 20 --proxy "socks5h://localhost:$Port" -I $PortalUrl)
  if ($status -eq "200") {
    Write-Host "$PortalUrl -> HTTP $status  (portal reachable)" -ForegroundColor Green
  } elseif ($status -eq "403") {
    Write-Host "$PortalUrl -> HTTP 403  (IP allow-list not letting us through - contact ACC/IDBI)" -ForegroundColor Red
  } else {
    Write-Host "$PortalUrl -> HTTP $status" -ForegroundColor Yellow
  }

  if ($Test) {
    Write-Host "`n-Test done, closing tunnel." -ForegroundColor Cyan
    Stop-Tunnel
    exit 0
  }

  if ($Browser) {
    $chrome = @(
      "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
      "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
      "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe"
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1

    if (-not $chrome) {
      Write-Host "Chrome not found - open your browser manually and set SOCKS proxy to localhost:$Port" -ForegroundColor Yellow
    } else {
      Write-Host "Launching isolated Chrome (only this window uses the proxy)..." -ForegroundColor Cyan
      Start-Process $chrome -ArgumentList @(
        "--proxy-server=socks5://localhost:$Port",
        "--user-data-dir=$env:TEMP\chrome-innobox",
        $PortalUrl
      )
    }
  }

  Write-Host "`nTunnel is up on localhost:$Port. Leave this window open. Ctrl+C to stop.`n" -ForegroundColor Cyan
  Wait-Process -Id $ssh.Id
}
finally {
  Stop-Tunnel
  Write-Host "Tunnel closed." -ForegroundColor Cyan
}
