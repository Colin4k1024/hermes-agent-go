# Architecture Design: Hermes Agent SaaS Production Hardening (v0.7.0)

> 日期: 2026-04-30 | 主责: architect | 状态: handoff-ready

---

## 系统边界

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Layer                            │
│   WebUI (Vue 3)  │  curl / SDK  │  OpenAI-compat clients       │
└────────┬────────────────┬─────────────────┬─────────────────────┘
         │                │                 │
         ▼                ▼                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                    API Gateway (Go net/http)                     │
│  CORS → RequestID → Auth → Tenant → Audit → RBAC → RateLimit   │
│                                                                 │
│  POST /v1/chat/completions  (non-stream / SSE stream)           │
│  GET  /v1/gdpr/export       (扩展: memories + profiles)          │
│  DELETE /v1/gdpr/data       (扩展: 全表级联)                      │
│  DELETE /v1/tenants/{id}    (软删除 → 202 Accepted)              │
│  GET  /v1/audit-logs        (含 AUTH_FAILED 事件)                │
└────────┬────────────────────────────────────────┬───────────────┘
         │                                        │
         ▼                                        ▼
┌────────────────────┐              ┌──────────────────────────┐
│   AIAgent Engine   │              │    Background Jobs       │
│  Tool Loop + Soul  │              │  TenantCleanupJob        │
│  Skills + Memory   │              │  (advisory lock + 7d)    │
└────────┬───────────┘              └──────────┬───────────────┘
         │                                     │
         ▼                                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                    PostgreSQL (pgxpool)                          │
│  tenants │ sessions │ messages │ memories │ user_profiles       │
│  api_keys │ audit_logs │ users │ cron_jobs │ schema_version     │
│  所有子表: deleted_at IS NULL partial index                      │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
                ┌───────────────────┐
                │   MinIO (S3)      │
                │  tenant skills    │
                │  tenant soul      │
                └───────────────────┘
```

**外部依赖**: LLM Provider (OpenAI-compatible API), PostgreSQL 14+, MinIO, Redis (rate limit fallback)

**本轮不引入的外部依赖**: Kubernetes, Helm, Prometheus LLM metrics, JWT/OAuth provider

---

## 组件拆分与变更矩阵

### Sprint 1: 级联删除统一治理 (S1.1)

| 组件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/store/types.go` | 修改 | Tenant 增加 `DeletedAt *time.Time`; AuditLog 增加 `SourceIP`, `ErrorCode` |
| `internal/store/store.go` | 修改 | TenantStore 增加 `SoftDelete`, `ListDeleted`, `HardDelete`, `Restore` |
| `internal/store/pg/migrate.go` | 修改 | 新增 v28-v35 迁移 |
| `internal/store/pg/tenant.go` | 修改 | Delete → SoftDelete, 全部 SELECT 加 `deleted_at IS NULL` |
| `internal/store/pg/*.go` | 修改 | 所有 6 张子表查询加 `deleted_at IS NULL` (tenants 表) |
| `internal/api/gdpr.go` | 修改 | 注入 `store.Store`, DELETE 覆盖 memories/profiles/keys, EXPORT 覆盖 memories/profiles |
| `internal/api/tenant.go` | 修改 | DELETE 返回 202, 调用 SoftDelete |
| `internal/jobs/tenant_cleanup.go` | **新建** | 7 天异步硬删除 + advisory lock |

### Sprint 1: SSE 流式响应 (S1.2, 并行)

| 组件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/api/agent_chat.go` | 修改 | 增加 `serveSSE()` 分支, stream=true 走 SSE |
| `internal/agent/conversation.go` | 修改 | 暴露 streaming channel 给 HTTP 层 |

### Sprint 2: 审计失败认证 (S2.1)

| 组件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/middleware/auth.go` | 修改 | 注入 AuditLogStore, 两个 401 分支写 AUTH_FAILED |
| `internal/middleware/audit.go` | 修改 | 适配 nullable tenant_id |
| `internal/store/pg/auditlog.go` | 修改 | 适配 nullable tenant_id 查询 |

### Sprint 2: RBAC 粒度增强 (S2.2)

| 组件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/middleware/rbac.go` | 修改 | method+path 组合权限匹配 |

### Sprint 3: 运维基线 (S3.1-S3.3)

| 组件 | 变更类型 | 说明 |
|------|---------|------|
| `cmd/hermes/main.go` | 修改 | JSON slog handler 切换 |
| `internal/store/pg/migrate.go` | 修改 | advisory lock 包裹 |
| `internal/config/secrets.go` | **新建** | 环境变量密钥加载 + 验证 |
| `internal/api/chat_handler.go` | 修改 | 移除 "123456" fallback |
| `docker-compose*.yml` | 修改 | .env 引用替代硬编码 |

---

## 关键数据流

### 1. Tenant 软删除 + 异步清理

```
Client                    API Server                   PostgreSQL
  │                          │                             │
  │  DELETE /v1/tenants/{id} │                             │
  │─────────────────────────▶│                             │
  │                          │  UPDATE tenants             │
  │                          │  SET deleted_at = now()     │
  │                          │  WHERE id = $1              │
  │                          │  AND deleted_at IS NULL     │
  │                          │────────────────────────────▶│
  │                          │                             │
  │  202 Accepted            │◀────────────────────────────│
  │◀─────────────────────────│                             │
  │                          │                             │
  │                          │                             │
  │            TenantCleanupJob (background, every 1h)     │
  │                          │                             │
  │                          │  pg_try_advisory_lock(...)  │
  │                          │────────────────────────────▶│
  │                          │                             │
  │                          │  SELECT id FROM tenants     │
  │                          │  WHERE deleted_at < now()   │
  │                          │        - INTERVAL '7 days'  │
  │                          │────────────────────────────▶│
  │                          │                             │
  │                          │  FOR EACH tenant:           │
  │                          │    DELETE messages           │
  │                          │    DELETE sessions           │
  │                          │    DELETE memories           │
  │                          │    DELETE user_profiles      │
  │                          │    DELETE api_keys           │
  │                          │    DELETE audit_logs         │
  │                          │    DELETE users              │
  │                          │    DELETE cron_jobs          │
  │                          │    DELETE tenants            │
  │                          │────────────────────────────▶│
  │                          │                             │
  │                          │  pg_advisory_unlock(...)    │
  │                          │────────────────────────────▶│
```

**FK 依赖删除顺序**: messages → sessions → memories → user_profiles → api_keys → audit_logs → users → cron_jobs → tenants

### 2. SSE 流式响应

```
Client                    API Server            LLM Provider
  │                          │                       │
  │  POST /v1/chat/completions                       │
  │  { stream: true }        │                       │
  │─────────────────────────▶│                       │
  │                          │                       │
  │  Content-Type:           │  ChatStream(prompt)   │
  │  text/event-stream       │──────────────────────▶│
  │◀─────────────────────────│                       │
  │                          │                       │
  │  data: {"choices":[{     │◀──── token chunk ─────│
  │    "delta":{"content":   │                       │
  │    "Hello"}}]}           │                       │
  │◀─────────────────────────│                       │
  │                          │                       │
  │  : heartbeat (15s)       │  (tool loop: buffer)  │
  │◀─────────────────────────│                       │
  │                          │                       │
  │  data: {"choices":[{     │◀──── final chunk ─────│
  │    "finish_reason":      │                       │
  │    "stop"}]}             │                       │
  │◀─────────────────────────│                       │
  │                          │                       │
  │  data: [DONE]            │                       │
  │◀─────────────────────────│                       │
```

**SSE 格式 (OpenAI chat.completion.chunk 兼容)**:
```
data: {"id":"sess_xxx","object":"chat.completion.chunk","created":1714470000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

: heartbeat

data: {"id":"sess_xxx","object":"chat.completion.chunk","created":1714470000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

**Tool loop 处理**: 当 agent 进入 tool execution 阶段时，内部缓冲不发送 delta；tool 完成后恢复流式输出。期间 15s 心跳保持连接活跃。

### 3. Auth 失败审计

```
Client                   Auth Middleware            AuditLogStore
  │                          │                          │
  │  Bearer sk-invalid...    │                          │
  │─────────────────────────▶│                          │
  │                          │                          │
  │                          │  chain.Extract(r)        │
  │                          │  → error (INVALID_KEY)   │
  │                          │                          │
  │                          │  Append(AuditLog{        │
  │                          │    TenantID: nil,         │
  │                          │    Action: "AUTH_FAILED", │
  │                          │    SourceIP: remoteAddr,  │
  │                          │    ErrorCode: "INVALID_KEY"│
  │                          │    RequestID: ...,        │
  │                          │  })                      │
  │                          │─────────────────────────▶│
  │                          │                          │
  │  401 Unauthorized        │                          │
  │◀─────────────────────────│                          │
```

**错误码枚举**: `INVALID_KEY` | `EXPIRED_KEY` | `REVOKED_KEY` | `MISSING_AUTH`

---

## Schema 迁移计划 (v28-v35)

```sql
-- v28: Tenant 软删除
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_tenants_active ON tenants(id) WHERE deleted_at IS NULL;

-- v29: 审计日志扩展 — tenant_id 可空 + 新字段
ALTER TABLE audit_logs ALTER COLUMN tenant_id DROP NOT NULL;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS source_ip TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS error_code TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT;

-- v30: sessions partial index (软删除租户查询优化)
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_active
  ON sessions(tenant_id) WHERE tenant_id IN (
    SELECT id FROM tenants WHERE deleted_at IS NULL
  );
-- 注: 此 index 在实现时可简化为应用层 WHERE + 已有 idx_sessions_tenant

-- v31: memories partial index
CREATE INDEX IF NOT EXISTS idx_memories_tenant_user
  ON memories(tenant_id, user_id);

-- v32: user_profiles index
CREATE INDEX IF NOT EXISTS idx_profiles_tenant_user
  ON user_profiles(tenant_id, user_id);

-- v33: api_keys 增加 deleted_at (级联标记)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- v34: audit_logs error_code index (认证失败查询)
CREATE INDEX IF NOT EXISTS idx_audit_error_code
  ON audit_logs(error_code) WHERE error_code IS NOT NULL;

-- v35: schema_version advisory lock 标记 (元数据)
-- 实际 lock 通过 pg_try_advisory_lock 在代码层实现，此迁移为占位确认
SELECT 1;
```

**迁移编号分配**: Sprint 1 使用 v28-v35; Sprint 2/3 从 v36 起编号。

---

## 接口约定变更

### DELETE /v1/tenants/{id}

**Before (v0.6)**: 硬删除, 返回 204
**After (v0.7)**: 软删除, 返回 202 Accepted

```json
// Response 202
{
  "id": "tenant-uuid",
  "status": "scheduled_for_deletion",
  "deleted_at": "2026-04-30T15:00:00Z",
  "hard_delete_after": "2026-05-07T15:00:00Z"
}
```

### DELETE /v1/gdpr/data

**Before (v0.6)**: 仅删除 sessions + messages, 返回 204
**After (v0.7)**: 删除 sessions + messages + memories + user_profiles + api_keys, 返回 200 + JSON

```json
// Response 200
{
  "tenant_id": "tenant-uuid",
  "deleted": {
    "sessions": 15,
    "messages": 342,
    "memories": 8,
    "user_profiles": 3,
    "api_keys": 2
  }
}
```

### GET /v1/gdpr/export

**Before (v0.6)**: 仅导出 sessions + messages
**After (v0.7)**: 导出 sessions + messages + memories + user_profiles

```json
{
  "tenant_id": "tenant-uuid",
  "sessions": [...],
  "memories": [
    { "key": "user_name", "content": "Alice", "updated_at": "..." }
  ],
  "user_profiles": [
    { "user_id": "uid-1", "content": "...", "updated_at": "..." }
  ]
}
```

### POST /v1/chat/completions (stream=true)

**新增**: 当请求体包含 `"stream": true` 时:
- Content-Type: `text/event-stream`
- Transfer-Encoding: `chunked`
- 每个 token 以 `data: {chunk}\n\n` 格式发送
- 15s 心跳: `: heartbeat\n\n`
- 结束: `data: [DONE]\n\n`

### GET /v1/audit-logs (变更)

**新增 error_code 过滤**:
```
GET /v1/audit-logs?action=AUTH_FAILED&limit=50
```

返回的 AuditLog 对象新增字段:
```json
{
  "source_ip": "192.168.1.100",
  "error_code": "INVALID_KEY",
  "user_agent": "curl/7.88.1"
}
```

---

## 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| 软删除标记 | `deleted_at TIMESTAMPTZ` | 标准模式, 支持 partial index, 7 天窗口可恢复 |
| 异步清理锁 | `pg_try_advisory_lock` | 非阻塞, 不依赖连接关闭释放, 多实例安全 |
| SSE 格式 | OpenAI chat.completion.chunk | 客户端兼容性最广, WebUI 已有解析逻辑 |
| SSE 心跳 | SSE comment (`: heartbeat`) | 不影响数据解析, 标准 SSE 规范 |
| 日志格式 | slog.JSONHandler | Go 标准库, 零外部依赖 |
| 密钥管理 | 环境变量 + .env | POC 阶段足够, 不过度设计 Vault 集成 |
| PG RLS | **不做** (v0.8) | pgxpool SET 连接状态泄漏风险, 应用层 WHERE 已覆盖 50+ 处 |

---

## 风险与约束

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 软删除 WHERE 遗漏 | 已删除租户数据泄漏 | 每个 pg/*.go 逐行 review, CI grep 检查 `FROM tenants` 不含 `deleted_at` |
| SSE 超时 | 客户端断连 | 15s heartbeat + server WriteTimeout=150s + client 30s timeout |
| advisory lock 泄漏 | 清理 job 死锁 | pg_try_advisory_lock (非阻塞) + 显式 unlock + defer |
| audit_logs tenant_id NULL | 下游查询 panic | COALESCE + 应用层 NULL 检查 |
| GDPR 删除不完整 | 合规风险 | 返回 JSON 计数, 单测验证每张表被清理 |
| 迁移并发竞争 | 重复 DDL 或部分失败 | advisory lock 包裹整个 runMigrations |

---

## 当前不做项

| 项 | 原因 | 目标版本 |
|----|------|---------|
| PostgreSQL RLS | pgxpool 连接状态污染 | v0.8 |
| SSE 断点续传 (Last-Event-ID) | 复杂度高, POC 无需 | v0.8 |
| Helm chart / K8s | 当前 Docker Compose | v0.9 |
| Prometheus LLM 指标 | 非阻塞 | v0.8 |
| JWT/OAuth | API Key 满足当前需求 | v0.8 |
| 租户级工具沙箱 | 架构复杂度高 | v0.9 |
