# 认证与授权

> Hermes SaaS API 的认证链、API Key 管理和 RBAC 访问控制。

## 认证链（Auth Chain）

Hermes 使用链式认证策略，按顺序尝试每种认证方式，首个匹配成功的方式生效：

```
请求 → Static Token → API Key → JWT → 匿名（401）
```

| 顺序 | 方式 | 适用场景 |
|------|------|----------|
| 1 | Static Token | 开发环境 / 管理员访问 |
| 2 | API Key | 多租户用户访问 |
| 3 | JWT | 生产环境集成（预留） |

所有认证方式提取后生成统一的 `AuthContext`：

```go
type AuthContext struct {
    Identity   string   // 用户 ID 或 API Key ID
    TenantID   string   // 从凭证派生，非请求头
    Roles      []string // ["user"] 或 ["admin"]
    AuthMethod string   // "static_token" / "api_key" / "jwt"
}
```

> 租户 ID 始终从凭证中派生，永远不会从请求头读取。这是多租户隔离的核心安全保障。

## Static Token 认证

最简单的认证方式，适用于开发测试和初始管理操作。

**配置**：设置 `HERMES_ACP_TOKEN` 环境变量。

**行为**：
- Bearer Token 与 `HERMES_ACP_TOKEN` 匹配时认证成功
- 使用 `crypto/subtle.ConstantTimeCompare` 防止时序攻击
- 自动映射到默认租户 `00000000-0000-0000-0000-000000000001`
- 固定角色为 `admin`

```bash
# 使用 Static Token 进行管理操作
curl http://localhost:8080/v1/tenants \
  -H "Authorization: Bearer your-acp-token"
```

**默认租户**：首次启动时自动创建，参数为：
- ID: `00000000-0000-0000-0000-000000000001`
- Name: `Default Tenant`
- Plan: `pro`
- Rate Limit: 120 RPM
- Max Sessions: 10

## API Key 认证

为每个租户创建独立的 API Key，支持细粒度权限控制。

### Key 格式

- 前缀：`hk_`（hermes key）
- 长度：随机生成，Base64 编码
- 存储：原始 Key 经 SHA-256 哈希后存入 `api_keys.key_hash` 字段

### 生命周期

```
创建 Key → 返回原始值（仅一次）→ 使用中 → 过期 / 撤销
```

### 创建 API Key

```bash
curl -X POST http://localhost:8080/v1/api-keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-service",
    "tenant_id": "tenant-uuid",
    "roles": ["user"]
  }'
```

响应中 `key` 字段仅在创建时返回，务必保存。

### 认证流程

1. 从 `Authorization: Bearer hk_xxx` 请求头提取 Token
2. 计算 SHA-256 哈希
3. 在 `api_keys` 表中查找匹配的 `key_hash`
4. 检查是否已撤销（`revoked_at IS NOT NULL`）
5. 检查是否已过期（`expires_at < now()`）
6. 返回 AuthContext（包含 `tenant_id` 和 `roles`）

### 撤销 Key

```bash
curl -X DELETE http://localhost:8080/v1/api-keys/key-uuid \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## JWT 认证（预留）

JWT 认证框架已内置，生产环境集成时启用。

**预期 Claims**：

| Claim | 说明 |
|-------|------|
| `sub` | 用户 ID |
| `tenant_id` | 租户 ID |
| `roles` | 角色数组 |
| `exp` | 过期时间 |

**签名算法**：RS256

在 `saas.go` 中取消注释以启用：

```go
authChain.Add(auth.NewJWTExtractor(jwtConfig))
```

## RBAC 访问控制

基于路径前缀的角色访问控制。

### 角色定义

| 角色 | 说明 |
|------|------|
| `admin` | 管理员，可访问所有端点 |
| `user` | 普通用户，仅可访问用户端点 |

### 端点权限矩阵

| 路径前缀 | 所需角色 | 说明 |
|----------|----------|------|
| `/v1/tenants` | `admin` | 租户管理 |
| `/v1/api-keys` | `admin` | API Key 管理 |
| `/v1/audit-logs` | `admin` | 审计日志查询 |
| `/v1/gdpr/` | `admin` | GDPR 数据管理 |
| `/v1/chat/completions` | `user` | Chat 接口 |
| `/v1/me` | `user` | 当前身份 |
| `/v1/usage` | `user` | 使用量统计 |
| `/v1/mock-sessions` | `user` | 会话管理 |
| `/v1/openapi` | `user` | API 文档 |
| `/health/*` | 无需认证 | 健康检查 |
| `/metrics` | 无需认证 | Prometheus 指标 |

### RBAC 判断逻辑

```
1. 从 Context 获取 AuthContext
2. 未认证 → 401 Unauthorized
3. 匹配路径前缀找到所需角色
4. 用户角色包含所需角色 → 放行
5. 用户角色包含 "admin" → 放行（admin 可访问所有端点）
6. 否则 → 403 Forbidden
```

## 中间件执行顺序

认证和授权在中间件链中的位置：

```
Tracing → Metrics → RequestID → Auth → Tenant → Logging → Audit → RBAC → RateLimit → Handler
```

| 中间件 | 职责 |
|--------|------|
| Auth | 从请求提取 AuthContext，写入 Context |
| Tenant | 从 AuthContext 提取 tenant_id |
| Audit | 记录所有认证请求到审计日志 |
| RBAC | 检查角色权限 |
| RateLimit | 按租户维度限流 |

## 安全设计

### 租户隔离

- 租户 ID 从凭证中派生，不接受请求头传入
- 所有数据库查询自动附加 `WHERE tenant_id = $1`
- 不同租户的 API Key 无法访问其他租户的数据

### 时序攻击防护

Static Token 比较使用 `crypto/subtle.ConstantTimeCompare`，防止通过响应时间推断 Token 值。

### Key 安全

- API Key 以 SHA-256 哈希存储，数据库泄露不会暴露原始 Key
- Key 的 `prefix` 字段（前 8 字符）用于管理界面识别，不包含完整 Key

### 速率限制

- 按租户维度统计请求频率（RPM）
- 同一租户下所有 API Key 共享配额
- 支持分布式限流（Redis）+ 本地 LRU 降级
- 匿名请求按 IP 地址限流

## 相关文档

- [API 参考](api-reference.md) — 完整端点文档
- [配置指南](configuration.md) — 环境变量
- [架构概览](architecture.md) — 中间件链详解
