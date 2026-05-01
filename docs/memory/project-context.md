# Project Context: hermes-agent-go

| 字段 | 值 |
|------|-----|
| 项目名 | hermes-agent-go |
| 当前版本 | v1.0.0 |
| 阶段 | **企业级平台加固全链路完成** |
| 更新时间 | 2026-05-01 |
| 最新 commit | `a15922f` (fix: gofmt formatting for enterprise hardening files) |

---

## Tech Stack

### 后端
- Go 1.25.9
- PostgreSQL 16+ (primary store, pgx/v5, 27 个迁移已应用)
- SQLite (local dev store, noop for SaaS features)
- Redis (分布式限流，本地 LRU 降级)
- MinIO (per-tenant skills + soul 对象存储)
- OpenTelemetry (otel SDK + OTLP gRPC exporter, Batch 导出)
- Prometheus (client_golang v1.23.2)
- hashicorp/golang-lru/v2 v2.0.7 (LRU rate limiter 本地降级)
- golang-jwt/jwt/v5 (JWT RS256，已激活 extractor)
- sony/gobreaker v2.4.0 (LLM 断路器)
- Helm chart (Kubernetes)
- Docker Compose (local dev / quickstart / webui)

### 前端 (webui/ 子目录)
- Vue 3 + TypeScript + Vite
- Naive UI (组件库)
- Pinia (状态管理)
- Vue Router (hash mode)
- Nginx (生产静态服务 + API 反代)

## 已完成的里程碑

### v0.7.0 (2026-04-28) — SaaS Production Hardening
- GDPR 级联删除统一治理
- SSE 流式响应（基础版）
- 审计日志增强（AUTH_FAILED 事件）
- RBAC 粒度增强（method+path 组合）
- JSON 结构化日志
- DB 迁移 advisory lock
- Secrets 管理（移除硬编码默认值）

### v0.8.0 (2026-04-30) — Phase 1: 基础加固
- Store 接口补全（memories / user_profiles）
- RBAC 细粒度权限矩阵
- Auth Chain 完善（Static → API Key → JWT 链式认证）
- Secrets 治理（env 隔离）
- 无状态化（soulCache TTL/LRU、PairingStore 持久化）
- 租户 SQL 强制执行（go-sql-tenant-enforcement 静态分析 + 集成测试）

### v0.9.0 (2026-04-30) — Phase 2: 可观测性与韧性
- Lifecycle Manager（有序关闭 15s grace period）
- OTel Tracing 激活（InitTracer + TracingMiddleware wired to StackConfig）
- Prometheus 基数修复（路径归一化 UUID/数字/Hex → `:id`）
- Redis 分布式限流（RateLimiter 接口 + Allow() 方法对接）
- 断路器（gobreaker v2.4.0 + ResilientTransport 装饰器）
- 真实 SSE 流式响应（替换伪流式实现）
- 对话可配置超时（`HERMES_CONVERSATION_TIMEOUT`，默认 120s）

### v0.9.5 (2026-04-30) — Phase 3: RLS 与 GDPR 全链路
- Row-Level Security（所有 9 张业务表启用 RLS）
- GDPR 全链路（级联删除覆盖 MinIO 对象存储）
- 审计日志增强（request_id / status_code / latency_ms 字段）

### v1.0.0 (2026-05-01) — Phase 4+5: Schema 治理与运维
- Migration 工具升级（幂等 PL/pgSQL DO 块）
- Schema 治理约束（unique/not-null/audit 不可篡改）
- 健康探针完善（Database + MinIO + Redis 多组件）
- 用户记忆限制（max_memories per tenant）
- Skills 并发同步（分页 + 并发 goroutine provisioning）
- 审批队列治理（5 分钟超时 + Stale Reaper + Prometheus counter）

## 当前架构关键特性

### 中间件栈（9 层，固定顺序）
```
Tracing → Metrics → RequestID → Auth → Tenant
→ Logging → Audit → RBAC → RateLimit → Handler
```

### 多租户隔离（纵深防御）
1. **凭证派生 TenantID**：永不接受请求头 tenant_id
2. **Store 层 WHERE tenant_id = $1**：应用层过滤
3. **RLS**：数据库层行级安全（v0.9.5+）
4. **FK 约束**：9 张表全部引用 `tenants(id)`

### Store 接口（9 个子接口）
Tenants / Sessions / Messages / Users / AuditLogs / APIKeys / CronJobs / Memories / UserProfiles

### LLM 集成
ResilientTransport（断路器）+ Transport 接口 + Provider 自动检测

## 数据库状态

- 27 个迁移已应用（v1~v27）
- 9 张业务表，全部 RLS 已启用
- Schema 治理约束（v57-v59 幂等 DO 块）：unique/not-null/audit-immutable

## 文档状态

| 文档 | 位置 | 状态 |
|------|------|------|
| 架构概览 | `docs/architecture.md` | ✅ 当前 |
| API 参考 | `docs/api-reference.md` | ✅ 当前 |
| 认证系统 | `docs/authentication.md` | ✅ 当前 |
| 配置指南 | `docs/configuration.md` | ✅ 当前 |
| 数据库 | `docs/database.md` | ✅ 当前 |
| 部署指南 | `docs/deployment.md` | ✅ 当前 |
| 可观测性 | `docs/observability.md` | ✅ 当前 |
| 快速开始 | `docs/saas-quickstart.md` | ✅ 当前 |
| Skills 指南 | `docs/skills-guide.md` | ✅ 当前 |
| 企业加固总结 | `docs/enterprise-hardening.md` | ✅ 当前（新建）|

## 已知遗留项（下一阶段候选）

| 项目 | 优先级 | 说明 |
|------|--------|------|
| Agent Chat 接入真实 LLM | 高 | 当前 chat handler 为 mock 实现 |
| OIDC 接入 | 中 | 等企业 IdP 就绪后解锁，JWT extractor 已预置 |
| ApprovalQueue 分布式化 | 中 | 多副本 Gateway 场景需求明确后 |
| WebUI 功能增强 | 中 | 独立 Vue 3 项目维护 |
| Batch RL Training 接入 | 低 | RL tools 当前为 stub |

## 依赖

- PostgreSQL 16+
- Redis 7（可选，缺失时本地 LRU 降级）
- MinIO（可选，缺失时 skills provisioning 跳过）
- LLM Provider（可选，chat handler 当前为 mock）
- Docker Compose（本地开发）
- Helm / Kind（K8s 部署）
