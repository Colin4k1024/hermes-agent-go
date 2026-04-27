# PRD: Hermes Agent Go — 上游同步 (v0.11.0+)

| 字段 | 值 |
|------|-----|
| Slug | `hermes-upstream-sync` |
| 日期 | 2026-04-27 |
| 主责 | tech-lead |
| 状态 | draft |
| 阶段 | intake |

---

## 背景

当前 hermes-agent-go 是对上游 Python 项目 [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent) 的 Go 重写。上游已发展到 v0.11.0（2026-04-23 发布，1,556 commits，761 merged PRs），引入了大量新特性。Go 实现已具备完整的 agent loop、工具系统、LLM 双模式客户端、MCP、网关骨架等核心能力，但与上游在以下维度存在显著差距。

**触发原因：** 上游 v0.11.0 引入了 pluggable transport layer、20+ LLM provider、17 messaging platform、TUI、Web Dashboard、plugin 增强等重大变更，Go 端需要追赶。

**当前约束：**
- Go 项目已有 ~64 tool 文件、63 test 文件，架构基本稳定
- 上游是 Python 实现，Go 端需按 Go idiom 重新设计而非直接翻译
- 部分上游特性依赖 Python 生态（React/Ink TUI、Atropos RL 环境），需评估是否移植

---

## 目标与成功标准

### 业务目标
- Go 实现达到与上游 v0.11.0 的功能对等（核心路径）
- 补齐 LLM provider、gateway platform、安全加固等关键差距

### 用户价值
- 用户可通过 Go 版本接入更多 LLM provider（Bedrock、Gemini、xAI 等）
- 用户可在更多平台使用 agent（Telegram、Discord、Slack、WeChat 等）
- 更强的安全保护（reasoning leak blocking、secret exfiltration blocking 等）

### 成功指标
- P0 差距项全部实现且有测试覆盖
- P1 差距项完成 ≥80%
- 所有已有测试继续通过
- 新增特性测试覆盖率 ≥80%

---

## 差异分析与用户故事

### P0 — 核心能力差距（必须补齐）

#### 1. Transport Layer 抽象（上游 v0.11.0 新增）

**差距：** 上游新增 `agent/transports/` 可插拔传输层，将 LLM 通信从 agent loop 中解耦。Go 端 LLM 调用硬编码在 `llm/client.go` 中。

**用户故事：**
- 作为开发者，我希望新增 LLM provider 时只需实现一个 Transport 接口，不需要修改 agent loop
- 验收标准：定义 `Transport` 接口；OpenAI、Anthropic 各实现一个；Bedrock 和 Codex 预留

**涉及模块：** `internal/llm/` → 新增 `internal/llm/transports/`

#### 2. 新增 LLM Provider 支持

**差距：** Go 仅支持 OpenAI-compatible + Anthropic direct。上游支持 20+ provider。

**P0 Provider（用户需求量最大）：**

| Provider | 上游文件 | Go 现状 | 优先级 |
|----------|---------|---------|--------|
| AWS Bedrock | `bedrock_adapter.py` + `transports/bedrock.py` | 无 | P0 |
| Google Gemini (AI Studio) | `gemini_native_adapter.py` | 无 | P0 |
| OpenAI Codex/Responses API | `codex_responses_adapter.py` + `transports/codex.py` | 无 | P0 |
| xAI (Grok) | provider routing | 无 | P1 |
| Ollama (本地) | provider routing | 无 | P1 |
| Kimi/Moonshot | `moonshot_schema.py` | 无 | P1 |
| MiniMax | provider routing | 无 | P2 |
| Hugging Face | provider routing | 无 | P2 |
| NVIDIA NIM | provider routing | 无 | P2 |

**用户故事：**
- 作为 AWS 用户，我希望直接使用 Bedrock 的 Claude/Llama 而不需要额外的 API key 中转
- 作为本地部署用户，我希望用 Ollama 运行本地模型
- 验收标准：Bedrock Converse API 集成可用；Gemini native 集成可用；Codex Responses API 可用

#### 3. Gateway Platform 扩展

**差距：** Go 仅有 DMWork adapter。上游支持 17 个平台。

**P0 Platform：**

| Platform | 优先级 | 理由 |
|----------|--------|------|
| Telegram | P0 | 全球用户量最大的 bot 平台 |
| Discord | P0 | 开发者社区核心平台 |
| Slack | P0 | 企业场景刚需 |
| API Server (OpenAI-compatible) | P0 | 编程接入的标准方式 |
| WeChat/Weixin | **P0** | 中国市场核心（已确认提升） |
| DingTalk | **P0** | 中国企业市场（已确认提升） |
| Feishu/Lark | **P0** | 中国企业市场（已确认提升） |
| WeCom | **P0** | 中国企业市场（已确认提升） |
| WhatsApp | P1 | 全球用户量大 |
| Email | P2 | 通用场景 |
| Matrix | P2 | 开源社区 |

**用户故事：**
- 作为 Telegram 用户，我希望在 Telegram 中直接与 Hermes agent 对话
- 作为开发者，我希望通过 OpenAI-compatible API 接入 Hermes
- 验收标准：Telegram webhook/polling 可用；Discord bot 可用；Slack bot 可用；API Server /v1/chat/completions 可用

#### 4. 安全加固

**差距：** Go 端缺少多项上游安全特性。

| 安全特性 | 上游 | Go 现状 | 优先级 |
|----------|------|---------|--------|
| Cross-provider reasoning leak blocking | ✅ | ❌ | P0 |
| Secret exfiltration blocking (URL/base64/injection scanning) | ✅ | ❌ | P0 |
| Git argument injection prevention | ✅ | ❌ | P0 |
| Credential directory protection | ✅ | ❌ | P0 |
| API bind guard (non-loopback) | ✅ | ❌ | P0 |
| Per-process UUID session keys | ✅ | ❌ | P1 |
| Atomic provider config writes | ✅ | ❌ | P1 |
| MCP OAuth 2.1 PKCE | ✅ | ❌ | P1 |
| WhatsApp identity path traversal guard | ✅ | ❌ | P1 |

**用户故事：**
- 作为用户，我的 API key 不应该通过工具调用结果被泄漏到外部
- 验收标准：reasoning 内容不跨 provider 泄漏；工具结果经过 exfiltration scanning；git 命令参数注入被阻断

#### 5. Agent Loop 增强

**差距：** 上游新增多项 agent loop 特性。

| 特性 | 描述 | 优先级 |
|------|------|--------|
| `/steer <prompt>` | 运行中注入新指令而不打断 prompt cache | P0 |
| Activity heartbeats | 防止 gateway 误判 agent 不活跃 | P0 |
| Auto-continue after restart | gateway 重启后自动恢复进行中的任务 | P1 |
| Thinking-only prefill continuation | 结构化推理的延续 | P1 |
| Truncated tool call detection | 执行前检测截断的 tool call | P0 |
| Empty response recovery | 空响应自动重试+nudge | P0 |
| Per-turn primary runtime restoration | fallback 后恢复主 provider | P1 |
| Tiered context pressure warnings | 分层上下文压力告警 | P1 |

### P1 — 重要增强

#### 6. Browser 工具完善

**差距：** Go 有 stub 实现，上游有完整的 browser_use、browserbase、camofox、CDP passthrough。

**用户故事：**
- 作为用户，我希望 agent 能真正操作浏览器完成网页任务
- 验收标准：至少 browserbase 后端完全可用；CDP passthrough 可用

#### 7. Plugin 系统增强

**差距：** Go 有基础 plugin 系统，上游新增了 LLM call hooks、tool result transform、shell hooks 等。

| Hook | 上游 | Go 现状 |
|------|------|---------|
| pre_tool_call / post_tool_call | ✅ | ❌ |
| transform_tool_result | ✅ | ❌ |
| transform_terminal_output | ✅ | ❌ |
| pre_llm_call / post_llm_call | ✅ | ❌ |
| register_command | ✅ | ❌ |
| on_session_start / on_session_end | ✅ | ❌ |
| Shell hooks | ✅ | ❌ |

#### 8. Skills 系统增强

| 特性 | 上游 | Go 现状 |
|------|------|---------|
| Skills Hub (agentskills.io) | ✅ | ❌ |
| Install from HTTP(S) URL | ✅ | ❌ |
| Autonomous skill creation | ✅ | ❌ |
| Skills self-improve | ✅ | ❌ |
| Namespaced registration | ✅ | ❌ |
| Skills guard | ✅ | ❌ |

#### 9. 新增工具

| 工具 | 描述 | 优先级 |
|------|------|--------|
| Discord tool | Discord 特定操作 | P1 |
| Feishu doc/drive | 飞书文档/云盘 | P1 |
| Browser CDP | DevTools Protocol passthrough | P1 |
| Browser dialog | 浏览器对话框处理 | P1 |
| Managed Tool Gateway | Nous Tool Gateway 集成 | P1 |
| Transcription (STT) | 语音转文字 | P2 |
| Voice mode | 语音对话模式 | P2 |
| File state coordination | 跨 agent 文件状态协调 | P1 |

#### 10. Auxiliary 特性增强

| 特性 | 描述 | 优先级 |
|------|------|--------|
| models.dev registry | 动态模型元数据解析 | P1 |
| Fast Mode | OpenAI/Anthropic 快速通道 | P1 |
| Debug share (paste.rs) | 上传调试报告 | P2 |
| Backup/import | 完整备份/恢复 | P2 |
| Self-update | 自我更新 | P2 |

### P2 — 可选 / 后续批次

#### 11. Web Dashboard（已确认纳入 → Phase 6）
- 浏览器本地管理界面，i18n，主题，session/skills/gateway 管理
- **决策：** 纳入范围，前端技术栈待定（Go template / embedded SPA / 独立前端）

#### 12. Bubble Tea TUI（已确认采用 → Phase 5）
- 用 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 替代上游 React/Ink TUI
- 纯 Go 实现，sticky composer、live streaming、model picker、slash command autocomplete

#### 13. ~~RL Training 环境增强~~（已确认排除）
- 不是当前项目目标，从范围中移除

#### 14. 更多 Gateway 平台（P2，Phase 6+）
- WhatsApp、Signal、Matrix、Email、SMS、Mattermost、BlueBubbles、QQBot 等

---

## 范围

### In Scope
- P0 全部：Transport Layer、核心 LLM Provider（Bedrock/Gemini/Codex）、核心 Gateway（Telegram/Discord/Slack/API Server）、安全加固、Agent Loop 增强
- P1 按优先级择取：Browser 完善、Plugin 增强、Skills 增强、新增工具

### Out of Scope（本批次）
- RL Training 环境（已确认不是当前目标）
- Camofox 反检测浏览器（已确认不需要）
- 全部 P2 Gateway 平台（Email、Matrix 等）

### 新增 In Scope（基于确认）
- Web Dashboard（已确认纳入，前端技术栈待定）
- Bubble Tea TUI（替代 React/Ink）
- 中国市场 Gateway 平台提升至 P0（WeChat/DingTalk/Feishu/WeCom）
- Plugin 内存 provider（Honcho/mem0/supermemory）

---

## 风险与依赖

| 风险 | 影响 | 缓解 |
|------|------|------|
| 上游 Python 特性依赖 Python 生态（asyncio、React/Ink） | 部分特性无法直译 | 按 Go idiom 重新设计接口和并发模型 |
| Gateway 平台 SDK 质量参差不齐 | 部分平台 Go SDK 不完善 | 优先选择有成熟 Go SDK 的平台 |
| Bedrock/Gemini 需要额外认证配置 | 增加配置复杂度 | 提供 setup wizard 引导 |
| 安全特性实现不完整可能引入新漏洞 | 安全回归 | 安全特性必须有对应测试 |
| 工作量巨大（P0+P1 ≈ 40+ 模块） | 交付周期风险 | 分阶段交付，每阶段独立可用 |

---

## 待确认项（已确认 2026-04-27）

| # | 问题 | 决策 | 影响 |
|---|------|------|------|
| 1 | Bedrock 认证方式 | **AWS SDK Go v2 credential chain** | 支持 IAM Role、SSO、环境变量等完整链路 |
| 2 | Gemini 认证方式 | **仅 API key** | 简化实现，不需要 OAuth 流程 |
| 3 | Web Dashboard | **是，纳入范围** | 新增 Phase，需确定前端技术栈（待定） |
| 4 | TUI 替代方案 | **采用 Bubble Tea** | 纯 Go 实现，无 Node.js 依赖 |
| 5 | RL Training | **不是当前目标** | 从范围中移除 |
| 6 | 中国市场平台 | **提升至 P0** | WeChat/DingTalk/Feishu/WeCom 进入 Phase 3 |
| 7 | Browser 后端 | **仅 Browserbase** | 不做 Camofox，降低 browser 工作量 |
| 8 | Plugin 内存 provider | **需要** | Honcho/mem0/supermemory 纳入 Plugin 增强 |

---

## 建议交付阶段（已基于确认更新）

| 阶段 | 内容 | 预估规模 | 说明 |
|------|------|----------|------|
| Phase 1 | Transport Layer 重构 + Bedrock(SDK)/Gemini(API key)/Codex provider | L | 架构基础，所有后续 provider 依赖此 |
| Phase 2 | 安全加固全量 + Agent Loop 增强 | M | 9 项安全 + 8 项 loop 增强 |
| Phase 3 | Gateway P0: Telegram + Discord + Slack + API Server + WeChat + DingTalk + Feishu + WeCom | XXL | 8 个平台，含中国市场 4 个 |
| Phase 4 | Browser(Browserbase only) + Plugin 增强(含 Honcho/mem0/supermemory) + Skills 增强 | L | |
| Phase 5 | Bubble Tea TUI + 新增工具 + Auxiliary 特性 | L | 替代 React/Ink |
| Phase 6 | Web Dashboard + 剩余 Gateway(WhatsApp 等) | XL | 前端技术栈待定 |
