# Project Context: hermes-agent-go
| 字段 | 值 |
|------|-----|
| 项目名 | hermes-agent-go |
| 当前任务 | `2026-04-28-saas-hardening-fixes` |
| 阶段 | `closed` |
| 更新时间 | 2026-04-28 |

---

## Tech Stack

- Go 1.25.0
- PostgreSQL (primary store, pgx/v5, migration v27)
- SQLite (local dev store, noop for SaaS features)
- Redis (session lock, rate limit fallback)
- OpenTelemetry (otel SDK + OTLP gRPC exporter)
- Prometheus (client_golang v1.23.2, HTTP + LLM + PG metrics)
- hashicorp/golang-lru/v2 v2.0.7 (rate limiter)
- golang-jwt/jwt/v5 (JWT RS256)
- Helm chart (Kubernetes)
- Docker Compose (local dev)

## 当前任务

SaaS Hardening 完成 — 14 项修复 + 1 项设计 + 10 项 review 修复。
26 files, +442/-167 lines。Security + code review 通过。

## 依赖

- SaaS Readiness P0-P7 已全量交付
- PG migration baseline: v27 (audit_logs enrichment)
- OTel SDK 已加入 go.mod
- hashicorp/golang-lru/v2 已升为 direct dep

## 风险

- R1: Prometheus tenant_id 标签高基数 — accepted, bounded tenant set
- R2: 非 HTTP 路径 slog 未迁移 — LOW, 渐进迁移
- R3: LLM credential 加密未实现 — 设计就绪, 待计费系统

## 下一步

1. **Commit + Push** — 当前变更待提交到 GitHub
2. **Prometheus cardinality guard** — 添加 _overflow fallback
3. **E2E integration test** — docker-compose + PG + Redis + Jaeger
4. **LLM credential encryption** — 按设计文档实现
5. **CLI slog migration** — 非 HTTP 路径渐进迁移
