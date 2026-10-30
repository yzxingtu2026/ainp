# 分支策略

- `main` 只接受 PR；开工前同步 `main`，一 Issue 一短分支，超范围先拆。
- 命名：`feat|fix|docs|chore/issue-<编号>-<slug>`；`hotfix/<slug>`；`spike/<name>-<slug>`（小写与连字符）。
- 与 open PR 改动重叠多或命中公共/核心文件时，先确认依赖再开工。
- 流程：`main` 更新 → 开分支 → 小步提交 → PR → 合入后删分支。
