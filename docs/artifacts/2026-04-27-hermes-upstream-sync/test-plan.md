# Test Plan: Upstream Sync v0.11.0 + SaaS Stateless MVP

| 字段 | 值 |
|------|-----|
| 日期 | 2026-04-27 |
| QA 角色 | qa-engineer |
| 范围 | 3 commits, 80 files, +8,731 lines |
| 状态 | review |

---

## 测试范围

### 功能范围
1. **Transport Layer** — 5 transport 实现（OpenAI/Anthropic/Bedrock/Gemini/Codex）
2. **Security** — 5 安全模块（reasoning guard/exfiltration/git injection/credential guard/tool validation）
3. **Agent Loop** — 4 增强（empty recovery/steer/heartbeat/context pressure）
4. **Gateway** — 13 platform adapters
5. **Plugin/Skills** — lifecycle hooks, memory providers, URL install, guard wiring
6. **TUI** — Bubble Tea model
7. **Dashboard** — embedded SPA + API endpoints
8. **SaaS Stateless** — Store interface, PG/SQLite dual impl, Redis client, AgentFactory, ContextLoader
9. **Concurrency** — atomic.Bool, WaitGroup timeout, channel safety

### 非功能范围
- 并发安全：`go test -race` 全通过
- 编译：`go build ./...` 零错误
- 回归：14 packages 全通过

### 不覆盖项
- E2E 集成测试（需真实 PG/Redis/LLM endpoint）
- 平台 adapter 真实连接测试（需 bot token）
- 性能基准测试（延迟/吞吐量）

---

## 测试矩阵

| 场景 | 类型 | 前置条件 | 预期结果 | 状态 |
|------|------|---------|---------|------|
| Transport 接口定义编译 | 单元 | - | 5 transport 实现 `llm.Transport` | ✅ Pass |
| ToolDef → openai.Tool 转换 | 单元 | - | 字段正确映射 | ✅ Pass |
| ToolDef → anthropicTool 转换 | 单元 | - | InputSchema 正确映射 | ✅ Pass |
| ChatRequest.Tools 类型迁移 | 回归 | - | 所有现有测试通过 | ✅ Pass |
| Reasoning guard 剥离 | 单元 | - | 原始消息不变，返回副本 | ✅ Pass |
| Exfiltration URL token 检测 | 单元 | - | sk-/ghp_/AKIA 在 URL 中检出 | ✅ Pass |
| Exfiltration base64 检测 | 单元 | - | base64 编码密钥检出 | ✅ Pass |
| Exfiltration prompt injection 检测 | 单元 | - | 注入模式检出 | ✅ Pass |
| Git injection 检测 | 单元 | - | --upload-pack/ext:: 等模式检出 | ✅ Pass |
| Credential path 阻断 | 单元 | - | .ssh/.aws/.docker 路径被阻断 | ✅ Pass |
| Tool call truncation 检测 | 单元 | - | 未闭合 JSON 被识别 | ✅ Pass |
| Context pressure 分级 | 单元 | - | 50%/70%/85%/95% 阈值正确 | ✅ Pass |
| atomic.Bool interrupt | 竞态 | -race | 无竞态告警 | ✅ Pass |
| WaitGroup tool timeout | 竞态 | -race | 5min 超时后返回错误 | ✅ Pass |
| ClearSession channel 安全 | 竞态 | -race | 无 panic | ✅ Pass |
| SSE DroppedEvents 计数 | 单元 | - | 丢事件时计数递增 | ✅ Pass |
| Store interface PG 编译 | 编译 | - | PGStore 实现 Store 接口 | ✅ Pass |
| Store interface SQLite 编译 | 编译 | - | SQLiteStore 实现 Store 接口 | ✅ Pass |
| AgentFactory 编译 | 编译 | - | Run() 方法签名正确 | ✅ Pass |
| ContextLoader 窗口计算 | 单元 | - | >50 条时 offset 正确 | ✅ Pass |
| 全量回归 | 回归 | - | 14 packages 0 failures | ✅ Pass |

---

## 风险

### 阻塞项（来自 Security Review）

| ID | 严重度 | 问题 | 影响 |
|----|--------|------|------|
| H1 | HIGH | Dashboard `/api/config` POST 无认证 | SSRF 可修改配置 |
| H2 | HIGH | WeCom callback 无签名验证 | 消息注入 |
| H3 | HIGH | Redis lock DEL 无 owner 验证 | 幻影释放 |

### 非阻塞风险

| ID | 严重度 | 问题 |
|----|--------|------|
| M1 | MEDIUM | Rate limiter Redis 故障时 fail-open |
| M2 | MEDIUM | Tenant ID 无格式校验 |
| M3 | MEDIUM | Dashboard CORS `*` 过于宽松 |
| M4 | MEDIUM | reasoning_guard 未使用 targetProvider 参数 |

### 缺测路径

| 文件 | 缺失测试 |
|------|---------|
| 13 gateway adapters | 无单元测试（需 mock SDK） |
| `internal/store/pg/` | 需 PG 集成测试 |
| `internal/store/rediscache/` | 需 Redis 集成测试 |
| `internal/agent/factory.go` | 需 mock Store 测试 |
| `internal/tui/app.go` | 需 bubbletea test helpers |
| `internal/dashboard/server.go` | 需 HTTP handler 测试 |
| `internal/plugins/hooks.go` | 需 hook 注册/触发测试 |
| `internal/plugins/memory_*.go` | 需 mock HTTP 测试 |

---

## 放行建议

**结论：有条件放行（Conditional Pass）**

- ✅ 全量编译通过
- ✅ 全量测试通过（14 packages, 0 failures）
- ✅ Race detector 零告警
- ⚠️ 3 个 HIGH 安全问题需修复后放行
- ⚠️ ~10 个新模块缺少单元测试（需后续补齐）

**建议：** 修复 H1/H2/H3 后可放行。M1-M4 和缺测路径作为后续 sprint 跟踪。
