# Lessons Learned

---

## 2026-04-28 — SaaS Readiness: 安全审查驱动的批量修复

**场景：** P0-P5 一次性交付 23 个新文件后，并行 code-reviewer + security-reviewer 发现 29 个安全问题（5 CRITICAL + 10 HIGH + 9 MEDIUM + 5 LOW）。

**问题：**
1. 批量交付后的安全审查修复成本高 — 5 CRITICAL + 6 HIGH 需要跨 7 个文件的协调修改。
2. Pre-existing 安全问题（CRIT-1 ACP auth bypass）在新代码审查中被发现，但修复涉及 out-of-scope 代码。
3. 新增代码虽然编译和集成测试通过，但缺少专门的单元测试，导致安全修复缺乏回归保护。

**建议：**
1. 每个 Phase 交付后立即运行安全审查，不要等全量完成 — 修复成本随积累指数增长。
2. Pre-existing 安全问题应在 intake 阶段显式列入 backlog 并评估优先级，不要等到新代码审查时才发现。
3. 新增安全关键代码（auth、RBAC、tenant isolation）应在实现阶段同步补充单元测试，不要作为"后续补充"。
4. Store interface 扩展（如 `GetByID`）应在设计阶段预判，避免安全修复时才发现缺少必要的数据访问方法。

---

## 2026-04-30 — Enterprise Hardening: Requirement Challenge 的价值

**场景：** Phase 1-5 企业级加固，6 轮需求挑战将原始计划从"高风险并行"调整为"分阶段可控交付"。

**问题：**
1. 原计划 Phase 1 并行 OIDC + RBAC + RLS + 无状态化 — 四个高风险改造同期，任一阻塞则整个 Phase 停摆。
2. 外部依赖（企业 IdP）未确认就排进 Phase 1，导致关键交付路径不可控。
3. 组件依赖顺序错误 — Lifecycle Manager 排在 Phase 4，但 Phase 2 引入的 OTel/断路器已需要生命周期管理。

**建议：**
1. Requirement Challenge Session 优先识别外部依赖 — 将依赖外部 IdP 的工作推后，优先交付"内部可控"slice。
2. 依赖分析要前置 — 新组件引入基础设施依赖（如后台 goroutine 需 Lifecycle Manager），必须在设计阶段发现。
3. 安全机制分阶段激活 — RLS 可延后，先用集成测试覆盖应用层路径，降低并行风险。
4. 用户体验关键路径（SSE 真实流式）不应排到最后版本 — 感知延迟是第一印象，应在中期交付。

---

## 2026-04-30 — gofmt CI 门禁的价值

**场景：** Phase 1-5 全量代码推送后，CI gofmt 检查发现 8 个文件格式不合规，需要额外修复 commit。

**问题：**
1. 多文件并行编辑时，部分文件在工具调用中产生格式漂移（多余空行、缩进不一致）。
2. gofmt 未在本地预提交检查中执行，问题在 CI 层才暴露，增加了一个 fix commit。

**建议：**
1. Go 代码每次 commit 前执行 `gofmt -l .` — 零容忍格式问题，不留到 CI 阶段。
2. 多文件并行写入时，完成后统一运行 `gofmt -w .` 再检查，不依赖 IDE 自动格式化。
3. Makefile 的 `make lint` target 应包含 gofmt 检查，作为本地门禁。

---
