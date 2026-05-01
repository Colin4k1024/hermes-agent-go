# 企业级平台加固总结

> Hermes Agent Go 企业级分布式平台加固全链路完成记录。
> 起始版本：v0.7.0 | 完成版本：v1.0.0 | 完成时间：2026-05-01

## 目标

将 Hermes Agent Go 从 POC / Early SaaS 提升至**企业级分布式 Agent 平台**，满足：

- 1000+ 租户 / 10,000+ 并发会话稳定运行
- 企业 IdP（OIDC）接入与细粒度权限控制
- 全链路可观测（指标 + 追踪 + 日志）
- 安全合规（数据隔离、审计不可篡改、GDPR 全覆盖）
- 零停机水平扩展

## Phase 1 — 基础加固（v0.8.0）

**Commit**: `15ad5bc`

| Slice | 内容 | 说明 |
|-------|------|------|
| S0 | Store 接口补全 | memories / user_profiles 统一到 Store 接口 |
| S1 | RBAC 细粒度权限 | method+path 组合控制，`admin` / `user` 角色矩阵 |
| S2 | Auth Chain 完善 | Static Token → API Key → JWT 链式认证 |
| S3 | Secrets 治理 | 移除所有硬编码默认凭证，env 隔离 |
| S4 | 无状态化 | soulCache TTL/LRU + PairingStore 持久化 |
| S5 | 租户 SQL 强制 | `go-sql-tenant-enforcement` 静态分析 + 集成测试覆盖所有 WHERE tenant_id 路径 |

**Requirement Challenge 决策**：
- RLS 推迟到 Phase 3（pgxpool 变量泄漏风险待验证）
- OIDC 推迟到 Phase 4（依赖企业 IdP 就绪）
- Store 补全从 Phase 4 提前到 Phase 1 S0

## Phase 2 — 可观测性与韧性（v0.9.0）

**Commits**: `8858721` → `7930cd6`

| Slice | 内容 | 说明 |
|-------|------|------|
| S0 | Lifecycle Manager | `saas.go` 重构为有序关闭，15s grace period |
| S1 | OTel Tracing 激活 | `InitTracer` 接入 `StackConfig`，W3C Trace Context |
| S2 | Prometheus 基数修复 | `normalizePath`：UUID/数字/Hex/SessID → `:id` |
| S3 | Redis 分布式限流 | `rediscache.Client` 实现 `middleware.RateLimiter` 接口 |
| S4 | 断路器 | `gobreaker v2.4.0` + `ResilientTransport` 装饰器 + `HERMES_CIRCUIT_BREAKER_DISABLED` 逃生开关 |
| S5 | 真实 SSE | 替换伪流式（首 token 延迟 = 完整推理时间 → 逐 token 流出）|
| S6 | 对话超时 | `HERMES_CONVERSATION_TIMEOUT`，默认 120s |

**Requirement Challenge 决策**：
- Lifecycle Manager 从 Phase 4 提前到 Phase 2 S0（OTel/断路器新增后台组件依赖）
- 真实 SSE 从 v1.1.0 提前到 Phase 2（用户体验关键路径）

## Phase 3 — RLS 与 GDPR 全链路（v0.9.5）

**Commit**: `cf1d11b`

| Slice | 内容 | 说明 |
|-------|------|------|
| S1 | Row-Level Security | 所有 9 张业务表启用 RLS，`app.tenant_id` 上下文隔离 |
| S2 | GDPR 全链路 | 级联删除覆盖 MinIO 对象存储（soul + skills）|
| S3 | 审计日志增强 | 新增 `request_id` / `status_code` / `latency_ms` 字段（迁移 v24~v27）|

## Phase 4+5 — Schema 治理与运维增强（v1.0.0）

**Commit**: `9ebc0ce`

| Slice | 内容 | 说明 |
|-------|------|------|
| 4-S1 | Migration 工具升级 | 使用 `PL/pgSQL DO` 块保证幂等，IF NOT EXISTS 防重复执行 |
| 4-S2 | Schema 治理约束 | `session_key` 非空唯一、`api_keys.key_hash` 唯一、`audit_logs` 不可修改触发器 |
| 4-S3 | 健康探针完善 | `/health/ready` 扩展检查 Database + MinIO + Redis（任一不可用返回 503）|
| 5-S1 | 记忆限制 | `max_memories` per tenant 配额，超限返回 429 |
| 5-S2 | Skills 并发同步 | `SyncAllTenants` 升级为分页 + 并发 goroutine provisioning |

## Phase 5-S3 — 审批队列治理（v1.0.0 patch）

**Commits**: `86e0f3b` → `a15922f`

| 内容 | 说明 |
|------|------|
| 超时调整 | `DefaultApprovalTimeout`: 60s → 5 分钟（支持人工审批场景）|
| 时间戳 | `pendingApproval` 增加 `CreatedAt` 字段 |
| Stale Reaper | 后台 goroutine 定期清理超时审批请求 |
| 可观测性 | Prometheus counter 记录 approval 事件 |

## 架构演进总览

```
v0.7.0 基线
  ├── 认证：Static Token → API Key（链式，未激活 JWT）
  ├── RBAC：基础路径前缀
  ├── 限流：本地 LRU only
  ├── 可观测：Prometheus（高基数问题），OTel 未接入
  ├── 隔离：应用层 WHERE tenant_id（无 RLS）
  └── LLM：直接 HTTP，无断路器

v1.0.0 当前
  ├── 认证：完整三段链（Static → API Key → JWT），时序攻击防护
  ├── RBAC：method+path 细粒度，admin 超级权限
  ├── 限流：Redis 分布式 + 本地 LRU 降级，租户维度
  ├── 可观测：OTel + W3C Trace Context，Prometheus 低基数，结构化审计日志
  ├── 隔离：应用层 WHERE + RLS 纵深防御
  ├── LLM：断路器（gobreaker v2.4.0）+ 可配置超时
  ├── SSE：真实流式响应
  ├── 生命周期：有序关闭 Lifecycle Manager
  └── Schema：治理约束 + 审计不可篡改
```

## 关键决策记录

| 决策 | 结论 | 原因 |
|------|------|------|
| RLS 时机 | Phase 1 → Phase 3 | pgxpool 变量泄漏风险需先验证 |
| OIDC 接入 | Phase 4（可选解锁）| 依赖企业 IdP，不阻塞核心交付 |
| 真实 SSE 时机 | v1.1 → Phase 2 | 用户体验关键路径，首 token 延迟影响感知 |
| Lifecycle Manager 时机 | Phase 4 → Phase 2 S0 | OTel/断路器引入后台组件，需先建基础设施 |
| ApprovalQueue 分布式化 | 推迟（Phase 5+）| 当前无多副本 Gateway 需求 |
| 审批超时 | 60s → 5 分钟 | 人工审批需要合理操作窗口 |

## 遗留项（下一阶段）

| 项目 | 优先级 | 触发条件 |
|------|--------|----------|
| Agent Chat 接入真实 LLM | 高 | 产品决策：切换 mock → 真实 LLM |
| OIDC 接入 | 中 | 企业 IdP 就绪（JWT extractor 已预置）|
| ApprovalQueue 分布式化 | 中 | 多副本 Gateway 上线 |
| WebUI 功能增强 | 中 | 独立 Vue 3 项目版本节奏 |

## 相关文档

- [架构概览](architecture.md) — 中间件栈、Store 设计
- [API 参考](api-reference.md) — 完整端点文档
- [认证系统](authentication.md) — Auth Chain 与 RBAC
- [数据库](database.md) — Schema 与迁移版本
- [可观测性](observability.md) — 指标与追踪
- [配置指南](configuration.md) — 环境变量
- [部署指南](deployment.md) — Docker / Helm / Kind
