# 数据库

> Hermes SaaS API 的 Schema 设计、数据模型、迁移系统和租户隔离。

## 存储后端

| 后端 | 驱动 | 用途 | 包路径 |
|------|------|------|--------|
| PostgreSQL 16+ | pgx/v5 | SaaS 多租户模式 | `internal/store/pg/` |
| SQLite | go-sqlite3 | CLI 单用户模式 | `internal/store/sqlite/` |

SaaS 模式必须使用 PostgreSQL。连接通过 `DATABASE_URL` 环境变量配置。

## 数据表

### tenants — 租户

```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    plan TEXT NOT NULL DEFAULT 'free',
    rate_limit_rpm INT NOT NULL DEFAULT 60,
    max_sessions INT NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | 租户唯一标识 |
| `name` | TEXT | 租户名称 |
| `plan` | TEXT | 套餐（free / pro / enterprise） |
| `rate_limit_rpm` | INT | 每分钟请求限制 |
| `max_sessions` | INT | 最大会话数 |

### sessions — 会话

```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    platform TEXT NOT NULL,
    user_id TEXT NOT NULL,
    model TEXT,
    system_prompt TEXT,
    parent_session_id TEXT,
    title TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    end_reason TEXT,
    message_count INT DEFAULT 0,
    tool_call_count INT DEFAULT 0,
    input_tokens INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    cache_read_tokens INT DEFAULT 0,
    cache_write_tokens INT DEFAULT 0,
    estimated_cost_usd NUMERIC(10,6),
    metadata JSONB DEFAULT '{}',
    session_key TEXT
);
```

| 字段 | 说明 |
|------|------|
| `tenant_id` | 所属租户（FK） |
| `platform` | 平台标识 |
| `user_id` | 用户标识 |
| `model` | 使用的 LLM 模型 |
| `system_prompt` | 系统提示词 |
| `parent_session_id` | 父会话（分支场景） |
| `message_count` | 消息计数 |
| `input_tokens` / `output_tokens` | Token 用量 |
| `estimated_cost_usd` | 预估费用 |
| `session_key` | 唯一会话 Key（v20 新增） |

**索引**：
- `idx_sessions_tenant` — `(tenant_id)`
- `idx_sessions_user` — `(tenant_id, user_id)`
- `idx_sessions_platform` — `(tenant_id, platform)`
- `idx_sessions_key` — `UNIQUE (session_key) WHERE session_key IS NOT NULL`

### messages — 消息

```sql
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT,
    tool_call_id TEXT,
    tool_calls JSONB,
    tool_name TEXT,
    reasoning TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    token_count INT,
    finish_reason TEXT
);
```

| 字段 | 说明 |
|------|------|
| `role` | 消息角色（user / assistant / system / tool） |
| `content` | 消息内容 |
| `tool_calls` | 工具调用数据（JSONB） |
| `tool_name` | 工具名称 |
| `reasoning` | 推理过程 |
| `finish_reason` | 结束原因（stop / tool_calls / length） |

**索引**：
- `idx_messages_session` — `(tenant_id, session_id)`
- `idx_messages_ts` — `(tenant_id, session_id, timestamp)`
- `idx_messages_fts` — `GIN(to_tsvector('english', coalesce(content, '')))` 全文搜索

### users — 用户

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    external_id TEXT NOT NULL,
    username TEXT,
    display_name TEXT,
    role TEXT DEFAULT 'user',
    approved_at TIMESTAMPTZ,
    metadata JSONB DEFAULT '{}'
);
```

**索引**：
- `idx_users_external` — `UNIQUE (tenant_id, external_id)`

### api_keys — API 密钥

```sql
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    prefix TEXT NOT NULL,
    roles TEXT[] DEFAULT '{user}',
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

| 字段 | 说明 |
|------|------|
| `key_hash` | 原始 Key 的 SHA-256 哈希 |
| `prefix` | Key 前缀（`hk_` + 前几字符），用于管理界面识别 |
| `roles` | 角色数组（`{user}` 或 `{admin}`） |
| `expires_at` | 过期时间（NULL = 永不过期） |
| `revoked_at` | 撤销时间（NOT NULL = 已撤销） |

**索引**：
- `idx_apikeys_hash` — `UNIQUE (key_hash)`
- `idx_apikeys_tenant` — `(tenant_id)`

### audit_logs — 审计日志

```sql
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID,
    session_id TEXT,
    action TEXT NOT NULL,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_id TEXT,      -- v24 新增
    status_code INT,      -- v25 新增
    latency_ms INT        -- v26 新增
);
```

**索引**：
- `idx_audit_tenant` — `(tenant_id)`
- `idx_audit_request` — `(request_id)`

### cron_jobs — 定时任务

```sql
CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    prompt TEXT NOT NULL,
    schedule TEXT NOT NULL,
    deliver TEXT,
    enabled BOOLEAN DEFAULT true,
    model TEXT,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    run_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB DEFAULT '{}'
);
```

**索引**：
- `idx_cron_tenant` — `(tenant_id)`
- `idx_cron_next` — `(next_run_at) WHERE enabled = true` 条件索引

### memories — 记忆

```sql
CREATE TABLE memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL,
    key TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, key)
);
```

Memories 表存储 Agent 的长期记忆，按 `(tenant_id, user_id, key)` 唯一约束，支持 upsert 操作。

### user_profiles — 用户画像

```sql
CREATE TABLE user_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL,
    content TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id)
);
```

User Profiles 存储 Agent 构建的用户画像，按 `(tenant_id, user_id)` 唯一。

## 租户隔离

所有业务表均通过外键 `tenant_id UUID NOT NULL REFERENCES tenants(id)` 实现租户隔离：

```
tenants
  ├── sessions (FK: tenant_id)
  ├── messages (FK: tenant_id)
  ├── users (FK: tenant_id)
  ├── api_keys (FK: tenant_id)
  ├── audit_logs (FK: tenant_id)
  ├── cron_jobs (FK: tenant_id)
  ├── memories (FK: tenant_id)
  └── user_profiles (FK: tenant_id)
```

**隔离机制**：
- 租户 ID 从认证凭证中派生，永远不从请求头读取
- 所有 Store 方法的查询自动附加 `WHERE tenant_id = $1`
- 外键约束确保引用完整性

## 索引清单

| 索引名 | 表 | 列 | 类型 |
|--------|-----|-----|------|
| `idx_sessions_tenant` | sessions | `(tenant_id)` | B-tree |
| `idx_sessions_user` | sessions | `(tenant_id, user_id)` | B-tree |
| `idx_sessions_platform` | sessions | `(tenant_id, platform)` | B-tree |
| `idx_sessions_key` | sessions | `(session_key)` | Unique, Partial |
| `idx_messages_session` | messages | `(tenant_id, session_id)` | B-tree |
| `idx_messages_ts` | messages | `(tenant_id, session_id, timestamp)` | B-tree |
| `idx_messages_fts` | messages | `to_tsvector(content)` | GIN |
| `idx_users_external` | users | `(tenant_id, external_id)` | Unique |
| `idx_apikeys_hash` | api_keys | `(key_hash)` | Unique |
| `idx_apikeys_tenant` | api_keys | `(tenant_id)` | B-tree |
| `idx_audit_tenant` | audit_logs | `(tenant_id)` | B-tree |
| `idx_audit_request` | audit_logs | `(request_id)` | B-tree |
| `idx_cron_tenant` | cron_jobs | `(tenant_id)` | B-tree |
| `idx_cron_next` | cron_jobs | `(next_run_at)` | B-tree, Partial |

## 迁移系统

### 工作原理

Hermes 使用内嵌的 Go 代码管理数据库迁移，定义在 `internal/store/pg/migrate.go`。

```
启动 → 创建 schema_version 表 → 读取当前版本 → 顺序执行新迁移 → 记录版本号
```

- **版本跟踪**：`schema_version` 表记录已应用的迁移版本和时间
- **幂等执行**：使用 `IF NOT EXISTS` 和 `ADD COLUMN IF NOT EXISTS`
- **顺序保证**：迁移按版本号升序执行
- **启动自动执行**：每次 `hermes saas-api` 启动时自动检查并执行

### 当前版本

共 27 个迁移，分为以下阶段：

| 版本范围 | 内容 |
|----------|------|
| v1 | 创建 `tenants` 表 |
| v2-v5 | 创建 `sessions` 表 + 3 个索引 |
| v6-v9 | 创建 `messages` 表 + 3 个索引（含 GIN 全文搜索） |
| v10-v11 | 创建 `users` 表 + 唯一索引 |
| v12-v13 | 创建 `audit_logs` 表 + 索引 |
| v14-v16 | 创建 `cron_jobs` 表 + 2 个索引 |
| v17-v19 | 创建 `api_keys` 表 + 2 个索引 |
| v20-v21 | `sessions` 新增 `session_key` 列 + 唯一索引 |
| v22 | 创建 `memories` 表 |
| v23 | 创建 `user_profiles` 表 |
| v24-v27 | `audit_logs` 新增 `request_id`、`status_code`、`latency_ms` + 索引 |

### 添加新迁移

在 `internal/store/pg/migrate.go` 的 `migrations` 切片中追加新条目：

```go
var migrations = []migration{
    // ... 现有 27 个迁移 ...

    // 新迁移示例
    {28, `ALTER TABLE sessions ADD COLUMN IF NOT EXISTS tags TEXT[]`},
}
```

**注意事项**：
- 版本号必须递增且不重复
- 使用 `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` 保证幂等性
- DDL 语句应向后兼容（避免删除列、修改类型等破坏性操作）
- 测试时可通过清空 `schema_version` 表重新执行所有迁移

## Go 数据模型

### Session

```go
type Session struct {
    ID              string
    TenantID        string
    Platform        string
    UserID          string
    Model           string
    SystemPrompt    string
    ParentSessionID string
    Title           string
    StartedAt       time.Time
    EndedAt         *time.Time
    EndReason       string
    MessageCount    int
    ToolCallCount   int
    InputTokens     int
    OutputTokens    int
    CacheReadTokens int
    CacheWriteTokens int
    EstimatedCostUSD *float64
    Metadata        json.RawMessage
    SessionKey      string
    // ... 更多字段
}
```

### Tenant

```go
type Tenant struct {
    ID           string
    Name         string
    Plan         string
    RateLimitRPM int
    MaxSessions  int
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### APIKey

```go
type APIKey struct {
    ID        string
    TenantID  string
    Name      string
    KeyHash   string
    Prefix    string
    Roles     []string
    ExpiresAt *time.Time
    RevokedAt *time.Time
    CreatedAt time.Time
}
```

完整数据模型定义见 `internal/store/types.go`。

## 连接管理

推荐配置：

```bash
# 基本连接
DATABASE_URL="postgres://hermes:password@host:5432/hermes?sslmode=require"

# 连接池参数（通过 URL 参数）
DATABASE_URL="postgres://hermes:password@host:5432/hermes?sslmode=require&pool_max_conns=20&pool_min_conns=5"
```

生产环境建议：
- 使用 PgBouncer 作为连接池代理
- 配置 `sslmode=require` 或 `sslmode=verify-full`
- 通过 Kubernetes Secret 注入 `DATABASE_URL`

## 相关文档

- [架构概览](architecture.md) — Store 层设计
- [配置指南](configuration.md) — DATABASE_URL 配置
- [部署指南](deployment.md) — PostgreSQL 部署选项
- [可观测性](observability.md) — pgx Tracer 和数据库监控
