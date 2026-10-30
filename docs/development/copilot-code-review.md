# Copilot PR 语义审查

## 定位

GitHub Copilot Code Review 在 AINP 中定位为 PR 语义审查员：读取仓库规则和完整项目上下文，使用中文发现难以由 lint 或测试直接判断的架构、范围和规范问题。

Copilot 不是合并门禁。它固定提交 `Comment` 类型的 review，不会提交 `Approve` 或 `Request changes`，也不能替代 Required Check、人工审批和 `tested-pass` 流程。

## 仓库配置

- `.github/copilot-instructions.md`：仓库级审查目标、中文输出和问题格式。
- `.github/instructions/*.instructions.md`：根据变更路径路由到相关 `.agents/rules/`。
- `.agents/rules/`：团队硬约束的唯一规则源。
- `docs/development/`：规则引用的详细设计与实施说明。

Copilot 使用 PR 基础分支中的自定义指令。因此，修改审查规则的 PR 合入 `main` 后，后续 PR 才会稳定使用新规则。

## GitHub 启用步骤

前提：仓库作者具有包含 Code Review 的付费 Copilot 方案，或者组织启用了 Copilot Business/Enterprise、相关策略及 AI credits 付费。GitHub Free 组织本身不包含 Copilot Code Review。

仓库管理员在 GitHub 中执行：

1. 打开仓库 `Settings -> Rules -> Rulesets`。
2. 新建或编辑针对默认分支的 Branch ruleset。
3. 启用 `Automatically request Copilot code review`。
4. 启用 `Review new pushes`，确保作者每次推送修复后触发复审。
5. 可选启用 `Review draft pull requests`，在 Draft 阶段提前发现问题。
6. 对严格质量仓库，可在 `Settings -> Copilot -> Code review` 选择 Medium review effort；该能力可能仍处于预览，且消耗更多 AI credits 和 Actions minutes。

如果当前 GitHub 方案不提供 Rulesets 或自动请求选项，可以在 PR 的 Reviewers 区域手动请求 `Copilot`，但仍需要有效的 Copilot Code Review 资格。

## 审查与复审流程

1. 作者创建 PR，并在描述中填写 `Fixes #编号`、验证结果和规则/文档影响。
2. Copilot 根据最新 diff、基础分支规则和仓库上下文提交中文审查意见。
3. 作者修复阻塞问题并推送新提交。
4. `Review new pushes` 触发重新审查；未启用时由作者手动重新请求 Copilot。
5. 人工审核者确认阻塞问题已处理并完成审批；需要测试的 PR 继续遵循先 Approve、后 `tested-pass`。

不要把 PR 评论回复当作 Copilot 的持续对话上下文。GitHub 官方说明，回复 Copilot review comment 的内容对 Copilot 不可见；复审还可能重复已经解决的评论。因此每次复审必须依据最新 diff 和当前规则独立判断，人工审核者负责处理旧线程。

## 能力边界

Copilot 适合判断架构边界、公共能力复用、规则与实现一致性、测试缺口和范围偏移，但不保证发现所有问题。可确定判断的约束应逐步沉淀为独立的 `rules-compliance` CI，并配置为 Required Check；例如 Issue 关联、提交信息、禁止依赖、测试目录和文档同步检查。

不得通过“是否存在 Copilot 评论”自动决定 PR 通过或失败：无评论不等于完全合规，旧评论也不等于最新代码仍然违规。

## 官方参考

- [配置 Copilot 自动代码审查](https://docs.github.com/en/copilot/how-tos/copilot-on-github/set-up-copilot/configure-automatic-review)
- [使用 Copilot Code Review](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/request-a-code-review/use-code-review)
- [Copilot Code Review 能力与许可证](https://docs.github.com/en/copilot/concepts/agents/code-review)
