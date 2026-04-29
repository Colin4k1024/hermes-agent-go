# Skills 指南

> Hermes Agent 的 Skills 系统：格式、来源、安装、多租户隔离和内置技能。

## 概述

Skills 是 Hermes Agent 的可插拔能力模块，通过 `SKILL.md` 文件定义指令和行为，赋予 Agent 特定领域的专业能力。

## SKILL.md 格式

每个 Skill 由一个 `SKILL.md` 文件定义，包含 YAML frontmatter 和 Markdown 正文：

```markdown
---
name: "skill-name"
description: "一句话描述"
version: "1.0.0"
author: "作者"
tags: ["tag1", "tag2"]
---

# Skill 标题

这里写 Skill 的详细指令、行为规则和示例。
Agent 会将这些内容作为系统提示的一部分来指导其行为。
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | Skill 唯一标识，用于安装和引用 |
| `description` | 是 | 简短描述，用于搜索和展示 |
| `version` | 否 | 语义化版本号 |
| `author` | 否 | 作者信息 |
| `tags` | 否 | 标签数组，用于分类和搜索 |

### 正文编写

Markdown 正文是 Agent 实际读取的指令内容，应包含：

- 角色定义和行为准则
- 可用的工具和操作说明
- 输入/输出格式约定
- 示例对话或交互流程
- 限制和注意事项

## Skills 来源

### 1. 本地 Skills

存储在用户主目录下：

```
~/.hermes/skills/{skill-name}/SKILL.md
```

CLI 模式下直接加载本地 Skills。

### 2. 内置 Skills

仓库 `skills/` 目录包含 77+ 个预置 Skills，分为 26 个类别：

| 类别 | 说明 | 示例 |
|------|------|------|
| `software-development` | 软件开发 | 代码审查、重构、调试 |
| `research` | 研究分析 | 文献综述、数据分析 |
| `creative` | 创意写作 | 故事创作、文案生成 |
| `data-science` | 数据科学 | 数据清洗、可视化 |
| `devops` | DevOps | CI/CD、容器化 |
| `gaming` | 游戏相关 | 游戏设计、NPC 对话 |
| `github` | GitHub 操作 | PR 审查、Issue 管理 |
| `productivity` | 生产力工具 | 任务管理、笔记 |
| `red-teaming` | 安全测试 | 渗透测试、漏洞分析 |
| `smart-home` | 智能家居 | Home Assistant 控制 |
| `domain` | 领域特定 | 自定义业务技能 |
| ... | ... | ... |

完整列表参见仓库 `skills/` 目录。

### 3. MinIO 租户 Skills（SaaS 模式）

在 SaaS 多租户模式下，每个租户拥有独立的 Skills，存储在 MinIO/S3 对象存储中：

```
MinIO Bucket: hermes-skills
├── {tenant-id-1}/
│   ├── .manifest.json          # 技能清单（hash、来源、修改状态）
│   ├── _soul/SOUL.md           # 租户人格文件
│   ├── skill-a/SKILL.md
│   └── skill-b/SKILL.md
├── {tenant-id-2}/
│   ├── .manifest.json
│   ├── _soul/SOUL.md
│   ├── skill-c/SKILL.md
│   └── skill-d/SKILL.md
```

**隔离保证**：
- 每个租户只能访问自己的 Skills
- Skill 路径以 `{tenant-id}/` 作为前缀隔离
- 不同租户可以拥有同名但内容不同的 Skills

### 自动 Provisioning

**创建租户时自动触发**：当通过 `POST /v1/tenants` 创建新租户时，系统异步执行：

1. **技能同步**：将 `skills/` 目录中的 77+ 内置技能复制到租户的 MinIO 前缀
2. **Soul 创建**：生成默认 `SOUL.md` 人格文件到 `{tenant-id}/_soul/SOUL.md`
3. **清单写入**：创建 `.manifest.json` 记录每个技能的 SHA-256 hash 和来源

**服务启动时全量同步**：`hermes saas-api` 启动时遍历所有租户，执行增量同步：
- 新增的内置技能会自动安装
- 已更新的内置技能会覆盖（除非用户已修改）
- 用户修改过的技能（`user_modified: true`）不会被覆盖

#### 技能清单（.manifest.json）

```json
{
  "version": 1,
  "skills": {
    "code-review": {
      "hash": "a1b2c3d4...",
      "source": "builtin",
      "installed_at": "2026-04-29T12:00:00Z",
      "user_modified": false
    },
    "my-custom-skill": {
      "hash": "",
      "source": "user",
      "installed_at": "2026-04-29T13:00:00Z",
      "user_modified": true
    }
  },
  "synced_at": "2026-04-29T12:00:00Z"
}
```

#### 配置 MinIO

```bash
export MINIO_ENDPOINT="localhost:9000"
export MINIO_ACCESS_KEY="hermes"
export MINIO_SECRET_KEY="hermespass"
export MINIO_BUCKET="hermes-skills"
export MINIO_USE_SSL="false"
```

### 技能管理 API

SaaS 模式提供 RESTful API 管理租户技能，无需直接操作 MinIO：

#### 列出技能

```bash
curl http://localhost:8080/v1/skills \
  -H "Authorization: Bearer hk_your_api_key"
```

#### 上传/更新技能

```bash
curl -X PUT http://localhost:8080/v1/skills/my-custom-skill \
  -H "Authorization: Bearer hk_your_api_key" \
  -d '---
name: "my-custom-skill"
description: "自定义业务技能"
version: "1.0.0"
---

# My Custom Skill
...'
```

上传的技能会自动标记为 `user_modified`，后续内置技能同步不会覆盖。

#### 删除技能

```bash
curl -X DELETE http://localhost:8080/v1/skills/my-custom-skill \
  -H "Authorization: Bearer hk_your_api_key"
```

详细 API 参考见 [API 参考文档](api-reference.md#技能管理-v1skills)。

#### 使用 mc CLI 直接上传（高级）

```bash
mc alias set hermes http://localhost:9000 hermes hermespass
mc cp ./my-skill/SKILL.md hermes/hermes-skills/{tenant-id}/my-skill/SKILL.md
```

或使用种子脚本批量上传：

```bash
./scripts/seed_minio_skills.sh
```

### 4. Skills Hub

Hermes 支持从在线 Hub 发现和安装 Skills。

#### 默认 Hub 源

| 源 | 类型 | 信任级别 | 说明 |
|----|------|----------|------|
| agentskills.io | URL | community | 社区 Skill 市场 |
| hermes-official | GitHub | trusted | 官方可选 Skills |

#### 搜索 Skills

```bash
hermes skill search "code review"
```

搜索引擎会查询所有配置的 Hub 源，返回匹配的 Skills 列表。

#### 安装 Skill

```bash
hermes skill install <skill-name> --source <source-url>
```

安装过程：
1. 从 Hub 下载 `SKILL.md`
2. 执行安全扫描（根据信任级别）
3. 写入 `~/.hermes/skills/{name}/SKILL.md`
4. 更新锁文件 `~/.hermes/skills/.hub/lock.json`

#### 卸载 Skill

```bash
hermes skill uninstall <skill-name>
```

## 安全扫描

从 Hub 安装的 Skills 会经过安全扫描：

| 信任级别 | 扫描强度 | 失败处理 |
|----------|----------|----------|
| `builtin` | 无扫描 | 直接安装 |
| `trusted` | 标准扫描 | 警告但允许安装 |
| `community` | 严格扫描 | 阻止可疑 Skill |

扫描检查项包括：
- 危险指令（如尝试执行系统命令）
- 敏感数据访问模式
- 注入攻击模式

当安全扫描决策为 `InstallBlock` 时，Skill 文件会被自动删除。

## 锁文件

安装的 Hub Skills 记录在锁文件中：

```
~/.hermes/skills/.hub/lock.json
```

```json
[
  {
    "name": "code-review",
    "source": "https://agentskills.io/api/skills/code-review",
    "installed": "2026-04-29T12:00:00Z"
  }
]
```

锁文件用于：
- 跟踪已安装的 Skills 及其来源
- 支持批量更新和版本管理
- 审计 Skill 安装历史

## SaaS 模式下的 Skill 隔离

在 SaaS 多租户环境中，Skills 实现了完整的租户隔离：

### 加载优先级

```
1. 租户专属 Skills（MinIO: {tenant-id}/skill-name/）
2. 全局共享 Skills（本地 skills/ 目录，通过自动同步复制到 MinIO）
```

### Chat 中的技能注入

当用户发送聊天请求时，系统自动从 MinIO 加载租户已安装的所有技能，并将技能列表注入到系统提示中：

```
## Available Skills
- code-review: 代码审查助手
- debugging: 调试专家
- my-custom-skill: 自定义业务技能
```

### 隔离测试

项目包含完整的 Skill 隔离测试脚本：

```bash
# 运行 Skill 隔离测试
./scripts/test_real_skill_isolation.sh
```

测试验证：
- 不同租户分配不同 Skills 后，各自的 Agent 行为符合预期
- 跨租户不会加载到其他租户的 Skills
- Skill 内容变更不影响其他租户

### 示例：为租户分配不同人格

```bash
# 租户 A：海盗风格
mc cp pirate-skill/SKILL.md hermes/hermes-skills/tenant-${TENANT_A}/pirate/SKILL.md

# 租户 B：科学家风格
mc cp scientist-skill/SKILL.md hermes/hermes-skills/tenant-${TENANT_B}/scientist/SKILL.md
```

两个租户调用同一个 Chat API，但 Agent 展现不同的性格和专业知识。

## 创建自定义 Skill

### 1. 创建目录和文件

```bash
mkdir -p ~/.hermes/skills/my-custom-skill
cat > ~/.hermes/skills/my-custom-skill/SKILL.md << 'EOF'
---
name: "my-custom-skill"
description: "自定义业务 Skill"
version: "1.0.0"
author: "your-name"
tags: ["custom", "business"]
---

# My Custom Skill

## 角色
你是一个专业的 XX 助手。

## 行为准则
- 始终使用专业术语
- 回答要简洁准确

## 输出格式
使用 Markdown 格式输出结果。
EOF
```

### 2. 验证加载

```bash
hermes skill list
```

### 3. 部署到 SaaS（推荐使用 API）

```bash
curl -X PUT http://localhost:8080/v1/skills/my-custom-skill \
  -H "Authorization: Bearer hk_your_api_key" \
  --data-binary @~/.hermes/skills/my-custom-skill/SKILL.md
```

或直接通过 MinIO：

```bash
mc cp ~/.hermes/skills/my-custom-skill/SKILL.md \
  hermes/hermes-skills/${TENANT_ID}/my-custom-skill/SKILL.md
```

## 相关文档

- [快速开始](saas-quickstart.md) — 基础环境搭建
- [配置指南](configuration.md) — MinIO 和 Skills 配置
- [架构概览](architecture.md) — Skills 系统在架构中的位置
- [认证系统](authentication.md) — 租户和 API Key 管理
