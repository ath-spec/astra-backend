#!/usr/bin/env bash
#
# Open an SSH SOCKS proxy through the IP-whitelisted EC2 box so you can reach
# the IDBI Atlas portal / APIs (https://innobox.idbi.bank.in).
#
# The portal only allows the EC2 IP 43.205.52.148. This opens a SOCKS5 proxy
# on localhost that exits from that IP, verifies access, and optionally opens
# an isolated Chrome pointed at it.
#
# Usage:
#   scripts/idbi-proxy.sh            # open tunnel, leave running (Ctrl+C to stop)
#   scripts/idbi-proxy.sh --test     # open tunnel, check portal returns 200, close
#   scripts/idbi-proxy.sh --browser  # open tunnel + launch proxied Chrome
#
# Env overrides: PORT (1080), KEY (~/.ssh/zeyro-idbi.pem), EC2_USER (ec2-user)

set -euo pipefail

PORT="${PORT:-1080}"
KEY="${KEY:-$HOME/.ssh/zeyro-idbi.pem}"
EC2_USER="${EC2_USER:-ec2-user}"
EC2_IP="${EC2_IP:-43.205.52.148}"
PORTAL_URL="${PORTAL_URL:-https://innobox.idbi.bank.in}"

MODE="run"
case "${1:-}" in
  --test)    MODE="test" ;;
  --browser) MODE="browser" ;;
  "")        MODE="run" ;;
  *) echo "unknown option: $1" >&2; exit 2 ;;
esac

[ -f "$KEY" ] || { echo "SSH key not found at: $KEY  (set KEY=<path>)" >&2; exit 1; }

# SSH refuses a group/world-readable key.
chmod 600 "$KEY" 2>/dev/null || true

# Bail if the port is already taken.
if command -v lsof >/dev/null && lsof -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Local port $PORT is already in use. Set PORT=<n> to pick another." >&2
  exit 1
fi

echo "Opening SOCKS proxy on localhost:$PORT via $EC2_USER@$EC2_IP ..."

ssh -i "$KEY" -D "$PORT" -N -C \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o StrictHostKeyChecking=accept-new \
  "$EC2_USER@$EC2_IP" &
SSH_PID=$!

cleanup() { kill "$SSH_PID" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

sleep 3
kill -0 "$SSH_PID" 2>/dev/null || {
  echo "SSH tunnel failed to start. Try:  ssh -v -i \"$KEY\" $EC2_USER@$EC2_IP" >&2
  exit 1
}

# Confirm traffic actually exits the whitelisted IP.
EGRESS="$(curl -s --max-time 15 --proxy "socks5h://localhost:$PORT" https://checkip.amazonaws.com || true)"
if [ "$EGRESS" = "$EC2_IP" ]; then
  echo "Egress IP via proxy: $EGRESS  (matches whitelist)"
else
  echo "Egress IP via proxy: '$EGRESS'  (expected $EC2_IP - proxy may not be routing correctly)"
fi

STATUS="$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 --proxy "socks5h://localhost:$PORT" -I "$PORTAL_URL" || true)"
case "$STATUS" in
  200) echo "$PORTAL_URL -> HTTP 200  (portal reachable)" ;;
  403) echo "$PORTAL_URL -> HTTP 403  (IP allow-list not letting us through - contact ACC/IDBI)" ;;
  *)   echo "$PORTAL_URL -> HTTP $STATUS" ;;
esac

if [ "$MODE" = "test" ]; then
  echo "--test done, closing tunnel."
  exit 0
fi

if [ "$MODE" = "browser" ]; then
  CHROME=""
  for c in \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "$(command -v google-chrome || true)" \
    "$(command -v google-chrome-stable || true)" \
    "$(command -v chromium || true)"; do
    [ -n "$c" ] && [ -x "$c" ] && { CHROME="$c"; break; }
  done

  if [ -z "$CHROME" ]; then
    echo "Chrome not found - open your browser manually and set SOCKS proxy to localhost:$PORT"
  else
    echo "Launching isolated Chrome (only this window uses the proxy)..."
    "$CHROME" --proxy-server="socks5://localhost:$PORT" \
      --user-data-dir="${TMPDIR:-/tmp}/chrome-innobox" "$PORTAL_URL" >/dev/null 2>&1 &
  fi
fi

echo
echo "Tunnel is up on localhost:$PORT. Leave this running. Ctrl+C to stop."
echo
wait "$SSH_PID"
