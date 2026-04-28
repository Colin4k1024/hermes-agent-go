# Execute Log: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Owner | backend-engineer |
| Status | Complete |

---

## Plan vs Actual

| Slice | Plan | Actual | Deviation |
|-------|------|--------|-----------|
| 1: Config race | 0.5d | Done | None — atomic.Pointer + Mutex as planned |
| 2: Memory + RateLimiter | 0.5d | Done | Used activeProviderMu (simpler than atomic.Value) for memory.go |
| 3: API Key tenant | 0.5d | Done | None — all 8 call sites updated |
| 4: Structured logging | 1d | Done | Created LoggingMiddleware + Audit slot in chain config |
| 5: LLM observability | 0.5d | Done | None |
| 6: OpenTelemetry | 2d | Done | Tracing middleware + tracer init; pgx tracer deferred to pgtracer helper |
| 7: DB + Audit | 1d | Done | Migration v24-v27 added; audit middleware with status capture |
| 8: Tenant metrics + design | 1d | Done | Design doc written; tenant_id label on HTTP metrics |

## Key Decisions

1. **atomic.Pointer[Config]** for config Load() — zero-contention reads, mutex only for writes.
2. **activeProviderMu.Lock()** for memory func pointer — simpler than atomic.Value, already under same lock in read path.
3. **hashicorp/golang-lru/v2** for rate limiter — already indirect dep, promoted to direct. 10K max entries.
4. **SQL + app-level tenant check** — defense in depth; SQL prevents future callers from bypassing.
5. **OTel noop when OTEL_EXPORTER_OTLP_ENDPOINT unset** — zero overhead for CLI mode.
6. **AES-256-GCM for LLM credential design** — server-side key, design only not implemented.

## Impact

| Area | Files Changed | Description |
|------|---------------|-------------|
| Config | 2 | config.go, profiles.go — sync primitive replacement |
| Tools | 1 | memory.go — lock protection |
| Middleware | 5 | ratelimit, audit, metrics, chain, logging (new), tracing (new) |
| Store | 5 | store.go interface, pg/apikey.go, pg/auditlog.go, pg/migrate.go, sqlite/noop.go |
| LLM | 2 | client.go, metrics.go (new) |
| Observability | 3 | logger.go (new), tracer.go (new), pgtracer.go (new) |
| API | 1 | apikeys.go handler |
| Tests | 3 | apikeys_test.go, server_test.go, apikey_test.go — mock signature updates |
| Dependencies | 2 | go.mod, go.sum — OTel SDK + LRU promotion |
| Gateway | 1 | runner.go — enriched logger injection + slog migration |
| **Total** | **25 files, +387 / -162 lines** | |

## Test Results

- `go build ./...` — PASS
- `go test -race ./internal/config/...` — PASS
- `go test -race ./internal/tools/...` — PASS
- `go test -race ./internal/middleware/...` — PASS
- `go test -race ./internal/auth/...` — PASS
- `go test -race ./internal/llm/...` — PASS
- All 7 test packages pass with race detector

## Previously Deferred — Now Completed

- **pgx QueryTracer**: `PGXTracer` implementing `pgx.QueryTracer` wired into `pg.New()` via `pgxpool.ParseConfig` + `ConnConfig.Tracer`. Slow queries >500ms logged at WARN with `hermes_pgx_query_duration_seconds` Prometheus histogram.
- **LLM streaming observability**: `CreateChatCompletionStream` wrapped with goroutine that measures total stream duration, logs `llm_call_stream` with model/tenant/latency, and records Prometheus metrics.
- **slog.Error/Warn migration**: SaaS HTTP path migrated — `middleware/auth.go`, `middleware/ratelimit.go`, `api/gdpr.go`, `gateway/runner.go` (handleMessage + processWithAgent). Non-HTTP paths (CLI, init, background goroutines) retain bare slog since they lack request context.

## Out of Scope (by design)

- LLM credential encryption implementation — design only as planned (US-6)

## New Files

| File | Purpose |
|------|---------|
| `internal/observability/logger.go` | Context-aware slog.Logger |
| `internal/observability/tracer.go` | OTel TracerProvider init |
| `internal/observability/pgtracer.go` | PG query duration Prometheus histogram (helper) |
| `internal/observability/pgxtracer.go` | pgx.QueryTracer implementation wired into pgxpool |
| `internal/middleware/logging.go` | Logger enrichment middleware |
| `internal/middleware/tracing.go` | OTel HTTP tracing middleware |
| `internal/llm/metrics.go` | LLM Prometheus metrics |
| `docs/artifacts/.../llm-credential-design.md` | Per-tenant LLM credential design |
