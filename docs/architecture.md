# 架构概览

> Hermes Agent Go 的系统设计、两种运行模式和 SaaS 多租户架构。

## 两种运行模式

Hermes 支持两种独立的运行模式：

| 模式 | 命令 | 用途 | 存储 |
|------|------|------|------|
| CLI 模式 | `hermes` | 本地交互式 Agent | SQLite / 文件系统 |
| SaaS 模式 | `hermes saas-api` | 多租户 HTTP API 服务 | PostgreSQL |

两种模式共享 LLM 集成层和 Skills 系统，但网络层、存储层和认证体系完全独立。

## SaaS 模式架构

```
┌─────────────────────────────────────────────────┐
│                   HTTP 请求                       │
└───────────────────────┬─────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────┐
│              CORS Middleware                      │
│         （SAAS_ALLOWED_ORIGINS 配置）              │
└───────────────────────┬─────────────────────────┘
                        │
                   ┌────▼────┐
                   │ SPA 路由 │ ←── 静态文件（index.html, admin.html）
                   └────┬────┘
                        │
┌───────────────────────▼─────────────────────────┐
│            9 层中间件栈（固定顺序）                  │
│                                                   │
│  Tracing → Metrics → RequestID → Auth → Tenant   │
│  → Logging → Audit → RBAC → RateLimit → Handler │
└───────────────────────┬─────────────────────────┘
                        │
          ┌─────────────▼─────────────┐
          │        路由分发             │
          │                           │
          │  /health/*    → 公开       │
          │  /metrics     → Prometheus │
          │  /v1/tenants  → Admin      │
          │  /v1/api-keys → Admin      │
          │  /v1/chat/*   → User       │
          │  /v1/me       → User       │
          └─────────┬─────────────────┘
                    │
     ┌──────────────▼──────────────┐
     │        Store 层              │
     │   PostgreSQL（多租户隔离）     │
     └─────────────────────────────┘
```

## 中间件栈

中间件按固定顺序执行，由 `middleware.MiddlewareStack` 强制保证：

```
Tracing → Metrics → RequestID → Auth → Tenant → Logging → Audit → RBAC → RateLimit → Handler
```

| 层 | 职责 | 源文件 |
|----|------|--------|
| **Tracing** | OpenTelemetry span 创建与传播 | `internal/observability/tracer.go` |
| **Metrics** | Prometheus 请求计数、延迟、并发量 | `internal/middleware/metrics.go` |
| **RequestID** | 生成或提取 `X-Request-ID` | `internal/middleware/requestid.go` |
| **Auth** | 链式认证（Static Token → API Key → JWT） | `internal/auth/extractor.go` |
| **Tenant** | 从 AuthContext 提取 tenant_id 写入 Context | `internal/middleware/tenant.go` |
| **Logging** | 注入 tenant_id 和 request_id 到 slog Logger | `internal/observability/logger.go` |
| **Audit** | 记录所有认证请求到 audit_logs 表 | `internal/middleware/audit.go` |
| **RBAC** | 基于路径前缀的角色访问控制 | `internal/middleware/rbac.go` |
| **RateLimit** | 按租户维度限流（分布式 + 本地 LRU 降级） | `internal/middleware/ratelimit.go` |

**设计要点**：
- Logging 层位于 Auth + Tenant 之后，确保日志自动包含 tenant_id
- Auth 错误使用 `ContextLogger` 降级（slog.Default + request_id）
- 所有中间件槽位可为 `nil`（passthrough），便于测试和按需启用

## 多租户模型

### 租户隔离

Hermes 使用**数据库级租户隔离**：

1. **tenant_id 派生自凭证**：永远不从请求头读取，防止租户伪造
2. **所有表 FK 到 tenants**：9 张业务表均包含 `tenant_id UUID NOT NULL REFERENCES tenants(id)`
3. **查询自动过滤**：所有 Store 方法按 tenant_id 过滤数据

```
AuthContext.TenantID（从凭证派生）
       │
       ▼
┌──────────────┐
│ Context 传播  │ ← Tenant 中间件写入
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  Store 查询   │ ← WHERE tenant_id = $1
└──────────────┘
```

### 默认租户

首次启动时自动创建默认租户：

- ID: `00000000-0000-0000-0000-000000000001`
- Name: `Default Tenant`
- Plan: `pro`
- Rate Limit: 120 RPM
- Max Sessions: 10

Static Token 认证自动映射到此租户。

## Store 架构

### 接口抽象

`store.Store` 接口定义统一的数据访问层，支持多种后端实现：

```go
type Store interface {
    Tenants()   TenantStore
    Sessions()  SessionStore
    Messages()  MessageStore
    Users()     UserStore
    AuditLogs() AuditLogStore
    APIKeys()   APIKeyStore
    CronJobs()  CronJobStore
}
```

### 存储后端

| 后端 | 用途 | 包路径 |
|------|------|--------|
| PostgreSQL | SaaS 模式（多租户） | `internal/store/pg/` |
| SQLite | CLI 模式（单用户） | `internal/store/sqlite/` |

PostgreSQL 后端在启动时自动执行 27 个 migration（参见 [数据库文档](database.md)）。

## LLM 集成

### 可插拔 Transport

LLM 调用通过 `Transport` 接口抽象，支持多种提供商：

```
请求 → Provider 自动检测 → Transport 适配 → LLM API
```

支持的提供商：
- OpenAI（openai 协议）
- Anthropic（anthropic 协议）
- 自动检测（根据 API URL 和 Key 格式推断）

### 配置

| 环境变量 | 说明 |
|----------|------|
| `LLM_API_URL` | LLM API 端点 |
| `LLM_API_KEY` | API 认证密钥 |
| `LLM_MODEL` | 默认模型名称 |

## Chat 请求流程

一次完整的 `/v1/chat/completions` 请求经历以下路径：

```
1. HTTP 请求到达
2. CORS 检查（如果配置了 SAAS_ALLOWED_ORIGINS）
3. Tracing：创建 span
4. Metrics：记录请求开始
5. RequestID：生成唯一请求标识
6. Auth：链式认证，生成 AuthContext
7. Tenant：从 AuthContext 提取 tenant_id
8. Logging：注入 tenant_id 到 logger
9. Audit：记录审计日志（action、status_code、latency_ms）
10. RBAC：检查 "user" 角色权限
11. RateLimit：检查租户 RPM 配额
12. Handler：
    a. 解析 ChatCompletionRequest
    b. 获取或创建 Session（按 tenant_id 隔离）
    c. 调用 LLM API（通过 Transport）
    d. 存储 Message（关联 tenant_id）
    e. 返回 ChatCompletionResponse
```

## HTTP 服务器配置

| 参数 | 值 | 说明 |
|------|-----|------|
| Read Timeout | 30s | 读取请求体超时 |
| Write Timeout | 60s | 写入响应超时（LLM 调用可能较慢） |
| Idle Timeout | 120s | Keep-Alive 空闲超时 |
| Listen Address | `0.0.0.0:{port}` | 监听所有网卡 |

## 项目结构

```
hermes-agent-go/
├── cmd/hermes/
│   ├── main.go           # CLI 入口
│   └── saas.go           # SaaS API 入口（hermes saas-api）
├── internal/
│   ├── api/              # HTTP handlers 和路由
│   │   ├── server.go     # APIServer、路由注册、中间件组装
│   │   ├── tenants.go    # 租户 CRUD
│   │   ├── apikeys.go    # API Key 管理
│   │   ├── mockchat.go   # Chat completions + session store
│   │   ├── health.go     # 健康探针
│   │   └── gdpr.go       # GDPR 数据导出/删除
│   ├── auth/             # 认证链
│   │   ├── context.go    # AuthContext 定义
│   │   ├── extractor.go  # ExtractorChain 接口
│   │   ├── static.go     # Static Token 认证
│   │   ├── apikey.go     # API Key 认证
│   │   └── jwt.go        # JWT 认证（预留）
│   ├── middleware/        # HTTP 中间件
│   │   ├── chain.go      # 固定顺序中间件栈
│   │   ├── rbac.go       # 角色访问控制
│   │   ├── ratelimit.go  # 速率限制
│   │   ├── metrics.go    # Prometheus 指标
│   │   └── audit.go      # 审计日志
│   ├── store/            # 数据存储抽象
│   │   ├── types.go      # 数据模型定义
│   │   ├── pg/           # PostgreSQL 实现
│   │   └── sqlite/       # SQLite 实现
│   ├── skills/           # Skills 系统
│   │   ├── hub.go        # Skills Hub 发现与安装
│   │   ├── scanner.go    # 安全扫描
│   │   └── loader.go     # 本地加载
│   ├── observability/    # 可观测性
│   │   ├── tracer.go     # OpenTelemetry 初始化
│   │   └── logger.go     # Context-enriched 日志
│   ├── objstore/         # MinIO/S3 对象存储
│   ├── gateway/          # CLI 模式 Gateway
│   ├── config/           # 配置管理
│   └── dashboard/        # 管理面板静态文件
│       └── static/       # HTML/CSS/JS
├── skills/               # 77 个内置 Skills
├── deploy/               # 部署配置
│   ├── helm/             # Helm Chart
│   └── kind/             # Kind 本地 K8s
├── scripts/              # 测试和工具脚本
└── docs/                 # 文档
```

## 相关文档

- [快速开始](saas-quickstart.md) — 5 分钟上手
- [API 参考](api-reference.md) — 完整端点文档
- [认证系统](authentication.md) — Auth Chain 和 RBAC
- [数据库](database.md) — Schema 和数据模型
- [可观测性](observability.md) — 监控和追踪
- [Skills 指南](skills-guide.md) — 技能系统
- [配置指南](configuration.md) — 环境变量
- [部署指南](deployment.md) — Docker / Helm / Kind
