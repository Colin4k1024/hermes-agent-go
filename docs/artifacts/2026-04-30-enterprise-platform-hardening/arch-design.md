# Architecture Design: Hermes Agent Go 企业级分布式平台加固

> 日期: 2026-04-30 | 主责: architect | 状态: draft | 阶段: plan

---

## 系统边界

- **内部组件**: API Server (SaaS)、Gateway Runner、Agent Core、LLM Client、Store Layer、Cron Scheduler
- **外部依赖**: PostgreSQL 16、Redis 7、MinIO、LLM API (OpenAI-compatible + Anthropic + Gemini + Bedrock)
- **集成点**: Telegram/Discord/Slack/WhatsApp adapters, ACP Server, WebUI (Vue 3)

---

## 1. Store 层重构方案

### 现状问题

- `store.Store` 定义 6 个 sub-store（Sessions, Messages, Users, Tenants, AuditLogs, APIKeys）
- `memories`/`user_profiles` 表通过 `PGMemoryProvider` raw pgxpool 操作，未纳入 Store 接口
- `cron_jobs` 表有 PG schema（v14-v16），但 `cron.JobStore` 使用本地 JSON 文件
- `saas.go` 对 `*pg.PGStore` 类型断言暴露实现细节

### 新增接口

```go
type MemoryStore interface {
    Get(ctx context.Context, tenantID, userID, key string) (string, error)
    List(ctx context.Context, tenantID, userID string) ([]MemoryEntry, error)
    Upsert(ctx context.Context, tenantID, userID, key, content string) error
    Delete(ctx context.Context, tenantID, userID, key string) error
    DeleteAllByUser(ctx context.Context, tenantID, userID string) (int64, error)
    DeleteAllByTenant(ctx context.Context, tenantID string) (int64, error)
}

type UserProfileStore interface {
    Get(ctx context.Context, tenantID, userID string) (string, error)
    Upsert(ctx context.Context, tenantID, userID, content string) error
    Delete(ctx context.Context, tenantID, userID string) error
    DeleteAllByTenant(ctx context.Context, tenantID string) (int64, error)
}

type CronJobStore interface {
    Create(ctx context.Context, job *CronJob) error
    Get(ctx context.Context, tenantID, jobID string) (*CronJob, error)
    Update(ctx context.Context, job *CronJob) error
    Delete(ctx context.Context, tenantID, jobID string) error
    List(ctx context.Context, tenantID string) ([]*CronJob, error)
    ListDue(ctx context.Context, now time.Time) ([]*CronJob, error)
    MarkRun(ctx context.Context, tenantID, jobID string, success bool, errMsg string, nextRun time.Time) error
}
```

### Store 接口扩展

```go
type Store interface {
    Sessions() SessionStore
    Messages() MessageStore
    Users() UserStore
    Tenants() TenantStore
    AuditLogs() AuditLogStore
    APIKeys() APIKeyStore
    Memories() MemoryStore         // NEW
    UserProfiles() UserProfileStore // NEW
    CronJobs() CronJobStore        // NEW
    Close() error
    Migrate(ctx context.Context) error
}
```

### 类型断言消除

引入 `PoolProvider` 可选接口：

```go
type PoolProvider interface {
    Pool() *pgxpool.Pool
}
```

`saas.go` 改为 `dataStore.(store.PoolProvider)` 接口断言，不再依赖 `*pg.PGStore` 具体类型。`PGMemoryProvider` 改为使用 `MemoryStore` + `UserProfileStore` 接口。

### 迁移步骤

1. 新增类型到 `store/types.go`
2. 新增接口到 `store/store.go`
3. 在 `pg/` 新增 `memories.go`、`user_profiles.go`、`cron_jobs.go`
4. 重构 `PGMemoryProvider` 接受 Store 接口而非 pool
5. `PGStore` 增加 sub-store 字段
6. `saas.go` 消除类型断言

---

## 2. RBAC 数据模型

### 新表结构

```sql
-- v36: roles
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

-- v37: role_permissions
CREATE TABLE role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(role_id, resource, action)
);
```

### 权限矩阵

| Resource | Actions |
|----------|---------|
| `tenants` | read, write, delete |
| `sessions` | read, write, delete |
| `messages` | read, write, search |
| `memories` | read, write, delete |
| `users` | read, write, approve |
| `audit_logs` | read |
| `api_keys` | read, write, revoke |
| `cron_jobs` | read, write, delete |
| `gdpr` | export, delete |

### 中间件兼容方案

RBAC 中间件保持 method+path 匹配，新增 permission-based 路径：

```go
type RBACConfig struct {
    DefaultRole string
    Rules       map[string]string         // 保留：path → role name (向后兼容)
    Permissions map[string]Permission     // 新增：path → resource+action
}
```

检查流程：Rules 匹配 → Permissions 匹配 → admin bypass。

### 热加载

`RBACPermissionResolver` 接口 + Redis 缓存（TTL 5 分钟）+ PG LISTEN/NOTIFY invalidation。

---

## 3. 无状态化架构

### 3.1 soulCache: TTL + LRU

```go
import lru "github.com/hashicorp/golang-lru/v2/expirable"

soulCache := lru.NewLRU[string, string](500, nil, 30*time.Minute)
```

- 最大 500 条目，TTL 30 分钟，内置 sync.Mutex

### 3.2 agentCache: per-request 对齐

消除 Gateway `agentCache`，改为 AgentFactory 模式：

```go
type AgentFactory struct {
    pool        *pgxpool.Pool
    llmClient   *llm.Client
    skillLoader *skills.Loader
}

func (f *AgentFactory) Build(ctx context.Context, session *SessionEntry) (*agent.AIAgent, error) {
    history, _ := f.loadHistory(ctx, session.TenantID, session.SessionID, 50)
    return agent.NewAIAgent(
        agent.WithModel(session.Model),
        agent.WithHistory(history),
        agent.WithSkills(f.skillLoader.ForTenant(ctx, session.TenantID)),
    ), nil
}
```

**迁移策略**: Phase 1 先加 LRU + TTL（5min, max 200）防 OOM；后续彻底移除。

### 3.3 PairingStore 持久化

复用 `users` 表（`approved_at IS NOT NULL` = paired）：

```go
type PersistentPairingStore struct {
    users    store.UserStore
    tenantID string
    cache    *lru.LRU[string, bool] // platform:userID → allowed
    mu       sync.Mutex
    pendingCodes map[string]*PairingRequest // 内存，10min 过期，丢失可接受
}
```

---

## 4. Lifecycle Manager

### Service 接口

```go
type Service interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

type Manager struct {
    services []Service
    eg       *errgroup.Group
    ctx      context.Context
    cancel   context.CancelFunc
}
```

### saas.go 改造

```go
mgr := lifecycle.New()
mgr.Register(saasServer)    // HTTP server
mgr.Register(runner)        // Gateway runner
mgr.Register(acpServer)     // ACP server
mgr.Register(tenantSyncer)  // Background sync
mgr.Start()

<-sigCh
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
mgr.Shutdown(shutdownCtx) // LIFO 顺序
```

### 关键保证

- errgroup 追踪所有 goroutine，无泄漏
- Context 级联取消
- LIFO shutdown（先停接入层，再停处理层，最后关存储）
- 30 秒 grace period

---

## 5. LLM 调用韧性

### 装饰器模式 + per-model breaker

```go
type ResilientTransport struct {
    inner         llm.Transport
    breaker       *gobreaker.CircuitBreaker[*llm.ChatResponse]
    streamBreaker *gobreaker.CircuitBreaker[streamResult]
}
```

### gobreaker 配置

- `MaxRequests`: 3（半开探针数）
- `Interval`: 60s（closed 重置周期）
- `Timeout`: 30s（open → half-open 等待）
- `ReadyToTrip`: 连续 5 次失败 OR 10s 内失败率 > 50%

### Streaming 交互

- **连接建立**: breaker 保护，failure 计入
- **流式传输中**: 不计入 breaker，通过 errCh 上报
- **Context 取消**: 流正常关闭，不计入 failure

### 退避策略

```go
type BackoffPolicy struct {
    InitialDelay time.Duration // 500ms
    MaxDelay     time.Duration // 10s
    Multiplier   float64       // 2.0
    Jitter       float64       // 0.1 (±10%)
}
```

---

## 6. 真实 SSE 流式方案

### 架构

```
Client ◄──SSE── SSE Handler ◄──chan── Agent Loop ◄──stream── LLM Provider
                 (flush/delta)        (tool loop)
```

### Agent 改造

```go
type StreamingConversation struct {
    Events <-chan SSEEvent
    Done   <-chan ConversationResult
}

type SSEEvent struct {
    Type    string         // "delta", "tool_start", "tool_end", "error", "done"
    Content string
    Meta    map[string]any
}
```

### SSE 事件格式（OpenAI 兼容）

```
event: delta
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"Hello"}}]}

event: tool_call
data: {"tool":"web_search","status":"started","call_id":"tc_1"}

event: tool_result
data: {"tool":"web_search","status":"completed","call_id":"tc_1"}

event: error
data: {"error":"rate_limit_exceeded","retry_after":5}

data: [DONE]
```

### Tool Loop 缓冲

Tool 执行期间发送 `tool_start`/`tool_end` 事件保持连接活跃，Client 展示"正在搜索..."等状态。心跳 15 秒。

---

## 7. RLS 方案

### 双连接池架构

```
┌─ App Pool (hermes_app) ─── RLS enforced ─── 业务查询
│   AfterRelease: RESET ALL
│
└─ Admin Pool (hermes_admin) ─── RLS bypassed ─── 迁移/健康检查
```

### SET LOCAL + Transaction 模式

```go
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
    conn, err := pool.Acquire(ctx)
    if err != nil { return err }
    defer conn.Release()

    tx, err := conn.Begin(ctx)
    if err != nil { return err }

    _, err = tx.Exec(ctx, "SET LOCAL app.current_tenant = $1", tenantID)
    if err != nil { tx.Rollback(ctx); return err }

    if err := fn(tx); err != nil { tx.Rollback(ctx); return err }
    return tx.Commit(ctx)
}
```

### AfterRelease Hook

```go
poolCfg.AfterRelease = func(conn *pgx.Conn) bool {
    _, err := conn.Exec(context.Background(), "RESET ALL")
    return err == nil // corrupted connection discarded
}
```

### 关键约束

- `SET LOCAL` 作用域限定在事务内，commit/rollback 自动重置
- `AfterRelease` 作为额外安全层
- 所有 8 张 tenant-scoped 表启用 `FORCE ROW LEVEL SECURITY`

---

## 8. 组件依赖图

```
Phase 1 (v0.8.0) — Foundation
━━━━━━━━━━━━━━━━━━━━━━━━━━━━

S0 Store 补全 ──┬──▶ S1 RBAC（需 RoleStore）
                ├──▶ S4 无状态化（PairingStore 需 UserStore 接口）
                └──▶ Phase 3 S2 GDPR（需 MemoryStore.DeleteAllByTenant）

S1 RBAC ────────┬──▶ S2 JWT（roles claim 映射）
                └──▶ Phase 3 S4 数据导出（权限控制）

S5 租户SQL强制 ─────▶ Phase 3 S1 RLS（在验证基础上加 policy）

S3 Secrets ─────────▶ Phase 2 S5 分布式限流（Redis auth）


Phase 2 (v0.9.0) — Resilience & Experience
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

S0 Lifecycle ───┬──▶ S1 OTel（InitTracer 注册 shutdown hook）
                ├──▶ S3 熔断（breaker 注册 lifecycle）
                └──▶ Phase 4 S2 基础设施（服务编排就位）

S1 OTel ────────────▶ S3 熔断（span 包裹 breaker 调用）
S3 熔断 ────────────▶ S4 真实 SSE（熔断时 SSE 发送 error 事件）


Phase 3 (v0.9.5) — Data Governance
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

S1 RLS ─────────────▶ S2 GDPR（bypass RLS 用 admin pool 执行 purge）
S2 GDPR ────────────▶ S4 数据导出（导出范围与删除范围一致）


Phase 4 (v1.0.0) — Production
━━━━━━━━━━━━━━━━━━━━━━━━━━━━

S1 迁移工具 ────────▶ Phase 3 RLS migration down support（追溯）


Phase 5 (v1.1.0) — Scale
━━━━━━━━━━━━━━━━━━━━━━━━

S1 记忆治理 ◀──────── Phase 1 S0 MemoryStore
S3 ApprovalQueue ◀─── Phase 2 S5 Redis infra
```

---

## 技术选型

| 组件 | 选型 | 原因 |
|------|------|------|
| LRU Cache | `hashicorp/golang-lru/v2` | 已在 go.mod，支持 TTL，goroutine-safe |
| Circuit Breaker | `sony/gobreaker/v2` | Go 社区标准，轻量，泛型支持 |
| Lifecycle | `golang.org/x/sync/errgroup` | 标准库扩展，已在 go.mod |
| Migration (P4) | `golang-migrate/migrate/v4` | 支持 up/down，PG driver 成熟 |
| Rate Limit | Redis `EVALSHA` sliding window | 精确，原子，Redis 7 支持 |
| OIDC (P4) | `coreos/go-oidc/v3` | 成熟，JWKS rotation 支持 |

---

## 风险与约束

| 风险 | 影响 | 缓解 |
|------|------|------|
| RLS + pgxpool 变量泄漏 | 跨租户数据泄漏 | SET LOCAL + AfterRelease 双保险；集成测试覆盖 |
| agentCache 移除后延迟增加 | 用户体验下降 | 历史限 50 条；provider-side prompt cache |
| gobreaker 误触发 | 健康 LLM 被熔断 | 区分 timeout vs connection refused |
| SSE + 熔断交互 | 流式中途熔断处理 | 定义 error event 格式；断开连接 |
| 迁移工具切换 | 兼容性风险 | 桥接脚本 POC 先行 |
| Prometheus tenant_id label | 高基数 | exemplars 替代 label；或聚合到 plan 维度 |
