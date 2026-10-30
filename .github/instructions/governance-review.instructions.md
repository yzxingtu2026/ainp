---
applyTo: ".agents/**,docs/**,.github/**,README.md"
---

# 规则与文档审查路由

审查规则、文档和 GitHub 配置时，读取 `.agents/rules/project-context-and-docs.md` 与 `.agents/rules/github-issues-workflow.md`。

重点检查：

- 详细说明是否归入 `docs/`，`.agents/rules/` 是否只保留 Agent 硬约束要点。
- 新增或修改公共约定时，详规与规则入口是否同步。
- 工作流权限是否最小化，是否避免把 token、个人身份或环境机密写入仓库。
- PR 模板、Issue 关联和测试决策是否与仓库工作流一致。
