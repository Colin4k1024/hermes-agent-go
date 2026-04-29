#!/usr/bin/env bash
set -euo pipefail

# PostgreSQL-backed integration test for hermes-agent-go.
# Prerequisites: PostgreSQL running on localhost:5432 (docker exec hermes-pg).

DATABASE_URL="${DATABASE_URL:-postgres://hermes:hermes@localhost:5432/hermes?sslmode=disable}"
API_PORT="${HERMES_API_PORT:-8090}"
API_KEY="${HERMES_API_KEY:-test-secret-key}"
BASE_URL="http://127.0.0.1:${API_PORT}"
HERMES_BIN="${HERMES_BIN:-./hermes}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES+1)); }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

FAILURES=0
GW_PID=""

cleanup() {
    if [ -n "$GW_PID" ]; then
        kill "$GW_PID" 2>/dev/null || true
        wait "$GW_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

echo "============================================"
echo "  Hermes PG Integration Test"
echo "============================================"
echo ""

# --- Phase 1: PG connectivity ---
info "Checking PostgreSQL connectivity..."
if docker exec hermes-pg psql -U hermes -d hermes -c "SELECT 1" &>/dev/null; then
    pass "PostgreSQL is reachable"
else
    fail "Cannot connect to PostgreSQL"
    exit 1
fi

# --- Phase 2: Start gateway with PG backend ---
info "Starting gateway with PG backend (port ${API_PORT})..."
set -o allexport && source .env && set +o allexport 2>/dev/null || true
DATABASE_URL="${DATABASE_URL}" \
HERMES_API_PORT="${API_PORT}" \
HERMES_API_KEY="${API_KEY}" \
"${HERMES_BIN}" gateway &>/tmp/hermes-pg-test.log &
GW_PID=$!

for i in $(seq 1 10); do
    if curl -sf "${BASE_URL}/v1/health" &>/dev/null; then
        break
    fi
    sleep 1
done

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/health" 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Gateway started with PG backend"
else
    fail "Gateway failed to start (HTTP ${HTTP_CODE})"
    tail -20 /tmp/hermes-pg-test.log
    exit 1
fi

# Verify PG store was selected (not SQLite)
if grep -q "Using PostgreSQL session store" /tmp/hermes-pg-test.log; then
    pass "PG session store active (not local)"
else
    fail "Gateway did not select PG session store"
fi

# --- Phase 3: Run standard isolation tests ---
info "Running standard isolation tests..."
echo ""
ISOLATION_EXIT=0
HERMES_API_PORT="${API_PORT}" HERMES_API_KEY="${API_KEY}" \
    bash "$(dirname "$0")/test_isolation.sh" || ISOLATION_EXIT=$?

if [ "$ISOLATION_EXIT" -eq 0 ]; then
    pass "All isolation tests passed with PG backend"
else
    fail "Isolation tests failed (exit ${ISOLATION_EXIT})"
fi

# --- Phase 4: Verify PG has session data ---
echo ""
info "Verifying PostgreSQL data..."

SESSION_COUNT=$(docker exec hermes-pg psql -U hermes -d hermes -t -c \
    "SELECT count(*) FROM sessions WHERE session_key IS NOT NULL;" 2>/dev/null | tr -d ' ' || echo "0")
SESSION_COUNT="${SESSION_COUNT:-0}"
if [ "$SESSION_COUNT" -ge 2 ] 2>/dev/null; then
    pass "Sessions stored in PG (count=${SESSION_COUNT})"
else
    fail "Expected >=2 sessions in PG, got ${SESSION_COUNT}"
fi

TENANT_COUNT=$(docker exec hermes-pg psql -U hermes -d hermes -t -c \
    "SELECT count(*) FROM tenants;" 2>/dev/null | tr -d ' ' || echo "0")
TENANT_COUNT="${TENANT_COUNT:-0}"
if [ "$TENANT_COUNT" -ge 1 ] 2>/dev/null; then
    pass "Default tenant exists in PG (count=${TENANT_COUNT})"
else
    fail "No tenants found in PG"
fi

# --- Phase 5: Restart persistence test ---
echo ""
info "Testing session persistence across gateway restart..."

kill "$GW_PID" 2>/dev/null || true
wait "$GW_PID" 2>/dev/null || true
GW_PID=""
sleep 2

DATABASE_URL="${DATABASE_URL}" \
HERMES_API_PORT="${API_PORT}" \
HERMES_API_KEY="${API_KEY}" \
"${HERMES_BIN}" gateway &>/tmp/hermes-pg-test2.log &
GW_PID=$!

for i in $(seq 1 10); do
    if curl -sf "${BASE_URL}/v1/health" &>/dev/null; then
        break
    fi
    sleep 1
done

# Verify session key still exists in PG after restart
SESSION_EXISTS=$(docker exec hermes-pg psql -U hermes -d hermes -t -c \
    "SELECT count(*) FROM sessions WHERE session_key LIKE '%user-alice-session%';" 2>/dev/null | tr -d ' ')
if [ "${SESSION_EXISTS:-0}" -ge 1 ] 2>/dev/null; then
    pass "Session key persists in PG across gateway restart (count=${SESSION_EXISTS})"
else
    fail "Session key not found in PG after restart"
fi

# Verify restarted gateway accepts requests for the old session (HTTP 200)
RESTART_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${BASE_URL}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "X-Hermes-Session-Id: user-alice-session" \
    -d '{"model":"default","messages":[{"role":"user","content":"Hello"}]}' 2>/dev/null)

info "Post-restart HTTP status: ${RESTART_HTTP}"
if [ "$RESTART_HTTP" = "200" ]; then
    pass "Restarted gateway accepts requests to persisted session (HTTP 200)"
else
    fail "Restarted gateway rejected persisted session (HTTP ${RESTART_HTTP})"
fi
# NOTE: full message-history restoration across restarts requires writing messages
# to the PG messages table (not yet implemented — messages table is currently empty).

# --- Summary ---
echo ""
echo "============================================"
if [ $FAILURES -eq 0 ]; then
    echo -e "  ${GREEN}ALL PG INTEGRATION TESTS PASSED${NC}"
else
    echo -e "  ${RED}${FAILURES} TEST(S) FAILED${NC}"
fi
echo "============================================"
exit $FAILURES
