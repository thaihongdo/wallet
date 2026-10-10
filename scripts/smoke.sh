#!/usr/bin/env bash
# Smoke test for a running go-wallet instance (local or Cloud Run).
#
# Usage: scripts/smoke.sh <BASE_URL> <API_KEY>
# Example: scripts/smoke.sh http://localhost:8080 local-dev-key
#
# Exercises: create 2 wallets, top up one, transfer between them, replay
# the same transfer with the same Idempotency-Key (expects the replay
# header), list the destination wallet's ledger entries, and call
# /readyz. Exits non-zero on the first unexpected response.
set -euo pipefail

BASE_URL="${1:?usage: scripts/smoke.sh <BASE_URL> <API_KEY>}"
API_KEY="${2:?usage: scripts/smoke.sh <BASE_URL> <API_KEY>}"
RUN_ID="$(date +%s)"

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq is required" >&2
  exit 1
fi

pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

req() {
  # req METHOD PATH [EXTRA_HEADER ...] -- BODY
  local method="$1" path="$2"
  shift 2
  curl -sS -w '\n%{http_code}' -X "$method" "$BASE_URL$path" \
    -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" "$@"
}

split_status() {
  # Accepts output from `req` as $1; sets BODY and STATUS.
  STATUS="${1##*$'\n'}"
  BODY="${1%$'\n'"$STATUS"}"
}

echo "== /readyz =="
_raw=$(req GET /readyz); split_status "$_raw"
[ "$STATUS" = "200" ] || fail "/readyz expected 200, got $STATUS: $BODY"
pass "/readyz is ready"

echo "== create wallet A =="
_raw=$(req POST /v1/wallets -d '{"owner_id":"smoke-test-a"}'); split_status "$_raw"
[ "$STATUS" = "201" ] || fail "create wallet A expected 201, got $STATUS: $BODY"
WALLET_A="$(echo "$BODY" | jq -r .id)"
pass "created wallet A=$WALLET_A"

echo "== create wallet B =="
_raw=$(req POST /v1/wallets -d '{"owner_id":"smoke-test-b"}'); split_status "$_raw"
[ "$STATUS" = "201" ] || fail "create wallet B expected 201, got $STATUS: $BODY"
WALLET_B="$(echo "$BODY" | jq -r .id)"
pass "created wallet B=$WALLET_B"

echo "== top up wallet A =="
_raw=$(req POST "/v1/wallets/$WALLET_A/topup" -H "Idempotency-Key: smoke-topup-$RUN_ID" -d '{"amount":100000}'); split_status "$_raw"
[ "$STATUS" = "200" ] || fail "top up expected 200, got $STATUS: $BODY"
pass "topped up wallet A: $(echo "$BODY" | jq -c .wallet)"

echo "== transfer A -> B =="
_raw=$(req POST /v1/transfers -H "Idempotency-Key: smoke-transfer-$RUN_ID" \
  -d "{\"from_wallet_id\":\"$WALLET_A\",\"to_wallet_id\":\"$WALLET_B\",\"amount\":30000}"); split_status "$_raw"
[ "$STATUS" = "201" ] || fail "transfer expected 201, got $STATUS: $BODY"
pass "transferred: from_balance=$(echo "$BODY" | jq -r .from_balance) to_balance=$(echo "$BODY" | jq -r .to_balance)"

echo "== replay transfer with same Idempotency-Key =="
REPLAY_HEADERS="$(curl -sS -D - -o /dev/null -X POST "$BASE_URL/v1/transfers" \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -H "Idempotency-Key: smoke-transfer-$RUN_ID" \
  -d "{\"from_wallet_id\":\"$WALLET_A\",\"to_wallet_id\":\"$WALLET_B\",\"amount\":30000}")"
echo "$REPLAY_HEADERS" | grep -qi '^Idempotent-Replayed: true' || fail "expected Idempotent-Replayed: true header on replay"
pass "replay returned Idempotent-Replayed: true"

echo "== list wallet B entries =="
_raw=$(req GET "/v1/wallets/$WALLET_B/entries?limit=20"); split_status "$_raw"
[ "$STATUS" = "200" ] || fail "list entries expected 200, got $STATUS: $BODY"
COUNT="$(echo "$BODY" | jq '.items | length')"
[ "$COUNT" -ge 1 ] || fail "expected at least 1 ledger entry for wallet B, got $COUNT"
pass "wallet B has $COUNT ledger entr(y/ies)"

echo
echo "ALL SMOKE TESTS PASSED"
