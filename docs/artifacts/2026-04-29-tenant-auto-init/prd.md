# PRD: Tenant Auto-Initialization (Soul, Memory & Skills Provisioning)

- **状态**: challenge-complete
- **日期**: 2026-04-29
- **主责**: tech-lead
- **来源**: docs/tenant-auto-init-plan.md

---

## 背景

新租户创建后缺乏自动初始化机制：

1. **Skills** — 无自动 seed 流程，需手动执行脚本上传到 MinIO。
2. **Soul (人格)** — SaaS 模式下 `skipContextFiles=true` 跳过本地文件系统，`SOUL.md` 不加载。
3. **Memory** — `PGMemoryProvider.SystemPromptBlock()` 已实现（`memory_pg.go:137-155`），但 `AIAgent.buildSystemPrompt()` 未注入。

**核心矛盾**: SaaS 唯一的 HTTP API chat 路径 (`mockchat.go`) 直接调用 LLM HTTP 接口，绕过了 `AIAgent` 的完整工具调用循环。无法测试 memory tools、skills 管理、code exec、browser 等任何内置能力。

| Chat 路径 | 完整 Agent? | Soul | Memory | Skills | 工具调用 | HTTP API? |
|-----------|:---------:|:----:|:------:|:------:|:-------:|:---------:|
| `cli/app.go` | ✅ | 本地 FS | Builtin | 本地 FS | 全部 | 否 |
| `gateway/runner.go` | ✅ | **缺失** | PG ✅ | MinIO ✅ | 全部 | 否 (仅 Slack/WeCom) |
| `mockchat.go` | **否** | MinIO ✅ | PG ✅ | MinIO ✅ | **无** | 是 |

**目标**: 实现 HTTP API → 完整 `AIAgent` → 全部内置能力（memory tools, skill evolution, browser, code exec...）的端到端通路，并在租户创建时自动初始化 soul + skills。

## 目标与成功标准

### 业务目标

1. 租户创建时自动初始化 skills + soul 到 MinIO
2. SaaS HTTP API 使用完整 `AIAgent` 能力（工具调用、memory、skills）
3. 每个租户拥有独立可定制的 soul、memory 和 skills

### 成功指标

1. POST `/v1/tenants` 后，MinIO 中 `{tenantID}/SOUL.md` + skills 自动出现
2. 通过 HTTP API 发送 chat，agent 可调用 memory 工具存储/读取记忆
3. 通过 HTTP API 发送 chat，agent 可列出和使用已安装的 skills
4. 修改租户 SOUL.md 后再次 chat，agent 人格变化
5. 多租户并发 chat，memory 和 skills 隔离正确
6. `scripts/test_web_isolation.sh` 回归通过

## 用户故事

### US-1: 管理员创建租户自动初始化
**作为** SaaS 管理员，**我希望** 通过 API 创建租户后自动完成 skills + soul provisioning，**以便** 新租户立即可用。

**验收标准**:
- POST `/v1/tenants` 成功后，MinIO `{tenantID}/SOUL.md` 存在
- MinIO `{tenantID}/` 下存在 bundled skills（`skills/` 目录，排除 `optional-skills/`）
- API 响应不被 provisioning 阻塞（异步执行）

### US-2: Agent core 注入 soul + memory 到 system prompt
**作为** 通过任意路径使用 `AIAgent` 的开发者，**我希望** soul 和 memory 自动注入 system prompt，**以便** agent 拥有人格和记忆上下文。

**验收标准**:
- `buildSystemPrompt()` 输出包含 soul 内容（`## Persona` 块）
- `buildSystemPrompt()` 输出包含 memory 内容（通过 `SystemPromptProvider` 接口）
- 不破坏 `ephemeralSystemPrompt` 路径
- `gateway/runner.go` 路径受益（当前缺少 soul 注入）

### US-3: SaaS HTTP API 使用完整 AIAgent
**作为** 需要端到端测试 Hermes Agent 全部能力的开发者，**我希望** SaaS HTTP chat 端点走完整的 `AIAgent.RunConversation()` 路径，**以便** 测试 memory tools、skill evolution、code exec 等全部内置工具。

**验收标准**:
- 新建 HTTP platform adapter（`PlatformAPI = "api"` 已定义）注册到 gateway runner
- HTTP chat 请求通过 gateway runner 的 `processWithAgent()` → `AIAgent.RunConversation()` → 完整工具调用循环
- 支持 per-tenant agent 缓存（复用 `getOrCreateAgent` 模式）
- agent 可调用 memory_read/memory_save/memory_delete 工具
- agent 可调用 skills_list/skill_view/skill_manage 工具
- `mockchat.go` 保留作为轻量兼容路径

### US-4: 启动时同步已有租户
**作为** 运维人员，**我希望** 服务启动时为已有租户补齐缺失的 soul + skills。

**验收标准**:
- 后台 goroutine 遍历所有租户并同步
- 已存在的文件不被覆盖（仅补缺）
- 不阻塞 HTTP server 启动

## 范围

### In Scope

| Part | 优先级 | 描述 | 涉及文件 |
|------|:-----:|------|----------|
| A | P0 | `buildSystemPrompt()` 注入 soul + memory | `agent.go`, `types.go`, `prompt.go`, `memory_providers.go` |
| B | P0 | 新建 `provisioner.go` + TenantHandler callback + wiring | `skills/provisioner.go` (新), `api/tenants.go`, `api/server.go` |
| C | P0 | 新建 HTTP platform adapter，注册到 gateway runner | `gateway/platforms/api_adapter.go` (新), `cmd/hermes/saas.go` |
| D | P0 | 启动时同步已有租户 soul + skills | `cmd/hermes/saas.go` |

### Out of Scope

| 项目 | 原因 |
|------|------|
| Memory seed（自动写入初始 memory 记录） | AIAgent 的 memory tools 可运行时写入 |
| 租户删除时清理 MinIO 资源 | 后续迭代 |
| Soul/Skills 管理 REST API（CRUD） | 超出本次范围 |
| 统一两套 prompt 构建路径 | 架构重构，记入 backlog 技术债 |
| `mockchat.go` 废弃 | 保留作为轻量兼容路径 |

## 需求挑战会结论 (2026-04-29)

### 计划文档 vs 代码差异（已修正）

| 计划声称 | 实际状态 | 修正 |
|----------|---------|------|
| `skills/provisioner.go` 已实现 3 个函数 | **文件不存在** | 需从零编写 |
| Soul key path: `{tenantID}/_soul/SOUL.md` | `mockchat.go` 读 `{tenantID}/SOUL.md` | **统一为 `{tenantID}/SOUL.md`** |
| `SystemPromptProvider` 接口需新建 | 已存在于 `memory_manager.go:42` | 仅需 adapter delegation |
| `mockchat.go` 需加 soul 加载 | `getSoulPrompt()` 已实现 | 跳过 |

### 架构决策（挑战会确认）

**ADR-1: SaaS HTTP chat 路径采用 gateway runner + HTTP platform adapter**

- `PlatformAPI Platform = "api"` 已在 `gateway/types.go:33` 定义
- 新建 `gateway/platforms/api_adapter.go` 实现 `PlatformAdapter` 接口
- HTTP 请求 → `MessageEvent` → gateway runner `processWithAgent()` → `AIAgent.RunConversation()` → 完整工具调用
- 复用 `getOrCreateAgent` 的 agent 缓存 + session 管理
- `mockchat.go` 保留不动，作为轻量兼容路径
- 理由：架构最干净，无代码重复，全部 platform 统一走 gateway 路由

**ADR-2: Part A 恢复 P0**

- `gateway/runner.go:540-573` 已使用 `AIAgent` + `WithMemoryProvider` + `WithSkillLoader`，但 `buildSystemPrompt()` 无 soul/memory 注入 — 这是 gateway 路径的实质缺陷
- 修复后所有使用 `AIAgent` 的路径（CLI、gateway、batch）都受益

**ADR-3: Skills 全量 provisioning（`skills/` 目录）**

- 用户需端到端测试 skill evolution、skill manage 等全部内置能力
- 仅上传 `skills/` 目录，排除 `optional-skills/`
- `MinIOSkillLoader` 已按 `{tenantID}/{skillName}/SKILL.md` 格式扫描

## 风险与依赖

| 风险 | 等级 | 缓解 |
|------|:----:|------|
| HTTP adapter 需实现 `PlatformAdapter` 接口（Send/Connect/Disconnect 等） | 中 | 参考 `platforms/slack.go` 模式；HTTP adapter 的 Connect/Disconnect 为 no-op |
| Agent 实例资源开销（每个 session 一个） | 中 | 复用 `runner.go` 的 agent cache + session expiry watcher |
| `skills/` 全量上传数据量（100+ skills） | 中 | 仅首次 provision；后续 startup sync 仅补缺 |
| `provisioner.go` 从零编写 | 中 | MinIO `PutObject`/`ListObjects`/`ObjectExists` API 已就绪 |
| Fire-and-forget provisioning 失败仅 log | 低 | startup sync 兜底 + `slog.Error` 可接入告警 |
| 两套 prompt 路径长期分裂 | 低 | 记入 backlog 技术债；本轮通过 Part A 缩小差距 |

## 待确认项（已在挑战会收敛）

| # | 问题 | 结论 |
|---|------|------|
| 1 | Memory seed 本轮是否需要？ | 不需要 — AIAgent memory tools 运行时写入 |
| 2 | bundled skills 路径？ | `skills/` 目录（排除 `optional-skills/`） |
| 3 | startup sync 频率？ | 仅启动时一次 |

## 实施顺序

1. **Part A**: Agent core — `WithSoulContent` + `buildSystemPrompt()` 注入 soul + memory
2. **Part B**: Provisioner — 新建 `provisioner.go` + TenantHandler callback + server wiring
3. **Part C**: HTTP Platform Adapter — 新建 `api_adapter.go` + 注册到 gateway runner + saas.go wiring
4. **Part D**: Startup sync — 后台 goroutine 同步已有租户

依赖关系: A → C（adapter 创建 AIAgent 需要 soul 注入能力），B → D（sync 需要 provisioner 函数）。A 和 B 可并行。

---

*PRD 更新于 2026-04-29，挑战会后修订版*
