# GitHub Issue / PR 工作流

- 可交付工作必须有 Issue（背景、AC、范围/非范围）；范围变化先更新或拆 Issue。
- PR 关联：`Fixes #编号` 或 `Refs #编号`；远端用 `gh`，禁止写入 token。
- 运行时变更加 `needs-testing`；纯文档/规则/CI 可直接合入。
- 测试通过收口顺序：先 `gh pr review --approve`，再打 `tested-pass`（禁止颠倒）；失败打 `tested-fail` 并关联 Bug。
