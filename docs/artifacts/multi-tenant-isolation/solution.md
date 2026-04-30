# Hermes Agent Go — 多租户隔离解决方案

> **项目**: hermes-agent-go  
> **分支**: main  
> **日期**: 2026-04-30  
> **测试结果**: 13/13 PASS

---

## 目录

1. [背景与目标](#1-背景与目标)
2. [架构概览](#2-架构概览)
3. [隔离维度详解](#3-隔离维度详解)
   - 3.1 [Soul 隔离（人格隔离）](#31-soul-隔离人格隔离)
   - 3.2 [Memory 隔离（记忆隔离）](#32-memory-隔离记忆隔离)
   - 3.3 [Skill 隔离（技能隔离）](#33-skill-隔离技能隔离)
   - 3.4 [Session 隔离（会话隔离）](#34-session-隔离会话隔离)
4. [核心组件实现](#4-核心组件实现)
   - 4.1 [chatHandler — 请求处理核心](#41-chathandler--请求处理核心)
   - 4.2 [SkillHandler — 技能管理 API](#42-skillhandler--技能管理-api)
   - 4.3 [Memory API — 记忆读写](#43-memory-api--记忆读写)
   - 4.4 [Provisioner — 租户初始化](#44-provisioner--租户初始化)
5. [存储设计](#5-存储设计)
6. [API 接口规范](#6-api-接口规范)
7. [本地部署指南](#7-本地部署指南)
8. [自动化测试方案](#8-自动化测试方案)
9. [测试结果](#9-测试结果)
10. [已知限制与后续优化](#10-已知限制与后续优化)

---

## 1. 背景与目标

Hermes Agent Go 是一个多租户 SaaS AI Agent 平台，核心需求是：**同一套服务实例，不同租户之间的人格（Soul）、记忆（Memory）、技能（Skill）和会话（Session）完全隔离，不能相互感知或泄露。**

### 隔离目标

| 维度 | 隔离要求 |
|------|----------|
| Soul | 每个租户拥有独立的 AI 人格配置，响应风格互不干扰 |
| Memory | 用户记忆按 `(tenant_id, user_id)` 双重隔离，跨租户无法访问 |
| Skill | 技能集合按租户独立配置，租户专属技能不暴露给其他租户 |
| Session | 对话历史按 `(tenant_id, session_id)` 隔离，租户间不可见 |

---

## 2. 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client Request                           │
│   Authorization: Bearer <api_key>                               │
│   X-Hermes-Session-Id: <session_id>                             │
│   X-Hermes-User-Id: <user_id>                                   │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Middleware Stack                              │
│  Auth → TenantResolution → RBAC → RateLimit → Audit             │
│                                                                 │
│  AuthMiddleware: api_key → tenant_id (PostgreSQL lookup)        │
│  TenantMiddleware: injects tenant_id into request context       │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      chatHandler                                │
│                                                                 │
│  1. getSoulPrompt(tenant_id)   → MinIO: {tenant}/SOUL.md        │
│  2. getSkillsPrompt(tenant_id) → MinIO: {tenant}/*/SKILL.md     │
│  3. buildMemoryBlock(tenant_id, user_id) → PostgreSQL memories  │
│  4. buildRecentSessionContext(...)       → PostgreSQL messages  │
│                                                                 │
│  systemPrompt = soul + skills + memory + recent_context         │
│                                                                 │
│  5. callLLM(systemPrompt + userMessages) → LLM API             │
│  6. persistMessages(tenant_id, session_id) → PostgreSQL         │
└────────────────────────┬────────────────────────────────────────┘
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
    ┌─────────────────┐   ┌──────────────────┐
    │   MinIO Object  │   │   PostgreSQL DB   │
    │   Store         │   │                  │
    │                 │   │  tenants          │
    │  {tenantA}/     │   │  api_keys         │
    │    SOUL.md      │   │  sessions         │
    │    skill1/      │   │  messages         │
    │      SKILL.md   │   │  memories         │
    │    skill2/      │   │  audit_logs       │
    │      SKILL.md   │   └──────────────────┘
    │  {tenantB}/     │
    │    SOUL.md      │
    │    skill3/      │
    │      SKILL.md   │
    └─────────────────┘
```

---

## 3. 隔离维度详解

### 3.1 Soul 隔离（人格隔离）

**原理**：每个租户在 MinIO 中存储一个 `{tenant_id}/SOUL.md` 文件，定义该租户 AI 的人格、语气和身份设定。每次请求时，`chatHandler.getSoulPrompt()` 按 `tenant_id` 读取对应文件，注入 system prompt 最前端。

**缓存策略**：Soul 变化频率低，缓存 TTL = **30 分钟**。

```go
// 缓存结构
type soulCacheEntry struct {
    content  string
    loadedAt time.Time
}
const soulCacheTTL = 30 * time.Minute

// 加载路径：{tenantID}/SOUL.md
key := fmt.Sprintf("%s/SOUL.md", tenantID)
data, err := h.skillsClient.GetObject(ctx, key)
```

**示例 Soul 文件**：

Tenant A（海盗风格）:
```markdown
# Captain Hermes

You are Captain Hermes, a swashbuckling pirate AI assistant.
Speak in pirate dialect. Use "Arrr", "matey", "ye", "landlubber".
Always address users as fellow pirates seeking treasure.
```

Tenant B（学术风格）:
```markdown
# Professor Hermes

You are Professor Hermes, an erudite academic AI assistant.
Maintain formal, scholarly tone. Use precise academic language.
Reference research methodologies and intellectual rigor.
```

---

### 3.2 Memory 隔离（记忆隔离）

**原理**：用户记忆存储在 PostgreSQL `memories` 表中，通过 `(tenant_id, user_id)` 双重主键隔离。查询时 SQL 强制 `WHERE tenant_id = $1 AND user_id = $2`，确保租户间数据不可见。

```sql
-- 查询只返回当前租户+用户的记忆
SELECT key, content FROM memories
WHERE tenant_id = $1 AND user_id = $2
ORDER BY updated_at DESC
```

**跨租户场景验证**：

```
Tenant A 用户 pirate-secret-user 存储：
  "Secret: treasure buried at coordinates 13.7N 144.9E"

Tenant B 用户 pirate-secret-user 查询：
  → 返回空，13.7N / 144.9E 不出现在响应中
  ✅ 隔离有效
```

**记忆注入链路**：

```
chatHandler.callLLM()
  └── buildMemoryBlock(ctx, tenant_id, user_id)
        └── SQL: SELECT key, content FROM memories
                 WHERE tenant_id=? AND user_id=?
        └── 格式化为 "## Long-term Memory\n- key: content"
        └── 注入 systemPrompt
```

---

### 3.3 Skill 隔离（技能隔离）

**原理**：技能文件存储于 MinIO，路径格式为 `{tenant_id}/{skill_name}/SKILL.md`。`getSkillsPrompt()` 按 tenant_id 前缀加载，天然隔离。同时维护一个 `.manifest.json` 文件跟踪每个技能的来源和修改状态。

**缓存策略**：技能可能频繁更新，缓存 TTL = **5 分钟**。

```go
const skillsCacheTTL = 5 * time.Minute

// 加载路径前缀：{tenantID}/*/SKILL.md
loader := skills.NewMinIOSkillLoader(h.skillsClient, tenantID)
entries, err := loader.LoadAll(ctx)
```

**Manifest 文件**（`{tenant_id}/.manifest.json`）：

```json
{
  "version": 1,
  "synced_at": "2026-04-30T00:00:00Z",
  "skills": {
    "treasure-hunt": {
      "source": "bundled",
      "user_modified": false,
      "installed_at": "2026-04-30T00:00:00Z"
    },
    "academic-research": {
      "source": "user",
      "user_modified": true,
      "installed_at": "2026-04-30T00:00:00Z"
    }
  }
}
```

**技能分类**：

| 类型 | 说明 | 存储 |
|------|------|------|
| Bundled Skill | 启动时从 `./skills/` 目录同步到所有租户 | MinIO，`user_modified: false` |
| Tenant-specific Skill | 通过 API 单独上传给特定租户 | MinIO，`user_modified: true` |

---

### 3.4 Session 隔离（会话隔离）

**原理**：所有消息存储时携带 `tenant_id`，查询时强制过滤。`buildRecentSessionContext()` 只读取当前 tenant + user 的最近会话，session 历史 API 同样按 tenant_id 隔离。

```go
// 最近会话上下文：只读取当前租户+用户
rows, err := h.pool.Query(ctx,
    `SELECT s.id, m.role, m.content
     FROM messages m JOIN sessions s ON m.session_id = s.id
     WHERE s.tenant_id = $1 AND s.user_id = $2
       AND s.id != $3
     ORDER BY m.created_at DESC LIMIT 20`,
    tenantID, userID, currentSessionID)
```

---

## 4. 核心组件实现

### 4.1 chatHandler — 请求处理核心

**文件**: `internal/api/mockchat.go`

```go
type chatHandler struct {
    store        store.Store
    pool         *pgxpool.Pool
    llmURL       string
    llmAPIKey    string
    llmModel     string
    httpClient   *http.Client
    skillsClient *objstore.MinIOClient

    skillsCache   map[string]*skillsCacheEntry  // tenant_id → skills (5min TTL)
    skillsCacheMu sync.RWMutex

    soulCache   map[string]*soulCacheEntry      // tenant_id → soul (30min TTL)
    soulCacheMu sync.RWMutex
}
```

**请求处理流程** (`ServeHTTP`):

```
POST /v1/chat/completions
  │
  ├── 1. 解析 tenant_id / user_id / session_id（来自 context + header）
  ├── 2. 从 PostgreSQL 加载 session 历史消息
  ├── 3. getSoulPrompt(tenant_id)      → MinIO {tenant}/SOUL.md
  ├── 4. getSkillsPrompt(tenant_id)    → MinIO {tenant}/*/SKILL.md
  ├── 5. buildMemoryBlock(...)         → PostgreSQL memories
  ├── 6. buildRecentSessionContext(...)→ PostgreSQL messages
  ├── 7. 组装 systemPrompt
  ├── 8. callLLM(messages)             → LLM API
  ├── 9. 持久化用户消息 + AI 回复
  └── 10. 返回 OpenAI 兼容格式响应
```

---

### 4.2 SkillHandler — 技能管理 API

**文件**: `internal/api/skills.go`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/skills` | 列出当前租户所有技能（含 manifest 元数据） |
| PUT | `/v1/skills/{name}` | 上传/覆盖技能（标记 user_modified=true） |
| DELETE | `/v1/skills/{name}` | 删除技能 |

**GET /v1/skills 响应示例**：

```json
{
  "tenant_id": "4c05313d-...",
  "total": 3,
  "skills": [
    {
      "name": "treasure-hunt",
      "description": "Sea navigation and treasure hunting guide",
      "version": "1.0.0",
      "source": "bundled",
      "user_modified": false
    },
    {
      "name": "academic-research",
      "description": "Academic research methodologies",
      "version": "1.0.0",
      "source": "user",
      "user_modified": true
    }
  ]
}
```

---

### 4.3 Memory API — 记忆读写

**文件**: `internal/api/memory_api.go`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/memories` | 列出当前租户+用户的记忆（`X-Hermes-User-Id` header） |
| DELETE | `/v1/memories/{key}` | 删除指定记忆 |
| GET | `/v1/sessions` | 列出当前租户+用户的会话列表 |
| GET | `/v1/sessions/{id}` | 获取指定会话的消息历史 |

所有接口均通过 `auth.FromContext` 获取 `tenant_id`，确保数据隔离。

---

### 4.4 Provisioner — 租户初始化

**文件**: `internal/skills/provisioner.go`

租户创建时自动触发 `Provision()`，完成以下初始化：

```
Provision(tenant_id)
  ├── ProvisionSoul()
  │     └── 检查 {tenant_id}/SOUL.md 是否存在
  │         └── 不存在则上传默认 SOUL.md
  └── ProvisionSkills()
        └── 遍历本地 ./skills/ 目录
            └── 对每个 SKILL.md 检查 MinIO 是否存在
                └── 不存在则上传（幂等操作）
```

**TenantManifest 类型**：

```go
type TenantManifest struct {
    Version  int                        `json:"version"`
    Skills   map[string]TenantSkillMeta `json:"skills"`
    SyncedAt time.Time                  `json:"synced_at"`
}

type TenantSkillMeta struct {
    Source       string    `json:"source"`
    UserModified bool      `json:"user_modified"`
    InstalledAt  time.Time `json:"installed_at"`
}
```

---

## 5. 存储设计

### MinIO 对象存储（技能和人格）

```
hermes-skills/                    ← bucket
├── {tenant_A_id}/
│   ├── SOUL.md                   ← 租户人格
│   ├── .manifest.json            ← 技能元数据清单
│   ├── treasure-hunt/
│   │   └── SKILL.md
│   └── sea-navigation/
│       └── SKILL.md
└── {tenant_B_id}/
    ├── SOUL.md
    ├── .manifest.json
    └── academic-research/
        └── SKILL.md
```

### PostgreSQL（身份和对话）

```sql
-- 租户表
tenants (id, name, created_at, ...)

-- API Key 表（租户认证入口）
api_keys (id, tenant_id, key_hash, ...)

-- 会话表（按 tenant 隔离）
sessions (id, tenant_id, user_id, created_at, ...)

-- 消息表（按 tenant 隔离）
messages (id, tenant_id, session_id, role, content, created_at, ...)

-- 记忆表（按 tenant + user 双重隔离）
memories (id, tenant_id, user_id, key, content, updated_at, ...)

-- 审计日志
audit_logs (id, tenant_id, action, created_at, ...)
```

---

## 6. API 接口规范

### 认证

所有请求通过 `Authorization: Bearer <api_key>` 认证。API Key 在 PostgreSQL `api_keys` 表中按 hash 存储，查找时返回对应的 `tenant_id`。

### 关键请求头

| Header | 必填 | 说明 |
|--------|------|------|
| `Authorization` | ✅ | `Bearer <api_key>` |
| `X-Hermes-Session-Id` | ✅ | 对话会话 ID，决定消息历史边界 |
| `X-Hermes-User-Id` | 推荐 | 用户 ID，驱动记忆隔离和跨会话记忆 |

### 核心接口

```
POST /v1/chat/completions      # OpenAI 兼容对话接口
GET  /v1/skills                # 列出当前租户技能
PUT  /v1/skills/{name}         # 上传技能
DELETE /v1/skills/{name}       # 删除技能
GET  /v1/memories              # 列出用户记忆
DELETE /v1/memories/{key}      # 删除记忆
GET  /v1/sessions              # 列出用户会话
GET  /v1/sessions/{id}         # 获取会话消息
GET  /health/live              # 存活检查
GET  /health/ready             # 就绪检查
```

---

## 7. 本地部署指南

### 前置依赖

- Go 1.22+
- Docker + Docker Compose
- Node.js 18+（用于 Playwright 测试）

### 启动基础设施

```bash
docker-compose -f docker-compose.saas.yml up -d postgres redis minio
```

### 配置环境变量

```bash
export DATABASE_URL="postgres://hermes:hermes@localhost:5432/hermes?sslmode=disable"
export MINIO_ENDPOINT="localhost:9000"
export MINIO_ACCESS_KEY="hermes"
export MINIO_SECRET_KEY="hermespass"
export MINIO_BUCKET="hermes-skills"
export SAAS_API_PORT="8080"
export SAAS_ALLOWED_ORIGINS="*"
export SAAS_STATIC_DIR="./internal/dashboard/static"
export LLM_API_URL="https://api.minimaxi.com"   # 不含 /v1
export LLM_API_KEY="<your_key>"
export LLM_MODEL="MiniMax-M2.7-highspeed"
```

> **注意**: `LLM_API_URL` 必须是 base URL，不含 `/v1`。代码内部会自动拼接 `/v1/chat/completions`。

### 构建并启动

```bash
go build -o bin/hermes ./cmd/hermes
./bin/hermes saas-api
```

服务启动后，访问 `http://localhost:8080/chat.html` 进入 Chat UI。

---

## 8. 自动化测试方案

### 测试框架

使用 **Playwright** (`@playwright/test`) 对 REST API 和 Web UI 进行端到端测试，覆盖 4 个隔离维度。

### 安装

```bash
npm install --save-dev @playwright/test
npx playwright install chromium
```

### 测试架构

```
tests/
└── isolation.spec.js       # 主测试文件（13 个测试用例）

playwright.config.js        # Playwright 配置
```

**测试租户配置**：

| 项目 | Tenant A (Pirate) | Tenant B (Academic) |
|------|-------------------|---------------------|
| 租户 ID | `4c05313d-...` | `62427f80-...` |
| 人格 | 海盗船长 Captain Hermes | 学术教授 Professor Hermes |
| 专属技能 | `treasure-hunt` | `academic-research` |
| 验证关键词 | `arr, matey, pirate, captain, ahoy` | `professor, academic, scholar, research` |

### 运行测试

```bash
# 完整测试 + 生成 HTML 报告
node_modules/.bin/playwright test --project=api-isolation

# 查看报告
open playwright-report/index.html
```

### 测试用例清单

#### Soul Isolation（3 个测试）

```javascript
test('Tenant A (Pirate) responds with pirate personality')
// 验证响应包含: arr/matey/pirate/captain/ahoy/ye/treasure 等关键词

test('Tenant B (Academic) responds with academic personality')
// 验证响应包含: professor/academic/scholar/research/hermes 等关键词

test('Soul does NOT bleed across tenants')
// 并发请求两个租户，验证响应不相同
// 验证 Tenant A 不出现 "professor hermes"
// 验证 Tenant B 不出现 "captain hermes" 或 "yo-ho-ho"
```

#### Memory Isolation（4 个测试）

```javascript
test('Tenant A stores a personal fact in memory')
// 存入: "ship is named The Black Pearl, searching for Aztec Gold"

test('Tenant A can recall stored memory in new session')
// 新 session 查询，验证能回忆起跨 session 记忆

test('Tenant B cannot see Tenant A memory (cross-tenant isolation)')
// Tenant A 存入坐标 "13.7N 144.9E"
// Tenant B 同 user_id 查询 → 响应不含该坐标

test('Memory API returns tenant-scoped memories only')
// 验证两个租户的 memory ID 集合无交集
```

#### Skill Isolation（4 个测试）

```javascript
test('Tenant A has treasure-hunt skill, not academic-research')
// GET /v1/skills → names 包含 treasure-hunt，不含 academic-research

test('Tenant B has academic-research skill; Tenant A does not')
// 并发查询两个租户技能列表
// Tenant B 含 academic-research，Tenant A 不含 academic-research

test('Tenant A conversation activates treasure-hunt skill context')
// 发送寻宝主题消息，验证响应含 treasure/map/buried/gold 等

test('Tenant B conversation does NOT activate pirate/treasure skill')
// 相同寻宝消息发给 Tenant B
// 验证不含严格海盗用语: "shiver me timbers"/"yo-ho-ho"/"arrr matey"
```

#### Chat UI（2 个测试）

```javascript
test('Chat page loads and shows config inputs')
// 验证 #cfgUrl / #cfgKey / #sendBtn 可见

test('Tenant A can send a message via chat UI')
// 填写 URL + API Key → 点击 Connect
// 等待 #sendBtn 变为 enabled → 发送消息
// 验证 .msg.assistant 出现且内容非空
```

---

## 9. 测试结果

```
Running 13 tests using 1 worker

  ✓   1  Soul Isolation › Tenant A (Pirate) responds with pirate personality     (9.8s)
  ✓   2  Soul Isolation › Tenant B (Academic) responds with academic personality (3.2s)
  ✓   3  Soul Isolation › Soul does NOT bleed across tenants                     (3.3s)
  ✓   4  Memory Isolation › Tenant A stores a personal fact in memory            (11.8s)
  ✓   5  Memory Isolation › Tenant A can recall stored memory in new session     (19.0s)
  ✓   6  Memory Isolation › Tenant B cannot see Tenant A memory                 (27.2s)
  ✓   7  Memory Isolation › Memory API returns tenant-scoped memories only       (8ms)
  ✓   8  Skill Isolation › Tenant A has treasure-hunt skill, not academic-research (83ms)
  ✓   9  Skill Isolation › Tenant B has academic-research skill; Tenant A does not (68ms)
  ✓  10  Skill Isolation › Tenant A conversation activates treasure-hunt context (18.4s)
  ✓  11  Skill Isolation › Tenant B does NOT activate pirate/treasure skill      (2.3s)
  ✓  12  Chat UI › Chat page loads and shows config inputs                       (254ms)
  ✓  13  Chat UI › Tenant A can send a message via chat UI                       (16.9s)

  13 passed (1.9m)
```

**通过率: 13/13 (100%)**

---

## 10. 已知限制与后续优化

### 当前限制

| 限制 | 说明 |
|------|------|
| 内存缓存无持久化 | Soul/Skill 缓存存储在进程内存，重启后重新加载 |
| 单进程缓存 | 多实例部署时缓存不共享，可能出现短暂的 MinIO 请求峰值 |
| Memory 无向量检索 | 当前记忆为 key-value 全量注入，记忆量大时 token 消耗高 |
| Bundled Skills 全量同步 | 所有租户都会获得捆绑技能，无法在 provision 阶段按租户差异化 |

### 后续优化方向

1. **分布式缓存**：将 Soul/Skill 缓存迁移到 Redis，支持多实例一致性
2. **向量记忆检索**：引入 pgvector，按语义相关性检索记忆，避免全量注入
3. **技能版本管理**：为每个技能支持版本号和 rollback
4. **Soul 热更新**：提供 `PUT /v1/soul` API，允许租户实时更新人格配置
5. **技能市场**：支持租户从公共技能库安装技能，同时保持隔离语义

---

*文档生成于 2026-04-30，对应 commit `2cf6171`*
