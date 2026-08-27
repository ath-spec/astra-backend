#!/usr/bin/env bash
# End-to-end smoke test for the RM/Admin console API (OTP auth).
#
#   BASE=https://your-app.up.railway.app OTP=123456 ./scripts/rm_smoketest.sh
#
# Prereqs on the target:
#   * migrations applied (automatic on boot)
#   * `go run ./cmd/rmseed` run once -> creates EMP001 admin@astra.in,
#     EMP002 rm1@astra.in, EMP003 rm2@astra.in
#   * RM_OTP_DEV_CODE set on the server to a fixed code, and passed here as
#     OTP=... (otherwise the real code is only in the server logs and this
#     script can't complete a login).
# Requires: curl, jq.

set -u
BASE="${BASE:-http://localhost:8080}"
OTP="${OTP:-}"
pass=0; fail=0

hr(){ printf '\n\033[1m== %s\033[0m\n' "$1"; }
check(){
  local name="$1" want="$2"; shift 2
  local body code
  body=$(curl -sS -m 20 -w $'\n%{http_code}' "$@") || { printf '  \033[31mFAIL\033[0m %s (curl error)\n' "$name"; fail=$((fail+1)); return 1; }
  code="${body##*$'\n'}"; body="${body%$'\n'*}"
  LAST_BODY="$body"
  if [ "$code" = "$want" ]; then
    printf '  \033[32mOK\033[0m   %-46s [%s]\n' "$name" "$code"; pass=$((pass+1))
  else
    printf '  \033[31mFAIL\033[0m %-46s [got %s want %s]\n     %s\n' "$name" "$code" "$want" "$(echo "$body" | head -c 300)"
    fail=$((fail+1))
  fi
}
jqv(){ echo "$LAST_BODY" | jq -r "$1" 2>/dev/null; }

# login <identifier> -> sets TOKEN / REFRESH
login(){
  local who="$1"
  check "otp/send ($who)" 200 -X POST "$BASE/api/rm/auth/otp/send" \
    -H 'Content-Type: application/json' -d "{\"identifier\":\"$who\"}"
  echo "     masked: $(jqv '.data.masked_phone')"
  check "otp/verify ($who)" 200 -X POST "$BASE/api/rm/auth/otp/verify" \
    -H 'Content-Type: application/json' -d "{\"identifier\":\"$who\",\"otp\":\"$OTP\"}"
  TOKEN=$(jqv '.data.access_token'); REFRESH=$(jqv '.data.refresh_token')
  echo "     role=$(jqv '.data.role')  emp=$(jqv '.data.rm.employee_code')"
}

if [ -z "$OTP" ]; then
  echo "!! OTP not provided. Set OTP=<RM_OTP_DEV_CODE value> so logins can complete."
  echo "   Continuing with the no-auth checks only."
fi

hr "liveness"
check "GET /" 200 "$BASE/"

hr "auth negatives"
check "protected route w/o token -> 401" 401 "$BASE/api/rm/clients"
check "otp/send missing identifier -> 400" 400 -X POST "$BASE/api/rm/auth/otp/send" \
  -H 'Content-Type: application/json' -d '{}'
check "otp/send unknown identifier -> 200 (no enumeration)" 200 -X POST "$BASE/api/rm/auth/otp/send" \
  -H 'Content-Type: application/json' -d '{"identifier":"EMP999"}'
check "otp/verify wrong code -> 401" 401 -X POST "$BASE/api/rm/auth/otp/verify" \
  -H 'Content-Type: application/json' -d '{"identifier":"EMP001","otp":"000000x"}'

[ -z "$OTP" ] && { printf '\n\033[1mRESULT: %d passed, %d failed (auth-gated checks skipped)\033[0m\n' "$pass" "$fail"; exit $((fail>0)); }

hr "admin login (EMP001)"
login "EMP001"
ADMIN_TOKEN="$TOKEN"; ADMIN_REFRESH="$REFRESH"
AUTH_ADMIN=(-H "Authorization: Bearer $ADMIN_TOKEN")
check "GET /api/rm/auth/me" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/auth/me"
check "GET /api/rm/admin/overview" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/overview"
echo "     $(jqv '.data | {total_clients,total_aum,unassigned_count,active_rm_count,rms_at_capacity}')"
check "GET /api/rm/admin/rms" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/rms"
echo "     roster: $(jqv '.data | map({emp:.employee_code,name,role,status,client_count,total_aum})')"
RM1_ID=$(jqv '.data[] | select(.email=="rm1@astra.in") | .id')
RM2_ID=$(jqv '.data[] | select(.email=="rm2@astra.in") | .id')
check "GET /api/rm/admin/clients?assigned=false" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/clients?assigned=false"
echo "     unassigned total: $(jqv '.data.total')"
check "GET /api/rm/admin/assignments/history" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/assignments/history?limit=5"

hr "admin: create + patch + offboard a throwaway RM"
STAMP=$(date +%s)
check "POST /api/rm/admin/rms -> 201" 201 "${AUTH_ADMIN[@]}" -X POST "$BASE/api/rm/admin/rms" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Smoke RM\",\"employee_code\":\"SMOKE$STAMP\",\"email\":\"smoke+$STAMP@astra.in\",\"phone\":\"+919111$STAMP\",\"max_portfolios\":10}"
NEW_RM_ID=$(jqv '.data.id')
check "POST /api/rm/admin/rms dup emp code -> 409" 409 "${AUTH_ADMIN[@]}" -X POST "$BASE/api/rm/admin/rms" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"Dup\",\"employee_code\":\"SMOKE$STAMP\",\"email\":\"smoke2+$STAMP@astra.in\",\"phone\":\"+91922$STAMP\"}"
check "PATCH new RM -> inactive" 200 "${AUTH_ADMIN[@]}" -X PATCH "$BASE/api/rm/admin/rms/$NEW_RM_ID" \
  -H 'Content-Type: application/json' -d '{"status":"inactive"}'
check "POST offboard (no clients) -> 200" 200 "${AUTH_ADMIN[@]}" -X POST "$BASE/api/rm/admin/rms/$NEW_RM_ID/offboard" \
  -H 'Content-Type: application/json' -d '{"reason":"smoke test"}'

hr "rm1 login (EMP002) + book"
login "EMP002"
AUTH_RM1=(-H "Authorization: Bearer $TOKEN")
check "GET /api/rm/dashboard/summary" 200 "${AUTH_RM1[@]}" "$BASE/api/rm/dashboard/summary"
echo "     $(jqv '.data | {client_count,total_aum,avg_portfolio_value,utilisation,alerts:(.alerts|length)}')"
check "GET /api/rm/clients" 200 "${AUTH_RM1[@]}" "$BASE/api/rm/clients?sort=wealth&order=desc&limit=5"
CLIENT_ID=$(jqv '.data.items[0].user_id')
echo "     first client: $CLIENT_ID  ($(jqv '.data.total') total)"

hr "rm1: client 360"
if [ -n "${CLIENT_ID:-}" ] && [ "$CLIENT_ID" != "null" ]; then
  check "GET /api/rm/clients/{id}" 200 "${AUTH_RM1[@]}" "$BASE/api/rm/clients/$CLIENT_ID"
  echo "     $(jqv '.data | {wealth:.summary.total_wealth, dna_level:.dna.level, stocks:(.holdings.stocks|length), mf:(.holdings.mf|length), fd:(.holdings.fd|length), goals:(.goals|length), growth:(.growth|length)}')"
  check "GET /api/rm/clients/{id}/growth?days=90" 200 "${AUTH_RM1[@]}" "$BASE/api/rm/clients/$CLIENT_ID/growth?days=90"
  check "GET /api/rm/clients/{id}/portfolio-history" 200 "${AUTH_RM1[@]}" "$BASE/api/rm/clients/$CLIENT_ID/portfolio-history?days=365"
  echo "     history points: alloc=$(jqv '.data.allocation_series|length') dna=$(jqv '.data.dna_series|length')"
else
  echo "     (skipped - rm1 has no clients; run cmd/rmseed)"
fi

hr "rbac: rm token must not reach admin routes"
check "rm1 -> /api/rm/admin/overview -> 403" 403 "${AUTH_RM1[@]}" "$BASE/api/rm/admin/overview"

hr "admin: transfer rm1 -> rm2, verify audit, revert"
if [ -n "${CLIENT_ID:-}" ] && [ "$CLIENT_ID" != "null" ] && [ -n "${RM2_ID:-}" ]; then
  check "POST /assignments/transfer -> 200" 200 "${AUTH_ADMIN[@]}" -X POST "$BASE/api/rm/admin/assignments/transfer" \
    -H 'Content-Type: application/json' -d "{\"user_id\":\"$CLIENT_ID\",\"to_rm_id\":\"$RM2_ID\",\"reason\":\"smoke\"}"
  check "history shows the transfer" 200 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/assignments/history?user_id=$CLIENT_ID&limit=1"
  echo "     latest: $(jqv '.data.items[0] | {action,from_rm_name,to_rm_name,reason}')"
  check "POST /assignments/transfer (revert) -> 200" 200 "${AUTH_ADMIN[@]}" -X POST "$BASE/api/rm/admin/assignments/transfer" \
    -H 'Content-Type: application/json' -d "{\"user_id\":\"$CLIENT_ID\",\"to_rm_id\":\"$RM1_ID\",\"reason\":\"revert\"}"
fi

hr "token refresh rotation + logout"
check "POST /api/rm/auth/refresh -> 200" 200 -X POST "$BASE/api/rm/auth/refresh" \
  -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$ADMIN_REFRESH\"}"
NEW_REFRESH=$(jqv '.data.refresh_token')
check "old refresh token now rejected -> 401" 401 -X POST "$BASE/api/rm/auth/refresh" \
  -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$ADMIN_REFRESH\"}"
check "POST /api/rm/auth/logout -> 200" 200 -X POST "$BASE/api/rm/auth/logout" \
  -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$NEW_REFRESH\"}"

hr "single-use OTP: verify again with same code -> 401"
check "otp/verify reuse -> 401" 401 -X POST "$BASE/api/rm/auth/otp/verify" \
  -H 'Content-Type: application/json' -d "{\"identifier\":\"EMP002\",\"otp\":\"$OTP\"}"

hr "auto-assignment on new user signup (optional)"
if [ "${TEST_SIGNUP:-0}" = "1" ]; then
  PHONE="+9199$(date +%N | head -c 8)"
  curl -sS -m 20 -X POST "$BASE/api/auth/otp/send" -H 'Content-Type: application/json' -d "{\"phone_number\":\"$PHONE\"}" >/dev/null
  check "user OTP verify (new user) -> 200" 200 -X POST "$BASE/api/auth/otp/verify" -H 'Content-Type: application/json' \
    -d "{\"astra_user_id\":\"smoke-$STAMP\",\"phone_number\":\"$PHONE\",\"otp\":\"123456\",\"name\":\"Smoke User\"}"
  sleep 1
  login "EMP001"; AUTH_ADMIN=(-H "Authorization: Bearer $TOKEN")
  curl -sS -m 20 "${AUTH_ADMIN[@]}" "$BASE/api/rm/admin/assignments/history?limit=1" \
    | jq -r '.data.items[0] | "     newest history: \(.action) -> \(.to_rm_name)  (expect auto_assign)"'
else
  echo "     skipped (set TEST_SIGNUP=1 - permanently creates a user)"
fi

printf '\n\033[1mRESULT: %d passed, %d failed\033[0m\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
