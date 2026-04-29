# 可观测性

> Hermes SaaS API 的监控、追踪、日志和运维能力。

## Prometheus 指标

Hermes 通过 `GET /metrics` 端点暴露 Prometheus 格式的指标，无需认证。

### HTTP 指标

| 指标名 | 类型 | 标签 | 说明 |
|--------|------|------|------|
| `hermes_http_requests_total` | Counter | `method`, `path`, `status`, `tenant_id` | HTTP 请求总数 |
| `hermes_http_request_duration_seconds` | Histogram | `method`, `path`, `tenant_id` | 请求延迟分布 |
| `hermes_http_requests_in_flight` | Gauge | 无 | 当前正在处理的并发请求数 |

**特性**：
- 所有指标按 `tenant_id` 维度分割，未认证请求标记为 `anonymous`
- 路径自动归一化（截断 >64 字符），降低指标基数
- Histogram 使用 Prometheus 默认桶（`.005`, `.01`, `.025`, `.05`, `.1`, `.25`, `.5`, `1`, `2.5`, `5`, `10`）

### 采集配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'hermes'
    scrape_interval: 15s
    static_configs:
      - targets: ['hermes-api:8080']
    metrics_path: /metrics
```

### 推荐告警规则

```yaml
groups:
  - name: hermes
    rules:
      - alert: HighErrorRate
        expr: rate(hermes_http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        labels:
          severity: warning

      - alert: HighLatency
        expr: histogram_quantile(0.99, rate(hermes_http_request_duration_seconds_bucket[5m])) > 5
        for: 5m
        labels:
          severity: warning

      - alert: TenantRateLimited
        expr: rate(hermes_http_requests_total{status="429"}[5m]) > 0
        for: 1m
        labels:
          severity: info
```

## OpenTelemetry 追踪

Hermes 支持 OpenTelemetry 分布式追踪，通过 OTLP gRPC 协议导出 span。

### 配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 无（禁用追踪） | OTLP gRPC 端点地址 |
| `OTEL_EXPORTER_OTLP_INSECURE` | `false` | 是否使用不安全连接 |
| `OTEL_SERVICE_NAME` | `hermes-agent` | 服务名称 |

**当 `OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时，追踪完全禁用，零开销。**

### 启用追踪

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"
export OTEL_EXPORTER_OTLP_INSECURE="true"
export OTEL_SERVICE_NAME="hermes-saas"
./hermes saas-api
```

### 追踪特性

- **W3C Trace Context 传播**：支持 `traceparent` / `tracestate` 请求头
- **Baggage 传播**：支持 `baggage` 请求头传递上下文
- **Batch Exporter**：异步批量导出 span，不阻塞请求处理
- **pgx Tracer**：PostgreSQL 查询自动产生子 span

### 与 Jaeger 集成

```yaml
# docker-compose.dev.yml 追加
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "4317:4317"   # OTLP gRPC
      - "16686:16686" # Jaeger UI
    environment:
      COLLECTOR_OTLP_ENABLED: "true"
```

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"
export OTEL_EXPORTER_OTLP_INSECURE="true"
```

访问 `http://localhost:16686` 查看追踪数据。

## 结构化日志

Hermes 使用 Go 标准库 `log/slog` 进行结构化日志记录。

### Context-Enriched Logger

日志自动从 Context 中提取以下字段：

| 字段 | 来源 | 说明 |
|------|------|------|
| `request_id` | RequestID 中间件 | 请求唯一标识 |
| `tenant_id` | Tenant 中间件 | 租户 ID |
| `session_id` | Handler | 会话 ID（如适用） |
| `trace_id` | OTel | 分布式追踪 ID |

**工作原理**：
1. RequestID 中间件生成 `request_id` 并写入 Context
2. Auth + Tenant 中间件提取 `tenant_id` 并写入 Context
3. Logging 中间件创建携带上述字段的 `slog.Logger` 并注入 Context
4. 后续 Handler 通过 `observability.ContextLogger(ctx)` 获取增强的 Logger

```go
// Handler 中使用
logger := observability.ContextLogger(r.Context())
logger.Info("Processing request", "action", "chat_completion")
// 输出: level=INFO msg="Processing request" request_id=abc123 tenant_id=xxx action=chat_completion
```

### 日志级别

通过标准 `slog` 机制控制：

| 级别 | 用途 |
|------|------|
| `DEBUG` | Hub 搜索失败、详细查询信息 |
| `INFO` | 服务启动、迁移完成、请求处理 |
| `WARN` | 静态目录不存在、降级处理 |
| `ERROR` | 数据库连接失败、Handler 错误 |

## 审计日志

所有通过认证的请求自动记录到 `audit_logs` 表。

### 记录内容

| 字段 | 说明 |
|------|------|
| `tenant_id` | 请求方所属租户 |
| `user_id` | 认证身份 ID |
| `action` | `METHOD /path` 格式 |
| `detail` | 请求详情（已脱敏） |
| `request_id` | 请求唯一标识 |
| `status_code` | HTTP 响应状态码 |
| `latency_ms` | 请求处理耗时（毫秒） |
| `created_at` | 记录时间 |

### 查询审计日志

```bash
curl "http://localhost:8080/v1/audit-logs?limit=50" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 审计日志特性

- **自动记录**：Audit 中间件在响应写入后自动记录，无需 Handler 干预
- **查询脱敏**：敏感字段在写入前进行清理
- **按租户隔离**：审计日志自动关联到请求方的 tenant_id
- **索引优化**：`idx_audit_tenant` 和 `idx_audit_request` 加速查询

## 健康探针

### GET /health/live — 存活探针

服务启动即返回 200，表示进程存活。

```json
{"status": "ok"}
```

### GET /health/ready — 就绪探针

检查数据库连接状态，确认服务可以处理请求。

```json
{"status": "ready", "database": "ok"}
```

数据库不可用时返回 503：

```json
{"status": "not_ready", "database": "error: connection refused"}
```

### Kubernetes 探针配置

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 15
```

## Request ID 传播

每个请求都会携带唯一的 `X-Request-ID`：

1. 如果请求头包含 `X-Request-ID`，使用客户端提供的值
2. 否则服务端自动生成 UUID
3. 该 ID 贯穿整个请求生命周期：日志、审计、追踪、响应头

```
客户端 → X-Request-ID: abc123
                │
                ▼
日志:     request_id=abc123
审计:     request_id=abc123
追踪:     span.attribute("request_id", "abc123")
响应:     X-Request-ID: abc123
```

## 监控仪表板

### Grafana 推荐面板

1. **请求概览**：`hermes_http_requests_total` 按 status 分组
2. **延迟分布**：`hermes_http_request_duration_seconds` P50/P95/P99
3. **并发量**：`hermes_http_requests_in_flight`
4. **租户活跃度**：`hermes_http_requests_total` 按 tenant_id 分组
5. **错误率**：`rate(hermes_http_requests_total{status=~"5.."}[5m])`
6. **速率限制**：`hermes_http_requests_total{status="429"}` 按 tenant_id 分组

### PromQL 常用查询

```promql
# 每秒请求数（按状态码）
sum(rate(hermes_http_requests_total[5m])) by (status)

# P99 延迟
histogram_quantile(0.99, sum(rate(hermes_http_request_duration_seconds_bucket[5m])) by (le))

# 各租户请求分布
sum(rate(hermes_http_requests_total[5m])) by (tenant_id)

# 被限流的请求
sum(rate(hermes_http_requests_total{status="429"}[5m])) by (tenant_id)
```

## 相关文档

- [配置指南](configuration.md) — 可观测性环境变量
- [部署指南](deployment.md) — 生产环境检查清单
- [架构概览](architecture.md) — 中间件栈详解
- [API 参考](api-reference.md) — /metrics 和 /health 端点
