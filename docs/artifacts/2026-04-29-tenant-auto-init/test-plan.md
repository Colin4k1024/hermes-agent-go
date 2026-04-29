# Test Plan: Tenant Auto-Initialization

- **日期**: 2026-04-29
- **主责**: qa-engineer
- **来源**: delivery-plan.md + execute-log.md

---

## 测试范围

### 功能范围

| 场景 | 类型 | 前置条件 | 预期结果 |
|------|------|---------|---------|
| 新租户创建触发 provisioning | 集成 | MinIO + PG 可用 | MinIO 中出现 `{tenantID}/SOUL.md` + skills |
| 已有租户 soul 不被覆盖 | 单元 | MinIO 中已存在 SOUL.md | ObjectExists → skip |
| 非法 tenantID 被拒绝 | 单元 | tenantID 含 `../` 或 `/` | validateTenantID 返回 error |
| buildSystemPrompt 注入 soul | 单元 | `WithSoulContent("...")` | prompt 包含 `## Persona` 块 |
| buildSystemPrompt 注入 memory | 单元 | memoryProvider 实现 SystemPromptProvider | prompt 包含 memory block |
| Soul 超 64KB 不注入 | 单元 | MinIO 返回 >64KB 数据 | WithSoulContent 不调用 |
| HTTP API chat 走完整 agent | 端到端 | gateway runner + APIServerAdapter | agent 可调用 memory/skills 工具 |
| 多租户 memory 隔离 | 端到端 | 2 个租户各自 chat | memory 不串租户 |
| 启动 sync 补齐已有租户 | 集成 | 已有租户无 soul/skills | 日志出现 sync 进度 |
| onCreated 超时 30s | 单元 | MinIO 不可用 | goroutine 30s 后退出 |

### 非功能范围

| 场景 | 预期 |
|------|------|
| 并发租户创建 | provisioning goroutine 不 panic |
| MinIO 不可用时创建租户 | API 返回 201，provisioning 仅 log error |
| gofmt | 全部文件通过 |
| go test -race | 无 data race |

### 不覆盖项

- UI 变更（无前端改动）
- 租户删除清理 MinIO 资源（out of scope）
- Skills evolution 端到端（依赖 skills 工具运行时行为）
- 生产环境性能测试

## 代码审查结论

### Security Review (2 HIGH → 已修复)

| ID | 原始等级 | 修复状态 | 说明 |
|----|---------|---------|------|
| HIGH-1: tenantID 未校验 | HIGH | ✅ 已修复 | `validateTenantID()` 拒绝含 `../` `/` `\` 的 ID |
| HIGH-2: soul 无大小限制 | HIGH | ✅ 已修复 | runner.go 64KB 上限 |
| MEDIUM-1: context 无超时 | MEDIUM | ✅ 已修复 | 30s timeout |
| MEDIUM-2: soul fetch 用 Background | MEDIUM | 接受 | agent 缓存降低频率 |
| MEDIUM-3: Limit:1000 | MEDIUM | 接受 | POC known limitation |

### Code Review (2 HIGH → 已修复)

| ID | 原始等级 | 修复状态 | 说明 |
|----|---------|---------|------|
| HIGH-1: Provision() 吞错误 | HIGH | ✅ 已修复 | 返回 combined error |
| HIGH-2: SkipContextFiles 无条件 | HIGH | ✅ 已修复 | 移入 minioClient guard |
| MEDIUM-5: bundledDir 相对路径 | MEDIUM | 接受 | POC 够用 |
| MEDIUM-6: soul ordering | MEDIUM | 接受 | 加注释说明 |
| MEDIUM-7: Debug → Warn | MEDIUM | ✅ 已修复 | 改为 slog.Warn |
| LOW-8: interface assertion | LOW | ✅ 已修复 | 添加 compile-time 断言 |
| LOW-9: onCreated 无测试 | LOW | 接受 | POC 阶段 |

## 风险

| 风险 | 等级 | 处置 |
|------|------|------|
| skills 全量上传耗时 | 中 | 幂等 skip-if-exists 降低重复开销 |
| agent 缓存 soul 过期 | 低 | v1 接受，session 过期自动刷新 |
| `SyncAllTenants` Limit:1000 | 低 | POC known limitation |

## 放行建议

**建议放行** — 4 个 HIGH 全部修复，无阻塞项。3 个 MEDIUM 作为 known limitation 接受（POC 阶段合理）。
