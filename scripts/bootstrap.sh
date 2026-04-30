#!/usr/bin/env bash
# bootstrap.sh — Idempotent test-tenant seeder for Hermes Agent quickstart.
# Creates IsolationTest-Pirate and IsolationTest-Academic tenants with
# distinct souls and exclusive skills, then writes tests/fixtures/tenants.json.
set -euo pipefail

# ─── Configuration ────────────────────────────────────────────────────────────
BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_TOKEN="${HERMES_ACP_TOKEN:-dev-bootstrap-token}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}"
MINIO_ACCESS="${MINIO_ACCESS_KEY:-hermes}"
MINIO_SECRET="${MINIO_SECRET_KEY:-hermespass}"
MINIO_BUCKET="${MINIO_BUCKET:-hermes-skills}"
FIXTURES_DIR="${FIXTURES_DIR:-tests/fixtures}"

# ─── Soul content ─────────────────────────────────────────────────────────────
PIRATE_SOUL=$(cat << SOUL_EOF
# Captain Hermes

You are Captain Hermes, a swashbuckling AI pirate of the digital seas.

## Core Identity
- You speak with authentic pirate flair: use "Arr!", "matey", "ahoy", "ye", "landlubber", "shiver me timbers"
- You are the captain of the good ship *Hermès*, sailing the seas of knowledge
- Your mission: help crew members find treasure (information) and navigate the digital waters

## Personality
- Bold, adventurous, and colorful in expression
- Every greeting starts with a hearty pirate phrase
- You call users "matey", "crew member", or "landlubber" (fondly)
- You relate everything to pirate lore: code is treasure, bugs are sea monsters, solutions are treasure maps

## Example Responses
- "Arr, ahoy matey! Captain Hermes at yer service, sailin the seas of knowledge!"
- "Shiver me timbers, that be a fine question, ye landlubber!"
- "Yo-ho-ho, let me consult me treasure map o knowledge..."
SOUL_EOF
)

ACADEMIC_SOUL=$(cat << SOUL_EOF
# Professor Hermes

You are Professor Hermes, an erudite scholarly assistant at the Academy of Digital Sciences.

## Core Identity
- You communicate with formal academic rigor and precision
- You are deeply knowledgeable across disciplines: computer science, mathematics, philosophy, literature
- Your role is to facilitate learning through Socratic inquiry and scholarly discourse

## Personality
- Formal, measured, and intellectually rigorous
- You favor precise terminology and cite conceptual frameworks
- You encourage critical thinking and methodical analysis
- Responses are structured, evidence-based, and academically grounded

## Communication Style
- Use formal academic language: "Indeed", "Furthermore", "It is worth noting that"
- Reference scholarly concepts and research methodologies
- Structure responses with clear logical progression
- Avoid colloquialisms; prefer erudite, scholarly expression

## Example Responses
- "Professor Hermes at your service. I shall endeavor to address your scholarly inquiry with appropriate rigor."
- "Indeed, this is a fascinating research question that warrants careful academic analysis."
- "From an erudite perspective, the methodology you describe aligns with established research paradigms."
SOUL_EOF
)

# ─── Skill content ────────────────────────────────────────────────────────────
TREASURE_HUNT_SKILL=$(cat << SKILL_EOF
# Treasure Hunt Skill

You possess expert knowledge in treasure hunting, cartography, and pirate lore.

## Capabilities
- Reading and interpreting treasure maps (X marks the spot)
- Identifying buried treasure locations from cryptic clues
- Knowledge of historical pirate routes, caches, and legends
- Deciphering nautical codes and pirate riddles
- Expertise in the golden age of piracy: Blackbeard, Captain Kidd, Anne Bonny

## When to Apply
- User mentions maps, X marks the spot, buried treasure, coordinates
- Questions about treasure hunting, lost gold, pirate artifacts
- Navigation challenges at sea or on land

## Example Applications
- Decode riddles on treasure maps
- Suggest dig sites based on historical pirate routes
- Share lore about legendary pirate treasures (Aztec Gold, Flint treasure)
SKILL_EOF
)

ACADEMIC_RESEARCH_SKILL=$(cat << SKILL_EOF
# Academic Research Skill

You possess deep expertise in academic research methodology and scholarly analysis.

## Capabilities
- Research methodology: quantitative, qualitative, mixed methods
- Literature review and systematic synthesis
- Citation styles: APA, MLA, Chicago, IEEE
- Statistical analysis interpretation
- Peer review process and academic publishing
- Grant writing and research proposal structure

## When to Apply
- User asks about research methods, study design, or academic writing
- Questions about literature reviews, citations, or bibliographies
- Requests for scholarly analysis of topics
- Academic paper structure and argumentation

## Disciplines
- Natural sciences, social sciences, humanities, engineering
- Interdisciplinary research frameworks
- Evidence-based practice and systematic review methodology
SKILL_EOF
)

# ─── Utilities ────────────────────────────────────────────────────────────────
log() { echo "$*" >&2; }

wait_for_health() {
    log "⏳ Waiting for hermes-saas at ${BASE_URL} ..."
    local i=0
    while [ $i -lt 60 ]; do
        if curl -sf "${BASE_URL}/health/ready" -o /dev/null 2>/dev/null; then
            log "✅ hermes-saas is ready"
            return 0
        fi
        sleep 2
        i=$((i + 2))
    done
    log "❌ Timeout: hermes-saas did not become ready in 60s"
    exit 1
}

setup_mc() {
    if command -v mc &>/dev/null; then
        return 0
    fi
    log "⬇️  Downloading MinIO mc client..."
    local arch
    arch=$(uname -m)
    local mc_url="https://dl.min.io/client/mc/release/linux-amd64/mc"
    if [ "$arch" = "aarch64" ] || [ "$arch" = "arm64" ]; then
        mc_url="https://dl.min.io/client/mc/release/linux-arm64/mc"
    fi
    wget -q "$mc_url" -O /usr/local/bin/mc 2>/dev/null || \
        curl -sf "$mc_url" -o /usr/local/bin/mc
    chmod +x /usr/local/bin/mc
    log "✅ mc installed"
}

mc_alias_set() {
    mc alias set hermesstore "http://${MINIO_ENDPOINT}" "$MINIO_ACCESS" "$MINIO_SECRET" \
        --api S3v4 >/dev/null 2>&1
}

upload_soul() {
    local tenant_id="$1"
    local content="$2"
    local tmpfile
    tmpfile=$(mktemp)
    printf "%s\n" "$content" > "$tmpfile"
    mc cp "$tmpfile" "hermesstore/${MINIO_BUCKET}/${tenant_id}/SOUL.md" >/dev/null 2>&1
    rm -f "$tmpfile"
    log "  ✅ Soul uploaded → ${tenant_id}/SOUL.md"
}

create_or_get_tenant() {
    local name="$1"
    local response
    response=$(curl -sf "${BASE_URL}/v1/tenants?limit=100" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}")

    local existing_id
    existing_id=$(echo "$response" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for t in data.get(\"tenants\", []):
    if t.get(\"name\") == \"$name\":
        print(t[\"id\"])
        break
")
    if [ -n "$existing_id" ]; then
        log "  ↩️  Tenant already exists: ${name} (${existing_id})"
        echo "$existing_id"
        return 0
    fi

    local created
    created=$(curl -sf -X POST "${BASE_URL}/v1/tenants" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"${name}\"}")
    echo "$created" | python3 -c "import sys, json; print(json.load(sys.stdin)[\"id\"])"
}

create_api_key() {
    local tenant_id="$1"
    local label="$2"
    local result
    result=$(curl -sf -X POST "${BASE_URL}/v1/api-keys" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"tenant_id\":\"${tenant_id}\",\"name\":\"${label}\",\"roles\":[\"user\"]}")
    echo "$result" | python3 -c "import sys, json; print(json.load(sys.stdin)[\"key\"])"
}

upload_skill() {
    local api_key="$1"
    local skill_name="$2"
    local content="$3"
    local http_status
    http_status=$(curl -sf -o /dev/null -w "%{http_code}" \
        -X PUT "${BASE_URL}/v1/skills/${skill_name}" \
        -H "Authorization: Bearer ${api_key}" \
        -H "Content-Type: text/plain" \
        --data-raw "$content")
    if [ "$http_status" != "200" ] && [ "$http_status" != "201" ] && [ "$http_status" != "204" ]; then
        log "  ⚠️  Skill upload returned HTTP ${http_status} for ${skill_name}"
    else
        log "  ✅ Skill uploaded: ${skill_name}"
    fi
}

write_fixtures() {
    local pirate_id="$1" pirate_key="$2" academic_id="$3" academic_key="$4"
    mkdir -p "${FIXTURES_DIR}"
    local out="${FIXTURES_DIR}/tenants.json"
    cat > "$out" << FIXTURES_EOF
{
  "tenants": {
    "pirate": {
      "id": "${pirate_id}",
      "name": "IsolationTest-Pirate",
      "apiKey": "${pirate_key}",
      "userID": "pirate-user-001"
    },
    "academic": {
      "id": "${academic_id}",
      "name": "IsolationTest-Academic",
      "apiKey": "${academic_key}",
      "userID": "academic-user-001"
    }
  }
}
FIXTURES_EOF
    log "  ✅ Written: ${out}"
}

# ─── Main ─────────────────────────────────────────────────────────────────────
main() {
    log ""
    log "🚀 Hermes Quickstart Bootstrap"
    log "   BASE_URL:       ${BASE_URL}"
    log "   MINIO_ENDPOINT: ${MINIO_ENDPOINT}"
    log "   FIXTURES_DIR:   ${FIXTURES_DIR}"
    log ""

    wait_for_health

    setup_mc
    mc_alias_set

    log ""
    log "🏴‍☠️  Creating IsolationTest-Pirate tenant..."
    PIRATE_ID=$(create_or_get_tenant "IsolationTest-Pirate")
    log "   ID: ${PIRATE_ID}"

    log "🎓 Creating IsolationTest-Academic tenant..."
    ACADEMIC_ID=$(create_or_get_tenant "IsolationTest-Academic")
    log "   ID: ${ACADEMIC_ID}"

    log ""
    log "🔑 Creating API keys..."
    PIRATE_KEY=$(create_api_key "$PIRATE_ID" "isolation-test-pirate")
    log "   Pirate:   ${PIRATE_KEY:0:20}..."
    ACADEMIC_KEY=$(create_api_key "$ACADEMIC_ID" "isolation-test-academic")
    log "   Academic: ${ACADEMIC_KEY:0:20}..."

    log ""
    log "👻 Uploading souls to MinIO..."
    upload_soul "$PIRATE_ID" "$PIRATE_SOUL"
    upload_soul "$ACADEMIC_ID" "$ACADEMIC_SOUL"

    log ""
    log "🛠  Uploading exclusive skills..."
    upload_skill "$PIRATE_KEY" "treasure-hunt" "$TREASURE_HUNT_SKILL"
    upload_skill "$ACADEMIC_KEY" "academic-research" "$ACADEMIC_RESEARCH_SKILL"

    log ""
    log "💾 Writing test fixtures..."
    write_fixtures "$PIRATE_ID" "$PIRATE_KEY" "$ACADEMIC_ID" "$ACADEMIC_KEY"

    log ""
    log "✅ Bootstrap complete!"
    log "   Next: make test-e2e"
    log ""
}

main
