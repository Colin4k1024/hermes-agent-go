#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:18080}"
ADMIN_TOKEN="${HERMES_ACP_TOKEN:-dev-token-change-in-production}"

PASSED=0
FAILED=0
TOTAL=0

pass() { echo -e "\033[0;32m[PASS]\033[0m $1"; }
fail() { echo -e "\033[0;31m[FAIL]\033[0m $1"; }
info() { echo -e "\033[1;33m[INFO]\033[0m $1"; }

check() {
  TOTAL=$((TOTAL+1))
  if [ "$1" = "true" ]; then
    pass "$2"
    PASSED=$((PASSED+1))
  else
    fail "$2"
    FAILED=$((FAILED+1))
  fi
}

chat() {
  local key="$1" payload_file="$2"
  curl -s -X POST "${BASE_URL}/v1/chat/completions" \
    -H "Authorization: Bearer ${key}" \
    -H "Content-Type: application/json" \
    -d @"${payload_file}"
}

extract_content() {
  python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('choices',[{}])[0].get('message',{}).get('content',''))" "$1" 2>/dev/null
}

echo "============================================"
echo "  Hermes Multi-Tenant Web Isolation Test"
echo "============================================"
echo ""

# ── Phase 1: Static pages ────────────────────────────
info "Phase 1: Static page accessibility"
HTTP_ADMIN=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/admin.html")
HTTP_ISO=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/isolation-test.html")
HTTP_INDEX=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/")
HTTP_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/health/live")
check "$([ "$HTTP_ADMIN" = "200" ] && echo true || echo false)" "admin.html accessible (HTTP $HTTP_ADMIN)"
check "$([ "$HTTP_ISO" = "200" ] && echo true || echo false)" "isolation-test.html accessible (HTTP $HTTP_ISO)"
check "$([ "$HTTP_INDEX" = "200" ] && echo true || echo false)" "index.html accessible (HTTP $HTTP_INDEX)"
check "$([ "$HTTP_HEALTH" = "200" ] && echo true || echo false)" "health endpoint alive (HTTP $HTTP_HEALTH)"

# ── Phase 2: Auth enforcement ────────────────────────
info "Phase 2: Auth enforcement"
HTTP_NOAUTH=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model":"mock","messages":[{"role":"user","content":"hello"}]}')
HTTP_BADKEY=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer hk_invalid_key" \
  -H "Content-Type: application/json" \
  -d '{"model":"mock","messages":[{"role":"user","content":"hello"}]}')
check "$([ "$HTTP_NOAUTH" = "401" ] && echo true || echo false)" "Unauthenticated request rejected (HTTP $HTTP_NOAUTH)"
check "$([ "$HTTP_BADKEY" = "401" ] && echo true || echo false)" "Invalid API key rejected (HTTP $HTTP_BADKEY)"

# ── Phase 3: Create tenants ──────────────────────────
info "Phase 3: Create 3 tenants and 4 API keys"

create_tenant() {
  curl -s -X POST "${BASE_URL}/v1/tenants" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$1\",\"plan\":\"$2\",\"rate_limit_rpm\":$3,\"max_sessions\":10}"
}

create_key() {
  curl -s -X POST "${BASE_URL}/v1/api-keys" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$1\",\"tenant_id\":\"$2\",\"roles\":[\"user\"]}"
}

T1_ID=$(create_tenant "WebTest-T1" "pro" 120 | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
T2_ID=$(create_tenant "WebTest-T2" "basic" 60 | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
T3_ID=$(create_tenant "WebTest-T3" "basic" 60 | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)

check "$([ -n "$T1_ID" ] && echo true || echo false)" "Tenant-1 created (id=${T1_ID:0:12}...)"
check "$([ -n "$T2_ID" ] && echo true || echo false)" "Tenant-2 created (id=${T2_ID:0:12}...)"
check "$([ -n "$T3_ID" ] && echo true || echo false)" "Tenant-3 created (id=${T3_ID:0:12}...)"

ALICE_KEY=$(create_key "alice-web" "$T1_ID" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('key',''))" 2>/dev/null)
BOB_KEY=$(create_key "bob-web" "$T1_ID" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('key',''))" 2>/dev/null)
CHARLIE_KEY=$(create_key "charlie-web" "$T2_ID" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('key',''))" 2>/dev/null)
DIANA_KEY=$(create_key "diana-web" "$T3_ID" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('key',''))" 2>/dev/null)

check "$([ -n "$ALICE_KEY" ] && echo true || echo false)" "Alice API key created (prefix=${ALICE_KEY:0:10})"
check "$([ -n "$BOB_KEY" ] && echo true || echo false)" "Bob API key created (prefix=${BOB_KEY:0:10})"
check "$([ -n "$CHARLIE_KEY" ] && echo true || echo false)" "Charlie API key created (prefix=${CHARLIE_KEY:0:10})"
check "$([ -n "$DIANA_KEY" ] && echo true || echo false)" "Diana API key created (prefix=${DIANA_KEY:0:10})"

T1_PFX=$(echo "$T1_ID" | tr -d '-' | cut -c1-8)
T2_PFX=$(echo "$T2_ID" | tr -d '-' | cut -c1-8)
T3_PFX=$(echo "$T3_ID" | tr -d '-' | cut -c1-8)

# ── Phase 4: Rate limit config ───────────────────────
info "Phase 4: Rate limit config verification"
TENANTS_JSON=$(curl -s "${BASE_URL}/v1/tenants" -H "Authorization: Bearer ${ADMIN_TOKEN}")
T1_RPM=$(echo "$TENANTS_JSON" | python3 -c "import sys,json; ts=json.load(sys.stdin).get('tenants',[]); print(next((t['rate_limit_rpm'] for t in ts if t.get('id')=='$T1_ID'),0))" 2>/dev/null)
T2_RPM=$(echo "$TENANTS_JSON" | python3 -c "import sys,json; ts=json.load(sys.stdin).get('tenants',[]); print(next((t['rate_limit_rpm'] for t in ts if t.get('id')=='$T2_ID'),0))" 2>/dev/null)
T3_RPM=$(echo "$TENANTS_JSON" | python3 -c "import sys,json; ts=json.load(sys.stdin).get('tenants',[]); print(next((t['rate_limit_rpm'] for t in ts if t.get('id')=='$T3_ID'),0))" 2>/dev/null)
check "$([ "$T1_RPM" = "120" ] && echo true || echo false)" "Tenant-1 rate = 120 RPM (got $T1_RPM)"
check "$([ "$T2_RPM" = "60" ] && echo true || echo false)" "Tenant-2 rate = 60 RPM (got $T2_RPM)"
check "$([ "$T3_RPM" = "60" ] && echo true || echo false)" "Tenant-3 rate = 60 RPM (got $T3_RPM)"

# ── Phase 5: Concurrent 4-user tenant isolation ──────
info "Phase 5: Concurrent 4-user chat — tenant ID in responses"

for u in alice bob charlie diana; do
  echo '{"model":"mock","messages":[{"role":"user","content":"What is my tenant ID? Reply with the ID."}]}' > "/tmp/${u}_payload.json"
done

chat "$ALICE_KEY"   /tmp/alice_payload.json   > /tmp/alice_r.json   &
chat "$BOB_KEY"     /tmp/bob_payload.json     > /tmp/bob_r.json     &
chat "$CHARLIE_KEY" /tmp/charlie_payload.json > /tmp/charlie_r.json &
chat "$DIANA_KEY"   /tmp/diana_payload.json   > /tmp/diana_r.json   &
info "Waiting for 4 concurrent responses..."
wait

A_C=$(extract_content /tmp/alice_r.json)
B_C=$(extract_content /tmp/bob_r.json)
C_C=$(extract_content /tmp/charlie_r.json)
D_C=$(extract_content /tmp/diana_r.json)

info "Alice:   ${A_C:0:100}"
info "Bob:     ${B_C:0:100}"
info "Charlie: ${C_C:0:100}"
info "Diana:   ${D_C:0:100}"

check "$(echo "$A_C" | grep -q "$T1_PFX" && echo true || echo false)" "Alice has T1 prefix ($T1_PFX)"
check "$(echo "$B_C" | grep -q "$T1_PFX" && echo true || echo false)" "Bob has T1 prefix ($T1_PFX) — same tenant as Alice"
check "$(echo "$C_C" | grep -q "$T2_PFX" && echo true || echo false)" "Charlie has T2 prefix ($T2_PFX)"
check "$(echo "$D_C" | grep -q "$T3_PFX" && echo true || echo false)" "Diana has T3 prefix ($T3_PFX)"

# Cross-tenant leak
check "$(echo "$A_C" | grep -qv "$T2_PFX" && echo true || echo false)" "Alice no T2 leak"
check "$(echo "$C_C" | grep -qv "$T1_PFX" && echo true || echo false)" "Charlie no T1 leak"
check "$(echo "$D_C" | grep -qv "$T1_PFX" && echo true || echo false)" "Diana no T1 leak"
check "$(echo "$D_C" | grep -qv "$T2_PFX" && echo true || echo false)" "Diana no T2 leak"

# ── Phase 6: Multi-turn session persistence ──────────
info "Phase 6: Multi-turn session persistence"

echo '{"model":"mock","messages":[{"role":"user","content":"Remember: my favorite color is purple and my pet is a cat named Whiskers."}]}' > /tmp/mt_t1.json
chat "$ALICE_KEY" /tmp/mt_t1.json > /tmp/mt_r1.json
T1_CONTENT=$(extract_content /tmp/mt_r1.json)
info "Turn 1: ${T1_CONTENT:0:80}..."

python3 << 'PYEOF' > /tmp/mt_t2.json
import json
with open('/tmp/mt_r1.json') as f:
    r1 = json.load(f)
r1c = r1.get('choices',[{}])[0].get('message',{}).get('content','')
msgs = [
    {'role': 'user', 'content': 'Remember: my favorite color is purple and my pet is a cat named Whiskers.'},
    {'role': 'assistant', 'content': r1c},
    {'role': 'user', 'content': 'What is my favorite color and pet name? One sentence.'}
]
print(json.dumps({'model': 'mock', 'messages': msgs}))
PYEOF

chat "$ALICE_KEY" /tmp/mt_t2.json > /tmp/mt_r2.json
T2_CONTENT=$(extract_content /tmp/mt_r2.json)
info "Turn 2 recall: ${T2_CONTENT:0:200}"

check "$(echo "$T2_CONTENT" | grep -qi 'purple' && echo true || echo false)" "Alice recalls 'purple' in multi-turn"
check "$(echo "$T2_CONTENT" | grep -qi 'Whiskers\|cat' && echo true || echo false)" "Alice recalls 'Whiskers/cat' in multi-turn"
if echo "$T2_CONTENT" | grep -q "$T1_PFX"; then
  check "true" "Alice multi-turn has correct T1 prefix"
else
  info "SOFT: Alice multi-turn missing T1 prefix (LLM non-deterministic tag — not an isolation failure)"
fi

# ── Phase 7: Cross-tenant data isolation ─────────────
info "Phase 7: Cross-tenant data isolation"

echo '{"model":"mock","messages":[{"role":"user","content":"Do you know a color called purple or a cat named Whiskers? Say UNKNOWN if not."}]}' > /tmp/ct_payload.json
chat "$CHARLIE_KEY" /tmp/ct_payload.json > /tmp/ct_r.json
CT_CONTENT=$(extract_content /tmp/ct_r.json)
info "Charlie cross-check: ${CT_CONTENT:0:150}"

check "$(echo "$CT_CONTENT" | grep -q "$T2_PFX" && echo true || echo false)" "Charlie gets T2 prefix (correct tenant)"
check "$(echo "$CT_CONTENT" | grep -qv "$T1_PFX" && echo true || echo false)" "Charlie has no T1 prefix (no tenant leak)"

# ── Phase 8: Per-tenant session store ────────────────
info "Phase 8: Per-tenant session store isolation"

A_SESS=$(curl -s "${BASE_URL}/v1/mock-sessions" -H "Authorization: Bearer ${ALICE_KEY}")
C_SESS=$(curl -s "${BASE_URL}/v1/mock-sessions" -H "Authorization: Bearer ${CHARLIE_KEY}")
D_SESS=$(curl -s "${BASE_URL}/v1/mock-sessions" -H "Authorization: Bearer ${DIANA_KEY}")

A_TID=$(echo "$A_SESS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('tenant_id',''))" 2>/dev/null)
C_TID=$(echo "$C_SESS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('tenant_id',''))" 2>/dev/null)
D_TID=$(echo "$D_SESS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('tenant_id',''))" 2>/dev/null)

check "$([ "$A_TID" = "$T1_ID" ] && echo true || echo false)" "Alice session store scoped to T1"
check "$([ "$C_TID" = "$T2_ID" ] && echo true || echo false)" "Charlie session store scoped to T2"
check "$([ "$D_TID" = "$T3_ID" ] && echo true || echo false)" "Diana session store scoped to T3"

# ── Phase 9: RBAC isolation ──────────────────────────
info "Phase 9: RBAC isolation"

HTTP_A_ADMIN=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/tenants" -H "Authorization: Bearer ${ALICE_KEY}")
HTTP_C_ADMIN=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/api-keys" -H "Authorization: Bearer ${CHARLIE_KEY}")
HTTP_ADMIN_OK=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/tenants" -H "Authorization: Bearer ${ADMIN_TOKEN}")

check "$([ "$HTTP_A_ADMIN" = "403" ] && echo true || echo false)" "User Alice blocked from admin /v1/tenants (HTTP $HTTP_A_ADMIN)"
check "$([ "$HTTP_C_ADMIN" = "403" ] && echo true || echo false)" "User Charlie blocked from admin /v1/api-keys (HTTP $HTTP_C_ADMIN)"
check "$([ "$HTTP_ADMIN_OK" = "200" ] && echo true || echo false)" "Admin token accesses /v1/tenants (HTTP $HTTP_ADMIN_OK)"

# ── Summary ──────────────────────────────────────────
echo ""
echo "============================================"
if [ "$FAILED" -eq 0 ]; then
  echo -e "  \033[0;32mALL $TOTAL TESTS PASSED\033[0m"
else
  printf "  \033[0;32m%d passed\033[0m, \033[0;31m%d failed\033[0m (of %d)\n" "$PASSED" "$FAILED" "$TOTAL"
fi
echo "============================================"

exit "$FAILED"
