# Launch Acceptance: Tenant Auto-Initialization

- **日期**: 2026-04-29
- **主责**: qa-engineer + tech-lead
- **版本**: v0.6.0

---

## 验收概览

| 项目 | 值 |
|------|---|
| 对象 | Tenant Auto-Init (soul + memory + skills provisioning + HTTP full agent) |
| 时间 | 2026-04-29 |
| 角色 | tech-lead, backend-engineer, qa-engineer |
| 验收方式 | 代码审查 + 自动化测试 + 静态分析 |

## 验收范围

### 业务验收

- [x] 新租户创建后 MinIO 自动 provision soul + skills
- [x] `buildSystemPrompt()` 注入 soul + memory
- [x] HTTP API (`APIServerAdapter`) 走完整 `AIAgent.RunConversation()` 路径
- [x] 启动时后台 sync 已有租户

### 技术验收

- [x] `go build ./...` 通过
- [x] `go test ./internal/agent/... -race` PASS
- [x] `go test ./internal/api/... -race` PASS
- [x] `go test ./internal/skills/... -race` PASS
- [x] `gofmt -l .` 无输出
- [x] Security review: 2 HIGH 已修复
- [x] Code review: 2 HIGH 已修复

### 不在范围

- 端到端浏览器测试（无前端）
- 生产环境负载测试
- 租户删除清理

## 验收证据

| 证据 | 状态 |
|------|------|
| `go build ./...` | ✅ 0 errors |
| `go test -race` (3 packages) | ✅ all PASS |
| `gofmt -l .` | ✅ clean |
| Security review HIGH-1 (tenantID validation) | ✅ `validateTenantID()` added |
| Security review HIGH-2 (soul size limit) | ✅ 64KB cap in runner.go |
| Code review HIGH-1 (Provision error return) | ✅ combined error returned |
| Code review HIGH-2 (SkipContextFiles guard) | ✅ moved inside minioClient block |
| Git diff | 11 files, +121/-38, 1 new file |

## 风险判断

### 已满足

- 编译通过，测试通过，race detector clean
- 4 个 HIGH review issue 全部修复
- tenantID 注入防护到位
- 异步 goroutine 有 30s 超时

### 可接受风险

| 风险 | 理由 |
|------|------|
| `SyncAllTenants` Limit:1000 | POC 阶段不可能超过 1000 租户 |
| bundledDir 相对路径 | 开发环境确定性足够 |
| Soul fetch 用 context.Background | agent cache 降低调用频率 |
| onCreated 无专门单元测试 | 集成测试覆盖主路径 |

### 阻塞项

- 无

## 上线结论

| 项目 | 结论 |
|------|------|
| 是否允许上线 | **是** |
| 前提条件 | MinIO 可用 + DATABASE_URL 配置 |
| 观察重点 | 日志中 `tenant provisioning complete` + `tenant sync complete` |
| 回滚方案 | revert 本次 commit，soul/skills 数据留在 MinIO 不影响现有功能 |

---

*已创建 test-plan.md + launch-acceptance.md*
