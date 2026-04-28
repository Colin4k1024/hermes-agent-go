# Delivery Plan: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Status | Draft |
| Owner | tech-lead |
| PRD | `docs/artifacts/2026-04-28-saas-hardening-fixes/prd.md` |
| Baseline Commit | `ea76181` (main) |
| Go Version | 1.25.0 |
| Migration Baseline | v23 |

---

## Requirement Challenge Session Log

### Challenge 1: sync.RWMutex vs atomic.Pointer for Config

| Field | Content |
|-------|---------|
| Challenger | architect |
| Assumption | Replace sync.Once reassignment with sync.RWMutex for Load/Reload |
| Challenge | RWMutex adds read lock on every Load() call — hot path overhead |
| Alternative | `atomic.Pointer[Config]` for Load() (zero-contention read), mutex only for Reload/SetActiveProfile writes |
| Conclusion | **Adopt hybrid**: `atomic.Pointer[Config]` for read path, `sync.Mutex` for write path (Reload/SetActiveProfile). This gives lock-free reads (same perf as sync.Once after init) with safe writes. |
| Abort Condition | If atomic.Pointer introduces import cycle or Go version constraint — N/A, Go 1.25 supports it |

### Challenge 2: SQL-level vs App-level Tenant Enforcement

| Field | Content |
|-------|---------|
| Challenger | backend-engineer |
| Assumption | Move tenant check from app-level (apikeys.go:112-119) to SQL WHERE clause |
| Challenge | App-level check already exists in handler (GetByID → compare TenantID → 403). Is SQL redundant? |
| Alternative | Keep app-level only |
| Conclusion | **Defense in depth — do both**. SQL-level prevents any future caller from bypassing the check. App-level provides clear HTTP error responses. The interface signature change makes tenant-scoping a compile-time contract, not a runtime honor system. |
| Abort Condition | None — low-risk change |

### Challenge 3: OpenTelemetry Dependency Weight

| Field | Content |
|-------|---------|
| Challenger | architect |
| Assumption | Full OTel SDK is needed |
| Challenge | ~15 transitive deps for a CLI tool that also runs as SaaS server |
| Alternative | Build-tag gated: `//go:build otel` |
| Conclusion | **Accept full SDK, no build tags**. The SaaS binary is the primary deployment target. CLI binary size increase is marginal. Build tags add testing complexity for questionable benefit. Use `noop` TracerProvider when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset — zero runtime overhead for CLI mode. |
| Abort Condition | If OTel SDK > 50 transitive deps or conflicts with existing deps |

### Challenge 4: Rate Limiter Eviction Strategy

| Field | Content |
|-------|---------|
| Challenger | backend-engineer |
| Assumption | Periodic GC goroutine every 5min |
| Challenge | GC goroutine is yet another goroutine lifecycle to manage; lazy eviction simpler |
| Alternative | Use `hashicorp/golang-lru/v2` (already indirect dep in go.mod) with TTL wrapper |
| Conclusion | **Adopt LRU**: promote `hashicorp/golang-lru/v2` to direct dep. Use `lru.NewWithEvict[string, *bucket](maxSize, onEvict)`. MaxSize=10000. On each `allow()`, check bucket window expiry (lazy TTL). This eliminates the GC goroutine entirely. |
| Abort Condition | None — library already in dependency tree |

---

## Brownfield Context Snapshot

### Existing Module Boundaries

| Package | Responsibility | Hardening Impact |
|---------|---------------|-----------------|
| `internal/config` | Config loading, profiles | P1: sync.Once race fix |
| `internal/tools` | Tool registry, memory | P1: func pointer race |
| `internal/middleware` | HTTP middleware chain | P1: rate limiter; P3: metrics, audit, tracing |
| `internal/store/pg` | PG persistence | P2: apikey tenant scoping; P3: audit fields, migration |
| `internal/store/sqlite` | SQLite noop fallback | P2: interface sync |
| `internal/store` | Interface definitions | P2: APIKeyStore signature change |
| `internal/api` | REST API handlers | P2: apikey handler update |
| `internal/auth` | Authentication extractors | P2: test mock update |
| `internal/llm` | LLM client + transport | P3: call observability |
| `internal/observability` | **NEW** | P3: tracer, logger, db tracer |

### Key Dependencies Already Available

- `hashicorp/golang-lru/v2 v2.0.7` — indirect, promote for rate limiter
- `prometheus/client_golang v1.23.2` — already used for metrics
- `jackc/pgx/v5 v5.9.2` — supports QueryTracer interface
- `golang-jwt/jwt/v5 v5.3.1` — already used for auth

### Migration State

Current: v23 (api_keys indexes + memories + user_profiles). Next available: v24+.

---

## Story Slices

### Slice 1: CRITICAL Concurrency — Config + Profile Race [P1]

**Goal**: Eliminate all `sync.Once` reassignment patterns  
**Owner**: backend-engineer  
**Effort**: 0.5 day  
**Dependencies**: None  
**Files**:
- `internal/config/config.go` — Replace `configOnce`/`globalConfig` with `atomic.Pointer[Config]` + `sync.Mutex` for writes
- `internal/config/profiles.go` — Replace `activeProfileOnce`/`activeProfile`/`profileOverride` with mutex-protected state

**Changes**:
1. New package-level var: `var configPtr atomic.Pointer[Config]` + `var configMu sync.Mutex`
2. `Load()`: `if p := configPtr.Load(); p != nil { return p }` → fast path no lock. Slow path: `configMu.Lock()`, double-check, init, `configPtr.Store(cfg)`, unlock.
3. `Reload()`: `configMu.Lock()`, init new config, `configPtr.Store(cfg)`, unlock. No `sync.Once` reset.
4. Profile state: single `var profileMu sync.RWMutex` protecting `activeProfile` and `profileOverride`.
5. `SetActiveProfile()` / `OverrideActiveProfile()`: take `profileMu.Lock()`, update state, call `configPtr.Store(nil)` to invalidate.
6. `GetActiveProfile()`: `profileMu.RLock()` → read cached value.
7. Remove all `sync.Once` variables.

**Acceptance**:
- `go test -race ./internal/config/...` with concurrent Load/Reload/SetActiveProfile test
- Zero `sync.Once` reassignment in package
- `grep -r 'sync.Once{}' internal/config/` returns empty

**Handoff**: → qa-engineer for race detector verification

### Slice 2: HIGH Concurrency — Memory Func Pointer + Rate Limiter [P1]

**Goal**: Fix remaining HIGH concurrency issues  
**Owner**: backend-engineer  
**Effort**: 0.5 day  
**Dependencies**: None (parallel with Slice 1)  
**Files**:
- `internal/tools/memory.go:73-81` — Protect `getMemoryProviderName` with `activeProviderMu`
- `internal/middleware/ratelimit.go:80-112` — Replace unbounded map with LRU

**Changes — memory.go**:
1. `SetMemoryProviderNameFunc()`: acquire `activeProviderMu.Lock()` before writing `getMemoryProviderName`
2. Add read lock in `getMemoryProviderName` call site (already inside `getMemoryProvider()` which holds the lock — verify no double-lock)

**Changes — ratelimit.go**:
1. Replace `buckets map[string]*bucket` with `lru.Cache[string, *bucket]` (from `hashicorp/golang-lru/v2`)
2. Constructor: `lru.New[string, *bucket](10000)` — 10K max entries
3. `allow()`: `l.buckets.Get(key)` + check window expiry, `l.buckets.Add(key, newBucket)` on miss/expiry
4. Promote `hashicorp/golang-lru/v2` from indirect to direct in go.mod

**Acceptance**:
- `go test -race ./internal/tools/...` passes
- `go test -race ./internal/middleware/...` passes
- Rate limiter memory stable after 10K+ unique keys

**Handoff**: → qa-engineer for race + load verification

### Slice 3: Tenant Isolation — API Key Scoping [P2]

**Goal**: Enforce tenant_id at SQL level for GetByID and Revoke  
**Owner**: backend-engineer  
**Effort**: 0.5 day  
**Dependencies**: None (parallel with Slice 1-2)  
**Files** (8 call sites):
- `internal/store/store.go:71-74` — Interface change
- `internal/store/pg/apikey.go:38-48,74-80` — SQL change
- `internal/store/sqlite/noop.go:49,55` — Signature sync
- `internal/api/apikeys.go:112,121` — Pass tenantID from context
- `internal/api/server_test.go:73` — Update stub
- `internal/api/apikeys_test.go:45` — Update mock
- `internal/auth/apikey_test.go:33` — Update mock
- `internal/api/me_test.go:27` — Update mock (returns nil)

**Interface Change**:
```go
// Before
GetByID(ctx context.Context, id string) (*APIKey, error)
Revoke(ctx context.Context, id string) error

// After
GetByID(ctx context.Context, tenantID, id string) (*APIKey, error)
Revoke(ctx context.Context, tenantID, id string) error
```

**SQL Change**:
```sql
-- GetByID: add tenant filter
SELECT ... FROM api_keys WHERE tenant_id = $1 AND id = $2

-- Revoke: add tenant filter
UPDATE api_keys SET revoked_at = now() WHERE tenant_id = $1 AND id = $2
```

**Handler Change** (`apikeys.go:110-126`):
```go
func (h *APIKeyHandler) revoke(w http.ResponseWriter, r *http.Request, id string) {
    tenantID := middleware.TenantFromContext(r.Context())
    // GetByID now enforces tenant at SQL level — no separate tenant check needed
    key, err := h.store.GetByID(r.Context(), tenantID, id)
    // ... (remove manual TenantID comparison, SQL handles it)
    if err := h.store.Revoke(r.Context(), tenantID, id); err != nil {
```

**Acceptance**:
- `go build ./...` compiles clean
- New test: `TestCrossTenantGetByID_Rejected` — tenant A cannot see tenant B's key
- New test: `TestCrossTenantRevoke_Rejected` — tenant A cannot revoke tenant B's key
- `go test ./internal/api/... ./internal/store/... ./internal/auth/...` passes

**Handoff**: → qa-engineer for cross-tenant verification

### Slice 4: Structured Logging Foundation [P3]

**Goal**: Create observability package with context-aware logger  
**Owner**: backend-engineer  
**Effort**: 1 day  
**Dependencies**: Slice 1 (needs stable config for env var reading)  
**Files**:
- **NEW** `internal/observability/logger.go` — ContextLogger, WithLogger, context keys
- `internal/middleware/requestid.go` — Ensure request_id is in context (already exists)
- High-priority log sites: all `slog.Error` and `slog.Warn` calls in `internal/api/`, `internal/middleware/`, `internal/gateway/`

**Changes**:
1. `observability.ContextLogger(ctx)` extracts request_id, tenant_id, session_id, trace_id from context → returns `slog.With(...)` logger
2. `observability.WithLogger(ctx, logger)` stores enriched logger in context
3. New logging middleware (insert after RequestID + Auth + Tenant in chain): creates enriched logger, injects into context
4. Migrate all `slog.Error(...)` and `slog.Warn(...)` in `internal/api/`, `internal/middleware/`, `internal/gateway/` to use `observability.ContextLogger(ctx)`
5. Priority: error paths first, then middleware, then handlers

**Acceptance**:
- All `slog.Error`/`slog.Warn` in api/middleware/gateway include request_id + tenant_id
- Log search by request_id returns complete request lifecycle
- `go test ./internal/observability/...` passes

**Handoff**: → Slice 5 (LLM observability depends on logger)

### Slice 5: LLM Call Observability [P3]

**Goal**: Every LLM call produces structured log + Prometheus metrics  
**Owner**: backend-engineer  
**Effort**: 0.5 day  
**Dependencies**: Slice 4 (ContextLogger)  
**Files**:
- `internal/llm/client.go:209-216` — Wrap `CreateChatCompletion` and `CreateChatCompletionStream`
- **NEW** `internal/llm/metrics.go` — Prometheus histograms and counters

**Changes**:
1. Wrap `CreateChatCompletion`: record start time, call transport, log result with slog.Info("llm_call", ...), record Prometheus metrics
2. `hermes_llm_request_duration_seconds{model, tenant_id}` — histogram
3. `hermes_llm_tokens_total{model, tenant_id, direction}` — counter (input/output)
4. Extract tenant_id from context; extract token counts from `ChatResponse.Usage`
5. For streaming: record timing from first chunk to done, aggregate tokens from final chunk

**Acceptance**:
- Every LLM call produces slog.Info with trace_id/tenant_id/session_id/model/tokens/latency/cost/error
- `hermes_llm_request_duration_seconds` and `hermes_llm_tokens_total` visible in /metrics
- `go test ./internal/llm/...` passes

**Handoff**: → Slice 6 (OTel adds trace_id to these logs)

### Slice 6: OpenTelemetry Integration [P3]

**Goal**: Full distributed tracing across HTTP → PG → LLM  
**Owner**: backend-engineer  
**Effort**: 2 days  
**Dependencies**: Slice 4, Slice 5 (logger and LLM metrics in place)  
**Files**:
- **NEW** `internal/observability/tracer.go` — TracerProvider init, shutdown
- `internal/middleware/chain.go` — Add Tracing slot to StackConfig
- **NEW** `internal/middleware/tracing.go` — HTTP tracing middleware (root span, X-Trace-ID header)
- `internal/store/pg/pg.go` — pgx tracer hook registration
- `internal/llm/client.go` — Wrap HTTP client with otel transport
- `cmd/hermes/saas.go` — Initialize TracerProvider on startup
- `go.mod` — Add OTel dependencies

**New Dependencies**:
```
go.opentelemetry.io/otel
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

**Changes**:
1. `tracer.go`: Init TracerProvider with OTLP gRPC exporter (reads `OTEL_EXPORTER_OTLP_ENDPOINT`, noop if unset)
2. `tracing.go`: Create root span per request, inject trace_id/span_id into context, set `X-Trace-ID` response header
3. `chain.go`: Add `Tracing Middleware` slot, execute before Metrics
4. pgx tracer: Implement `pgx.QueryTracer` interface — create child spans for DB queries
5. LLM client: Wrap HTTP transport with `otelhttp.NewTransport`
6. `saas.go`: Call `observability.InitTracer()` on startup, defer `Shutdown()`

**Acceptance**:
- `docker compose up` with Jaeger shows request traces
- Traces include: HTTP handler → PG query → LLM call spans
- trace_id visible in response headers and logs
- When `OTEL_EXPORTER_OTLP_ENDPOINT` unset: noop, zero overhead

**Handoff**: → Slice 7

### Slice 7: Database + Audit Observability [P3]

**Goal**: Query-level visibility + enriched audit trail  
**Owner**: backend-engineer  
**Effort**: 1 day  
**Dependencies**: Slice 6 (pgx tracer hook from OTel)  
**Files**:
- `internal/store/pg/migrate.go` — Migration v24-v27
- `internal/store/types.go:86-94` — Add fields to AuditLog struct
- `internal/store/pg/auditlog.go` — Update INSERT/SELECT
- `internal/middleware/audit.go` — Capture status_code via ResponseWriter wrapper, add request_id
- **NEW** `internal/observability/pgtracer.go` — pgx.QueryTracer impl (if not in Slice 6)

**Migrations** (v24-v27):
```sql
-- v24
ALTER TABLE audit_logs ADD COLUMN request_id TEXT;
-- v25
ALTER TABLE audit_logs ADD COLUMN status_code INT;
-- v26
ALTER TABLE audit_logs ADD COLUMN latency_ms INT;
-- v27
CREATE INDEX idx_audit_request ON audit_logs(request_id);
```

**Audit Middleware Change**:
- Wrap ResponseWriter (reuse `statusWriter` from metrics.go or extract shared)
- Record start time
- After handler: capture status_code, latency_ms, request_id from context
- Populate new AuditLog fields

**DB Observability**:
- pgx QueryTracer: log query + duration at DEBUG, slow queries (>500ms) at WARN
- `hermes_pg_query_duration_seconds{operation}` Prometheus histogram
- Expose pgxpool stats: idle/total connections, wait count

**Acceptance**:
- Audit query by request_id returns full action trail
- Slow queries (>500ms) appear in logs with tenant context
- `hermes_pg_query_duration_seconds` visible in /metrics
- `go test ./internal/middleware/... ./internal/store/pg/...` passes

**Handoff**: → Slice 8

### Slice 8: LLM Credential Design + Tenant Metrics [P2+P3]

**Goal**: Design doc for per-tenant LLM credentials + tenant_id on HTTP metrics  
**Owner**: architect (2.3 design) + backend-engineer (3.6 metrics)  
**Effort**: 0.5 + 0.5 = 1 day  
**Dependencies**: Slices 1-7 complete  
**Files**:
- **NEW** `docs/artifacts/2026-04-28-saas-hardening-fixes/llm-credential-design.md` — Design only
- `internal/middleware/metrics.go` — Add tenant_id label

**2.3 Design Deliverable** (document only):
- Schema: `ALTER TABLE tenants ADD COLUMN llm_api_key_encrypted BYTEA`
- Encryption: AES-256-GCM with server-side key from env var
- AgentFactory change: check `tenant.LLMAPIKey` → fallback to global `cfg.APIKey`
- Migration path: backward compatible, empty = use global

**3.6 Metrics Change**:
- Add `tenant_id` label to `hermes_http_requests_total` and `hermes_http_request_duration_seconds`
- Extract from context after auth middleware
- Use `"anonymous"` for unauthenticated
- Note: monitor cardinality — document bounded tenant set assumption

**Acceptance**:
- Design doc reviewed by tech-lead
- `hermes_http_requests_total` queryable by tenant_id in Grafana
- `go test ./internal/middleware/...` passes

**Handoff**: → qa-engineer for final verification → tech-lead for closeout

---

## Execution Timeline

```
Day 1 (AM): Slice 1 + Slice 2 + Slice 3 [parallel — no dependencies]
Day 1 (PM): Race detector verification for Slices 1-3
Day 2 (AM): Slice 4 (structured logging)
Day 2 (PM): Slice 5 (LLM observability)
Day 3-4:    Slice 6 (OpenTelemetry — largest slice)
Day 5 (AM): Slice 7 (DB + audit observability)
Day 5 (PM): Slice 8 (design doc + tenant metrics)
Day 6:      Integration testing + closeout
```

**Total**: ~6 working days (compressed from 8.5 via parallelism in Slices 1-3)

---

## Parallelization Strategy

| Parallel Group | Slices | Rationale |
|----------------|--------|-----------|
| Group A | 1, 2, 3 | Zero dependency between config race, memory/ratelimit, and API key scoping |
| Group B | 4 → 5 → 6 | Sequential: logger → LLM metrics → OTel (each builds on previous) |
| Group C | 7 | Depends on Slice 6 (pgx tracer) |
| Group D | 8 | Depends on all above for final metrics integration |

---

## Risk & Mitigation

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| atomic.Pointer subtle bug in config init ordering | HIGH | LOW | Double-checked locking pattern; extensive race tests |
| APIKeyStore interface change breaks untested callers | MEDIUM | LOW | grep found all 8 call sites; compile will catch |
| OTel transitive dep conflicts | MEDIUM | LOW | Pin versions; test `go mod tidy` early |
| audit_logs ALTER TABLE on large table | LOW | LOW | PG 11+ ADD COLUMN NULL is instant |
| Prometheus cardinality explosion from tenant_id | MEDIUM | MEDIUM | Document bounded-set assumption; add exemplars fallback |

---

## Implementation Readiness

| Criterion | Status |
|-----------|--------|
| PRD exists and intake complete | ✅ |
| All 14 problems code-verified | ✅ |
| Challenge session complete (4 challenges) | ✅ |
| Story slices defined with acceptance criteria | ✅ |
| Dependency graph clear | ✅ |
| No unresolved blockers | ✅ |
| Brownfield context captured | ✅ |

**Conclusion**: `handoff-ready` — plan can proceed to `/team-execute`.

---

## Role Assignment Summary

| Role | Slices | Focus |
|------|--------|-------|
| backend-engineer | 1-7, 8(metrics) | All implementation |
| architect | 8(design) | LLM credential design doc |
| qa-engineer | All | Race detector, cross-tenant tests, integration verification |
| tech-lead | All | Coordination, closeout |

---

## Skill Kit

| Skill | Reason |
|-------|--------|
| `golang-patterns` | sync primitives, atomic.Pointer patterns |
| `golang-testing` | Race detector, table-driven tests |
| `security-review` | Cross-tenant isolation verification |
| `postgres-patterns` | Migration strategy, pgx tracer |
| `docker-patterns` | OTel collector in docker-compose |
