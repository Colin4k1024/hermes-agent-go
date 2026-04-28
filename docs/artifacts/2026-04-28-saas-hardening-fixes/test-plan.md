# Test Plan: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Owner | qa-engineer |
| Status | Draft |

---

## Test Scope

### Functional

| Area | Coverage |
|------|----------|
| Config concurrent Load/Reload/SetActiveProfile | Race detector test |
| Profile override thread safety | Race detector test |
| Memory func pointer sync | Race detector test |
| Rate limiter LRU bounded growth | Unit test under 10K+ unique keys |
| API Key GetByID tenant isolation | Unit test: cross-tenant rejection |
| API Key Revoke tenant isolation | Unit test: cross-tenant rejection |
| Audit log new fields (request_id, status_code, latency_ms) | Integration test with PG |
| LLM call structured logging | Unit test: log output verification |
| Prometheus LLM metrics | Unit test: counter/histogram registration |
| Prometheus tenant_id HTTP metrics | Unit test: label cardinality |
| OpenTelemetry noop mode | Unit test: no crash when OTEL endpoint unset |

### Non-Functional

| Area | Coverage |
|------|----------|
| Concurrency safety | `go test -race` on all modified packages |
| Memory bounded growth | Rate limiter stabilizes at 10K buckets |
| Build integrity | `go build ./...` clean |
| Backward compatibility | No existing test broken |

### Not Covered

- E2E with real PG + Redis (requires docker-compose environment)
- OTel trace export verification (requires Jaeger/OTLP collector)
- Load testing under sustained multi-tenant traffic
- LLM credential encryption (design only, not implemented)

---

## Test Matrix

| # | Scenario | Type | Pre-condition | Expected Result |
|---|----------|------|---------------|-----------------|
| T1 | Concurrent config Load + Reload | Race | Multiple goroutines | No race, consistent config |
| T2 | Concurrent SetActiveProfile + GetActiveProfile | Race | Multiple goroutines | No race, correct profile |
| T3 | SetMemoryProviderNameFunc during getMemoryProvider | Race | Concurrent access | No race |
| T4 | Rate limiter with 10K+ unique keys | Unit | Rotating IP addresses | Bucket count capped at 10K |
| T5 | GetByID with wrong tenant_id | Unit | Key belongs to tenant A | Returns not found for tenant B |
| T6 | Revoke with wrong tenant_id | Unit | Key belongs to tenant A | Returns not found for tenant B |
| T7 | Audit log write with enriched fields | Integration | Auth + request context | request_id, status_code, latency_ms populated |
| T8 | LLM call produces structured log | Unit | Mock transport | slog.Info("llm_call") with all fields |
| T9 | LLM streaming produces log on completion | Unit | Mock transport | slog.Info("llm_call_stream") after stream ends |
| T10 | OTel tracer init with empty endpoint | Unit | OTEL_EXPORTER_OTLP_ENDPOINT="" | Noop provider, no error |
| T11 | pgx slow query detection | Unit | Query >500ms | WARN log with sql_prefix |
| T12 | HTTP metrics include tenant_id | Unit | Authenticated request | Counter has tenant_id label |

---

## Test Results

| Package | Status | Race |
|---------|--------|------|
| `internal/config` | ✅ PASS | ✅ Clean |
| `internal/tools` | ✅ PASS | ✅ Clean |
| `internal/tools/environments` | ✅ PASS | ✅ Clean |
| `internal/middleware` | ✅ PASS | ✅ Clean |
| `internal/auth` | ✅ PASS | ✅ Clean |
| `internal/llm` | ✅ PASS | ✅ Clean |
| `go build ./...` | ✅ PASS | N/A |

---

## Risk

| Risk | Severity | Mitigation |
|------|----------|------------|
| Prometheus tenant_id high cardinality | MEDIUM | Bounded tenant set assumption; document exemplars fallback |
| pgx tracer SQL prefix may log sensitive literals | MEDIUM | Truncate to 40 chars; parameterized queries don't embed values |
| OTel traces may contain PII in span attributes | MEDIUM | Only log method + path, not request/response bodies |
| audit_logs ALTER TABLE on large tables | LOW | PG 11+ ADD COLUMN NULL is instant |

---

## Release Recommendation

**Recommend release** — all 7 test packages pass with `-race`, zero `sync.Once` reassignment, zero unscoped API key queries. Security and code review pending final integration.
