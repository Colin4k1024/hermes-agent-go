# Project Context: hermes-agent-go
| 字段 | 值 |
|------|-----|
| 项目名 | hermes-agent-go |
| 当前任务 | `2026-04-30-saas-production-hardening` |
| 阶段 | `handoff-ready` |
| 更新时间 | 2026-04-30 |

---

## Tech Stack

### 后端
- Go 1.25.0
- PostgreSQL (primary store, pgx/v5, migration v27 → v35 planned)
- SQLite (local dev store, noop for SaaS features)
- Redis (session lock, rate limit fallback)
- MinIO (per-tenant skills + soul storage)
- OpenTelemetry (otel SDK + OTLP gRPC exporter)
- Prometheus (client_golang v1.23.2)
- hashicorp/golang-lru/v2 v2.0.7 (rate limiter)
- golang-jwt/jwt/v5 (JWT RS256)
- Helm chart (Kubernetes)
- Docker Compose (local dev)

### 前端 (webui/ 子目录)
- Vue 3 + TypeScript + Vite
- Naive UI (组件库)
- Pinia (状态管理)
- Vue Router (hash mode)
- Nginx (生产静态服务 + API 反代)

## 当前任务

**SaaS Production Hardening v0.7.0** — 将 hermes-agent-go 从 POC 提升至 Early Production，修复 SaaS 审查中的 2 Critical + 5 Medium 缺口。

### Sprint 1 (P0 合规 + 流式, 并行双轨)
- S1.1 级联删除统一治理: Tenant 软删除 + GDPR 全表覆盖 + 7 天异步硬删除
- S1.2 SSE 流式响应: OpenAI chat.completion.chunk 兼容, 15s 心跳

### Sprint 2 (P1 安全)
- S2.1 审计失败认证: AUTH_FAILED 事件 + source_ip/error_code
- S2.2 RBAC 粒度增强: method+path 组合权限

### Sprint 3 (P2 运维基线)
- S3.1 JSON 结构化日志
- S3.2 DB 迁移 advisory lock
- S3.3 Secrets 管理 (移除硬编码 "123456")

**Plan 产出物**:
- `docs/artifacts/2026-04-30-saas-production-hardening/delivery-plan.md`
- `docs/artifacts/2026-04-30-saas-production-hardening/arch-design.md`

**前置任务已完成**:
- SaaS 多租户记忆功能 (v0.6.0, commit 6490f88)
- WebUI v1.0 (Vue 3 SPA, docker-compose.webui.yml)
- 全量 API 端到端验证通过

## 依赖

- PostgreSQL 14+ (migrate.go v28-v35 新迁移)
- MinIO (tenant cleanup 需删除 soul/skills 对象)
- LLM Provider streaming API (SSE 依赖 ChatStream transport)
- Playwright api-isolation 13/13 回归通过

## 风险

- R1: 软删除 WHERE 遗漏 — 已删除租户数据泄漏; 缓解: 逐文件 review + CI grep
- R2: SSE tool loop 超时 — 客户端断连; 缓解: 15s heartbeat + 150s WriteTimeout
- R3: advisory lock 泄漏 — 清理 job 死锁; 缓解: pg_try_advisory_lock + 显式 unlock
- R4: audit_logs tenant_id NULL — 下游查询; 缓解: COALESCE + NULL 检查
- R5: PG RLS 延期 — 应用层 WHERE 是唯一租户隔离; 缓解: 50+ 处已覆盖, v0.8 评估 RLS

## 下一步

1. **backend-engineer 开始 Sprint 1**: S1.1 (级联删除) + S1.2 (SSE) 并行开发
2. Sprint 1 完成后顺序推进 Sprint 2 → Sprint 3
3. 全部完成后: `go test ./...` 全绿 + GDPR 删除覆盖率 100% + SSE curl 验证
4. 放行标准: 全部 7 个 story slice 通过, 文档更新, compose 验证
