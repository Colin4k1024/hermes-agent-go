# PRD: 租户自动开通 — 记忆/画像/技能/Soul 全链路补齐

> Intake 日期：2026-04-29 | Owner：tech-lead | 状态：intake

---

## 背景

Hermes Agent Go 已完成 SaaS 多租户基础架构（认证链、RBAC、速率限制、审计、租户隔离），但本地测试发现 **租户创建后无法真正使用 Agent 能力**，存在以下严重缺陷：

1. **记忆/画像零初始化**：租户用户绑定 API Key 后发起 Chat 请求，无记忆存储、无用户画像、无 Soul 配置。`memories` 和 `user_profiles` 表存在但 `mockchat` handler 完全未接入 `PGMemoryProvider`。
2. **技能库未挂载**：新建租户在 MinIO 中无任何 SKILL.md 文件，Agent 无法提供领域能力。当前 `SyncBuiltinSkills()` 仅针对本地文件系统，无租户级 bootstrap。
3. **SOUL.md 仅 CLI 模式**：全局 `~/.hermes/SOUL.md` 不适用于多租户场景，缺少 per-tenant persona 配置机制。
4. **技能演化无隔离**：`SyncBuiltinSkills()` 基于本地 manifest 和文件 hash，无法区分租户。技能版本更新、用户自定义修改在 SaaS 模式下不隔离。

**业务影响**：新建租户绑定 API Key 后的首次 Chat 请求无法获得任何 Agent 增强能力（记忆、画像、技能、人格），SaaS 产品不可用。

---

## 目标与成功标准

### 业务目标

- 租户创建后自动具备完整的 Agent 能力，无需手工 seed 脚本
- 每个租户拥有独立的记忆空间、用户画像、技能库和 Agent 人格
- 技能支持自动演化（内置更新）和用户自定义修改，且不跨租户污染

### 成功指标

1. `POST /v1/tenants` 创建租户后，MinIO 中自动出现该租户的默认技能集
2. `POST /v1/chat/completions` 请求能自动读取/写入租户级记忆和用户画像
3. 不同租户的 Soul/persona 可独立配置，互不影响
4. 内置技能更新时，已有租户自动获得新技能，但不覆盖租户已修改的技能
5. 所有存储（memories、user_profiles、skills）按 `(tenant_id, user_id)` 完全隔离

---

## 用户故事

### US-1：租户自动开通默认技能

**作为** SaaS 管理员，**我希望** 创建租户时自动为其安装默认技能集，**以便** 租户用户首次 Chat 即可使用 Agent 的领域能力。

**验收标准**：
- 创建租户后 MinIO 中 `{tenant_id}/` 前缀下至少存在 N 个核心技能的 SKILL.md
- 默认技能集可通过配置 `DEFAULT_SKILL_SET` 环境变量或 tenant 表字段控制
- 创建失败时技能 seed 失败不阻塞租户记录写入（降级策略）

### US-2：Chat 请求接入记忆和画像

**作为** 租户用户，**我希望** 我的对话历史中 Agent 能记住之前的交互信息，**以便** 获得个性化、连续的对话体验。

**验收标准**：
- Chat handler 在构建 system prompt 时调用 `PGMemoryProvider.SystemPromptBlock()` 注入记忆和画像
- 记忆按 `(tenant_id, user_id)` 隔离 — 不同用户看不到彼此的记忆
- 用户画像在首次 Chat 时自动初始化（空白模板），后续由 Agent 自主更新

### US-3：Per-tenant Soul/Persona

**作为** SaaS 管理员，**我希望** 为不同租户配置不同的 Agent 人格，**以便** 每个租户的用户获得差异化的 Agent 体验。

**验收标准**：
- Soul 配置存储在 `user_profiles` 表（特殊 key `__tenant_soul__`）或 MinIO `{tenant_id}/_soul/SOUL.md`
- 创建租户时自动写入默认 Soul 模板
- Chat handler 在 system prompt 中注入 tenant-level Soul
- Soul 修改不影响其他租户

### US-4：技能自动演化与租户隔离

**作为** 平台运维，**我希望** 内置技能更新后所有租户自动获得新版本，但不覆盖租户已自定义的技能，**以便** 平台能力持续升级同时尊重租户个性化。

**验收标准**：
- 新增 `TenantSkillSync` 机制：比较内置技能 hash 与租户 MinIO 中已有技能 hash
- 新技能自动复制到所有租户
- 已修改的技能（hash 不匹配源）标记为 `user_modified`，跳过更新
- 同步结果记录到审计日志
- 每个租户维护独立的 `.manifest.json`（存于 MinIO `{tenant_id}/.manifest.json`）

### US-5：技能管理 API

**作为** 租户管理员，**我希望** 通过 API 管理我的租户技能（列出、上传、删除），**以便** 不依赖手工 MinIO 操作。

**验收标准**：
- `GET /v1/skills` — 列出当前租户所有技能（名称、版本、来源、是否修改）
- `PUT /v1/skills/{name}` — 上传/更新技能 SKILL.md 内容
- `DELETE /v1/skills/{name}` — 删除租户自定义技能
- 所有操作自动限定 `tenant_id` 前缀，不可跨租户操作

---

## 范围

### In Scope

| 模块 | 变更 |
|------|------|
| `internal/api/tenants.go` | 创建租户后调用 provisioning hook（seed 默认技能 + Soul） |
| `internal/api/mockchat.go` | 接入 `PGMemoryProvider`，system prompt 注入记忆/画像/Soul |
| `internal/api/skills.go` | **新建** — 租户技能 CRUD API |
| `internal/api/server.go` | 注册 `/v1/skills` 路由，传入 MinIO client |
| `internal/skills/provisioner.go` | **新建** — 租户技能 bootstrap 和 sync 逻辑 |
| `internal/skills/loader_minio.go` | 扩展：支持读取 `.manifest.json`、Soul 文件 |
| `internal/objstore/minio.go` | 新增 `DeleteObject`、`ObjectExists` 方法 |
| `internal/store/pg/migrate.go` | v28：`tenant_configs` 表（存储 per-tenant Soul 和偏好） |
| `cmd/hermes/saas.go` | 启动时执行全局技能同步（新增内置技能 → 所有租户） |

### Out of Scope

- 完整的 Agent tool-use 循环（当前保持 mock LLM 调用模式）
- 技能市场（Hub）的 SaaS 集成
- 记忆的自动清理/归档
- Gateway 模式（仅 SaaS API 模式）

---

## 风险与依赖

| 风险 | 影响 | 缓解 |
|------|------|------|
| MinIO 不可用时租户创建失败 | 高 | 技能 seed 异步执行，失败记录到 `tenant.metadata` 待重试 |
| 大量租户同时 sync 技能导致 MinIO 压力 | 中 | 启动时串行同步 + 限制并发 |
| 记忆注入增加 system prompt 长度 | 中 | 设置记忆摘要最大 token 数，超出时截断 |
| 已有租户（如默认租户）无技能 | 高 | 启动时的全局 sync 覆盖已有租户 |

### 关键依赖

- MinIO/S3 服务可用（`MINIO_ENDPOINT` 已配置）
- PostgreSQL 连接正常（`DATABASE_URL`）
- 内置技能目录 `skills/` 存在且非空

---

## 技术约束

### 数据隔离模型

```
tenant_id (from auth context, NEVER from request header)
  ├── memories      — PG: WHERE tenant_id=$1 AND user_id=$2
  ├── user_profiles — PG: WHERE tenant_id=$1 AND user_id=$2
  ├── skills        — MinIO: prefix={tenant_id}/
  ├── soul          — MinIO: {tenant_id}/_soul/SOUL.md 或 PG tenant_configs
  └── manifest      — MinIO: {tenant_id}/.manifest.json
```

### 技能演化流程

```
平台启动
  ├── 读取 skills/ 目录（77+ 内置技能）
  ├── 遍历所有租户
  │   ├── 读取 MinIO {tenant_id}/.manifest.json
  │   ├── 比较 hash：新技能 → 复制；已更新 → 覆盖（非 user_modified）；已修改 → 跳过
  │   └── 写回 .manifest.json
  └── 记录同步结果到日志

租户创建
  ├── INSERT tenants
  ├── 异步：复制默认技能集到 MinIO {tenant_id}/
  ├── 异步：写入默认 Soul 到 MinIO {tenant_id}/_soul/SOUL.md
  └── 返回租户信息（含 provisioning_status）
```

### Chat 请求增强流程

```
POST /v1/chat/completions
  ├── Auth → TenantID + UserID (from ac.Identity or X-Hermes-User-Id header)
  ├── 构建 PGMemoryProvider(pool, tenantID, userID)
  ├── 加载 tenant Soul: MinIO {tenantID}/_soul/SOUL.md
  ├── 加载 tenant 记忆: PGMemoryProvider.SystemPromptBlock()
  ├── 组装 system prompt = Soul + Memory + UserProfile + tenant context
  ├── 加载 tenant 技能（MinIOSkillLoader）→ 可用技能列表注入 prompt
  ├── 调用 LLM
  └── 持久化消息 + 更新 token 用量
```
