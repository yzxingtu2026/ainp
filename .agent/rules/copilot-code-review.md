# Copilot PR 语义审查

- Copilot Code Review 只作为语义审查员，不替代 Required Check、人工 Approve 或 `tested-pass`。
- 仓库级审查指令维护在 `.github/copilot-instructions.md`；路径级指令维护在 `.github/instructions/*.instructions.md`。
- Copilot 审查必须使用简体中文，并为阻塞问题给出规则依据、阻塞原因和最小修复要点。
- 自动审查必须启用 `Review new pushes`；复审以最新 diff 与基础分支规则为准，不依赖 PR 评论回复作为上下文。
- 可机器判定的规则应沉淀为 Required Check，不得以 Copilot 是否留言作为自动放行条件。
- 详细启用方式、许可证前提与能力边界见 `docs/development/copilot-code-review.md`。
