# Execute Log: Tenant Auto-Initialization

- **日期**: 2026-04-29
- **主责**: backend-engineer
- **Delivery Plan**: docs/artifacts/2026-04-29-tenant-auto-init/delivery-plan.md

---

## 计划 vs 实际

| Slice | 计划 | 实际 | 偏差 |
|-------|------|------|------|
| S1: Agent Core | ~24 行 | 23 行 (+1 agent.go, +5 types.go, +16 prompt.go, +1 memory_providers.go) | 无偏差 |
| S2: Provisioner | ~150 行新代码 + 20 行修改 | 131 行新 `provisioner.go` + 23 行 tenants.go + 5 行 server.go | 略少，设计紧凑 |
| S3: Gateway Wiring | ~40 行 | 41 行 (saas.go 重构 + runner.go 增强) | 无偏差 |
| S4: Startup Sync | ~15 行 | 合入 S3 的 saas.go 变更中 | 合并交付，减少独立改动 |
| **合计** | ~230 行 | 114 行净增 (11 files) | 实际更紧凑 |

## 关键决定

### D1: Import cycle 规避
`skills/provisioner.go` 需要 `DefaultSoulMD` 常量，但 `cli` → `agent` → `skills` 形成循环。
**决定**: 在 `provisioner.go` 中内联 `defaultSoulContent` 常量，添加注释说明来源。

### D2: S3+S4 saas.go 合并交付
Provisioner 创建 + 回调注册 + 后台 sync + gateway runner wiring 全部在 saas.go 中一次性重排，步骤编号从 7-10 重新组织。

### D3: runner.go soul 加载位置
Soul 在 `getOrCreateAgent()` 中加载，与 MinIO skill loader 同一 `if r.minioClient != nil` 块内。Agent 缓存意味着 soul 加载频率 = session 创建频率。

### D4: `WithSkipContextFiles(true)` 无条件追加
所有 gateway runner 创建的 agent 都跳过本地文件系统上下文文件，适用于 SaaS 模式。

## 阻塞与解决

| 阻塞 | 根因 | 解决 |
|------|------|------|
| S2 子 agent 未产出文件 | agent idle 但未写入 | 直接手工实现，用时 5 分钟 |
| `skills` → `cli` import cycle | `cli` 包传递依赖 `skills` | 内联常量，避免跨包依赖 |
| 4 个 gofmt 未格式化文件 | pre-existing + 新改动 | `gofmt -w` 修复 |

## 影响面

| 模块 | 影响 |
|------|------|
| `internal/agent/` | 新增 `soulContent` 字段 + `WithSoulContent` option + prompt 注入 + adapter delegation |
| `internal/skills/` | 新文件 `provisioner.go` — 131 行 |
| `internal/api/` | TenantHandler option pattern + server config 字段 + gofmt 清理 |
| `internal/gateway/runner.go` | soul 加载 + `WithSkipContextFiles(true)` |
| `cmd/hermes/saas.go` | 步骤重排: provisioner(7) → server(8) → ACP(9) → gateway runner(10) |

## 未完成项

- 无 — 4 个 slice 全部交付

## 自测结论

```
go build ./cmd/hermes/...                    ✅ 编译通过
go test ./internal/agent/... -race           ✅ PASS (3.2s)
go test ./internal/api/... -race             ✅ PASS (3.9s)
go test ./internal/skills/... -race          ✅ PASS (5.3s)
gofmt -l .                                   ✅ 无未格式化文件
```

## 交给 QA 的说明

### 验证步骤

1. **创建新租户**: `POST /v1/tenants` → 检查 MinIO `{tenantID}/SOUL.md` 和 skills 目录
2. **HTTP API chat (full agent)**: `POST :8081/v1/chat/completions` with `X-Hermes-Tenant-Id` → 验证 agent 有 soul + tools
3. **Memory 工具**: 通过 chat 让 agent 执行 `memory_save` / `memory_read` → 验证 per-user 隔离
4. **Skills 列表**: 通过 chat 让 agent 执行 `skills_list` → 验证 per-tenant skills
5. **Soul 定制**: 修改 MinIO 中 `{tenantID}/SOUL.md` → 驱逐 agent session → 再次 chat 验证人格变化
6. **启动 sync**: 重启服务 → 检查日志 `"starting tenant sync"` + `"tenant sync complete"`
7. **回归**: `scripts/test_web_isolation.sh`

---

*已创建 `docs/artifacts/2026-04-29-tenant-auto-init/execute-log.md`*
