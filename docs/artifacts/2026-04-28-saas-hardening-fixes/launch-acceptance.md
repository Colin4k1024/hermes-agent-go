# Launch Acceptance: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Owner | qa-engineer |
| Status | Accepted with conditions |

---

## Acceptance Overview

| Field | Value |
|-------|-------|
| Object | SaaS Hardening — 14 fixes + 1 design |
| Time | 2026-04-28 |
| Roles | qa-engineer, security-reviewer, code-reviewer |
| Method | `go test -race` + code review + security review |

---

## Acceptance Scope

### In Scope

- Concurrency safety: config race (atomic.Pointer), profile race (RWMutex), memory func pointer (mutex), rate limiter (LRU)
- Data isolation: API Key GetByID/Revoke SQL-level tenant enforcement
- Observability: OpenTelemetry, LLM call logging + metrics, structured logging, pgx query tracing, audit trail enrichment, tenant Prometheus labels
- Security: /metrics auth, OTLP TLS default, span PII redaction, audit query sanitization, config file permissions

### Out of Scope

- LLM credential encryption (design only)
- E2E testing with real PG/Redis/Jaeger
- Load testing under multi-tenant traffic
- Per-tenant billing integration

---

## Acceptance Evidence

### Test Results

| Package | Status | Race Detector |
|---------|--------|---------------|
| internal/config | PASS | Clean |
| internal/tools | PASS | Clean |
| internal/tools/environments | PASS | Clean |
| internal/middleware | PASS | Clean |
| internal/auth | PASS | Clean |
| internal/llm | PASS | Clean |
| `go build ./...` | PASS | N/A |

### Security Review

| Finding | Severity | Status |
|---------|----------|--------|
| /metrics tenant enumeration | HIGH | FIXED — behind auth stack |
| OTLP plaintext transport | MEDIUM | FIXED — TLS default |
| Full URL in OTLP spans | MEDIUM | FIXED — path only |
| SQL prefix in metrics | MEDIUM | VERIFIED — parameterized queries only |
| Raw query in audit detail | MEDIUM | FIXED — sensitive params redacted |
| config.yaml permissions | LOW | FIXED — 0600 |
| rand.Read return unchecked | LOW | ACCEPTED — pre-existing, Go 1.20+ panics on failure |

### Code Verification

| Check | Result |
|-------|--------|
| `sync.Once{}` reassignment | Zero occurrences in internal/config/ |
| `WHERE id = $1` without tenant | Zero occurrences in apikey.go |
| All 8 APIKeyStore call sites updated | Compiler-verified |
| OTel noop when endpoint unset | Verified in tracer.go |

---

## Risk Assessment

| Risk | Verdict |
|------|---------|
| Prometheus tenant_id cardinality | Accepted — bounded tenant set in SaaS deployment |
| pgx tracer overhead | Accepted — histogram observation is O(1) |
| Audit table ALTER on large data | Accepted — PG 11+ ADD COLUMN NULL is instant |
| Streaming LLM timing accuracy | Accepted — measures total stream duration, not TTFT |

---

## Launch Conclusion

| Field | Value |
|-------|-------|
| Decision | **Conditional Go** |
| Conditions | Code review findings (pending) must not surface CRITICAL issues |
| Observation | Monitor Prometheus cardinality after deployment; check pgx tracer WARN logs for false slow queries |
| Confirmed by | qa-engineer |
