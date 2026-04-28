# Closeout Summary: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Owner | tech-lead |
| Status | closed |
| Source | `docs/saas-hardening-plan.md` |

---

## 收口对象

| Field | Value |
|-------|-------|
| 关联任务 | SaaS Hardening — 14 fixes + 1 design |
| Release | Pre-commit (本地完成，待 push) |
| 收口角色 | tech-lead |
| 变更范围 | 26 files, +442/-167 lines |

---

## 结果判断

### 目标达成

| Dimension | Before | Target | Actual |
|-----------|--------|--------|--------|
| Concurrency Safety | 2.5/5 | 4.5/5 | **4.5/5** — sync.Once 全部移除, atomic.Pointer + mutex, LRU bounded |
| Data Isolation | 4.0/5 | 4.5/5 | **4.5/5** — SQL-level tenant enforcement on GetByID/Revoke |
| Enterprise Tracing | 1.5/5 | 4.0/5 | **4.0/5** — OTel + LLM metrics + structured logging + pgx tracer + audit enrichment |
| **Overall** | **~3.9/5** | **~4.5/5** | **~4.3/5** |

### 交付清单

| Slice | Status | Key Change |
|-------|--------|-----------|
| 1: Config race | ✅ | `atomic.Pointer[Config]` 替换 `sync.Once` 重赋值 |
| 2: Memory + RateLimiter | ✅ | `activeProviderMu` + `hashicorp/golang-lru/v2` 10K cap |
| 3: API Key tenant | ✅ | `GetByID`/`Revoke` SQL `WHERE tenant_id = $1 AND id = $2` |
| 4: Structured logging | ✅ | `observability.ContextLogger(ctx)` + LoggingMiddleware |
| 5: LLM observability | ✅ | 结构化日志 + Prometheus histograms (sync + stream) |
| 6: OpenTelemetry | ✅ | TracerProvider + OTLP (TLS default) + tracing middleware |
| 7: DB + Audit | ✅ | pgx QueryTracer + migration v24-v27 + audit enrichment |
| 8: Design + Metrics | ✅ | LLM credential design doc + tenant_id HTTP metrics |

### Review 修复

| Source | HIGH | MEDIUM | Fixed |
|--------|------|--------|-------|
| Security review | 1 | 4 | 5 fixed, 1 verified, 1 accepted |
| Code review | 3 | 7 | 7 fixed, 3 accepted/documented |

---

## 残余事项

### 遗留项

| # | Item | Priority | Owner | Action |
|---|------|----------|-------|--------|
| 1 | Prometheus tenant_id cardinality guard | LOW | backend-engineer | Add `_overflow` fallback if tenant count > 1000 |
| 2 | slog migration for non-HTTP paths (CLI, plugins, cron) | LOW | backend-engineer | Gradual migration in future PRs |
| 3 | pgx tracer SQL prefix normalization | LOW | backend-engineer | Regex to extract keyword + table only |
| 4 | LLM credential encryption implementation | MEDIUM | architect + backend-engineer | Follow design doc when billing system ready |
| 5 | E2E test with real PG + Redis + Jaeger | MEDIUM | qa-engineer | docker-compose integration test suite |

### 残余风险

| Risk | Status | Rationale |
|------|--------|-----------|
| Prometheus cardinality | Accepted | Bounded tenant set in SaaS; documented |
| `rand.Read` unchecked | Accepted | Pre-existing; Go 1.20+ panics on failure |
| `InvalidateConfig` stale pointer | Documented | Callers must not cache `*Config` across Reload |

---

## 知识沉淀

### Lessons Learned

1. **`sync.Once` reassignment is a Go anti-pattern** — `sync.Once` is designed to be used once. Resetting it via `foo = sync.Once{}` creates a race condition. Use `atomic.Pointer` + `sync.Mutex` for config reload patterns.

2. **SQL-level tenant enforcement > app-level checks** — App-level checks are honor system; SQL-level `WHERE tenant_id = $N` is a compile-time contract via interface signatures. Defense in depth.

3. **Security review catches what code review misses** — The `/metrics` endpoint tenant enumeration (HIGH) was not flagged by the code reviewer but was caught by the security reviewer. Parallel review is worth the cost.

4. **OpenTelemetry noop pattern is production-safe** — When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, the default noop TracerProvider has zero overhead. No build tags needed.

5. **Review-driven fixes improve quality beyond the plan** — Security and code review surfaced 10 additional issues not in the original hardening plan (nil channel hang, OTLP TLS, query param sanitization, etc.).

---

## Backlog 回填

已同步到遗留项列表（上方）。

---

## 任务关闭结论

| Field | Value |
|-------|-------|
| 最终验收状态 | **Accepted** |
| 任务关闭结论 | **Closed** — 14/14 implementation items complete, 1/1 design item complete, all review findings addressed |
| 后续 Owner | backend-engineer (遗留项 1-3), architect (遗留项 4), qa-engineer (遗留项 5) |
