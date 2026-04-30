# PRD: Hermes Agent Go 企业级分布式平台加固

> 日期: 2026-04-30 | 主责: tech-lead | 状态: draft | 阶段: intake（挑战会后修订版）

---

## 1. 背景

### 业务问题

Hermes Agent Go 当前处于 POC / Early SaaS 阶段（v0.6.0），已具备多租户 CRUD、API Key 认证、Agent 对话、Skills/Soul 自动化配置、基础审计和 WebUI 等能力。基于对全量代码（95+ 源文件、35 次数据库迁移、6 个 Store 接口、3 套 Docker Compose）的深度审读，识别出 **15 个核心差距**，分布在身份认证、数据隔离、水平扩展、可观测性、LLM 韧性、合规治理、工程质量 7 个能力域。

### 触发原因

- 企业内部大规模部署要求：多团队 / 多 BU 共用平台，单集群 1000+ 租户
- 生产安全红线：当前无 RLS、无 OIDC、默认凭证硬编码、审计不完整
- SRE 可运维性：OTel 已实现但未接入、Prometheus 指标会撑爆、无熔断器
- 合规准入：GDPR 删除不覆盖 MinIO、无数据导出 API、审计日志可被同事务删除

### 当前约束

- v0.7.0 已有一份 production hardening plan（GDPR 级联删除、SSE 基础格式、审计增强、RBAC 粒度、JSON 日志、迁移锁、secrets 初步治理），本 PRD 为 **v0.8.0 ~ v1.1.0 的全景路线**，承接 v0.7.0 之后的所有治理工作
- 代码库为 Go 单体，依赖 PostgreSQL 16 + Redis 7 + MinIO + LLM API（OpenAI 兼容）
- 当前团队规模有限，需按 phase 分批交付，不追求一次性全部解决
- 不涉及前端 WebUI 变更（WebUI 由独立 Vue 3 项目维护）

---

## 2. Requirement Challenge Session Log

### 挑战 1: RLS 时机过早 (Architect)

- **质疑**: Phase 1 同时做 OIDC + RBAC + RLS + 无状态化，四个高风险改造并行。v0.7.0 已决定"RLS 移入 v0.8 backlog"，原因是 pgxpool 连接池变量泄漏风险未验证。
- **替代路径**: RLS 解耦到 Phase 3，与 GDPR 全链路同期交付；Phase 1 改用 `go-sql-tenant-enforcement` 静态分析 + 集成测试覆盖所有 WHERE tenant_id 路径作为过渡防线。
- **结论**: 接受替代路径 — RLS 移入 Phase 3，Phase 1 做租户 SQL 静态分析覆盖。
- **处理**: Phase 1 新增"租户 SQL 强制执行"slice；RLS 移入 Phase 3。

### 挑战 2: 无状态化范围过大 (Architect)

- **质疑**: 四个组件（agentCache / soulCache / ApprovalQueue / PairingStore）状态性质完全不同，外置难度差异大。全部放在 Phase 1 风险不可控。
- **替代路径**: Phase 1 只做 soulCache TTL/LRU + PairingStore 持久化 + agentCache 对齐 per-request 模式（API 路径已经如此，Gateway 路径对齐）；ApprovalQueue 推迟到有明确多副本 Gateway 需求时再做。
- **结论**: 接受拆分 — ApprovalQueue 分布式化推迟到 Phase 5。
- **处理**: Phase 1 无状态化 scope 缩减为三项；ApprovalQueue 移入 Phase 5。

### 挑战 3: OIDC 非硬需求 (Product Manager)

- **质疑**: OIDC Provider 依赖企业 IT（待确认项 Q1），如果不就绪则 Phase 1 被阻塞。早期企业部署可能只需 API Key + JWT 认证。
- **替代路径**: Phase 1 做 RBAC 细粒度 + JWT extractor 激活 + Secrets 治理；OIDC 作为 Phase 4 可选项，当企业 IdP 就绪后解锁。
- **结论**: 接受降级 — OIDC 移入 Phase 4，Phase 1 不被外部依赖阻塞。
- **处理**: Phase 1 移除 OIDC；Phase 4 新增 OIDC 作为"基础设施就绪后解锁"项。

### 挑战 4: Lifecycle manager 依赖倒挂 (DevOps)

- **质疑**: Phase 4 的 lifecycle manager 修复 Runner.Stop() 未调用、裸 goroutine 等问题，但 Phase 2 引入 OTel/circuit breaker 时会新增更多后台组件。后者同样缺乏生命周期管理。
- **替代路径**: Lifecycle manager 提前到 Phase 2 第一个 slice，作为基础设施依赖。
- **结论**: 接受提前 — lifecycle manager 作为 Phase 2 首个交付。
- **处理**: US-4.4 从 Phase 4 移入 Phase 2 Slice 0。

### 挑战 5: 14-20 周过于乐观 (Product Manager)

- **质疑**: 8 个待确认项 + 6 个外部依赖，每个需 1-2 周协调。实际周期可能 20-30 周。
- **替代路径**: 定义 MVP 子集 — 只做不依赖外部基础设施的改造。依赖外部基础设施的项标记为"就绪后解锁"。
- **结论**: 接受 — 每个 Phase 拆为"内部可控"和"外部解锁"两层。
- **处理**: 各 Phase 标注哪些 slice 依赖外部基础设施；定义 MVP = 全部"内部可控" slice。

### 挑战 6: 真实 SSE 不应放最后 (QA)

- **质疑**: 真实 SSE 是用户体验最直接可感知的改善，当前伪流式首 token 延迟等于完整推理时间。放在 v1.1.0 太晚。
- **替代路径**: 提前到 Phase 2，与 LLM 韧性同 Phase（streaming + circuit breaker 有交互关系）。
- **结论**: 接受提前 — 真实 SSE 移入 Phase 2。
- **处理**: US-5.1 从 Phase 5 移入 Phase 2。

### 挑战 7: Store 补全优先级低估 (Architect)

- **质疑**: Phase 1-3 每个改造都触碰 Store 层。如果 memories/user_profiles 仍是 raw SQL，Phase 3 RLS 无法统一生效，GDPR 删除覆盖有漏洞。
- **替代路径**: Store 接口补全作为 Phase 1 前置 slice。
- **结论**: 接受提前 — Store 补全作为 Phase 1 Slice 0。
- **处理**: US-4.2 从 Phase 4 移入 Phase 1 Slice 0。

---

## 3. 目标与成功标准

### 业务目标

将 Hermes Agent Go 从 POC 提升至**企业级分布式 Agent 平台**，支持：
- 1000+ 租户 / 10,000+ 并发会话的稳定运行
- 企业 IdP（OIDC/SAML）接入与细粒度权限控制
- 全链路可观测（指标 + 追踪 + 日志）
- 安全合规（数据隔离、审计不可篡改、GDPR 全覆盖）
- 零停机水平扩展

### 用户价值

| 角色 | 价值 |
|------|------|
| 平台运维 | 可观测、可扩缩、可回滚、告警驱动运维 |
| 安全团队 | RLS 纵深防御、审计不可篡改、凭证轮换 |
| 租户管理员 | SSO 登录、细粒度角色权限、数据导出 |
| 终端用户 | 真实流式响应、低延迟、高可用 |
| 合规官 | GDPR 全链路删除证据、审计日志独立存储 |

### 成功指标

| 指标 | 当前值 | 目标值 |
|------|--------|--------|
| 水平扩展能力 | 1 Pod（有状态） | N Pod（无状态） |
| 租户数据隔离 | 应用层 WHERE | 应用层 + PG RLS |
| 认证方式 | 静态 Token + API Key | JWT + API Key（OIDC 可选） |
| 角色模型 | user / admin | RBAC with scopes（5+ 角色） |
| P95 首 token 延迟 | 全量推理时间（伪流式） | < 2s（真实 SSE） |
| OTel 链路覆盖 | 0%（no-op tracer） | 100% HTTP + LLM + Tool |
| Prometheus 指标基数 | 无界（路径含 ID） | 受控（< 500 label 组合） |
| GDPR 删除覆盖 | PG only | PG + MinIO + 审计证据 |
| LLM 熔断保护 | 无 | 全模型 circuit breaker |
| Secrets 管理 | 硬编码 | K8s Secrets / Vault |

---

## 4. 调整后的 Phase 路线

### 总览

| Phase | 版本 | 核心交付 | 预计周期 | 外部依赖 |
|-------|------|---------|---------|---------|
| 1 内部安全基线 | v0.8.0 | Store 补全 + RBAC + JWT + Secrets + 部分无状态化 + 租户 SQL 强制 | 3-4 周 | 无 |
| 2 韧性与体验 | v0.9.0 | Lifecycle manager + OTel + 指标治理 + 熔断 + 真实 SSE + 分布式限流 | 4-5 周 | OTel Collector（可降级） |
| 3 隔离与合规 | v0.9.5 | PG RLS + GDPR 全链路 + 审计增强 + 数据导出 | 3-4 周 | 无 |
| 4 基础设施与扩展认证 | v1.0.0 | 迁移工具 + HA + Helm + OIDC（就绪后解锁） | 3-4 周 | OIDC Provider, PG HA, Redis Cluster |
| 5 规模化工程质量 | v1.1.0 | 会话/记忆治理 + Skills 扩展 + ApprovalQueue 分布式 | 2-3 周 | 无 |

**MVP 定义**: Phase 1 + Phase 2 + Phase 3 全部"内部可控"slice = **v0.9.5-MVP**，不依赖任何外部基础设施团队，预计 10-13 周。

---

## 5. 用户故事（按调整后 Phase 排序）

### Phase 1: 内部安全基线（v0.8.0）— 无外部依赖

**S0: Store 接口补全（前置 slice）**

> 作为后端工程师，我希望所有数据库表都通过 Store 接口访问，为后续 RLS / GDPR / 无状态化提供统一抽象基础。

验收标准：
- [ ] 新增 `MemoryStore` 接口，覆盖 `memories` 表的 CRUD + 按租户/用户查询
- [ ] 新增 `UserProfileStore` 接口，覆盖 `user_profiles` 表
- [ ] 新增 `CronJobStore` 接口，覆盖 `cron_jobs` 表
- [ ] `memory_pg.go` 中 raw SQL 迁移到 `MemoryStore` 实现
- [ ] 消除 `saas.go` 中对 `*pg.PGStore` 的类型断言（通过接口方法暴露 `Pool()`，或引入 `PoolProvider` 接口）
- [ ] 所有现有测试继续通过

**S1: RBAC 细粒度权限**

> 作为平台管理员，我希望定义 viewer / operator / admin / billing 等角色，并按 resource + action 粒度授权。

验收标准：
- [ ] 角色定义存储在数据库（`roles` 表 + `role_permissions` 表），支持 CRUD
- [ ] 权限规则支持 resource（`sessions`、`tenants`、`skills`、`memories`、`audit_logs`）+ action（`read`、`write`、`delete`）组合
- [ ] admin 角色保持全权限兼容
- [ ] API Key 创建时可绑定角色
- [ ] RBAC 规则变更无需重启 — 启动时加载 + 定时刷新（TTL 5 分钟）或 cache invalidation
- [ ] 向后兼容：现有 `user` / `admin` 二元角色自动映射到新模型

**S2: JWT 认证激活 + 认证加固**

> 作为企业用户，我希望通过 JWT Bearer Token 认证访问 Hermes API，而非仅依赖静态 Token。

验收标准：
- [ ] 取消注释 JWT extractor，配置为可选启用（环境变量控制）
- [ ] JWT 验证支持 JWKS endpoint 自动发现（`AUTH_JWKS_URL`）
- [ ] 支持 `id_token` 中的 `roles` / `groups` claim 映射到 Hermes 角色
- [ ] JWT 与 API Key extractor 在 ExtractorChain 中共存
- [ ] 静态 Token 用户标记为 `system:acp-admin`（审计可区分）
- [ ] Session ID 改用 `crypto/rand` 生成（替代 `UnixNano`）

**S3: Secrets 治理**

> 作为 SRE，我希望所有凭证不再硬编码在 docker-compose 中。

验收标准：
- [ ] docker-compose 文件中所有密码/token 改为 `.env` 文件引用
- [ ] 提供 `.env.example` 模板（已有，需补全缺失项）
- [ ] Redis 启用 `requirepass`
- [ ] CORS `SAAS_ALLOWED_ORIGINS` 默认值改为空（强制配置），不再默认 `*`
- [ ] MinIO 镜像固定版本 tag（替代 `latest`）
- [ ] 文档说明 K8s Secrets / Vault 集成方式（不强制实现，提供接入点）

**S4: 部分无状态化**

> 作为 SRE，我希望核心缓存有 TTL 和上限，Pod 重启不丢失关键状态。

验收标准：
- [ ] `soulCache` 加 TTL（30 分钟保留）+ LRU 上限（最多 500 条目或 32MB）
- [ ] `PairingStore` 持久化到 PG（新增 `pairings` 表 或复用 `users` 表）
- [ ] Gateway `agentCache` 对齐 API 路径的 per-request 模式：每次请求从 PG 加载历史构建 agent，不再进程内缓存 agent 实例
- [ ] agentCache 改造后，Gateway 路径支持多副本部署（不同 Pod 处理同一用户请求结果一致）

**S5: 租户 SQL 强制执行（RLS 过渡防线）**

> 作为安全工程师，我希望在 RLS 上线前，有机制保证所有数据库查询都包含 tenant_id 条件。

验收标准：
- [ ] 编写集成测试：覆盖所有 `pg/*.go` 中含 tenant_id 列的表的查询方法，验证均传入 tenant_id 参数
- [ ] 编写静态分析检查（go vet 自定义 analyzer 或测试脚本）：扫描 SQL 字符串，检测对 tenant-scoped 表的查询是否包含 `tenant_id` 条件
- [ ] 所有新增查询方法必须在 PR 中通过此检查
- [ ] 文档记录当前所有 tenant-scoped 表和查询路径清单

### Phase 2: 韧性与体验（v0.9.0）

**S0: 统一 Lifecycle Manager（前置 slice）**

> 作为 SRE，我希望所有后台组件有统一的生命周期管理，SIGTERM 后有序关闭。

验收标准：
- [ ] 引入 `errgroup` 或自研 lifecycle manager 统一管理所有后台 goroutine
- [ ] shutdown 路径调用 `runner.Stop()`
- [ ] `SyncAllTenants` goroutine 接受 shutdown context，可被取消
- [ ] tenant cleanup job 在 shutdown 时等待当前 purge 完成（grace period 可配置，默认 30s）
- [ ] in-flight HTTP 请求等待完成（grace period 可配置，默认 15s）
- [ ] 所有后续 Phase 新增组件必须通过 lifecycle manager 注册

**S1: OTel 全链路追踪**

> 作为 SRE，我希望每个请求从 HTTP 入口到 LLM 调用到 Tool 执行都有完整 trace。

验收标准：
- [ ] `InitTracer()` 在 `saas.go` 启动时调用（已有实现，仅需接入）
- [ ] 采样率可配置（`OTEL_TRACE_SAMPLE_RATE`，默认 0.01 即 1%）
- [ ] HTTP middleware 创建 root span（已有 `TracingMiddleware`，需激活）
- [ ] LLM 调用创建 child span，携带 `llm.model`、`llm.tokens.input`、`llm.tokens.output`、`llm.latency_ms` 属性
- [ ] Tool 执行创建 child span，携带 `tool.name`、`tool.duration_ms`、`tool.success` 属性
- [ ] 响应头 `X-Trace-ID` 返回有效 trace ID
- [ ] **降级方案**: 若 OTel Collector 不可用，降级为 no-op tracer + 日志告警，不影响请求处理

**S2: Prometheus 指标治理**

> 作为 SRE，我希望 Prometheus 指标不会因高基数 label 导致内存爆炸。

验收标准：
- [ ] `normalizePath` 将 UUID / 数字 ID 参数化为 `:id` 占位符（正则匹配 UUID 格式和纯数字路径段）
- [ ] 新增 LLM 指标：`hermes_llm_requests_total`（model, status）、`hermes_llm_request_duration_seconds`（model）、`hermes_llm_tokens_total`（model, type=input|output）、`hermes_llm_fallback_total`（from_model, to_model）
- [ ] 新增 Gateway 指标：`hermes_gateway_messages_total`（platform）、`hermes_gateway_active_sessions`（platform）
- [ ] 新增 Skills 指标：`hermes_skills_sync_duration_seconds`、`hermes_skills_sync_errors_total`
- [ ] 定义基数预算文档：label 组合总数 < 500

**S3: LLM 熔断与降级**

> 作为平台用户，当 LLM 服务降级时，我希望快速收到降级提示而非等待超时。

验收标准：
- [ ] 引入 `gobreaker` 封装 LLM 调用，per-model 独立 breaker
- [ ] 熔断阈值可配置：连续失败次数、错误率百分比、半开探测间隔
- [ ] Fallback 模型链加入指数退避 + 随机抖动（base 1s, max 30s）
- [ ] `RunConversation` 增加总超时（可配置，默认 120s），超时后返回部分结果 + 超时提示
- [ ] Tool 执行传播请求 context（替代 `context.Background()`），客户端断开时取消
- [ ] 熔断状态变化写入日志 + 触发 Prometheus 指标

**S4: 真实 SSE 流式响应**

> 作为终端用户，我希望 Agent 回复逐 token 流出，而非等待完整回复再模拟打字。

验收标准：
- [ ] 对接 LLM streaming API（OpenAI `stream: true`）
- [ ] SSE 事件格式兼容 OpenAI（`data: {...}\n\n`、`data: [DONE]\n\n`）
- [ ] 心跳每 15s 发送 `:keepalive` 注释行
- [ ] Tool 调用阶段内部缓冲，tool 结果到达后继续流式输出
- [ ] 流式中途 LLM 错误发送 `event: error` + JSON payload
- [ ] 与 S3 circuit breaker 集成：熔断时 SSE 返回降级事件而非挂起
- [ ] P95 首 token 延迟 < 2s（排除 LLM 冷启动）

**S5: 分布式限流接入**

> 作为平台运维，我希望限流在多 Pod 间全局生效。

验收标准：
- [ ] `saas.go` 启动时正确注入 Redis 限流器（`rediscache.NewRateLimiter`）
- [ ] 租户限流配置加本地缓存（TTL 5 分钟），减少 PG 查询
- [ ] 支持 per-tenant / per-user 两维限流（per-model 维度预留接口）
- [ ] `Retry-After` 头返回实际剩余窗口秒数（替代硬编码 60s）
- [ ] **降级方案**: Redis 不可用时降级为进程内限流 + 日志告警

### Phase 3: 隔离与合规（v0.9.5）— 无外部依赖

**S1: PostgreSQL Row-Level Security**

> 作为安全工程师，我希望数据库层面存在租户隔离的最后防线。

验收标准：
- [ ] 所有含 `tenant_id` 的表启用 RLS policy
- [ ] 连接池获取连接后执行 `SET app.current_tenant = $1`，AfterRelease hook 执行 `RESET ALL`
- [ ] 编写集成测试：模拟连接归还后残留变量场景，验证 RESET 生效
- [ ] 绕过 RLS 的运维连接使用独立 PG 角色（`hermes_admin`），不走应用连接池
- [ ] 迁移脚本中 RLS policy 支持 up/down（依赖 Phase 4 迁移工具，此处先用 IF EXISTS 保证幂等）
- [ ] Phase 1 的 tenant SQL 强制执行测试继续通过（双重防线）
- [ ] MinIO 隔离策略：维持应用层 prefix 隔离 + 为每个租户设置 bucket policy（deny cross-prefix access）

**S2: GDPR 全链路删除**

> 作为合规官，我希望租户删除时所有存储后端的数据都被清除，且有不可篡改的审计证据。

验收标准：
- [ ] `purgeTenant` 扩展到删除 MinIO 对象（soul/`{tenantID}/`、skills/`{tenantID}/`、manifest）
- [ ] 大租户数据批量删除（batch 1000 rows per DELETE），避免长事务锁竞争
- [ ] Redis 中该租户相关缓存 key 清除（`rl:{tenantID}:*` 等）
- [ ] purge 操作写入**独立审计表** `purge_audit_logs`（不在同事务中删除，不受业务 audit_logs 清理影响）
- [ ] purge 完成后向 `purge_audit_logs` 写入终态记录，包含删除行数、MinIO 对象数、耗时

**S3: 审计增强**

> 作为安全审计员，我希望所有 HTTP 请求都有审计记录，敏感字段不泄漏。

验收标准：
- [ ] AuditMiddleware 覆盖未认证请求（ac == nil 时仍记录，tenant_id 和 user_id 为空）
- [ ] 请求 Body 中的 `password`、`secret`、`token`、`api_key`、`authorization` 字段脱敏
- [ ] Body 脱敏只在 audit 记录路径执行，不影响实际请求处理
- [ ] 静态 Token 用户在审计日志中标记为 `system:acp-admin`
- [ ] 审计日志保留期限可配置（环境变量 `AUDIT_RETENTION_DAYS`，默认 90 天）

**S4: 数据导出 API**

> 作为租户用户，我希望可以导出我的所有数据。

验收标准：
- [ ] `GET /v1/gdpr/export` 返回 JSON 档案，包含 sessions、messages、memories、user_profiles
- [ ] 导出限 per-tenant scope，RBAC 限制为 admin 或 operator 角色
- [ ] 大数据量时支持分页导出或异步导出（返回 202 + 轮询 URL）
- [ ] 导出记录写入审计日志

### Phase 4: 基础设施与扩展认证（v1.0.0）

**内部可控项:**

**S1: 迁移工具升级**

> 作为 DBA，我希望数据库迁移支持回滚，且并发安全。

验收标准：
- [ ] 引入 `golang-migrate` 替代自研迁移系统
- [ ] 编写适配脚本：将现有 `schema_version` 表映射到 golang-migrate 格式
- [ ] 所有新迁移支持 up/down
- [ ] 现有 35 次迁移保持 forward-only 兼容（不补 down），新迁移起始编号 36+
- [ ] `schema_version` 表加 UNIQUE(version) 约束
- [ ] 关键字段加 CHECK 约束：`tenants.plan`、`users.role`、`sessions.end_reason`

**S2: 基础设施生产化**

> 作为 SRE，我希望基础设施具备高可用、可复现、可审计的特性。

验收标准：
- [ ] 提供 `values.production.yaml` 示例（PG 主从、Redis Sentinel、MinIO 分布式）
- [ ] Docker 镜像加 resource limits（`mem_limit`、`cpus`）
- [ ] 提供 Helm Chart 骨架（hermes-saas chart）
- [ ] 健康检查增加 MinIO 连通性（readiness probe）
- [ ] 健康检查增加 Redis 连通性（readiness probe）
- [ ] 日志聚合方案文档（Loki sidecar 或 Fluentd 配置示例）

**外部依赖解锁项:**

**S3: OIDC 认证接入（需要企业 IdP 就绪）**

> 作为企业 IT 管理员，我希望通过公司的 OIDC Provider 登录 Hermes 平台。

验收标准：
- [ ] 支持 OIDC Authorization Code Flow + PKCE
- [ ] Token 验证支持 JWKS 自动发现（复用 Phase 1 JWT extractor 基础）
- [ ] 用户首次登录自动创建/关联 Hermes 用户记录
- [ ] 支持 `id_token` 中的 `groups` claim 映射到 RBAC 角色
- [ ] 提供 Keycloak / Azure AD / Okta 三种 Provider 的配置示例

**S4: 高可用部署（需要基础设施团队就绪）**

> 作为 SRE，我希望核心依赖具备 HA 能力。

验收标准：
- [ ] PostgreSQL 主从复制配置文档 + Helm values
- [ ] PgBouncer 连接池配置（替代应用层直连，减少 PG 连接数）
- [ ] Redis Sentinel 配置文档 + Helm values
- [ ] HPA 基于 `hermes_http_requests_in_flight` 自动扩缩
- [ ] Pod Disruption Budget 配置

### Phase 5: 规模化工程质量（v1.1.0）— 无外部依赖

**S1: 会话与记忆治理**

> 作为后端工程师，我希望记忆系统可扩展，会话安全。

验收标准：
- [ ] `ReadMemory()` 加分页上限（最多 50 条或 8KB），超出部分按时间截断
- [ ] 记忆在长对话中支持增量刷新（mid-conversation reload，每 N 轮或显式触发）
- [ ] Memory Provider 接口预留向量检索扩展点（`SearchSimilar(ctx, query, topK)` 方法签名）
- [ ] 文档记录向量数据库集成路径（pgvector / Qdrant / Weaviate）

**S2: Skills 系统扩展**

> 作为平台运维，我希望 Skills 同步支持 1000+ 租户且不阻塞启动。

验收标准：
- [ ] `SyncAllTenants` 改为分页查询（去掉 1000 硬编码上限）
- [ ] 并发同步（可配置并发数，默认 10，使用 semaphore 控制）
- [ ] Skill 名称冲突优先级策略：MinIO > Local，同源按版本时间戳取最新
- [ ] `bundledDir` 使用绝对路径配置（环境变量 `HERMES_SKILLS_DIR`），不依赖 CWD
- [ ] 同步失败降级为日志告警，不阻塞启动

**S3: Gateway ApprovalQueue 分布式化**

> 作为 SRE，我希望 Gateway 多副本部署时，tool approval 请求能路由到正确的 agent。

验收标准：
- [ ] ApprovalQueue 从内存 channel 改为 Redis Pub/Sub
- [ ] approve/deny 命令通过 Redis 路由到持有 agent 的 Pod
- [ ] 超时机制：approval 请求 5 分钟未响应自动 deny
- [ ] 单 Pod 部署时降级为内存 channel（零 Redis 依赖）

---

## 6. 范围

### In Scope

| Phase | 版本 | 内部可控 | 外部解锁 | 预计周期 |
|-------|------|---------|---------|---------|
| 1 | v0.8.0 | Store 补全 + RBAC + JWT + Secrets + 部分无状态化 + 租户 SQL 强制 | — | 3-4 周 |
| 2 | v0.9.0 | Lifecycle + OTel + 指标 + 熔断 + 真实 SSE + 限流 | OTel Collector | 4-5 周 |
| 3 | v0.9.5 | RLS + GDPR 全链路 + 审计增强 + 数据导出 | — | 3-4 周 |
| 4 | v1.0.0 | 迁移工具 + 基础设施模板 | OIDC Provider + PG HA + Redis Cluster | 3-4 周 |
| 5 | v1.1.0 | 会话/记忆治理 + Skills 扩展 + ApprovalQueue | — | 2-3 周 |

**MVP（v0.9.5-MVP）= Phase 1 + 2 + 3 内部可控项 = 10-13 周**

### Out of Scope

- WebUI（Vue 3 前端）功能迭代
- LLM 模型训练 / 微调
- Multi-region 部署方案
- 计费系统实现（仅预留 billing 角色）
- 向量数据库集成（仅预留接口）
- 平台 CLI 工具（hermes admin CLI）
- 移动端 SDK

---

## 7. 关键假设

| # | 假设 | 验证方式 | 失败影响 |
|---|------|---------|---------|
| A1 | pgxpool 连接归还时 `RESET ALL` 可靠清除 `app.current_tenant` | Phase 3 前编写集成测试 | 需改用 per-statement SET 子查询 |
| A2 | Redis 已部署且可用于限流和缓存 | 检查基础设施 | Phase 2 限流降级为进程内 |
| A3 | v0.7.0 的 GDPR 级联删除和 SSE 基础格式已交付 | 检查 git log | Phase 2/3 需合并遗留项 |
| A4 | Gateway per-request agent 模式对延迟影响可接受 | 基准测试 | 需引入 Redis session cache |
| A5 | 目标规模 1000 租户 / 10,000 并发在 v1.0 周期内 | 与产品确认 | 可延后部分高扩展需求 |
| A6 | golang-migrate 可兼容现有 35 次自研迁移 | 编写适配脚本 POC | 需自研 down migration 支持 |

---

## 8. 风险与依赖

| # | 风险 | 影响 | 缓解措施 | Owner |
|---|------|------|---------|-------|
| R1 | pgxpool RLS 变量泄漏 | 跨租户数据泄漏 | Phase 1 租户 SQL 强制执行作为过渡防线；Phase 3 前完成集成测试验证 | architect |
| R2 | Gateway per-request agent 延迟回归 | 用户体验下降 | 基准测试；必要时引入 Redis 会话缓存层 | backend-engineer |
| R3 | OTel 采样影响延迟 | P99 上升 | 默认 1% 采样；async exporter | backend-engineer |
| R4 | Circuit breaker 误判 | 健康 LLM 被熔断 | per-model 独立 breaker；半开探测；阈值可配 | backend-engineer |
| R5 | 迁移工具切换兼容性 | 迁移失败 | 适配脚本 POC 先行；现有迁移不补 down | backend-engineer |
| R6 | 审计日志独立存储增加运维复杂度 | 运维负担 | 初期用同库独立表 `purge_audit_logs`，后续再考虑独立存储 | devops-engineer |
| R7 | 真实 SSE + Circuit Breaker 交互复杂 | 流式中途熔断处理 | 定义 SSE error event 格式；流式中途熔断发送 error 事件后关闭连接 | backend-engineer |

### 关键依赖

| 依赖 | 提供方 | 阻塞 Phase | 状态 |
|------|--------|-----------|------|
| OIDC Provider | 企业 IT | Phase 4 S3 | 待确认 |
| PG HA (主从) | 基础设施团队 | Phase 4 S4 | 待确认 |
| Redis Cluster / Sentinel | 基础设施团队 | Phase 4 S4 | 待确认 |
| OTel Collector + 后端 | SRE 团队 | Phase 2 S1（可降级） | 待确认 |

---

## 9. 待确认项

| # | 问题 | 决策影响 | 需要谁确认 |
|---|------|---------|-----------|
| Q1 | v0.7.0 production hardening 是否已全部交付？ | Phase 排序 | tech-lead |
| Q2 | Gateway per-request agent 对延迟的影响是否可接受？需要基准测试 | Phase 1 S4 方案选择 | backend-engineer |
| Q3 | 审计日志保留期限要求？是否需要归档到冷存储？ | Phase 3 存储设计 | 合规团队 |
| Q4 | 目标企业是否已有 OIDC Provider？ | Phase 4 S3 何时解锁 | product-manager |
| Q5 | 是否有明确的性能 SLO（P99 < Xms、可用性 > 99.9%）？ | Phase 2 指标定义 | SRE + product-manager |
| Q6 | Skills 是否需要版本化管理（同一 skill 多版本共存）？ | Phase 5 complexity | product-manager |

---

## 10. 企业治理待确认项

| # | 维度 | 当前状态 | 待确认 |
|---|------|---------|--------|
| G1 | 应用等级 | 未定义 | 按业务重要性判定 T1-T4 |
| G2 | 技术架构等级 | 未定义 | 决定是否需要跨 AZ / 跨地域 |
| G3 | 数据分类 | 含对话内容、用户记忆 | 确认 PII / 敏感数据 / 跨境传输 |
| G4 | 集团组件约束 | 无（独立项目） | 如部署到集团环境需确认 |
| G5 | 合规框架 | GDPR 基础 | 确认 SOC 2 / ISO 27001 / 等保 |

---

## 11. 领域技能包启用建议

| 技能 | 适用 Phase | 理由 |
|------|-----------|------|
| `golang-patterns` | All | context 传播、errgroup、interface 设计 |
| `golang-testing` | All | 表驱动测试、并发测试、集成测试 |
| `postgres-patterns` | Phase 1, 3, 4 | RLS、迁移、索引 |
| `go-sql-tenant-enforcement` | Phase 1 | 租户 SQL 隔离强制执行 |
| `security-review` | Phase 1, 3 | 认证、审计安全审查 |
| `api-design` | Phase 1, 3 | RBAC API、GDPR API |
| `docker-patterns` | Phase 4 | compose 生产化、Helm |
| `deployment-patterns` | Phase 4 | K8s 资源配额、HPA |

---

## 12. 参与角色清单

| 角色 | 职责 | 输入缺口 |
|------|------|---------|
| tech-lead | 全局优先级仲裁、phase 排序 | 无 |
| product-manager | 业务优先级确认 | 企业需求反馈、增长预期 |
| architect | RLS 方案、无状态化架构 | pgxpool 集成测试结论 |
| backend-engineer | 全部 Phase 实现 | Gateway 基准测试数据 |
| security-reviewer | 认证/RBAC/审计安全审查 | 合规框架确认 |
| qa-engineer | 测试计划、GDPR 验证 | 性能 SLO 目标 |
| devops-engineer | Helm、HA、OTel 后端、日志 | 基础设施团队 SLA |

---

## 附录 A: 与 v0.7.0 Production Hardening 的关系

| v0.7.0 项 | 本 PRD 对应 | 关系 |
|-----------|------------|------|
| S1.1 GDPR 级联删除 | Phase 3 S2 | v0.7.0 做 PG 层；本 PRD 扩展到 MinIO + 审计证据 |
| S1.2 SSE 基础格式 | Phase 2 S4 | v0.7.0 做格式；本 PRD 做真实 token 级流式 |
| S2.1 审计失败认证 | Phase 3 S3 | v0.7.0 做 auth 失败审计；本 PRD 扩展到全请求 + Body 脱敏 |
| S2.2 RBAC 粒度 | Phase 1 S1 | v0.7.0 做 method+path；本 PRD 做 resource+action+DB 驱动 |
| S3.1 JSON 日志 | — | v0.7.0 完成，不重复 |
| S3.2 迁移 advisory lock | Phase 4 S1 | v0.7.0 做锁；本 PRD 做迁移工具替换 + up/down |
| S3.3 Secrets 初步治理 | Phase 1 S3 | v0.7.0 移除硬编码；本 PRD 补全 Redis auth + CORS + 版本固定 |

**原则**: v0.7.0 先交付，本 PRD 各 Phase 增量演进。

## 附录 B: 挑战会调整对照表

| # | 原方案 | 挑战结论 | 调整后 |
|---|--------|---------|--------|
| C1 | RLS 在 Phase 1 | Phase 1 scope 过大 + pgxpool 未验证 | RLS 移入 Phase 3；Phase 1 做租户 SQL 静态分析 |
| C2 | 四组件同时无状态化 | 难度差异大，ApprovalQueue 无紧迫需求 | 拆分：Phase 1 做三项；ApprovalQueue 移入 Phase 5 |
| C3 | OIDC 在 Phase 1 | 依赖外部 IdP，可能阻塞 | OIDC 移入 Phase 4 外部解锁项；Phase 1 做 JWT + RBAC |
| C4 | Lifecycle manager 在 Phase 4 | Phase 2 新组件缺乏生命周期管理 | 提前到 Phase 2 Slice 0 |
| C5 | 14-20 周串行 | 外部依赖不可控 | 定义 MVP = Phase 1+2+3 内部可控项（10-13 周） |
| C6 | 真实 SSE 在 Phase 5 | 用户体验最直接改善，不应放最后 | 移入 Phase 2（与 LLM 韧性同期） |
| C7 | Store 补全在 Phase 4 | Phase 1-3 均依赖 Store 接口 | 提前到 Phase 1 Slice 0 |
