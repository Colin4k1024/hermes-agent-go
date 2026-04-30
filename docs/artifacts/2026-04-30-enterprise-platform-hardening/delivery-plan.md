# Delivery Plan: Hermes Agent Go 企业级分布式平台加固

> 日期: 2026-04-30 | 主责: tech-lead | 状态: handoff-ready | 阶段: plan

---

## 版本目标

将 hermes-agent-go 从 POC（v0.6.0）提升至企业级分布式 Agent 平台（v1.1.0），分 5 个 Phase 滚动交付。

**MVP 定义**: Phase 1 + 2 + 3（v0.9.5-MVP），零外部依赖，预计 10-13 周。

**放行标准**: 每 Phase 全部 Slice 测试通过，无 CRITICAL/HIGH issue，文档同步更新。

---

## Requirement Challenge Session Log

7 项挑战已在 PRD 中完成并落盘，核心结论：

| # | 原方案 | 调整后 |
|---|--------|--------|
| C1 | RLS 在 Phase 1 | → Phase 3（pgxpool 需先验证） |
| C2 | 四组件同时无状态化 | → 拆分，ApprovalQueue → Phase 5 |
| C3 | OIDC 在 Phase 1 | → Phase 4（外部依赖解锁） |
| C4 | Lifecycle manager 在 Phase 4 | → Phase 2 S0（前置） |
| C5 | 14-20 周串行 | → MVP 10-13 周，外部依赖独立解锁 |
| C6 | 真实 SSE 在 Phase 5 | → Phase 2（用户体验优先） |
| C7 | Store 补全在 Phase 4 | → Phase 1 S0（前置） |

---

## Story Slice 列表

### Phase 1: 内部安全基线（v0.8.0）— 3-4 周

| Slice | 目标 | 修改文件 | 新增文件 | 迁移 | 影响面 | 依赖 | Owner | 验收标准 |
|-------|------|---------|---------|------|--------|------|-------|---------|
| S0 Store 补全 | MemoryStore/UserProfileStore/CronJobStore 接口化 | store.go, types.go, pg.go, sqlite/noop.go | pg/memories.go, pg/userprofile.go, pg/cronjob.go | 0 | M | — | backend | (1) 3 个新 Store 接口定义 (2) PG 实现通过测试 (3) memory_pg.go raw SQL 消除 (4) saas.go 类型断言消除 |
| S1 RBAC 细粒度 | resource+action 权限模型 | migrate.go, types.go, store.go, pg.go, rbac.go, server.go, saas.go | pg/roles.go, api/roles.go(可选) | 2-3 | L | S0 | backend | (1) roles+role_permissions 表创建 (2) middleware 支持 permission check (3) admin bypass 保持 (4) 热加载 TTL 5min |
| S2 JWT 激活 | JWT Bearer Token 认证 + Session ID 安全 | auth/, saas.go, agent_chat.go | auth/extractor_jwt.go(若不存在) | 0 | S-M | — | backend | (1) JWT extractor 可选启用 (2) JWKS 自动发现 (3) Session ID crypto/rand (4) 静态 Token 标记 system:acp-admin |
| S3 Secrets 治理 | 移除硬编码凭证 | docker-compose*.yml, .env.example | — | 0 | S | — | devops | (1) compose 改 .env 引用 (2) Redis requirepass (3) CORS 非 * (4) MinIO 固定版本 |
| S4 无状态化(部分) | 缓存有界 + 状态持久化 | runner.go, chat_handler.go | agent/soul_cache.go, store/pg/pairing.go(可选) | 0-1 | L | S0 | backend | (1) soulCache LRU 500+TTL 30min (2) PairingStore 持久化到 users 表 (3) agentCache 加 LRU 200+TTL 5min |
| S5 租户SQL强制 | 所有查询 tenant_id 覆盖验证 | pg/*.go, Makefile | tests/tenant_isolation_test.go, scripts/check_tenant_sql.sh | 0 | M | — | qa+backend | (1) 集成测试覆盖全部 tenant-scoped 表 (2) FTS Search 含 tenant_id 验证 (3) CI 集成 |

**Phase 1 高风险点:**
- `memory_pg.go` 双写路径并存导致不一致（S0）
- `api_keys.roles[]` 字符串到 role_id 引用的兼容性迁移（S1）
- `MessageStore.Search()` GIN FTS 跨租户泄漏风险（S5）

---

### Phase 2: 韧性与体验（v0.9.0）— 4-5 周

| Slice | 目标 | 修改文件 | 新增文件 | 迁移 | 影响面 | 依赖 | Owner | 验收标准 |
|-------|------|---------|---------|------|--------|------|-------|---------|
| S0 Lifecycle Manager | 统一服务生命周期 | saas.go | internal/lifecycle/manager.go | 0 | S | — | backend | (1) errgroup 管理所有 goroutine (2) runner.Stop() 在 shutdown 调用 (3) SyncAllTenants 可取消 (4) LIFO shutdown 30s grace |
| S1 OTel 追踪 | 全链路 trace | saas.go, server.go, llm/client.go | — | 0 | S | S0 | backend | (1) InitTracer() 启动时调用 (2) HTTP/LLM/Tool span (3) 采样率可配 1% (4) X-Trace-ID 有效 (5) 无 Collector 时降级 no-op |
| S2 指标治理 | Prometheus 基数受控 | middleware/metrics.go | — | 0 | S | — | backend | (1) normalizePath UUID→:id (2) LLM 4 指标 (3) Gateway 2 指标 (4) Skills 2 指标 (5) 基数 <500 |
| S3 LLM 熔断 | circuit breaker + 总超时 | go.mod, llm/client.go, agent/agent.go | llm/breaker.go | 0 | M | S1 | backend | (1) gobreaker per-model (2) 阈值可配 (3) 指数退避+抖动 (4) RunConversation 总超时 120s (5) Tool context 传播 |
| S4 真实 SSE | token 级流式 | agent/agent.go, agent/types.go, api/agent_chat.go | — | 0 | L | S3 | backend | (1) LLM streaming API 对接 (2) OpenAI SSE 格式 (3) tool_start/tool_end 事件 (4) 心跳 15s (5) 熔断时 error 事件 (6) P95 首 token <2s |
| S5 分布式限流 | Redis 全局限流 | rediscache/redis.go, saas.go, middleware/ratelimit.go | — | 0 | M | — | backend | (1) Redis limiter 注入 (2) 租户配置缓存 TTL 5min (3) Retry-After 实际值 (4) Redis 不可用降级 |

**Phase 2 高风险点:**
- SSE + 熔断交互：流式中途熔断需发送 error event 后关闭连接（S4）
- `rediscache.CheckRateLimit` 签名与 `RateLimiter.Allow` 不匹配，需适配（S5）
- Prometheus `tenant_id` label 高基数需改用 exemplars 或聚合到 plan 维度（S2）

---

### Phase 3: 隔离与合规（v0.9.5）— 3-4 周

| Slice | 目标 | 修改文件 | 新增文件 | 迁移 | 影响面 | 依赖 | Owner | 验收标准 |
|-------|------|---------|---------|------|--------|------|-------|---------|
| S1 PG RLS | 数据库层租户隔离 | pg/migrate.go, pg/pg.go, 全部 pg/*.go | — | ~16 | L | P1-S5 | backend+dba | (1) 8 表 ENABLE RLS + FORCE (2) SET LOCAL + Transaction (3) AfterRelease RESET ALL (4) Admin pool bypass (5) 集成测试验证 |
| S2 GDPR 全链路 | PG+MinIO+Redis 全删除 | gdpr.go, tenant_cleanup.go | — | 0 | M | S1 | backend | (1) MinIO soul/skills 删除 (2) batch 1000 rows (3) Redis 缓存清除 (4) purge_audit_logs 独立记录 (5) 终态审计 |
| S3 审计增强 | 全请求审计+脱敏 | middleware/audit.go | — | 0 | S | — | backend | (1) 未认证请求审计 (2) Body 敏感字段脱敏 (3) 静态 Token 标记 system:acp-admin (4) 保留期可配 |
| S4 数据导出 API | 租户数据导出 | gdpr.go | — | 0 | S | S2 | backend | (1) GET /v1/gdpr/export 含全量数据 (2) RBAC admin/operator 限制 (3) 大数据量异步导出 (4) 审计记录 |

**Phase 3 高风险点:**
- `current_setting('app.current_tenant', true)` missing_ok 返回空字符串导致全拒绝（S1）
- MinIO 删除失败不应阻止 PG 删除，需两阶段（S2）
- 未认证请求审计导致 audit_logs 因攻击流量膨胀（S3）

---

### Phase 4: 基础设施与扩展认证（v1.0.0）— 3-4 周

| Slice | 目标 | 修改文件 | 新增文件 | 迁移 | 影响面 | 依赖 | Owner | 验收标准 |
|-------|------|---------|---------|------|--------|------|-------|---------|
| S1 迁移工具升级 | golang-migrate 替换自研 | pg/migrate.go, go.mod | migrations/*.sql × 35 | — | M | — | backend | (1) schema_version → golang-migrate 适配 (2) 新迁移 up/down (3) UNIQUE(version) (4) CHECK 约束 |
| S2 基础设施模板 | Helm + readiness + limits | deploy/helm/*, Dockerfile.* | — | 0 | S | — | devops | (1) Helm Chart 骨架 (2) MinIO readiness (3) Redis readiness (4) resource limits (5) 日志聚合文档 |
| S3 OIDC(外部解锁) | OIDC Authorization Code | auth/, saas.go | auth/oidc.go | 0 | M | 企业 IdP | backend | (1) PKCE flow (2) JWKS discovery (3) 自动用户创建 (4) groups→roles 映射 (5) 配置示例 |
| S4 HA(外部解锁) | 高可用部署 | deploy/helm/* | — | 0 | S | 基础设施团队 | devops | (1) PG 主从文档 (2) PgBouncer 配置 (3) Redis Sentinel (4) HPA (5) PDB |

---

### Phase 5: 规模化工程质量（v1.1.0）— 2-3 周

| Slice | 目标 | 修改文件 | 新增文件 | 迁移 | 影响面 | 依赖 | Owner | 验收标准 |
|-------|------|---------|---------|------|--------|------|-------|---------|
| S1 记忆治理 | ReadMemory 分页+刷新 | memory_pg.go, store.go, pg/memory.go | — | 0 | M | P1-S0 | backend | (1) ReadMemory limit 50/8KB (2) mid-conversation reload (3) 向量检索接口预留 |
| S2 Skills 扩展 | 分页+并发同步 | provisioner.go, loader_composite.go | — | 0 | M | — | backend | (1) 分页去掉 1000 上限 (2) 并发 10 (3) 名称冲突策略文档化 (4) HERMES_SKILLS_DIR 绝对路径 |
| S3 ApprovalQueue | 分布式审批 | tools/approval.go, runner.go | store/pg/approval.go | 1 | L | P1-S0 | backend | (1) Redis Pub/Sub 路由 (2) 5min 超时自动 deny (3) 单 Pod 降级内存 |

---

## 角色分工

| Phase | 主责 | 协作 |
|-------|------|------|
| Phase 1 | backend-engineer | architect(S0 Store 设计), qa-engineer(S5 租户隔离测试), devops(S3 Secrets) |
| Phase 2 | backend-engineer | architect(S4 SSE 架构), devops(S0 Lifecycle) |
| Phase 3 | backend-engineer | architect(S1 RLS), security-reviewer(全 Phase), dba(S1 迁移) |
| Phase 4 | devops-engineer | backend-engineer(S1 迁移工具, S3 OIDC) |
| Phase 5 | backend-engineer | — |

---

## 风险与依赖清单

### 风险

| # | 风险 | 概率 | 影响 | 缓解 | Owner |
|---|------|------|------|------|-------|
| R1 | pgxpool RLS 变量泄漏 | 中 | 严重 | SET LOCAL + AfterRelease 双保险；P1-S5 集成测试 | architect |
| R2 | agentCache 移除延迟回归 | 中 | 中 | P1 先 LRU 限制；基准测试评估 | backend |
| R3 | gobreaker 误触发 | 低 | 中 | 区分 timeout vs connection refused；per-model 独立 | backend |
| R4 | SSE + 熔断交互 | 中 | 中 | error event 格式 + 断开连接 | backend |
| R5 | 迁移工具切换兼容性 | 中 | 高 | 桥接脚本 POC 先行；现有迁移不补 down | backend |
| R6 | Prometheus 高基数 | 高 | 中 | exemplars 或 plan 维度聚合 | backend |
| R7 | 审计表攻击流量膨胀 | 中 | 中 | 未认证审计采样 per-IP per-min | backend |

### 外部依赖

| 依赖 | 阻塞 | 状态 | 降级方案 |
|------|------|------|---------|
| OIDC Provider | P4-S3 | 待确认 | JWT + API Key 足以覆盖 MVP |
| PG HA | P4-S4 | 待确认 | 单实例 + 定期备份 |
| Redis Cluster | P4-S4 | 待确认 | 单实例 Redis + AOF |
| OTel Collector | P2-S1 | 待确认 | 降级 no-op tracer |

---

## Implementation Readiness 结论

### 已满足

- [x] Requirement Challenge Session 完成（7 项结论）
- [x] 架构设计完成（arch-design.md）
- [x] 实现影响面分析完成（全部 20+ Slice）
- [x] 每个 Slice 有明确验收标准、依赖、Owner
- [x] 风险识别与缓解措施定义
- [x] MVP 与外部依赖解耦

### 执行前提

- [ ] Q1: v0.7.0 production hardening 交付状态确认
- [ ] Q2: Gateway per-request agent 基准测试（P1-S4 方案验证）
- [ ] Q5: 性能 SLO 目标定义（P2 指标基线）

### 结论

**Phase 1 可立即进入 `/team-execute`**（无前置阻塞）。Q1/Q2/Q5 不阻塞 Phase 1 启动，但需在 Phase 2 前确认。

---

## 技能装配清单

| 技能 | Phase | 触发原因 | 主责 |
|------|-------|---------|------|
| `golang-patterns` | All | context 传播、errgroup、interface 设计 | backend |
| `golang-testing` | All | 表驱动测试、并发测试 | backend+qa |
| `postgres-patterns` | P1,P3,P4 | RLS、迁移、CHECK 约束 | backend+dba |
| `go-sql-tenant-enforcement` | P1-S5 | 租户 SQL 隔离强制执行 | backend |
| `security-review` | P1,P3 | 认证、审计安全审查 | security |
| `api-design` | P1,P3 | RBAC API、GDPR API | backend |
| `docker-patterns` | P4 | Helm、compose 生产化 | devops |
| `deployment-patterns` | P4 | K8s HPA、PDB | devops |

---

## 检查节点

| 节点 | 时间点 | 检查内容 | 决策人 |
|------|--------|---------|--------|
| Phase 1 完成 | +3-4w | 全部 6 Slice 通过；基准测试完成 | tech-lead |
| Phase 2 中期 | +5-6w | SSE 真实流式 demo；熔断集成测试 | tech-lead + architect |
| MVP 放行 | +10-13w | Phase 1+2+3 全部通过；安全审查完成 | tech-lead + security |
| Phase 4 解锁评估 | MVP 后 | 外部依赖就绪状态；OIDC Provider 确认 | tech-lead + devops |
| v1.1.0 放行 | +15-20w | 全 Phase 通过；性能 SLO 达标 | tech-lead |

---

## 附录: 与 v0.7.0 的关系

本 Delivery Plan 承接 v0.7.0 production hardening 之后。v0.7.0 的 7 项工作（GDPR 级联、SSE 格式、审计、RBAC 粒度、JSON 日志、迁移锁、Secrets 初步）为本计划的基础。各 Phase Slice 在 v0.7.0 基础上增量演进，不重复实现。
