# 部署指南

> Hermes SaaS API 的所有部署方式：Docker Compose、Kind 本地 K8s、Helm 生产部署。

## Dockerfile 选择

| Dockerfile | 用途 | 产物大小 |
|------------|------|----------|
| `Dockerfile` | 通用构建，CLI + 全功能 | ~50MB |
| `Dockerfile.local` | 本地开发，Docker Compose 用 | ~50MB |
| `Dockerfile.k8s` | Kubernetes 部署，含 health probe | ~50MB |
| `Dockerfile.k8s-slim` | 精简 K8s 镜像，多阶段构建 | ~30MB |
| `Dockerfile.saas` | SaaS API 专用，含静态文件 | ~55MB |

### Dockerfile.saas 特性

```dockerfile
# 多阶段构建
# Stage 1: 编译 Go 二进制
# Stage 2: 复制二进制 + 静态文件到 distroless 基础镜像
# 包含 /static 目录供 SAAS_STATIC_DIR 使用
# 默认 CMD: ["saas-api"]
```

## 方式一：Docker Compose 本地开发

最快的本地开发环境搭建方式。

### 启动全部服务

```bash
# 启动 PostgreSQL 16 + Redis 7 + MinIO + Gateway
docker compose -f docker-compose.dev.yml up -d

# 查看日志
docker compose -f docker-compose.dev.yml logs -f hermes-gateway
```

### 仅启动基础设施

```bash
# 启动数据层，手动运行 hermes
docker compose -f docker-compose.dev.yml up -d postgres redis minio

# 手动启动 SaaS API
export DATABASE_URL="postgres://hermes:hermes@127.0.0.1:5432/hermes?sslmode=disable"
export HERMES_ACP_TOKEN="admin-test-token"
export SAAS_ALLOWED_ORIGINS="*"
export SAAS_STATIC_DIR="./internal/dashboard/static"
./hermes saas-api
```

### 服务地址

| 服务 | 地址 | 说明 |
|------|------|------|
| PostgreSQL | `localhost:5432` | 用户 `hermes`，密码 `hermes`，数据库 `hermes` |
| Redis | `localhost:6379` | 无密码 |
| MinIO API | `localhost:9000` | 用户 `hermes`，密码 `hermespass` |
| MinIO Console | `localhost:9001` | Web 管理界面 |
| Hermes API | `localhost:8080` | SaaS API 端点 |

## 方式二：Kind 本地 K8s

使用 [Kind](https://kind.sigs.k8s.io/) 在本地运行 Kubernetes 集群。

### 1. 创建集群

```bash
kind create cluster --name hermes
```

### 2. 部署 PostgreSQL

```bash
kubectl apply -f deploy/kind/postgres.yaml
```

`postgres.yaml` 包含：
- PersistentVolumeClaim（1Gi）
- Deployment（PostgreSQL 16 单实例）
- Service（ClusterIP, port 5432）
- ConfigMap（初始化用户和数据库）

### 3. 构建并加载镜像

```bash
# 构建 SaaS 镜像
docker build -t hermes-agent-saas:local -f Dockerfile.saas .

# 加载到 Kind 集群
kind load docker-image hermes-agent-saas:local --name hermes
```

### 4. 安装 Helm Chart

```bash
helm install hermes deploy/helm/hermes-agent/ \
  -f deploy/kind/values.local.yaml
```

`values.local.yaml` 覆盖：
- `image.pullPolicy: Never`（使用本地镜像）
- `DATABASE_URL` 指向 Kind 内 PostgreSQL Service

### 5. 验证

```bash
kubectl get pods
kubectl port-forward svc/hermes-hermes-agent 8080:8080

curl http://localhost:8080/health/ready
```

## 方式三：Helm Chart 生产部署

### Chart 结构

```
deploy/helm/hermes-agent/
├── Chart.yaml          # Chart 元数据
├── values.yaml         # 默认值
└── templates/          # K8s 资源模板
```

### 安装

```bash
helm install hermes deploy/helm/hermes-agent/ \
  --namespace hermes \
  --create-namespace \
  --set env.DATABASE_URL="postgres://user:pass@pg-host:5432/hermes?sslmode=require" \
  --set env.HERMES_ACP_TOKEN="production-strong-token"
```

### values.yaml 关键配置

```yaml
replicaCount: 1

image:
  repository: hermes-agent-saas
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

# 启动参数
args:
  - saas-api

# 环境变量
env:
  DATABASE_URL: ""              # 必填
  HERMES_ACP_TOKEN: ""          # 必填
  SAAS_API_PORT: "8080"
  SAAS_ALLOWED_ORIGINS: "*"
  SAAS_STATIC_DIR: "/static"
  LLM_API_URL: ""
  LLM_API_KEY: ""
  LLM_MODEL: ""

# 资源限制
resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 250m
    memory: 256Mi

# 健康探针
probes:
  liveness:
    path: /health/live
    initialDelaySeconds: 5
    periodSeconds: 10
  readiness:
    path: /health/ready
    initialDelaySeconds: 10
    periodSeconds: 15

# 自动扩缩容
autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70

# TLS
tls:
  enabled: false
  certFile: ""
  keyFile: ""

# PostgreSQL 子 Chart（开发用）
postgresql:
  enabled: true
  auth:
    database: hermes
    username: hermes
    password: hermes-dev-password
```

### 使用外部 PostgreSQL

生产环境应使用外部管理的 PostgreSQL：

```bash
helm install hermes deploy/helm/hermes-agent/ \
  --set postgresql.enabled=false \
  --set env.DATABASE_URL="postgres://hermes:pass@rds-endpoint:5432/hermes?sslmode=require"
```

## 生产环境检查清单

### 安全

- [ ] `HERMES_ACP_TOKEN` 使用高强度随机字符串（32+ 字符）
- [ ] `SAAS_ALLOWED_ORIGINS` 设置为具体域名，禁止 `*`
- [ ] `DATABASE_URL` 通过 Kubernetes Secret 注入
- [ ] 启用 TLS（通过 Ingress 或 `tls.enabled`）
- [ ] API Key 定期轮换

### 可用性

- [ ] `replicaCount >= 2`
- [ ] 配置 `autoscaling`（建议 CPU 70% 阈值）
- [ ] 健康探针 `liveness` 和 `readiness` 已配置
- [ ] PostgreSQL 配置主从复制或使用云托管服务
- [ ] Redis 用于分布式速率限制（高可用场景）

### 可观测性

- [ ] `/metrics` 端点接入 Prometheus
- [ ] `OTEL_EXPORTER_OTLP_ENDPOINT` 配置 OpenTelemetry Collector
- [ ] 日志采集到集中式日志平台
- [ ] 配置审计日志保留策略

### 资源

- [ ] 设置合理的 CPU/Memory requests 和 limits
- [ ] PostgreSQL 配置连接池（推荐 PgBouncer）
- [ ] MinIO 使用持久化存储卷

## 相关文档

- [快速开始](saas-quickstart.md) — 本地开发环境
- [配置指南](configuration.md) — 所有环境变量
- [可观测性](observability.md) — 监控和追踪
- [架构概览](architecture.md) — 系统设计
