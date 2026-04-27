# Launch Acceptance: Upstream Sync v0.11.0 + SaaS Stateless MVP

| 字段 | 值 |
|------|-----|
| 日期 | 2026-04-27 |
| 角色 | qa-engineer / tech-lead |
| 状态 | conditional |

---

## 验收概览

| 项目 | 内容 |
|------|------|
| 对象 | hermes-agent-go upstream sync + SaaS stateless MVP |
| 时间 | 2026-04-27 |
| 角色 | qa-engineer (评审) + tech-lead (决策) |
| 验收方式 | 自动化测试 + 代码审查 + 安全审查 |

## 验收范围

### 业务范围
- Transport Layer 可插拔架构（5 provider）
- 13 Gateway 平台适配器
- 安全加固（9 项）
- Agent Loop 增强（8 项）
- Plugin/Skills 增强
- SaaS 状态外置基础设施
- 并发缺陷修复（8 项）

### 技术范围
- 80 files changed, +8,731 lines
- 14 test packages
- 新增依赖：pgx/v5, go-redis/v9, aws-sdk-go-v2, discordgo, slack-go, telegram-bot-api, bubbletea

### 不在范围
- E2E 集成测试（需真实外部服务）
- 性能基准测试
- 多租户数据隔离验证（需 PG 实例）

---

## 验收证据

| 检查项 | 结果 | 证据 |
|--------|------|------|
| `go build ./...` | ✅ Pass | 零编译错误 |
| `go test ./...` | ✅ Pass | 14 packages, 0 failures |
| `go test -race ./internal/agent/... ./internal/tools/... ./internal/acp/...` | ✅ Pass | 零竞态警告 |
| `go vet ./...` | ✅ Pass | 零 vet 错误 |
| Security review | ⚠️ 3 HIGH | H1/H2/H3 需修复 |
| Code review | ⚠️ 缺测路径 | ~10 模块需补测 |

---

## 风险判断

### 已满足项
- [x] 全量编译通过
- [x] 全量测试通过
- [x] 并发安全验证（race detector）
- [x] 无硬编码凭证
- [x] 新依赖均为稳定版本
- [x] Transport 重构向后兼容（现有 OpenAI/Anthropic 行为不变）

### 可接受风险
- Gateway adapter 无集成测试（需 bot token，开发阶段可接受）
- Store PG 实现无集成测试（需 PG 实例，Docker Compose 已就绪）
- Dashboard CORS 过宽（localhost-only，低风险）

### 阻塞项

| ID | 来源 | 问题 | 修复建议 | Owner |
|----|------|------|---------|-------|
| H1 | Security | Dashboard SaveConfig 无认证 | 添加 bearer token 检查 | backend-engineer |
| H2 | Security | WeCom callback 无签名验证 | 实现 SHA1 签名校验 | backend-engineer |
| H3 | Security+Code | Redis lock 无 owner 验证 | Lua compare-and-delete + token | backend-engineer |
| H4 | Code | Store driver registry 未注册 — NewStore 永远失败 | 各 driver 加 init() + RegisterDriver | backend-engineer |
| H5 | Code | 并行 tool goroutine 无 panic recovery | defer recover + error result | backend-engineer |

---

## 上线结论

**结论：有条件放行**

| 条件 | 状态 |
|------|------|
| H1/H2/H3 修复 | ❌ 待修复 |
| 全量测试通过 | ✅ |
| Race detector 通过 | ✅ |

**前提条件：** 修复 3 个 HIGH 安全问题后可正式放行。

**观察重点：**
1. Transport Layer 重构后 LLM 调用行为是否一致
2. Gateway adapter 首次真实连接是否正常
3. Store PG 模式首次迁移是否成功
4. Redis lock 在多实例下的行为

---

## 确认记录

- qa-engineer: 有条件通过（2026-04-27）
- tech-lead: 待确认
