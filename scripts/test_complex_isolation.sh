#!/usr/bin/env bash
# Complex multi-step isolation test: Alice (research) vs Bob (creative)
# Tests: skill injection via slash commands, concurrent access, cross-tenant isolation,
#        session persistence, multi-turn tasks.

set -euo pipefail

GATEWAY="http://localhost:8080"
API_KEY="test-secret-key"
PASS=0
FAIL=0
SKIP=0

# ── helpers ──────────────────────────────────────────────────────────────────
log()  { echo "[$(date +%H:%M:%S)] $*"; }
pass() { echo "  ✅ PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  ❌ FAIL: $*"; FAIL=$((FAIL+1)); }
info() { echo "  ℹ  $*"; }

chat() {
  # chat <tenant> <session> <message> [max_tokens]
  local tenant="$1" session="$2" message="$3" max_tokens="${4:-800}"
  curl -s --retry 2 --retry-delay 2 -X POST "$GATEWAY/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $API_KEY" \
    -H "X-Hermes-Tenant-Id: $tenant" \
    -H "X-Hermes-Session-Id: $session" \
    -d "{\"model\":\"hermes\",\"max_tokens\":$max_tokens,\"messages\":[{\"role\":\"user\",\"content\":$(echo "$message" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read().strip()))')}]}" || echo '{}'
}

extract_content() {
  python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    print(d.get('choices',[{}])[0].get('message',{}).get('content',''))
except:
    print('')
" 2>/dev/null || true
}

assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if echo "$haystack" | grep -qiE "$needle"; then
    pass "$label"
  else
    fail "$label (expected pattern '$needle' in response)"
    info "Response snippet: $(echo "$haystack" | head -c 300)"
  fi
}

assert_not_contains() {
  local label="$1" haystack="$2" needle="$3"
  if echo "$haystack" | grep -qiE "$needle"; then
    fail "$label (found pattern '$needle' — should NOT appear)"
    info "Response snippet: $(echo "$haystack" | head -c 300)"
  else
    pass "$label"
  fi
}

# ── pre-flight ────────────────────────────────────────────────────────────────
log "=== PRE-FLIGHT ==="
health=$(curl -s "$GATEWAY/v1/health" 2>/dev/null)
if echo "$health" | grep -q '"ok"'; then
  pass "Gateway healthy"
else
  fail "Gateway not responding — aborting"
  exit 1
fi

# ── Phase 1: Skill Activation — Alice /arxiv (skill injection) ────────────────
log ""
log "=== Phase 1: Alice /arxiv Skill Injection ==="

alice_arxiv=$(chat alice alice-p1-001 \
  "/arxiv search for 'transformer attention mechanism' papers" \
  1200 | extract_content)

info "Alice /arxiv response: $(echo "$alice_arxiv" | head -c 400)"
# After injection the LLM receives full SKILL.md content with arxiv API details
assert_contains "Alice /arxiv — skill activated" "$alice_arxiv" \
  "arxiv|export\.arxiv\.org|search_query|paper|academic|abstract|query"

# ── Phase 2: Skill Activation — Alice /duckduckgo-search ─────────────────────
log ""
log "=== Phase 2: Alice /duckduckgo-search Skill Injection ==="

alice_ddg=$(chat alice alice-p2-001 \
  "/duckduckgo-search Claude Code AI agent 2025" \
  1200 | extract_content)

info "Alice /duckduckgo response: $(echo "$alice_ddg" | head -c 400)"
assert_contains "Alice /duckduckgo — skill activated" "$alice_ddg" \
  "duckduckgo|ddgs|search|web|result|query|found"

# ── Phase 3: Skill Activation — Bob /ascii-art ───────────────────────────────
log ""
log "=== Phase 3: Bob /ascii-art Skill Injection ==="

bob_art=$(chat bob bob-p3-001 \
  "/ascii-art draw a simple cat" \
  800 | extract_content)

info "Bob /ascii-art response: $(echo "$bob_art" | head -c 400)"
assert_contains "Bob /ascii-art — skill activated" "$bob_art" \
  "pyfiglet|figlet|cowsay|ascii|cat|art|banner|/\\\\|=\\^"

# ── Phase 4: Skill Activation — Bob /notion ──────────────────────────────────
log ""
log "=== Phase 4: Bob /notion Skill Injection ==="

bob_notion=$(chat bob bob-p4-001 \
  "/notion create a page for my reading list" \
  800 | extract_content)

info "Bob /notion response: $(echo "$bob_notion" | head -c 400)"
assert_contains "Bob /notion — skill activated" "$bob_notion" \
  "Notion|API|page|block|database|property|curl|notion\.so"

# ── Phase 5: Cross-Tenant Isolation — Alice cannot use Bob's skills ───────────
log ""
log "=== Phase 5: Cross-Tenant Isolation (Alice tries Bob's skills) ==="

alice_try_ascii=$(chat alice alice-p5-ascii \
  "/ascii-art draw a cat" \
  400 | extract_content)
alice_try_notion=$(chat alice alice-p5-notion \
  "/notion create a page" \
  400 | extract_content)

info "Alice tries /ascii-art: $(echo "$alice_try_ascii" | head -c 200)"
info "Alice tries /notion:    $(echo "$alice_try_notion" | head -c 200)"

# Gateway enforces isolation before LLM: should return "not found" for this account
assert_contains "Alice — /ascii-art rejected by gateway" "$alice_try_ascii" \
  "not found|not available|cannot|don't have|no skill|unknown"
assert_contains "Alice — /notion rejected by gateway" "$alice_try_notion" \
  "not found|not available|cannot|don't have|no skill|unknown"

# ── Phase 6: Cross-Tenant Isolation — Bob cannot use Alice's skills ───────────
log ""
log "=== Phase 6: Cross-Tenant Isolation (Bob tries Alice's skills) ==="

bob_try_arxiv=$(chat bob bob-p6-arxiv \
  "/arxiv search for transformers" \
  400 | extract_content)
bob_try_ddg=$(chat bob bob-p6-ddg \
  "/duckduckgo-search Claude Code" \
  400 | extract_content)

info "Bob tries /arxiv:          $(echo "$bob_try_arxiv" | head -c 200)"
info "Bob tries /duckduckgo:     $(echo "$bob_try_ddg" | head -c 200)"

assert_contains "Bob — /arxiv rejected by gateway" "$bob_try_arxiv" \
  "not found|not available|cannot|don't have|no skill|unknown"
assert_contains "Bob — /duckduckgo-search rejected by gateway" "$bob_try_ddg" \
  "not found|not available|cannot|don't have|no skill|unknown"

# ── Phase 7: Concurrent Requests — both users simultaneously ─────────────────
# Use skill-description queries (no live tool calls) to keep latency predictable.
log ""
log "=== Phase 7: Concurrent Skill Requests ==="

alice_out=$(mktemp) bob_out=$(mktemp)

chat alice alice-p7-concurrent \
  "/arxiv What curl command searches for 'attention mechanism' papers? Show the URL only." 400 \
  | extract_content > "$alice_out" &
alice_pid=$!

chat bob bob-p7-concurrent \
  "/ascii-art Draw the text HERMES using figlet-style ASCII characters." 400 \
  | extract_content > "$bob_out" &
bob_pid=$!

wait $alice_pid $bob_pid

alice_concurrent=$(cat "$alice_out")
bob_concurrent=$(cat "$bob_out")
rm -f "$alice_out" "$bob_out"

info "Alice concurrent /arxiv:    $(echo "$alice_concurrent" | head -c 200)"
info "Bob concurrent /ascii-art:  $(echo "$bob_concurrent" | head -c 200)"

assert_contains "Alice concurrent — arxiv response" "$alice_concurrent" \
  "arxiv|export\.arxiv|search_query|curl|api|attention"
assert_contains "Bob concurrent — ascii-art response" "$bob_concurrent" \
  "HERMES|pyfiglet|figlet|ascii|banner|art|H.*E.*R.*M.*E.*S"

# Verify no cross-bleed in concurrent responses
assert_not_contains "Alice concurrent — no ascii-art bleed" "$alice_concurrent" "pyfiglet"
assert_not_contains "Bob concurrent — no arxiv bleed"       "$bob_concurrent"   "export\.arxiv"

# ── Phase 8: Session Persistence — Alice multi-turn ──────────────────────────
log ""
log "=== Phase 8: Session Persistence ==="

# Turn 1: establish context
chat alice alice-p8-persist \
  "Remember this: my research focus is 'quantum computing and error correction'. Confirm you've noted it." \
  300 > /dev/null

# Turn 2: same session — should remember
alice_persist=$(chat alice alice-p8-persist \
  "What research focus did I just tell you about?" \
  300 | extract_content)

info "Alice persistence: $(echo "$alice_persist" | head -c 300)"
assert_contains "Alice session retains context" "$alice_persist" \
  "quantum|error correction|computing"

# Turn 3: Bob fresh session — non-leading question to avoid LLM confabulation.
bob_nocontext=$(chat bob bob-p8-fresh \
  "I am brand new here. Have I ever told you anything about my research interests?" \
  300 | extract_content)

info "Bob no-context: $(echo "$bob_nocontext" | head -c 200)"
assert_not_contains "Bob cannot see Alice's session context" "$bob_nocontext" \
  "quantum computing"

# ── Phase 9: Multi-Turn — Alice uses /arxiv across turns ─────────────────────
log ""
log "=== Phase 9: Multi-Turn Arxiv Research (Alice) ==="

# Turn 1: activate skill
chat alice alice-p9-multi \
  "/arxiv search for 'attention is all you need'" \
  800 > /dev/null

# Turn 2: follow-up in same session
alice_followup=$(chat alice alice-p9-multi \
  "Based on those results, what is the key concept this paper introduced?" \
  600 | extract_content)

info "Alice multi-turn follow-up: $(echo "$alice_followup" | head -c 400)"
assert_contains "Alice multi-turn maintains arxiv context" "$alice_followup" \
  "attention|transformer|self-attention|mechanism|model|paper|found"

# ── Phase 10: Multi-Turn — Bob uses /ascii-art then /notion ──────────────────
log ""
log "=== Phase 10: Multi-Turn Creative + Productivity (Bob) ==="

# Turn 1: ascii-art
chat bob bob-p10-multi \
  "/ascii-art create a simple house ASCII drawing" \
  600 > /dev/null

# Turn 2: notion in same session
bob_multi=$(chat bob bob-p10-multi \
  "/notion I want to save this drawing as a page. What API calls do I need?" \
  800 | extract_content)

info "Bob multi-turn notion: $(echo "$bob_multi" | head -c 400)"
assert_contains "Bob multi-turn combines skills" "$bob_multi" \
  "Notion|API|page|block|create|curl|NOTION_API_KEY|notion\.so"

# ── Summary ───────────────────────────────────────────────────────────────────
log ""
log "════════════════════════════════════════════"
log "TEST RESULTS: PASS=$PASS  FAIL=$FAIL  SKIP=$SKIP"
log "════════════════════════════════════════════"

if [[ $FAIL -eq 0 ]]; then
  log "ALL TESTS PASSED ✅"
  exit 0
else
  log "SOME TESTS FAILED ❌"
  exit 1
fi
