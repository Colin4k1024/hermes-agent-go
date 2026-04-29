# 配置指南

> Hermes Agent Go 全部环境变量和配置项参考。

## 配置优先级

```
环境变量 > config.yaml > 默认值
```

## SaaS 服务配置

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | 是 | - | PostgreSQL 连接字符串，格式：`postgres://user:pass@host:5432/dbname?sslmode=disable` |
| `HERMES_ACP_TOKEN` | 是 | - | 静态管理员 Bearer Token，用于 admin 端点认证 |
| `SAAS_API_PORT` | 否 | `8080` | SaaS API 服务端口 |
| `SAAS_ALLOWED_ORIGINS` | 否 | -（不启用 CORS） | CORS 允许的来源，`*` 表示全部，或逗号分隔的域名列表 |
| `SAAS_STATIC_DIR` | 否 | -（不提供静态文件） | 静态文件目录路径，如 `./internal/dashboard/static` |
| `HERMES_API_PORT` | 否 | `8081` | OpenAI 兼容适配器端口 |
| `HERMES_API_KEY` | 否 | - | OpenAI 兼容适配器的 Bearer Token |
| `HERMES_ACP_PORT` | 否 | - | ACP 服务器端口（不设置则不启动 ACP） |

## LLM 配置

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `LLM_API_URL` | 否 | - | LLM API 端点 URL |
| `LLM_API_KEY` | 否 | - | LLM API 认证密钥 |
| `LLM_MODEL` | 否 | - | 默认 LLM 模型名称 |
| `HERMES_MODEL` | 否 | - | CLI 模式默认模型 |
| `HERMES_PROVIDER` | 否 | - | LLM 提供商（openai / anthropic / auto） |
| `HERMES_BASE_URL` | 否 | - | CLI 模式 LLM API Base URL |
| `HERMES_API_KEY_LLM` | 否 | - | CLI 模式 LLM API Key |
| `HERMES_API_MODE` | 否 | - | API 协议模式（openai / anthropic） |
| `HERMES_MAX_ITERATIONS` | 否 | `20` | Agent 最大迭代次数 |
| `HERMES_MAX_TOKENS` | 否 | `4096` | 单次响应最大 token 数 |

## 存储配置

### PostgreSQL

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DATABASE_URL` | 是（SaaS 模式） | - | PostgreSQL 连接字符串 |
| `DATABASE_DRIVER` | 否 | `postgres` | 数据库驱动类型 |

### Redis

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `REDIS_URL` | 否 | - | Redis 连接字符串，用于分布式速率限制 |

### MinIO / S3

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `MINIO_ENDPOINT` | 否 | - | MinIO 服务地址（如 `localhost:9000`） |
| `MINIO_ACCESS_KEY` | 否 | - | MinIO 访问密钥 |
| `MINIO_SECRET_KEY` | 否 | - | MinIO 密钥 |
| `MINIO_BUCKET` | 否 | `hermes-skills` | MinIO 存储桶名称 |
| `MINIO_USE_SSL` | 否 | `false` | 是否使用 SSL 连接 MinIO |
| `BUNDLED_SKILLS_DIR` | 否 | `skills` | 内置技能目录路径，用于租户自动 Provisioning |

当配置了 MinIO 后，系统在以下时机自动同步技能：
- **创建租户时**：异步将 `BUNDLED_SKILLS_DIR` 中的所有技能复制到租户 MinIO 前缀，并生成默认 SOUL.md
- **服务启动时**：遍历所有租户执行增量同步（新增/更新内置技能，跳过用户修改过的技能）

## 可观测性配置

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 否 | -（不启用 tracing） | OpenTelemetry OTLP gRPC 端点 |
| `OTEL_EXPORTER_OTLP_INSECURE` | 否 | `false` | 是否使用不安全连接 |
| `OTEL_SERVICE_NAME` | 否 | `hermes-agent` | OTel 服务名 |

## 调试与运行时

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `HERMES_DEBUG` | 否 | `false` | 启用调试日志（LLM 请求/响应详情） |
| `HERMES_DEFAULT_MODEL` | 否 | - | 全局默认模型（config fallback） |
| `HERMES_FILE_STATE` | 否 | - | 启用文件状态跟踪 |
| `HERMES_GATEWAY_URL` | 否 | - | Gateway URL，用于消息平台集成 |

## 记忆系统

Hermes 支持多种外部记忆提供商，按需配置：

### Honcho

| 变量 | 说明 |
|------|------|
| `HONCHO_API_KEY` | Honcho 记忆服务 API Key |
| `HONCHO_APP_ID` | Honcho 应用 ID |
| `HONCHO_BASE_URL` | Honcho 服务地址 |
| `HONCHO_USER_ID` | Honcho 用户标识 |

### Mem0

| 变量 | 说明 |
|------|------|
| `MEM0_API_KEY` | Mem0 记忆服务 API Key |
| `MEM0_BASE_URL` | Mem0 服务地址 |

### Supermemory

| 变量 | 说明 |
|------|------|
| `SUPERMEMORY_API_KEY` | Supermemory 服务 API Key |
| `SUPERMEMORY_BASE_URL` | Supermemory 服务地址 |

## Gateway 平台

消息网关（`hermes gateway`）支持多个平台适配器：

| 变量 | 说明 |
|------|------|
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token |
| `DISCORD_BOT_TOKEN` | Discord Bot Token |
| `SLACK_APP_TOKEN` | Slack App-Level Token |
| `SLACK_BOT_TOKEN` | Slack Bot Token |
| `DMWORK_API_URL` | DmWork API 端点 |
| `DMWORK_BOT_TOKEN` | DmWork Bot Token |

## 辅助视觉模型

用于图像识别等多模态场景：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUXILIARY_VISION_API_KEY` | 回退到 `OPENROUTER_API_KEY` | Vision LLM API Key |
| `AUXILIARY_VISION_BASE_URL` | OpenRouter 端点 | Vision LLM Base URL |
| `AUXILIARY_VISION_MODEL` | - | Vision 模型名称 |

## 工具 API Key

各类工具集成所需的 API Key：

| 变量 | 说明 |
|------|------|
| `OPENROUTER_API_KEY` | OpenRouter API Key |
| `GEMINI_API_KEY` | Google Gemini API Key |
| `GOOGLE_API_KEY` | Google API Key（搜索等） |
| `EXA_API_KEY` | Exa 搜索 API Key |
| `FIRECRAWL_API_KEY` | Firecrawl 网页抓取 API Key |
| `FAL_KEY` | fal.ai 图像生成 API Key |
| `ELEVENLABS_API_KEY` | ElevenLabs TTS API Key |
| `OSV_ENDPOINT` | Open Source Vulnerabilities API 端点 |

## 邮件配置

| 变量 | 说明 |
|------|------|
| `EMAIL_FROM` | 发件人地址 |
| `EMAIL_USERNAME` | 邮箱账号 |
| `EMAIL_PASSWORD` | 邮箱密码 |
| `EMAIL_SMTP_HOST` | SMTP 服务器地址 |
| `EMAIL_SMTP_PORT` | SMTP 端口 |
| `EMAIL_IMAP_HOST` | IMAP 服务器地址 |

## 浏览器工具

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BROWSER_BACKEND` | `local` | 浏览器后端（`local` 或 `browserbase`） |
| `BROWSER_CDP_URL` | - | Chrome DevTools Protocol 端点 |
| `HASS_URL` | - | Home Assistant 地址 |
| `HASS_TOKEN` | - | Home Assistant 长期访问令牌 |

## 终端与 SSH

| 变量 | 说明 |
|------|------|
| `TERMINAL_CWD` | 终端工具工作目录 |
| `SSH_PASSWORD` | SSH 认证密码 |

## CLI 模式配置

以下变量仅在 CLI 交互模式下生效：

| 变量 | 说明 |
|------|------|
| `HERMES_HOME` | Hermes 主目录（默认 `~/.hermes`） |
| `HERMES_PROFILE` | 当前 profile 名称 |
| `HERMES_DISPLAY_THEME` | CLI 主题 |
| `HERMES_TERMINAL_BACKEND` | 终端后端类型（local/docker/ssh/modal/daytona/singularity/persistent） |

## config.yaml 参考

CLI 模式的主配置文件位于 `~/.hermes/config.yaml`：

```yaml
# LLM 配置
model: "gpt-4o"
provider: "openai"
base_url: "https://api.openai.com/v1"
api_mode: "openai"

# Agent 行为
max_iterations: 20
max_tokens: 4096
context_compression: true
compression_threshold: 80000

# 终端
terminal:
  backend: "local"
  timeout: 30

# 显示
display:
  theme: "default"
  show_tool_calls: true
  show_reasoning: false

# 模型路由
smart_routing:
  enabled: false
  cheap_model: "gpt-4o-mini"
  threshold: 0.3

# 回退链
fallback:
  enabled: true
  models:
    - "gpt-4o"
    - "claude-sonnet-4-20250514"
```

## Docker Compose 配置示例

`docker-compose.dev.yml` 提供完整的本地开发环境：

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: hermes
      POSTGRES_USER: hermes
      POSTGRES_PASSWORD: hermes
    ports:
      - "5432:5432"

  redis:
    image: redis:7
    ports:
      - "6379:6379"

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: hermes
      MINIO_ROOT_PASSWORD: hermespass
    ports:
      - "9000:9000"   # API
      - "9001:9001"   # Console
```

## 安全注意事项

- `HERMES_ACP_TOKEN` 用于管理员认证，生产环境必须使用强密码
- `DATABASE_URL` 中的密码建议通过 Kubernetes Secret 或 Vault 注入
- API Key 以 SHA-256 哈希存储，无法逆向获取原始值
- 设置 `SAAS_ALLOWED_ORIGINS` 为具体域名，避免在生产环境使用 `*`
- MinIO 凭证应与 PostgreSQL 凭证独立管理

## 相关文档

- [快速开始](saas-quickstart.md) — 5 分钟上手
- [认证系统](authentication.md) — Auth Chain 和 RBAC 详解
- [部署指南](deployment.md) — 生产环境部署
