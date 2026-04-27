# PRD: Hermes Agent Go — SaaS 无状态化重构

| 字段 | 值 |
|------|-----|
| Slug | `saas-stateless` |
| 日期 | 2026-04-27 |
| 主责 | tech-lead |
| 状态 | draft |
| 阶段 | intake |
| 设计文档 | `saas-stateless-redesign.md` |

---

## 背景

hermes-agent-go 当前为本地单体架构，所有状态（Agent 实例、会话、审批队列、SSE 事件、Cron 调度等）持有在进程内存或本地 SQLite 文件中。这导致：

1. **无法水平扩展**：多实例部署会导致状态不一致
2. **无多租户隔离**：不支持 SaaS 多客户共享部署
3. **存在并发缺陷**：8 个已识别的并发问题（2 Critical、2 High、4 Medium/Low）

**触发原因：** 项目需要从开发者工具向 SaaS 产品演进，支持多租户、水平扩展和生产级部署。

**设计文档来源：** `saas-stateless-redesign.md` 已完成完整设计，包含目标架构、表结构、Redis Key 规范、并发修复方案、部署架构和 6 Phase 里程碑。

---

## 目标与成功标准

### 业务目标
- 实现 SaaS 多租户部署能力
- 支持 Gateway/ACP/Agent 实例水平扩展（3+ 实例无状态对等）
- 修复全部已知并发缺陷
- 保留本地单机模式作为开发/降级方案

### 成功标准
- 3 个 Gateway 实例同时运行，会话可在任意实例间无感切换
- 多租户数据隔离：PostgreSQL RLS + Redis key 前缀，无跨租户泄漏
- 并发修复：8 个已知问题全部修复并有对应测试
- P95 消息处理延迟 < 500ms（不含 LLM 调用时间）
- 所有已有测试继续通过

---

## 关键假设

1. **PostgreSQL 为主数据库**：替代 SQLite，承载 sessions/messages/tenants/users/audit_logs/cron_jobs
2. **Redis 为分布式状态层**：会话锁、审批队列、SSE 事件、Rate Limit、Pairing 缓存
3. **Agent 每次请求无状态**：从 PG+Redis 加载上下文 → 执行 → 持久化结果，不持有跨请求状态
4. **Cron Scheduler 保持单实例**：通过 Redis SETNX leader election 保证不重复执行
5. **Tool Registry 只读**：全局只读单例，不需要外置
6. **本地降级兼容**：当 PG/Redis 不可用时，回退到 SQLite + 内存模式

---

## 最小可行范围 (MVP)

基于设计文档的 6 Phase，MVP 定义为 **Phase 0 + Phase 1 + Phase 2**：

1. **Phase 0**：并发 Bug 修复（8 个已知问题）
2. **Phase 1**：状态外置基础设施（PG Schema + StateClient + Redis + Rate Limit + Tenant middleware）
3. **Phase 2**：Gateway 无状态化（AIAgentFactory + SessionStore PG 化 + PairingStore PG 化）

MVP 完成后，系统可以多实例部署但仅支持单租户。Phase 3+ 增加多租户和生产化能力。

---

## 非目标

- 不做 FTS5 → Elasticsearch 迁移（Phase 1 先用 PG `tsvector`，后续独立评估）
- 不做 SSO/OIDC 对接（Phase 3 范围，MVP 不含）
- 不做 Kubernetes Helm Chart（Phase 5 范围）
- 不做 Web Dashboard 的 SaaS 化改造（本次只改后端）
- 不做 Skills/Plugins 的多租户隔离（保持全局共享）

---

## 差异分析：11 个有状态组件

| # | 状态组件 | 当前位置 | 目标存储 | Phase | 优先级 |
|---|----------|---------|---------|-------|--------|
| 1 | AIAgent 实例缓存 | `runner.go` 内存 map | 移除，改为无状态工厂 | P2 | P0 |
| 2 | SessionStore | `session.go` 内存 map | PostgreSQL `sessions` 表 | P2 | P0 |
| 3 | ACP SessionStore | `acp/session.go` 内存 map | PostgreSQL `sessions` 表 | P2 | P0 |
| 4 | ApprovalQueue | `approval.go` 内存 channel | Redis List + BRPOP | P3 | P1 |
| 5 | EventBroker (SSE) | `events.go` 内存 map | Redis Pub/Sub | P3 | P1 |
| 6 | MediaCache | `media_cache.go` 内存/磁盘 | OSS/S3 | P4 | P2 |
| 7 | PairingStore | `pairing.go` 内存 map | PostgreSQL + Redis 缓存 | P2 | P0 |
| 8 | SQLite state.db | `state/db.go` 本地文件 | PostgreSQL | P1 | P0 |
| 9 | RuntimeStatus | `status.go` 内存计数器 | Redis Hash | P2 | P1 |
| 10 | Cron Jobs | `cron/jobs.go` 内存 map | PostgreSQL + Redis leader lock | P4 | P2 |
| 11 | Tool Registry | `registry.go` 全局单例 | 保持（只读） | - | - |

---

## 并发缺陷清单（Phase 0）

| # | 严重度 | 文件 | 问题 | 修复方案 |
|---|--------|------|------|---------|
| C-1 | Critical | `agent.go:391-394` | `isInterrupted()` 用写锁查 bool | `atomic.Bool` 替代 |
| C-2 | Critical | `agent.go:386-388` | `Interrupt()` 用写锁写 bool | `atomic.Bool` 替代 |
| C-3 | High | `agent.go:442-460` | 并行 tool 无 WaitGroup 超时保护 | `sync.WaitGroup` + `context.WithTimeout(5min)` |
| C-4 | High | `approval.go:272-281` | `ClearSession` 对已关闭 channel 写 → panic | `select` 保护 + 标记关闭 |
| C-5 | Medium | `approval.go:206-216` | `Submit` 持锁时 make channel | 提前 pool 化 |
| C-6 | Medium | `events.go:78-84` | SSE Publish 静默丢事件 | 增加 metrics 计数 + 日志 |
| C-7 | Low | `state/db.go:100+` | 有 FTS5 但无全文搜索 API 暴露 | 暴露搜索接口 |
| C-8 | Low | `approval.go:139-142` | `IsApproved` map 并发写非安全 | `sync.Map` 替代 |

---

## 用户故事

### US-1: 无状态多实例部署
- 作为运维工程师，我希望部署 3+ Gateway 实例并通过 Load Balancer 分流，会话在任意实例上正确处理
- 验收标准：Kill 任意实例后，进行中的会话在其他实例上自动恢复

### US-2: 多租户隔离
- 作为 SaaS 管理员，我希望为每个客户创建独立 tenant，数据完全隔离
- 验收标准：Tenant A 无法查询到 Tenant B 的 sessions/messages

### US-3: 并发安全
- 作为用户，我希望在高并发场景下系统不崩溃
- 验收标准：并发发送 100 条消息，无 panic、无数据丢失

### US-4: 分布式审批
- 作为消息平台用户，我希望审批请求在多实例间正确传递
- 验收标准：在 Instance A 提交的审批，可以在 Instance B 上响应

### US-5: 本地开发兼容
- 作为开发者，我希望不配置 PG/Redis 时仍可本地开发
- 验收标准：`hermes` 命令无外部依赖时自动降级到 SQLite + 内存模式

---

## 范围

### In Scope（按 Phase）

| Phase | 内容 | 预估周期 |
|-------|------|----------|
| **Phase 0** | 并发 Bug 修复（8 项）| 1 周 |
| **Phase 1** | 状态外置基础设施（PG Schema + StateClient + Redis + Rate Limit + Tenant middleware）| 2-3 周 |
| **Phase 2** | Gateway 无状态化（AIAgentFactory + SessionStore/PairingStore PG 化 + RuntimeStatus Redis 化）| 3-4 周 |
| **Phase 3** | 多租户 + SaaS 功能（Tenant CRUD + 配额系统 + 审计日志 + Approval Redis 化 + SSE Pub/Sub 化）| 3-4 周 |
| **Phase 4** | Cron + 工具系统适配（CronJob PG 化 + 分布式调度 + Tool Result OSS 化 + MediaCache OSS 化）| 2 周 |
| **Phase 5** | 生产化（Helm Chart + Prometheus + Grafana + HPA + 高可用测试 + 灰度发布）| 2-3 周 |

### Out of Scope
- FTS5 → Elasticsearch 迁移
- SSO/OIDC 对接
- Skills/Plugins 多租户隔离
- Web Dashboard SaaS 化
- 前端 UI 改造

---

## 风险与依赖

| 风险 | 影响 | 缓解 |
|------|------|------|
| LLM 延迟：每次请求从 PG 加载完整历史 | 延迟增加 50-200ms | 热点会话 Redis 缓存 (TTL 5min) |
| Redis 单点故障 | 审批队列不可用 | Redis Sentinel/Cluster，审批超时降级 deny |
| 多租户数据泄漏 | 严重合规问题 | PostgreSQL RLS + 所有查询 tenant_id 过滤 + 代码 review 检查项 |
| 会话中断：路由到不同实例 | Agent 上下文丢失 | Context Summary + Session History 从 StateClient 完整恢复 |
| 架构改动大，回归风险高 | 现有功能回归 | Phase 0 先修并发，每个 Phase 独立可测试 |
| PG 迁移脚本与现有 SQLite 数据兼容 | 数据丢失 | 编写 SQLite → PG 迁移工具，双写过渡期 |

---

## 待确认项（已确认 2026-04-27）

| # | 问题 | 决策 | 影响 |
|---|------|------|------|
| 1 | Session 并发锁 | **允许并发，加 Redis 分布式锁** | P1-P2：`SETNX session-lock:{tenant}:{session}` TTL 30s |
| 2 | 历史消息窗口 | **50 条 + 强制压缩** | P2：超过 50 条自动触发 compression，只加载最近 50 条 + summary |
| 3 | Tool 大结果上传 | **异步上传** | P4：本地 temp 缓存 + 后台 goroutine 上传 OSS |
| 4 | Redis Pub/Sub SSE 延迟 | **OK，需 benchmark 验证** | P3：验证 < 100ms 即可接受 |
| 5 | 多租户 MCP | **MVP 共享，后续按需隔离** | P3+：所有 tenant 共享 MCP 服务实例 |
| 6 | PG 选型 | **开发 Docker PG，生产托管** | P1：Docker Compose 内置 PG 16 |
| 7 | StateStore interface | **确认：PG + SQLite 双实现** | P1：定义 `StateStore` interface，`pgStore` + `sqliteStore` 各一个 |

---

## 技术栈新增

| 组件 | 选型 | 用途 |
|------|------|------|
| PostgreSQL 16 | 主数据库 | sessions/messages/tenants/users/audit/cron |
| Redis 7 | 分布式状态 | 锁/队列/缓存/Pub-Sub/Rate Limit |
| `github.com/jackc/pgx/v5` | Go PG driver | 高性能 PG 连接池 |
| `github.com/redis/go-redis/v9` | Go Redis client | Redis 连接 |
| `github.com/golang-migrate/migrate` | DB 迁移 | Schema 版本管理 |
| Docker Compose | 开发环境 | PG + Redis + 多实例 Gateway |
