# Accessing the IDBI Atlas Developer Portal & APIs

The IDBI / Applied Cloud Computing (ACC) **Atlas** portal and its APIs are
**IP allow-listed**. Only our EC2 instance `43.205.52.148` (ap-south-1,
Mumbai) is permitted — every request from any other IP gets `403 Forbidden`
from the load balancer (`Server: awselb/2.0`).

To use the portal or call the APIs, you tunnel your traffic through that EC2
box with an SSH SOCKS proxy. Nothing needs to be installed or changed on the
EC2 side.

- **Portal:** https://innobox.idbi.bank.in
- **Account / login:** in the team password manager (do **not** paste
  credentials into chat, tickets, or commits).

---

## Quick start (script)

Put `zeyro-idbi.pem` at `~/.ssh/zeyro-idbi.pem` (or `%USERPROFILE%\.ssh\` on
Windows), then:

**Windows (PowerShell):**
```powershell
.\scripts\idbi-proxy.ps1 -Browser   # tunnel + open the portal in an isolated Chrome
.\scripts\idbi-proxy.ps1 -Test       # just check access, then exit
.\scripts\idbi-proxy.ps1             # tunnel only (for Postman / curl / code)
```

**macOS / Linux:**
```bash
./scripts/idbi-proxy.sh --browser
./scripts/idbi-proxy.sh --test
./scripts/idbi-proxy.sh
```

The script fixes the key's permissions, opens the SOCKS proxy on
`localhost:1080`, verifies your traffic exits `43.205.52.148`, and reports
whether the portal returns `200`. Leave the window open while you work;
`Ctrl+C` closes the tunnel. Override the port with `-Port 1081` / `PORT=1081`.

The rest of this document is the manual version of what the script does, plus
Postman/code setup and troubleshooting.

---

## One-time setup

### 1. Put the SSH key somewhere outside any git repo

| OS | Location |
|----|----------|
| Windows | `%USERPROFILE%\.ssh\zeyro-idbi.pem` |
| macOS / Linux | `~/.ssh/zeyro-idbi.pem` |

> `*.pem` is in `.gitignore`, but keep the key out of the repo folder anyway —
> one `git add -f` away from a leak.

### 2. Lock down the key's permissions

SSH refuses a world-readable private key.

**Windows (PowerShell):**
```powershell
icacls "$env:USERPROFILE\.ssh\zeyro-idbi.pem" /inheritance:r
icacls "$env:USERPROFILE\.ssh\zeyro-idbi.pem" /grant:r "$($env:USERNAME):R"
```

**macOS / Linux:**
```bash
chmod 600 ~/.ssh/zeyro-idbi.pem
```

---

## Every time you need access

### Step 1 — open the tunnel

Leave this window running. **No output = it's working** (`-N` means "no shell,
just forward").

**Windows (PowerShell):**
```powershell
ssh -i "$env:USERPROFILE\.ssh\zeyro-idbi.pem" -D 1080 -N -C ec2-user@43.205.52.148
```

**macOS / Linux:**
```bash
ssh -i ~/.ssh/zeyro-idbi.pem -D 1080 -N -C ec2-user@43.205.52.148
```

`-D 1080` opens a SOCKS5 proxy on `localhost:1080` that exits from the EC2's
IP. If port `1080` is taken, pick another (e.g. `-D 1081`) and use it
everywhere below.

> User is `ec2-user` (Amazon Linux). If a future instance is Ubuntu, it's
> `ubuntu`.

### Step 2 — use it

Open a **separate** terminal / window.

**Quick check — should return `200`:**
```bash
curl --proxy socks5h://localhost:1080 -I https://innobox.idbi.bank.in
```
(`curl.exe` on Windows PowerShell, so PS doesn't alias it to `Invoke-WebRequest`.)

**Call an API through the proxy:**
```bash
curl --proxy socks5h://localhost:1080 https://innobox.idbi.bank.in/<api-path> \
  -H "Authorization: Bearer <token>"
```
Use `socks5h://` (not `socks5://`) so DNS also resolves through the tunnel.

**Open the portal in a browser** — an isolated Chrome profile so your normal
browsing is unaffected:

Windows (PowerShell):
```powershell
& "C:\Program Files\Google\Chrome\Application\chrome.exe" `
  --proxy-server="socks5://localhost:1080" `
  --user-data-dir="$env:TEMP\chrome-innobox"
```

macOS:
```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --proxy-server="socks5://localhost:1080" \
  --user-data-dir="/tmp/chrome-innobox"
```

Linux:
```bash
google-chrome --proxy-server="socks5://localhost:1080" \
  --user-data-dir="/tmp/chrome-innobox"
```

**Only that Chrome window uses the tunnel.** Every tab in it exits via
`43.205.52.148`, so don't use it for anything unrelated.

### Step 3 — done

Close the proxied Chrome window, then `Ctrl+C` the SSH tunnel.

---

## Postman / Insomnia / code

Point the client at the SOCKS proxy:

- **Postman:** Settings → Proxy → add custom proxy → `socks5` `localhost` `1080`.
  (Postman needs the SSH tunnel running first.)
- **Insomnia:** Preferences → General → Proxy → `socks5://localhost:1080`.
- **Go / Node / Python:** route the HTTP client through `socks5://localhost:1080`
  (e.g. Go `golang.org/x/net/proxy`, Node `socks-proxy-agent`, Python
  `requests[socks]` with `proxies={"https": "socks5h://localhost:1080"}`).

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `Permission denied (publickey)` | Wrong key or wrong user. Confirm the key, try `ubuntu@` instead of `ec2-user@`. Run `ssh -v -i <key> ec2-user@43.205.52.148` and read the offered-key lines. |
| `UNPROTECTED PRIVATE KEY FILE` / `bad permissions` | Run the `icacls` / `chmod 600` step above. |
| `ssh` times out on connect | Port 22 on the EC2 security group isn't open to your current IP. Add `<your-ip>/32` to the SG inbound rules (AWS console → EC2 → Security Groups). |
| `curl` → `connection refused` on `localhost:1080` | Tunnel isn't running, or it's on a different port. Re-open Step 1. |
| `curl` → `403 Forbidden`, `Server: awselb/2.0` | Traffic isn't exiting `43.205.52.148`. Verify: `curl --proxy socks5h://localhost:1080 -s https://checkip.amazonaws.com` should print `43.205.52.148`. If it doesn't, the tunnel/proxy isn't set up right. If it does and you still get 403, the allow-list changed on their side — contact ACC. |
| Portal loads but looks broken / assets fail | Use the dedicated Chrome profile command (don't set a system-wide proxy); the portal is a Next.js app and needs a real browser, not `curl`. |
| Chrome: "profile already in use" | Close all Chrome windows first, or change `chrome-innobox` to a new name. |

---

## Security rules

- **Never commit** `zeyro-idbi.pem` or portal credentials.
- **Never** put credentials in chat, Slack, tickets, or email. Password
  manager only.
- The proxied browser window exits from a bank-whitelisted IP — don't browse
  anything else through it.
- Rotate the portal password if it's ever shared insecurely.
- If someone leaves the team, rotate the EC2 key.

---

## Why this setup

The portal enforces access at the network edge (ALB listener rule / WAF),
allowing only `43.205.52.148`. An SSH SOCKS proxy is the lightest way to
borrow that IP without opening the EC2 as a shared HTTP proxy or handing out
AWS console access. `-D` dynamic forwarding means one command covers browser,
CLI, and API-client traffic.
