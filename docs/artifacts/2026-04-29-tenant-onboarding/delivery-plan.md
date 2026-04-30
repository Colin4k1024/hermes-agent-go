# Delivery Plan: 租户自动开通 — 记忆/画像/技能/Soul 全链路补齐

> 日期：2026-04-29 | Owner：tech-lead | 版本目标：v0.3.0

---

## 版本目标

**目标**：租户创建后自动具备完整 Agent 能力（记忆 + 画像 + 技能 + Soul），无需手工运维操作。

**范围**：SaaS API 模式下的租户 onboarding 全链路。

**放行标准**：
1. 创建租户后 MinIO 自动出现默认技能集和 Soul 文件
2. Chat 请求 system prompt 包含记忆、画像和 Soul 内容
3. 不同租户的记忆/技能/Soul 完全隔离
4. 技能同步不覆盖用户已修改的技能
5. 所有新增 API 通过 RBAC 保护

---

## 工作拆解

### Phase 1：基础设施层（无 API 变更）

| # | 工作项 | 主责 | 依赖 | 估时 |
|---|--------|------|------|------|
| 1.1 | `objstore/minio.go` 新增 `DeleteObject`、`ObjectExists` | backend | 无 | 0.5h |
| 1.2 | `store/pg/migrate.go` v28：`tenant_configs` 表（tenant_id UK, soul TEXT, config JSONB） | backend | 无 | 0.5h |
| 1.3 | `skills/provisioner.go` — `ProvisionTenantSkills(ctx, minioClient, tenantID, bundledDir)` | backend | 1.1 | 2h |
| 1.4 | `skills/provisioner.go` — `SyncAllTenantsSkills(ctx, minioClient, tenantStore, bundledDir)` | backend | 1.3 | 1.5h |
| 1.5 | `skills/provisioner.go` — `ProvisionTenantSoul(ctx, minioClient, tenantID, defaultSoul)` | backend | 1.1 | 0.5h |

### Phase 2：Chat Handler 增强

| # | 工作项 | 主责 | 依赖 | 估时 |
|---|--------|------|------|------|
| 2.1 | `api/mockchat.go` 重构：注入 `*pgxpool.Pool` 和 `*objstore.MinIOClient` 依赖 | backend | 无 | 1h |
| 2.2 | Chat handler：构建 `PGMemoryProvider`，调用 `SystemPromptBlock()` 注入记忆/画像 | backend | 2.1 | 1h |
| 2.3 | Chat handler：加载 tenant Soul（MinIO `{tenantID}/_soul/SOUL.md`）注入 system prompt | backend | 2.1, 1.5 | 1h |
| 2.4 | Chat handler：加载 tenant 技能列表，注入可用技能摘要到 system prompt | backend | 2.1, 1.3 | 1h |
| 2.5 | Chat handler：从请求 header 或 auth context 派生 `user_id`（非硬编码） | backend | 2.1 | 0.5h |

### Phase 3：租户 Provisioning Hook

| # | 工作项 | 主责 | 依赖 | 估时 |
|---|--------|------|------|------|
| 3.1 | `api/tenants.go` create：调用 `ProvisionTenantSkills` + `ProvisionTenantSoul`（异步，失败不阻塞） | backend | 1.3, 1.5 | 1h |
| 3.2 | `api/server.go`：`APIServerConfig` 新增 `MinIOClient` 和 `pgxpool.Pool` 字段 | backend | 无 | 0.5h |
| 3.3 | `cmd/hermes/saas.go`：启动时调用 `SyncAllTenantsSkills` | backend | 1.4 | 0.5h |

### Phase 4：技能管理 API

| # | 工作项 | 主责 | 依赖 | 估时 |
|---|--------|------|------|------|
| 4.1 | `api/skills.go` — `GET /v1/skills` 列出当前租户技能（名称、版本、来源、修改状态） | backend | 1.3 | 1h |
| 4.2 | `api/skills.go` — `PUT /v1/skills/{name}` 上传/更新技能 | backend | 4.1 | 1h |
| 4.3 | `api/skills.go` — `DELETE /v1/skills/{name}` 删除租户技能 | backend | 4.1 | 0.5h |
| 4.4 | `api/server.go`：注册 `/v1/skills` 路由 + RBAC 规则 | backend | 4.1 | 0.5h |

### Phase 5：测试与验证

| # | 工作项 | 主责 | 依赖 | 估时 |
|---|--------|------|------|------|
| 5.1 | 端到端测试脚本：租户创建 → 验证 MinIO 技能 → Chat 验证记忆/Soul → 跨租户隔离 | backend | P1-P4 | 2h |
| 5.2 | 技能同步测试：新增内置技能后重启 → 验证所有租户自动获得 | backend | 1.4 | 1h |
| 5.3 | 用户修改保护测试：修改租户技能 → 重启 → 验证未被覆盖 | backend | 1.4 | 0.5h |
| 5.4 | 文档更新：api-reference.md、skills-guide.md、configuration.md | docs | P1-P4 | 1h |

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 | Owner |
|------|------|----------|-------|
| MinIO 不可用时租户创建 | 高 — 无技能可用 | Provisioning 异步执行 + 返回 `provisioning_status` + 启动时全局 sync 兜底 | backend |
| 77 个技能全量复制到每个租户 | 中 — MinIO 存储成本 | 仅复制核心技能子集（可配置 `DEFAULT_SKILL_SET`），非核心按需安装 | backend |
| system prompt 过长 | 中 — LLM token 浪费 | 记忆摘要最大 2000 token，技能列表仅注入名称+描述 | backend |
| 已有租户无技能 | 高 — 存量用户不可用 | 启动时 `SyncAllTenantsSkills` 覆盖所有已有租户 | backend |

---

## 节点检查

| 节点 | 内容 | 时间 |
|------|------|------|
| 方案评审 | 确认 provisioning 流程、存储模型、API 设计 | Phase 1 完成后 |
| 开发完成 | Phase 1-4 全部实现 | Phase 4 完成后 |
| 测试完成 | Phase 5 全部通过 | Phase 5 完成后 |
| 发布准备 | 文档更新 + 迁移脚本验证 | Phase 5.4 完成后 |

---

## 关键决策点

### D1：默认技能集范围

**选项 A**：全量复制 77 个内置技能到每个租户
- 优点：租户开箱即用，能力最全
- 缺点：每个租户 ~77 个 SKILL.md 文件，MinIO 存储 × N 租户

**选项 B**：仅复制核心子集（~10-15 个高频技能），其余按需安装
- 优点：存储高效，按需付费
- 缺点：需要定义"核心集"，用户首次使用可能缺技能

**推荐**：选项 B，通过 `DEFAULT_SKILL_SET` 环境变量控制，默认包含 software-development、research、productivity 三个类别。

### D2：Soul 存储位置

**选项 A**：MinIO `{tenant_id}/_soul/SOUL.md`
- 优点：与技能存储统一，SKILL.md 格式一致
- 缺点：需要额外 MinIO 读取

**选项 B**：PostgreSQL `tenant_configs` 表
- 优点：查询快，与 auth context 一起加载
- 缺点：新增表和迁移

**推荐**：选项 A，与技能体系统一存储，运维一致性好。`tenant_configs` 表作为 v28 迁移保留，用于存储 Soul 之外的租户配置（未来扩展）。

### D3：user_id 来源

**选项 A**：从 API Key 的 `Identity`（key UUID）作为 user_id
- 优点：零改动，当前 auth context 已有
- 缺点：一个 key 对应一个"用户"，换 key 即丢失记忆

**选项 B**：请求 header `X-Hermes-User-Id`（由客户端传入）
- 优点：灵活，支持一个 API Key 多用户
- 缺点：客户端可伪造

**推荐**：选项 A 为默认，选项 B 为可选覆盖（当 header 存在时使用 header 值，否则回退到 key Identity）。user_id 始终限定在当前 tenant_id 范围内。
