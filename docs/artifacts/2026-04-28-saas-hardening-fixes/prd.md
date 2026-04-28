# PRD: SaaS Hardening — Concurrency / Isolation / Observability

## Metadata

| Field | Value |
|-------|-------|
| Slug | `saas-hardening-fixes` |
| Date | 2026-04-28 |
| Status | Draft |
| Owner | tech-lead |
| Source | `docs/saas-hardening-plan.md` |
| State | intake |

---

## 背景

hermes-agent-go 已完成 SaaS 无状态化重构（Phase 0-5），基础架构层就绪（PG+Redis+AgentFactory），
但 SaaS 就绪度审计显示三个维度存在系统性缺口：

1. **并发安全 (2.5/5)**: `sync.Once` 重赋值导致数据竞争，rate limiter 无界增长为 DoS 向量。
2. **数据隔离 (4.0/5)**: API Key 查询和撤销缺少 tenant_id 过滤，存在跨租户越权风险。
3. **企业可观测性 (1.5/5)**: 零分布式追踪、LLM 调用无日志、310 个 slog 调用仅 9 个含上下文标识。

这些问题在多租户生产负载下将导致数据腐败、安全越权和运维盲区。

## 目标与成功标准

| 目标 | 成功标准 |
|------|----------|
| 消除并发竞争 | `go test -race ./internal/config/... ./internal/tools/... ./internal/middleware/...` 全部通过 |
| 修复租户隔离 | GetByID/Revoke 接口签名强制 tenant_id；单元测试覆盖跨租户拒绝 |
| 企业可观测性 | OpenTelemetry 集成、LLM 调用结构化日志、Prometheus 指标可按租户查询 |
| SaaS 就绪度 | 从 ~3.9/5 提升至 ~4.5/5 |

## 用户故事与验收标准

### US-1: 并发安全 — Config Reload Race [CRITICAL]

**As** 运维工程师  
**I want** 配置热加载不会导致并发读写腐败  
**So that** 多 goroutine 下 globalConfig 始终一致

**验收标准**:
- `sync.Once` 重赋值模式全部移除，替换为 `sync.RWMutex`
- `go test -race` 含并发 Load/Reload 测试用例通过
- 涉及文件: `internal/config/config.go:145-170`

### US-2: 并发安全 — Profile Override Race [CRITICAL]

**As** 开发者  
**I want** `SetActiveProfile`/`OverrideActiveProfile` 线程安全  
**So that** CLI --profile 标志不会导致竞争条件

**验收标准**:
- `activeProfileOnce`、`profileOverride`、`activeProfile` 全部受 mutex 保护
- 零 `sync.Once` 重赋值
- 涉及文件: `internal/config/profiles.go`

### US-3: 并发安全 — Memory Function Pointer [HIGH]

**As** 系统  
**I want** `SetMemoryProviderNameFunc` 写入受锁保护  
**So that** 函数指针读写不竞争

**验收标准**:
- `getMemoryProviderName` 赋值受 `activeProviderMu` 保护或使用 `atomic.Pointer`
- `go test -race` 通过
- 涉及文件: `internal/tools/memory.go:73-81`

### US-4: 并发安全 — Rate Limiter Bucket Eviction [HIGH]

**As** SRE  
**I want** rate limiter 内存可控  
**So that** 恶意 IP 轮转不会导致 OOM

**验收标准**:
- 周期性 GC 或 LRU 策略，bucket 数量在 10K unique keys 后稳定
- 最大 bucket 数量安全阀
- 涉及文件: `internal/middleware/ratelimit.go:80-112`

### US-5: 数据隔离 — API Key Tenant Scoping [HIGH]

**As** 租户 A  
**I want** 无法通过 ID 查看或撤销租户 B 的 API key  
**So that** 多租户数据严格隔离

**验收标准**:
- `GetByID(ctx, tenantID, id)` — 接口签名强制 tenant_id
- `Revoke(ctx, tenantID, id)` — SQL 含 `WHERE tenant_id = $1 AND id = $2`
- `store.APIKeyStore` 接口同步更新
- 单元测试: 跨租户查询/撤销返回 not found
- 涉及文件: `internal/store/pg/apikey.go`, `internal/store/store.go`, `internal/api/apikeys.go`

### US-6: 数据隔离 — LLM Credential Isolation (Design Only) [MEDIUM]

**As** 平台运营  
**I want** per-tenant LLM credential 的 schema 设计就绪  
**So that** 后续可快速实现租户级计费

**验收标准**:
- `tenants` 表增加 `llm_api_key` 加密列的 migration 设计文档
- AgentFactory 检查 tenant config 的设计方案
- **本期仅设计，不实现**

### US-7: 可观测性 — OpenTelemetry Integration [HIGH]

**As** SRE  
**I want** 全链路分布式追踪  
**So that** 可以从 HTTP 请求追踪到 PG 查询和 LLM 调用

**验收标准**:
- `internal/observability/tracer.go` 初始化 TracerProvider (OTLP exporter)
- HTTP middleware 创建 root span，注入 trace_id/span_id
- 响应头 `X-Trace-ID`
- 涉及文件: 新建 `internal/observability/`, 修改 `internal/middleware/chain.go`

### US-8: 可观测性 — LLM Call Logging + Metrics [HIGH]

**As** SRE  
**I want** 每次 LLM 调用有结构化日志和 Prometheus 指标  
**So that** 可监控模型延迟、token 消耗和成本

**验收标准**:
- 每次 LLM 调用产生 slog.Info 含 trace_id/tenant_id/session_id/model/tokens/latency/cost/error
- Prometheus: `hermes_llm_request_duration_seconds{model,tenant_id}`, `hermes_llm_tokens_total{model,tenant_id,direction}`
- 涉及文件: `internal/llm/client.go`, `internal/agent/agent.go`

### US-9: 可观测性 — Structured Logging Enrichment [MODERATE]

**As** 开发者  
**I want** 日志自动携带 request_id/tenant_id/trace_id  
**So that** 可按请求 ID 搜索完整生命周期

**验收标准**:
- `internal/observability/logger.go` 提供 `ContextLogger(ctx)`
- 所有 slog.Error/Warn 调用含 request_id + tenant_id
- 涉及文件: 新建 `internal/observability/logger.go`, 修改高优先级日志调用点

### US-10: 可观测性 — Database Query Observability [MODERATE]

**As** DBA  
**I want** PG 查询耗时可监控，慢查询自动告警  
**So that** 数据库问题可快速定位

**验收标准**:
- pgx tracer hook 记录查询耗时，>500ms 的 WARN 级别
- Prometheus: `hermes_pg_query_duration_seconds{operation}`
- pgxpool stats 暴露到 /metrics

### US-11: 可观测性 — Audit Trail Enhancement [MODERATE]

**As** 合规审计员  
**I want** 审计日志包含 request_id、status_code、latency  
**So that** 可完整还原请求处理过程

**验收标准**:
- `audit_logs` 表增加 `request_id`, `session_id`, `status_code`, `latency_ms` 列
- Audit middleware 使用 ResponseWriter wrapper 捕获 status code
- 涉及文件: `internal/middleware/audit.go`, `internal/store/pg/auditlog.go`, `internal/store/pg/migrate.go`

### US-12: 可观测性 — Prometheus Tenant Labels [LOW]

**As** SRE  
**I want** HTTP 指标可按 tenant_id 过滤  
**So that** Grafana 看板可监控单租户表现

**验收标准**:
- `hermes_http_requests_total` 和 `hermes_http_request_duration_seconds` 增加 `tenant_id` label
- 未认证请求使用 `"anonymous"`
- 涉及文件: `internal/middleware/metrics.go`

---

## 范围

### In Scope

| Phase | Items | 级别 |
|-------|-------|------|
| P1 并发安全 | 1.1, 1.2 (CRITICAL), 1.3, 1.4, 1.5 (HIGH) | 5 项 |
| P2 数据隔离 | 2.1 (HIGH), 2.2 (MEDIUM), 2.3 设计 (MEDIUM) | 3 项 |
| P3 可观测性 | 3.1, 3.2 (HIGH), 3.3, 3.4, 3.5 (MODERATE), 3.6 (LOW) | 6 项 |

### Out of Scope

- Per-tenant LLM 计费集成（需外部计费系统）
- 多区域部署 / 数据驻留
- 自动密钥轮转
- 运行时日志级别动态调整
- PostgreSQL Row-Level Security (RLS)

---

## 风险与依赖

| 风险 | 影响 | 缓解 |
|------|------|------|
| OpenTelemetry 引入新依赖，go.mod 膨胀 | 构建时间增加 | 仅引入核心 SDK + OTLP exporter |
| Prometheus tenant_id label 高基数 | 指标存储爆炸 | 考虑 bounded tenant set 或 exemplars |
| audit_logs ALTER TABLE 在大表上锁表 | 写入短暂阻塞 | 使用 `ADD COLUMN ... DEFAULT NULL`（PG 11+ 瞬间完成） |
| `sync.RWMutex` 替换 `sync.Once` 性能回退 | 热路径读延迟微增 | RWMutex 读锁竞争极低，可忽略 |
| LLM Credential 加密方案选型 | 安全依赖 | 本期仅设计，不落地实现 |

## 待确认项

| # | 问题 | 建议 | 决策人 |
|---|------|------|--------|
| 1 | OpenTelemetry exporter 默认用 OTLP gRPC 还是 HTTP？ | 默认 gRPC，env var 可切换 | tech-lead |
| 2 | Rate limiter GC 间隔和 max bucket 上限？ | 5min GC、10K 上限 | tech-lead |
| 3 | LLM credential 加密用 AES-GCM 还是借助 PG pgcrypto？ | 设计阶段对比，不实现 | architect |
| 4 | 结构化日志迁移范围 — 全量还是仅 Error/Warn？ | 优先 Error/Warn，后续渐进 | tech-lead |
