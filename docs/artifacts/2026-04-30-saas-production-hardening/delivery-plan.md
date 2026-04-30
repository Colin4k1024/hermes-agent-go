# Delivery Plan: Hermes Agent SaaS Production Hardening (v0.7.0)

> 日期: 2026-04-30 | 主责: tech-lead | 状态: handoff-ready

## 版本目标

将 hermes-agent-go 从 POC 级别提升至 Early Production 级别，修复 SaaS 审查中发现的 2 个 Critical + 5 个 Medium 缺口。

**放行标准**: 全部 Sprint 1 + Sprint 2 任务完成，测试通过率 100%，GDPR 删除覆盖率 100%。

---

## Requirement Challenge Session Log

### 质疑 1: GDPR 紧迫度 (PM)
- **质疑**: POC 阶段是否已有欧盟客户？若无，P0 应让位给 RLS。
- **结论**: 接受风险排序调整 — GDPR 级联删除仍保持 P0（合规基础设施即使无客户也应尽早到位），但 PG RLS 本轮不做。
- **处理**: 保持原优先级，PG RLS 移入 v0.8 backlog。

### 质疑 2: 软删除全局影响面 (Architect)
- **质疑**: 6 张子表加 `deleted_at IS NULL`，所有查询需逐一排查改写，复合索引需重建。
- **结论**: 接受 — 通过 partial index 缓解性能风险，开发阶段逐文件 review WHERE 子句。
- **处理**: 在 Sprint 1 中增加"查询改写 review checklist"步骤。

### 质疑 3: PG RLS + pgxpool 冲突 (Architect)
- **质疑**: 连接池中 `SET app.current_tenant` 是连接级状态，归还池后可能泄漏。
- **替代路径**: 维持应用层 `WHERE tenant_id` 防线（已有 50+ 处），暂不引入 RLS。
- **结论**: 接受替代路径 — PG RLS 移入 v0.8，本轮不做。

### 质疑 4: Sprint 依赖与并行化 (Project Manager)
- **质疑**: Sprint 1 两任务共享 migrate.go 应合并；SSE 无 schema 依赖可提前并行。
- **结论**: 接受 — GDPR + Tenant 级联合并为一个 story；SSE 与 Sprint 1 并行。
- **处理**: 调整任务分组，SSE 提前到 Sprint 1 并行轨道。

---

## Story Slice 列表

### Sprint 1: 合规基线 + 流式体验 (并行双轨)

| Slice | 目标 | 验收标准 | 依赖 | Owner | 文件影响 |
|-------|------|---------|------|-------|---------|
| S1.1 级联删除统一治理 | Tenant 软删除 + GDPR 扩展覆盖 + 异步清理 | (1) DELETE /v1/tenants/{id} 返回 202, tenant 标记 deleted_at (2) DELETE /v1/gdpr/data 覆盖 memories/profiles/keys (3) GET /v1/gdpr/export 包含 memories/profiles (4) 7 天后异步硬删除 (5) 所有现有查询加 deleted_at IS NULL | 无 | backend | migrate.go, tenant.go, gdpr.go, store.go, types.go, 全部 pg/*.go 查询, 新建 jobs/tenant_cleanup.go |
| S1.2 SSE 流式响应 | 实现 OpenAI 兼容的 SSE 流式输出 | (1) stream:true 返回 text/event-stream (2) 心跳 15s (3) [DONE] 终止信号 (4) 错误事件格式正确 (5) tool loop 阶段内部缓冲 | 无(与 S1.1 并行) | backend | agent_chat.go, agent/conversation.go |

### Sprint 2: 安全加固

| Slice | 目标 | 验收标准 | 依赖 | Owner | 文件影响 |
|-------|------|---------|------|-------|---------|
| S2.1 审计失败认证 | Auth 失败事件入 audit_logs | (1) INVALID_KEY/EXPIRED_KEY/REVOKED_KEY/MISSING_AUTH 均记录 (2) 包含 source_ip 和 error_code (3) tenant_id 允许为空 | S1.1(schema) | backend | middleware/auth.go, middleware/audit.go, store/pg/auditlog.go, migrate.go |
| S2.2 RBAC 粒度增强 | method+path 组合权限 | (1) 支持 GET/POST/DELETE 分别配置权限 (2) 向后兼容现有前缀规则 | S1.1 | backend | middleware/rbac.go |

### Sprint 3: 运维基线

| Slice | 目标 | 验收标准 | 依赖 | Owner | 文件影响 |
|-------|------|---------|------|-------|---------|
| S3.1 JSON 结构化日志 | 生产模式 JSON 输出 | LOG_FORMAT=json 或 HERMES_ENV=production 时切换 JSONHandler | S1.1 | backend | cmd/hermes/main.go |
| S3.2 迁移 advisory lock | 防并发迁移竞争 | pg_try_advisory_lock 包裹迁移逻辑 | S1.1 | backend | store/pg/migrate.go |
| S3.3 Secrets 管理 | 移除硬编码密码 | (1) 移除 LLM_API_KEY fallback "123456" (2) compose 改 .env 引用 (3) 新增 config/secrets.go | S1.1 | backend | chat_handler.go, docker-compose*.yml, 新建 config/secrets.go |

---

## 角色分工

| 角色 | 职责 |
|------|------|
| tech-lead | Sprint 计划、方案收口、冲突仲裁、放行决策 |
| backend-engineer | 全部 7 个 story slice 的实现与单测 |
| qa-engineer | 回归验证、GDPR 删除覆盖率验证、SSE 兼容性测试 |

---

## 风险与缓解

| 风险 | 影响 | 缓解 | Owner |
|------|------|------|-------|
| 软删除 WHERE 子句遗漏 | 已删除数据泄漏 | 每个 pg/*.go 文件逐行 review checklist，CI 中增加 grep 检查 | backend |
| SSE tool loop 超时 | 客户端以为断连 | 15s 心跳 + 客户端 30s 超时检测 | backend |
| advisory lock 在 pgxpool 中泄漏 | 死锁 | 使用 pg_try_advisory_lock + 显式 unlock，不依赖连接关闭 | backend |
| audit_logs tenant_id 可空 | 下游查询需适配 | 新增 NULL 处理逻辑，audit list API 适配 | backend |
| 迁移版本号冲突 | 部署失败 | S1.1 统一分配 v28-v35，其他 sprint 从 v36 起 | tech-lead |

---

## 检查节点

| 节点 | 条件 | 角色 |
|------|------|------|
| Sprint 1 方案评审 | arch-design.md 锁定, 接口契约确认 | tech-lead + architect |
| Sprint 1 开发完成 | S1.1 + S1.2 代码合入, `go test ./...` 全绿 | backend |
| Sprint 1 测试完成 | GDPR 删除覆盖率 100%, SSE curl 验证通过 | qa-engineer |
| Sprint 2 开发完成 | S2.1 + S2.2 代码合入, 测试全绿 | backend |
| Sprint 3 开发完成 | S3.1-S3.3 代码合入, 测试全绿 | backend |
| 发布准备 | 全部测试通过, 文档更新, compose 验证 | tech-lead |

---

## 当前不做项 (Out of Scope)

| 项 | 原因 |
|----|------|
| PostgreSQL RLS | pgxpool 连接状态污染风险，v0.8 评估 |
| Helm chart / K8s | 当前部署形态为 Docker Compose，K8s 留 v0.9 |
| LLM 请求级 Prometheus 指标 | 非阻塞，v0.8 增强 |
| Tenant 级工具沙箱 | 架构复杂度高，v0.9 评估 |
| JWT/OAuth 认证 | 当前 API Key 满足需求，v0.8 扩展 |
| SSE 断点续传 | v0.7 记录 Last-Event-ID 但不实现续传，v0.8 支持 |

---

## 技能装配清单

| 能力 | 来源 | 触发原因 |
|------|------|---------|
| Go coding style | rules/golang/ | Go 项目 |
| Common testing | rules/common/testing.md | 所有代码变更需要测试 |
| Common security | rules/common/security.md | 审计、认证、GDPR 相关 |
| Database rules | rules/java/database.md | PG schema 变更参考 |

---

## 假设

1. 当前 POC 无生产流量，schema 变更无需在线 DDL 或蓝绿迁移。
2. 7 天软删除保留窗口对 POC 阶段足够，生产环境可配置化。
3. LLM provider 的 streaming API 稳定可用（已有 transport 层实现）。
4. 单人开发，Sprint 间可以串行推进，Sprint 内并行由开发者自行安排。
