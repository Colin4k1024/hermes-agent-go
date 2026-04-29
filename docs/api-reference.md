# API 参考

> Hermes SaaS API 完整端点文档。所有认证端点需要 `Authorization: Bearer <token>` 请求头。

## 基本信息

| 项目 | 值 |
|------|-----|
| Base URL | `http://localhost:8080` |
| 认证方式 | Bearer Token（静态 Token / API Key / JWT） |
| 内容类型 | `application/json` |
| 速率限制 | 通过 `X-RateLimit-Limit` 和 `X-RateLimit-Remaining` 响应头返回 |

## 公开端点（无需认证）

### GET /health/live

存活探针。服务启动即返回 200。

```bash
curl http://localhost:8080/health/live
# {"status":"ok"}
```

### GET /health/ready

就绪探针。检查数据库连接状态。

```bash
curl http://localhost:8080/health/ready
# {"status":"ready","database":"ok"}
```

### GET /metrics

Prometheus 指标端点。返回 `text/plain` 格式。

```bash
curl http://localhost:8080/metrics
```

指标包括：
- `hermes_http_requests_total{method, path, status, tenant_id}` — HTTP 请求总数
- `hermes_http_request_duration_seconds{method, path, tenant_id}` — 请求延迟直方图
- `hermes_http_requests_in_flight` — 当前并发请求数

---

## 管理端点（需要 `admin` 角色）

以下端点需要 admin 角色。使用静态 Token（`HERMES_ACP_TOKEN`）或具有 admin 角色的 API Key 访问。

### 租户管理 /v1/tenants

#### POST /v1/tenants — 创建租户

```bash
curl -X POST http://localhost:8080/v1/tenants \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Acme Corp",
    "plan": "pro",
    "rate_limit_rpm": 120,
    "max_sessions": 50
  }'
```

响应：

```json
{
  "id": "a1b2c3d4-...",
  "name": "Acme Corp",
  "plan": "pro",
  "rate_limit_rpm": 120,
  "max_sessions": 50,
  "created_at": "2026-04-29T12:00:00Z",
  "updated_at": "2026-04-29T12:00:00Z"
}
```

> 创建租户时，如果 MinIO 已配置，系统会异步为新租户 Provisioning 所有内置技能（77+）和默认 SOUL.md 人格文件。

#### GET /v1/tenants — 列出所有租户

```bash
curl http://localhost:8080/v1/tenants \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

响应：

```json
{
  "tenants": [
    {
      "id": "a1b2c3d4-...",
      "name": "Acme Corp",
      "plan": "pro",
      "rate_limit_rpm": 120,
      "max_sessions": 50,
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

#### GET /v1/tenants/{id} — 获取单个租户

```bash
curl http://localhost:8080/v1/tenants/a1b2c3d4-... \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

#### PUT /v1/tenants/{id} — 更新租户

```bash
curl -X PUT http://localhost:8080/v1/tenants/a1b2c3d4-... \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"plan": "enterprise", "rate_limit_rpm": 300}'
```

#### DELETE /v1/tenants/{id} — 删除租户

```bash
curl -X DELETE http://localhost:8080/v1/tenants/a1b2c3d4-... \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### API Key 管理 /v1/api-keys

#### POST /v1/api-keys — 创建 API Key

```bash
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-key",
    "tenant_id": "a1b2c3d4-...",
    "roles": ["user"]
  }'
```

响应：

```json
{
  "id": "key-uuid-...",
  "key": "hk_a1b2c3d4e5f6...",
  "prefix": "hk_a1b2c",
  "name": "production-key",
  "tenant_id": "a1b2c3d4-...",
  "roles": ["user"],
  "created_at": "..."
}
```

> `key` 字段仅在创建时返回一次。API Key 以 SHA-256 哈希存储在数据库中，无法再次获取原始值。

#### GET /v1/api-keys — 列出所有 API Key

```bash
curl http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

#### DELETE /v1/api-keys/{id} — 撤销 API Key

```bash
curl -X DELETE http://localhost:8080/v1/api-keys/key-uuid-... \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 审计日志 /v1/audit-logs

#### GET /v1/audit-logs — 查询审计日志

```bash
curl "http://localhost:8080/v1/audit-logs?limit=50" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

每条审计记录包含：`tenant_id`、`user_id`、`action`、`detail`、`request_id`、`status_code`、`latency_ms`、`created_at`。

### GDPR 合规 /v1/gdpr

#### GET /v1/gdpr/export — 导出用户数据

```bash
curl http://localhost:8080/v1/gdpr/export \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

#### DELETE /v1/gdpr/data — 删除用户数据

```bash
curl -X DELETE http://localhost:8080/v1/gdpr/data \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## 用户端点（需要认证，`user` 或 `admin` 角色）

### POST /v1/chat/completions — 发送 Chat 请求

OpenAI 兼容格式的聊天接口。自动关联到请求方所属的租户。

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer hk_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mock",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

响应：

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "..."
      },
      "finish_reason": "stop"
    }
  ]
}
```

支持的请求头：

| 请求头 | 说明 |
|--------|------|
| `X-Hermes-Session-Id` | 指定会话 ID 以维持多轮对话 |
| `X-Hermes-User-Id` | 指定用户 ID，用于记忆和画像隔离（不传则使用 API Key Identity） |

Chat 请求自动注入以下上下文（需要 MinIO 和 PostgreSQL 已配置）：

- **Soul**：从 MinIO 加载租户的 `SOUL.md` 人格文件
- **记忆和画像**：从 PostgreSQL 加载用户级别的记忆和画像
- **技能摘要**：从 MinIO 加载租户已安装技能的列表

### GET /v1/me — 当前身份信息

返回当前认证用户的身份、租户和角色。

```bash
curl http://localhost:8080/v1/me \
  -H "Authorization: Bearer hk_your_api_key"
```

响应：

```json
{
  "identity": "key-uuid-...",
  "tenant_id": "a1b2c3d4-...",
  "roles": ["user"],
  "auth_method": "api_key"
}
```

### GET /v1/usage — 使用量统计

返回当前租户的会话和消息使用统计。

```bash
curl http://localhost:8080/v1/usage \
  -H "Authorization: Bearer hk_your_api_key"
```

### GET /v1/mock-sessions — 会话列表

返回当前租户的 mock chat 会话列表。

```bash
curl http://localhost:8080/v1/mock-sessions \
  -H "Authorization: Bearer hk_your_api_key"
```

### DELETE /v1/mock-sessions/{id} — 清除会话

```bash
curl -X DELETE http://localhost:8080/v1/mock-sessions/session-id \
  -H "Authorization: Bearer hk_your_api_key"
```

### 技能管理 /v1/skills

#### GET /v1/skills — 列出租户技能

返回当前租户已安装的所有 Skills，包含来源和修改状态。

```bash
curl http://localhost:8080/v1/skills \
  -H "Authorization: Bearer hk_your_api_key"
```

响应：

```json
{
  "tenant_id": "a1b2c3d4-...",
  "skills": [
    {
      "name": "code-review",
      "description": "代码审查助手",
      "version": "1.0.0",
      "source": "builtin",
      "user_modified": false
    },
    {
      "name": "my-custom-skill",
      "description": "自定义业务技能",
      "version": "1.0.0",
      "source": "user",
      "user_modified": true
    }
  ],
  "total": 2
}
```

#### PUT /v1/skills/{name} — 上传/更新技能

上传 SKILL.md 内容作为租户自定义技能。上传后技能会被标记为 `user_modified`，不会被系统自动同步覆盖。

```bash
curl -X PUT http://localhost:8080/v1/skills/my-custom-skill \
  -H "Authorization: Bearer hk_your_api_key" \
  -H "Content-Type: text/plain" \
  -d '---
name: "my-custom-skill"
description: "自定义业务技能"
version: "1.0.0"
---

# My Custom Skill

You are a specialized assistant for my business domain.'
```

响应：

```json
{
  "status": "uploaded",
  "skill": "my-custom-skill"
}
```

限制：请求体最大 1MB。

#### DELETE /v1/skills/{name} — 删除技能

```bash
curl -X DELETE http://localhost:8080/v1/skills/my-custom-skill \
  -H "Authorization: Bearer hk_your_api_key"
```

成功返回 `204 No Content`。

### GET /v1/openapi — OpenAPI 规范

返回 JSON 格式的 OpenAPI 3.0 规范文档。

```bash
curl http://localhost:8080/v1/openapi
```

---

## 错误码

| HTTP 状态码 | 说明 |
|-------------|------|
| 200 | 成功 |
| 204 | 成功（无内容，如 OPTIONS preflight） |
| 400 | 请求参数错误 |
| 401 | 未认证（缺少或无效的 Token） |
| 403 | 权限不足（角色不满足要求） |
| 404 | 资源不存在 |
| 429 | 速率限制超出 |
| 500 | 服务器内部错误 |

## 速率限制

每个请求的响应头包含速率限制信息：

| 响应头 | 说明 |
|--------|------|
| `X-RateLimit-Limit` | 当前窗口允许的请求数（RPM） |
| `X-RateLimit-Remaining` | 剩余可用请求数 |
| `Retry-After` | 限流时返回，建议等待秒数（固定 60s） |

速率限制按租户维度统计，同一租户下的所有 API Key 共享配额。未认证请求按 IP 地址限流。

## CORS

通过 `SAAS_ALLOWED_ORIGINS` 环境变量配置：

- 设置为 `*` 允许所有来源
- 设置为逗号分隔的域名列表精确控制

允许的请求方法：`GET, POST, PUT, DELETE, OPTIONS`
允许的请求头：`Authorization, Content-Type, X-Hermes-Session-Id, X-Hermes-User-Id`

## 静态页面

当配置 `SAAS_STATIC_DIR` 时，以下路由由静态文件服务：

| 路径 | 说明 |
|------|------|
| `/` | 首页（index.html） |
| `/admin.html` | 管理面板 |
| `/isolation-test.html` | 多租户隔离测试页面 |
| `/static/*` | 静态资源目录 |
