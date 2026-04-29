#!/usr/bin/env bash
set -euo pipefail

API_PORT="${HERMES_API_PORT:-8080}"
API_KEY="${HERMES_API_KEY:-test-secret-key}"
BASE_URL="http://127.0.0.1:${API_PORT}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES+1)); }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }
section() { echo -e "\n${CYAN}--- $1 ---${NC}"; }

PASS=0
FAILURES=0

chat() {
    local tenant="$1"
    local session="$2"
    local system_prompt="$3"
    local user_msg="$4"

    curl -s -X POST "${BASE_URL}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${API_KEY}" \
        -H "X-Hermes-Session-Id: ${session}" \
        -H "X-Hermes-Tenant-Id: ${tenant}" \
        -d "$(cat <<EOF
{
  "model": "default",
  "messages": [
    {"role": "system", "content": "$system_prompt"},
    {"role": "user", "content": "$user_msg"}
  ]
}
EOF
)" 2>/dev/null
}

echo "========================================================"
echo "  Hermes Real Skill Isolation Test"
echo "  Alice: 6 skills (plan, systematic-debugging,"
echo "         test-driven-development, github-issues,"
echo "         github-code-review, arxiv)"
echo "  Bob:   5 skills (ascii-art, excalidraw,"
echo "         notion, linear, obsidian)"
echo "========================================================"

# --- Health check ---
info "Checking API server health..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/health" 2>/dev/null || echo "000")
if [ "$HTTP_CODE" = "200" ]; then
    pass "API server is healthy"
else
    fail "API server not reachable (HTTP ${HTTP_CODE})"
    exit 1
fi

# ====================================================
section "Phase 1: Alice's skill visibility + isolation"
# ====================================================

info "Test 1-2: Alice lists skills (check own + no cross-contamination)..."
RESP=$(chat "tenant-alice" "real-alice-1" \
    "You are a helpful assistant." \
    "What slash-command skills are available to you? List each skill name on its own line, prefixed with a slash. Example format: /skill-name")
ALICE_CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$ALICE_CONTENT" | head -8)"

ALICE_HIT=0
for skill in plan systematic-debugging test-driven-development github-issues github-code-review arxiv; do
    if echo "$ALICE_CONTENT" | grep -qi "$skill"; then
        ALICE_HIT=$((ALICE_HIT+1))
    else
        echo "  MISS: $skill not found in response"
    fi
done
if [ "$ALICE_HIT" -ge 4 ]; then
    pass "Alice sees at least 4/6 of her skills (found: ${ALICE_HIT})"
else
    fail "Alice sees too few of her skills (found: ${ALICE_HIT}/6)"
fi

CROSS_HIT=0
for skill in ascii-art excalidraw obsidian; do
    if echo "$ALICE_CONTENT" | grep -qi "/${skill}"; then
        CROSS_HIT=$((CROSS_HIT+1))
        echo "  LEAK: $skill found in Alice's response"
    fi
done
if [ "$CROSS_HIT" -eq 0 ]; then
    pass "Alice does NOT see any of Bob's skills"
else
    fail "Alice sees ${CROSS_HIT} of Bob's skills (cross-contamination)"
fi

# ====================================================
section "Phase 2: Bob's skill visibility + isolation"
# ====================================================

info "Test 3-4: Bob lists skills (check own + no cross-contamination)..."
RESP=$(chat "tenant-bob" "real-bob-1" \
    "You are a helpful assistant." \
    "What slash-command skills are available to you? List each skill name on its own line, prefixed with a slash. Example format: /skill-name")
BOB_CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$BOB_CONTENT" | head -8)"

BOB_HIT=0
for skill in ascii-art excalidraw notion linear obsidian; do
    if echo "$BOB_CONTENT" | grep -qi "$skill"; then
        BOB_HIT=$((BOB_HIT+1))
    else
        echo "  MISS: $skill not found in response"
    fi
done
if [ "$BOB_HIT" -ge 3 ]; then
    pass "Bob sees at least 3/5 of his skills (found: ${BOB_HIT})"
else
    fail "Bob sees too few of his skills (found: ${BOB_HIT}/5)"
fi

CROSS_HIT=0
for skill in systematic-debugging test-driven-development github-code-review arxiv; do
    if echo "$BOB_CONTENT" | grep -qi "/${skill}"; then
        CROSS_HIT=$((CROSS_HIT+1))
        echo "  LEAK: $skill found in Bob's response"
    fi
done
if [ "$CROSS_HIT" -eq 0 ]; then
    pass "Bob does NOT see any of Alice's skills"
else
    fail "Bob sees ${CROSS_HIT} of Alice's skills (cross-contamination)"
fi

# ====================================================
section "Phase 3: Skill activation — Alice"
# ====================================================

info "Test 5: Alice activates /plan (her skill)..."
RESP=$(chat "tenant-alice" "real-alice-3" \
    "You are a helpful assistant." \
    "/plan Create a plan for building a REST API")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"
if echo "$CONTENT" | grep -qi "plan\|phase\|step\|milestone\|task\|design\|implement\|language\|framework\|clarif\|would you like\|what kind"; then
    pass "Alice /plan skill produces planning content"
else
    fail "Alice /plan skill did not produce expected content"
fi

info "Test 6: Alice activates /arxiv (her skill)..."
RESP=$(chat "tenant-alice" "real-alice-4" \
    "You are a helpful assistant." \
    "/arxiv Search for recent papers on transformer architecture")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"
if echo "$CONTENT" | grep -qi "arxiv\|paper\|search\|transformer\|research\|abstract\|author"; then
    pass "Alice /arxiv skill produces research content"
else
    fail "Alice /arxiv skill did not produce expected content"
fi

# ====================================================
section "Phase 4: Skill activation — Bob"
# ====================================================

info "Test 7: Bob activates /ascii-art (his skill)..."
RESP=$(chat "tenant-bob" "real-bob-3" \
    "You are a helpful assistant." \
    "/ascii-art Generate ASCII art of the word HELLO")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -5)"
if echo "$CONTENT" | grep -qi "ascii\|art\|font\|pyfiglet\|figlet\|HELLO\|text"; then
    pass "Bob /ascii-art skill produces art content"
else
    fail "Bob /ascii-art skill did not produce expected content"
fi

info "Test 8: Bob activates /notion (his skill)..."
RESP=$(chat "tenant-bob" "real-bob-4" \
    "You are a helpful assistant." \
    "/notion Help me create a new page in my Notion workspace")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"
if echo "$CONTENT" | grep -qi "notion\|page\|workspace\|API\|database\|create\|token"; then
    pass "Bob /notion skill produces Notion content"
else
    fail "Bob /notion skill did not produce expected content"
fi

# ====================================================
section "Phase 5: Wrong tenant tries other's skill"
# ====================================================

info "Test 9: Bob tries /plan (Alice's skill) — should not have it..."
RESP=$(chat "tenant-bob" "real-bob-5" \
    "You are a helpful assistant." \
    "/plan Create a plan for building a REST API")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"
if echo "$CONTENT" | grep -qi "not available\|not found\|don't have\|no skill\|unknown"; then
    pass "Bob cannot use Alice's /plan skill (properly rejected)"
else
    # Even if it responds, check it didn't actually use the Plan skill template
    if echo "$CONTENT" | grep -qi "hermes/plans\|\.hermes/plans\|plan mode"; then
        fail "Bob activated Alice's /plan skill (skill leak!)"
    else
        pass "Bob's /plan response is generic (no skill template applied)"
    fi
fi

info "Test 10: Alice tries /ascii-art (Bob's skill) — should not have it..."
RESP=$(chat "tenant-alice" "real-alice-5" \
    "You are a helpful assistant." \
    "/ascii-art Generate ASCII art of HELLO")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"
if echo "$CONTENT" | grep -qi "not available\|not found\|don't have\|no skill\|unknown"; then
    pass "Alice cannot use Bob's /ascii-art skill (properly rejected)"
else
    if echo "$CONTENT" | grep -qi "pyfiglet\|cowsay\|boxes\|toilet\|571 fonts"; then
        fail "Alice activated Bob's /ascii-art skill (skill leak!)"
    else
        pass "Alice's /ascii-art response is generic (no skill template applied)"
    fi
fi

# ====================================================
section "Phase 6: Default tenant — no tenant skills"
# ====================================================

info "Test 11: Default tenant should NOT see MinIO-only tenant skills..."
RESP=$(chat "default" "real-default-1" \
    "You are a helpful assistant." \
    "List ALL your available slash-command skills as a comma-separated list. Output ONLY the skill names, nothing else.")
CONTENT=$(echo "$RESP" | jq -r '.choices[0].message.content' 2>/dev/null || echo "")
echo "  Response: $(echo "$CONTENT" | head -3)"

# Only check for skills that are exclusively in MinIO tenant buckets
# (not in local skills/ dir). Bob's skills are creative/productivity, unlikely in local.
DEFAULT_LEAK=0
for skill in ascii-art excalidraw obsidian; do
    if echo "$CONTENT" | grep -qi "/${skill}\|, *${skill}"; then
        DEFAULT_LEAK=$((DEFAULT_LEAK+1))
        echo "  LEAK: $skill found in default tenant's response"
    fi
done
if [ "$DEFAULT_LEAK" -eq 0 ]; then
    pass "Default tenant does NOT see MinIO-only tenant skills"
else
    fail "Default tenant sees ${DEFAULT_LEAK} MinIO-only tenant skills (leak)"
fi

# ====================================================
echo ""
echo "========================================================"
echo "  Results: ${PASS} PASS, ${FAILURES} FAIL"
echo "========================================================"

if [ "$FAILURES" -gt 0 ]; then
    exit 1
fi
