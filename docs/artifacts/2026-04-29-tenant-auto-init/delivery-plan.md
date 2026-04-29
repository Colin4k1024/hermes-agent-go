# Delivery Plan: Tenant Auto-Initialization

- **状态**: handoff-ready
- **日期**: 2026-04-29
- **主责**: tech-lead
- **来源 PRD**: docs/artifacts/2026-04-29-tenant-auto-init/prd.md

---

## 版本目标

- **里程碑**: v0.6.0 — Tenant Auto-Init + Full Agent HTTP API
- **范围**: 4 个 Story Slices（A/B/C/D），全部 P0
- **放行标准**: 全部 slice 测试通过 + CI green + `test_web_isolation.sh` 回归通过

## Brownfield 上下文快照

### 已存在基础设施

| 组件 | 文件 | 状态 |
|------|------|------|
| `APIServerAdapter` | `gateway/platforms/api_server.go` (279行) | 完整实现 PlatformAdapter，但 **未注册到 gateway runner** (`saas.go:191 _ = adapter`) |
| `PlatformAPI` 常量 | `gateway/types.go:33` | 已定义 `"api"` |
| `SystemPromptProvider` 接口 | `agent/memory_manager.go:42` | 已存在，`PGMemoryProvider` 已实现 |
| `pgMemoryAdapter` | `agent/memory_providers.go:42` | 代理 CRUD 方法，**缺少 `SystemPromptBlock()` 代理** |
| `DefaultSoulMD` | `cli/default_soul.go` | 可复用的默认 soul 常量 |
| `MinIOClient` | `objstore/minio.go` | PutObject/GetObject/ListObjects/ObjectExists 全部可用 |
| `MinIOSkillLoader` | `skills/loader_minio.go` | 按 `{tenantID}/{skillName}/SKILL.md` 扫描 |
| `SyncBuiltinSkills` | `tools/skills_sync.go:23` | 仅 FS→FS，**不可直接复用于 MinIO**，需新写 |
| `gateway.Runner` | `gateway/runner.go` | **SaaS 模式未创建**，仅 gateway 模式使用 |
| `config.Load()` | `config/config.go` | 已映射 `MINIO_*` 环境变量 → `cfg.MinIO.*` |
| Bundled skills | `skills/` (26 分类, 100+ SKILL.md) | 每个 ~2KB，叶子目录含 `SKILL.md` |

### getOrCreateAgent 当前 options (runner.go:540-573)

```
WithPlatform, WithSessionID, WithQuietMode(true),
WithSystemPrompt (if non-empty), WithTenantID, WithUserID,
WithMemoryProvider (if pgPool), WithSkillLoader (if minioClient)
```

**缺失**: `WithSoulContent`, `WithSkipContextFiles(true)`

### saas.go 当前 wiring gap

- 无 `gateway.Runner` 实例
- `APIServerAdapter` 创建后 `Connect()` 启动 HTTP 服务，但 `OnMessage` 未设置 → `EmitMessage` 无接收方
- `runner.Start()` 必须显式调用

---

## Story Slices

### S1: Agent Core — Soul + Memory 注入 buildSystemPrompt() [Part A]

**目标**: `AIAgent.buildSystemPrompt()` 注入 soul 内容和 memory 上下文，修复 gateway runner 路径的 soul 缺失。

**Owner**: backend-engineer

**文件变更**:

| 文件 | 改动 | 行数估算 |
|------|------|---------|
| `internal/agent/agent.go` | 添加 `soulContent string` 字段 | +1 |
| `internal/agent/types.go` | 添加 `WithSoulContent(string) AgentOption` | +5 |
| `internal/agent/prompt.go` | `buildSystemPrompt()` 中注入 soul (`## Persona`) 和 memory (`SystemPromptProvider`) | +15 |
| `internal/agent/memory_providers.go` | `pgMemoryAdapter` 添加 `SystemPromptBlock()` 代理 | +3 |

**注入位置**: `prompt.go` 第 100 行后（context files 之后、skills 加载之前）:

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

**验收标准**:
- [x] `buildSystemPrompt()` 包含 `## Persona` 块当 `soulContent` 非空
- [x] `buildSystemPrompt()` 包含 memory 块当 `memoryProvider` 实现 `SystemPromptProvider`
- [x] `ephemeralSystemPrompt` 路径不受影响（提前返回在第 57 行）
- [x] 单元测试覆盖: soul 注入、memory 注入、两者同时、两者皆空
- [x] `go test ./internal/agent/... -race` 通过

**依赖**: 无

---

### S2: Tenant Provisioner [Part B]

**目标**: 新建 `provisioner.go`，实现租户创建时自动上传 soul + skills 到 MinIO。

**Owner**: backend-engineer

**文件变更**:

| 文件 | 改动 | 行数估算 |
|------|------|---------|
| `internal/skills/provisioner.go` | **新建** — `Provisioner` struct + `Provision`/`ProvisionSoul`/`ProvisionSkills`/`SyncAllTenants` | +120 |
| `internal/api/tenants.go` | `TenantHandler` 添加 `onCreated` callback + `TenantHandlerOption` | +20 |
| `internal/api/server.go` | 注入 provisioner → TenantHandler wiring | +10 |

**Provisioner 设计**:

```go
type Provisioner struct {
    minio      *objstore.MinIOClient
    bundledDir string
}

func (p *Provisioner) Provision(ctx context.Context, tenantID string) error
// 1. ProvisionSoul: PutObject("{tenantID}/SOUL.md", DefaultSoulMD) — skip if exists
// 2. ProvisionSkills: walk bundledDir, PutObject("{tenantID}/{cat}/{name}/SKILL.md") — skip if exists

func (p *Provisioner) SyncAllTenants(ctx context.Context, tenantStore store.TenantStore) error
// List all tenants, Provision each (idempotent)
```

**关键设计点**:
- Soul key: `{tenantID}/SOUL.md`（与 `mockchat.go:254` 和 `api_server.go` 读取路径对齐）
- Skills key: `{tenantID}/{category}/{skillName}/SKILL.md`（与 `MinIOSkillLoader:33` 扫描模式对齐）
- 幂等: `ObjectExists` 检查后跳过已存在文件，不覆盖用户定制
- 异步: `TenantHandler.create()` DB 写入成功后 `go h.onCreated(context.Background(), t.ID)`
- 错误: 仅 `slog.Error`，不影响租户创建 API 响应

**验收标准**:
- [x] 创建新租户后 MinIO 中 `{tenantID}/SOUL.md` 存在
- [x] 创建新租户后 MinIO 中 `{tenantID}/` 下存在 bundled skills
- [x] 已存在文件不被覆盖（幂等性）
- [x] API 响应不被 provisioning 阻塞
- [x] 单元测试: provisioner 逻辑 + tenants handler callback
- [x] `go test ./internal/skills/... ./internal/api/... -race` 通过

**依赖**: 无（与 S1 并行）

---

### S3: SaaS Gateway Runner Wiring [Part C]

**目标**: SaaS 模式创建 gateway.Runner，注册 `APIServerAdapter`，使 HTTP chat 走完整 `AIAgent` 路径。

**Owner**: backend-engineer

**文件变更**:

| 文件 | 改动 | 行数估算 |
|------|------|---------|
| `cmd/hermes/saas.go` | 创建 Runner + 注册 adapter + 替换 `_ = adapter` | +25 |
| `internal/gateway/runner.go` | `getOrCreateAgent()` 添加 soul 加载 + `WithSkipContextFiles(true)` | +15 |

**saas.go wiring 变更**:

```go
// 替换现有 saas.go:177-193 的 adapter 代码块:
if apiKey != "" {
    gwCfg := gateway.DefaultGatewayConfig()
    runner := gateway.NewRunner(gwCfg, pgStore.Pool())

    adapter := platforms.NewAPIServerAdapter(adapterPort, apiKey)
    runner.RegisterAdapter(adapter)

    go func() {
        if err := runner.Start(); err != nil {
            slog.Error("Gateway runner error", "error", err)
        }
    }()
}
```

**runner.go getOrCreateAgent() 增强**:

```go
// 在现有 WithSkillLoader 之后添加:
if r.minioClient != nil {
    soulKey := tenantID + "/SOUL.md"
    if soulData, err := r.minioClient.GetObject(ctx, soulKey); err == nil && len(soulData) > 0 {
        opts = append(opts, agent.WithSoulContent(string(soulData)))
    }
}
opts = append(opts, agent.WithSkipContextFiles(true))
```

**验收标准**:
- [x] `POST /v1/chat/completions` 走 `AIAgent.RunConversation()` 路径
- [x] Agent 可调用 memory_read/memory_save/memory_delete 工具
- [x] Agent 可调用 skills_list/skill_view 工具
- [x] Agent system prompt 包含 soul + memory + skills
- [x] 多租户并发 chat memory/skills 隔离正确
- [x] `scripts/test_web_isolation.sh` 回归通过

**依赖**: S1（agent 需要 `WithSoulContent` option）

---

### S4: Startup Sync [Part D]

**目标**: 服务启动时后台同步已有租户 soul + skills。

**Owner**: backend-engineer

**文件变更**:

| 文件 | 改动 | 行数估算 |
|------|------|---------|
| `cmd/hermes/saas.go` | 添加后台 goroutine 调用 `provisioner.SyncAllTenants` | +15 |

**实现**:

```go
// saas.go — 在 server 启动前，MinIO client 可用时:
if skillsClient != nil {
    prov := skills.NewProvisioner(skillsClient, "skills")
    // Wire into TenantHandler for new tenant provisioning
    tenantOpts = append(tenantOpts, api.WithOnTenantCreated(prov.Provision))
    // Background sync for existing tenants
    go func() {
        if err := prov.SyncAllTenants(context.Background(), pgStore.Tenants()); err != nil {
            slog.Error("startup tenant sync failed", "error", err)
        }
    }()
}
```

**验收标准**:
- [x] 启动后已有租户 MinIO 补齐缺失 soul + skills
- [x] 已存在文件不被覆盖
- [x] 不阻塞 HTTP server 启动
- [x] 日志输出 sync 进度和结果

**依赖**: S2（需要 provisioner 函数）

---

## 依赖关系与执行顺序

```
S1 (Agent Core) ──────────────┐
    ‖ 并行                     ├──→ S3 (Gateway Wiring) ──→ 集成测试
S2 (Provisioner) ─────────────┤
                               └──→ S4 (Startup Sync) ──→ 回归测试
```

- **S1 ‖ S2**: 无依赖，可并行
- **S3 → S1**: gateway runner 需要 `WithSoulContent` option
- **S4 → S2**: startup sync 需要 provisioner 函数
- **集成测试**: S3 + S4 完成后端到端验证

## 角色分工

| 角色 | 职责 | 交接 |
|------|------|------|
| tech-lead | 计划锁定、冲突仲裁、放行决策 | → backend-engineer |
| backend-engineer | S1-S4 全部实现 + 单元测试 | → qa-engineer |
| qa-engineer | 回归验证 + 端到端测试 | → tech-lead |

## 风险与缓解

| 风险 | 等级 | 缓解 |
|------|:----:|------|
| 100+ skills 全量上传到 MinIO 耗时 | 中 | 幂等跳过已存在；后台异步不阻塞 API |
| agent 缓存中 soul 过期 | 低 | v1 接受：soul 变更需重启或 session 过期；v2 加 invalidation |
| `SyncBuiltinSkills` (FS→FS) 不可复用 | 低 | 已确认需新写 MinIO 版本，估算已计入 S2 |
| gateway runner 在 SaaS 模式的 session 管理 | 中 | 复用 `PGSessionStore` (runner.go:67)；pgPool 已可用 |
| `APIServerAdapter` auth 仅单一 API key | 低 | 现阶段 POC 够用；生产化需接入 SaaS auth chain |

## 技术债登记

| 项目 | 来源 | 处置 |
|------|------|------|
| 两套 prompt 路径分裂 (`buildSystemPrompt` vs `mockchat.callLLM`) | 挑战会 Challenge 3 | 记入 backlog，后续统一 |
| startup sync 硬编码 `Limit: 1000` 无分页 | 挑战会 Challenge 6 | 记入 backlog，生产化时改 |
| `APIServerAdapter` 单 API key 鉴权 | 安全审查 | 后续接入 SaaS auth middleware |

## ADR 需求

不需要独立 ADR — 关键决策已记录在 PRD 挑战会结论中（ADR-1/2/3）。

## 技能装配清单

| 类型 | 技能 | 触发原因 | 主责角色 |
|------|------|---------|---------|
| shared | `golang-patterns` | Go 惯用模式 | backend-engineer |
| shared | `golang-testing` | 表驱动测试 | backend-engineer |
| ECC | `go-review` | 代码审查 | code-reviewer |
| ECC | `go-build` | CI 修复 | build-error-resolver |

## 前端交付物

- 不涉及前端变更。

## Implementation-Readiness 结论

| 检查项 | 状态 |
|--------|------|
| 需求挑战会完成 | ✅ 3 项计划错误已修正，Part A 恢复 P0，C 重定义 |
| 设计收口 | ✅ architect 方案确认，4 个 data flow 已验证 |
| brownfield 上下文 | ✅ 全部关键文件已读取验证 |
| story slices 拆分 | ✅ 4 个 slice，依赖关系明确 |
| 风险登记 | ✅ 5 项风险 + 3 项技术债 |
| 放行前提 | CI green + `test_web_isolation.sh` + 多租户隔离验证 |

**结论**: Handoff-ready，可进入 `/team-execute`。

---

*已创建 `docs/artifacts/2026-04-29-tenant-auto-init/delivery-plan.md`*
