# Tenant Auto-Initialization: Soul, Memory & Skills Provisioning

## Context

新租户创建后缺乏自动初始化：skills 需要手动 seed 脚本，SOUL.md 在 SaaS 模式下完全不加载（`skipContextFiles=true` 跳过本地文件系统），memory 的 `SystemPromptBlock()` 虽然实现了但未接入 system prompt。导致新租户首次对话时 agent 没有人格、没有记忆上下文、技能需要手动部署。

**目标**：租户创建时自动初始化 skills + soul + memory seed，agent 对话时自动加载 per-tenant soul 和 memory 到 system prompt，实现每个租户拥有独立可定制的完整配置。

## Existing Infrastructure (Already Implemented, Not Wired)

| 模块 | 文件 | 状态 |
|------|------|------|
| Skill Provisioning | `internal/skills/provisioner.go` — `ProvisionTenantSkills()`, `ProvisionTenantSoul()`, `SyncAllTenantsSkills()` | 已实现，从未被调用 |
| Memory SystemPrompt | `internal/agent/memory_pg.go:137-155` — `PGMemoryProvider.SystemPromptBlock()` | 已实现，未接入 prompt |
| Default Soul | `internal/cli/default_soul.go` — `DefaultSoulMD` 常量 | 仅 CLI 模式使用 |

---

## Changes

### Part A: Agent Core — Soul & Memory 注入 System Prompt

#### A1. `internal/agent/types.go` — 新增 `WithSoulContent` option

在 `agent.go` 的 `AIAgent` struct 中添加 `soulContent string` 字段，并新增 option function：

```go
func WithSoulContent(content string) AgentOption {
    return func(a *AIAgent) { a.soulContent = content }
}
```

#### A2. `internal/agent/prompt.go` — 注入 soul + memory 到 system prompt

在 `buildSystemPrompt()` 中，`skipContextFiles` 分支后、skills 加载前插入：

```go
// Soul content (per-tenant, loaded from MinIO in SaaS mode)
if a.soulContent != "" {
    sb.WriteString("\n\n## Persona\n")
    sb.WriteString(a.soulContent)
}

// Memory context (from PG memory provider)
if a.memoryProvider != nil {
    if sp, ok := a.memoryProvider.(SystemPromptProvider); ok {
        if block := sp.SystemPromptBlock(); block != "" {
            sb.WriteString("\n\n")
            sb.WriteString(block)
        }
    }
}
```

定义 `SystemPromptProvider` 接口：

```go
type SystemPromptProvider interface {
    SystemPromptBlock() string
}
```

#### A3. `internal/agent/memory_providers.go` — pgMemoryAdapter 实现 SystemPromptProvider

```go
func (a *pgMemoryAdapter) SystemPromptBlock() string {
    return a.inner.SystemPromptBlock()
}
```

使 `pgMemoryAdapter` 同时满足 `tools.MemoryProvider` 和 `SystemPromptProvider` 接口。

---

### Part B: Tenant 创建时自动 Provisioning

#### B1. `internal/api/tenants.go` — 添加 provision callback

```go
type TenantHandler struct {
    store     store.TenantStore
    onCreated func(ctx context.Context, tenantID string)  // 新增
}

type TenantHandlerOption func(*TenantHandler)

func WithOnTenantCreated(fn func(ctx context.Context, tenantID string)) TenantHandlerOption {
    return func(h *TenantHandler) { h.onCreated = fn }
}

func NewTenantHandler(s store.TenantStore, opts ...TenantHandlerOption) *TenantHandler {
    h := &TenantHandler{store: s}
    for _, o := range opts { o(h) }
    return h
}
```

在 `create()` 方法中，DB 写入成功后异步触发：

```go
if h.onCreated != nil {
    go h.onCreated(context.Background(), t.ID)
}
```

#### B2. `internal/api/provisioner.go` — 新文件，编排 provisioning

```go
package api

type TenantProvisioner struct {
    mc         *objstore.MinIOClient
    bundledDir string
}

func NewTenantProvisioner(mc *objstore.MinIOClient, bundledDir string) *TenantProvisioner

func (p *TenantProvisioner) Provision(ctx context.Context, tenantID string) {
    // 1. skills.ProvisionTenantSkills(ctx, p.mc, tenantID, p.bundledDir)
    // 2. skills.ProvisionTenantSoul(ctx, p.mc, tenantID, "")
    // 3. SeedDefaultMemory (optional)
    // All errors logged but not fatal (async fire-and-forget)
}
```

#### B3. `internal/api/server.go` — 注入 provisioner 到 TenantHandler

```go
var tenantOpts []TenantHandlerOption
if cfg.SkillsClient != nil {
    prov := NewTenantProvisioner(cfg.SkillsClient, "skills")
    tenantOpts = append(tenantOpts, WithOnTenantCreated(prov.Provision))
}
tenantHandler := NewTenantHandler(cfg.Store.Tenants(), tenantOpts...)
api.Handle("/v1/tenants", tenantHandler)
api.Handle("/v1/tenants/", tenantHandler)
```

---

### Part C: Chat 时加载 Per-Tenant Soul

#### C1. `internal/api/mockchat.go` — runAgent 加载 soul from MinIO

在 `runAgent()` 中，`skillsClient` 可用时从 MinIO 读取 `{tenantID}/_soul/SOUL.md`：

```go
if h.skillsClient != nil {
    soulKey := tenantID + "/_soul/SOUL.md"
    if soulData, err := h.skillsClient.GetObject(ctx, soulKey); err == nil {
        opts = append(opts, agent.WithSoulContent(string(soulData)))
    }
}
```

---

### Part D: 启动时同步已有租户

#### D1. `cmd/hermes/saas.go` — 启动后台 goroutine

在 server 启动前，MinIO client 可用时执行一次全量同步：

```go
if skillsClient != nil {
    go func() {
        listFn := func(ctx context.Context) ([]string, error) {
            tenants, _, err := pgStore.Tenants().List(ctx, store.ListOptions{Limit: 1000})
            if err != nil { return nil, err }
            ids := make([]string, len(tenants))
            for i, t := range tenants { ids[i] = t.ID }
            return ids, nil
        }
        skills.SyncAllTenantsSkills(context.Background(), skillsClient, listFn, "skills")
    }()
}
```

---

## File Summary

| File | Action | Lines Changed |
|------|--------|--------------|
| `internal/agent/agent.go` | Add `soulContent string` field to AIAgent struct | ~1 line |
| `internal/agent/types.go` | Add `WithSoulContent` option function | ~5 lines |
| `internal/agent/prompt.go` | Inject soul + memory into `buildSystemPrompt()` | ~15 lines |
| `internal/agent/memory_providers.go` | Add `SystemPromptBlock()` to `pgMemoryAdapter` | ~3 lines |
| `internal/api/tenants.go` | Add `onCreated` callback + `TenantHandlerOption` pattern | ~20 lines |
| `internal/api/provisioner.go` | **NEW** — Orchestrate skill+soul+memory provisioning | ~60 lines |
| `internal/api/server.go` | Wire provisioner into TenantHandler construction | ~8 lines |
| `internal/api/mockchat.go` | Load soul from MinIO in `runAgent()` | ~5 lines |
| `cmd/hermes/saas.go` | Background startup sync goroutine | ~15 lines |

## Implementation Order

1. **A1-A3**: Agent core changes (soul + memory in system prompt)
2. **C1**: Chat handler loads soul from MinIO
3. **B1-B3**: Tenant creation auto-provisioning
4. **D1**: Startup sync (last, optional)

## Verification

1. 创建新租户 → 检查 MinIO 中 `{tenantID}/` 下自动出现 skills + `_soul/SOUL.md` + `.manifest.json`
2. 使用新租户 API key 发送 chat → 确认 `input_tokens` 显著高于裸 prompt（说明 soul+skills 已注入）
3. 检查 server 日志中出现 `provisioned tenant soul` 和 `tenant skill sync complete`
4. 修改租户 soul → 再次 chat → 确认 agent 人格变化
5. 运行 `scripts/test_web_isolation.sh` 确保已有功能不回归
