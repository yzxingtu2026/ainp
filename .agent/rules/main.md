# 团队协作主规则

规则文件只写硬约束要点；细节以 `docs/` 为准，避免在此重复展开。

## 必守原则

1. **先 Issue 后代码**：可交付工作必须关联 Issue；范围/AC 以 Issue/PR 为准。
2. **禁止直改 main**：短分支 + PR 合入。
3. **小范围交付**：一分支一任务；超范围先拆 Issue。
4. **文档归位**：详规进 `docs/`；规则只留要点，变更同步沉淀。
5. **远端用 gh**：不得把 token 写入仓库。
6. **中文提交**：说明关联 Issue 与目的。
7. **团队信息**：查 `.agent/team/members.yml` / `roles.yml`。

## 开发硬约束

- 公共能力优先扩展，禁止业务内重复实现；新约定写入 `docs/development/`，Agent 要点写入 `.agent/rules/`。
- 业务逻辑下沉到服务/领域/公共模块，不堆在路由或 UI 事件里。
- 关键流程用统一日志，禁止零散 `print`。
- 前端栈、图标协议见关联规则。

## 关联规则

- `github-issues-workflow` / `branching-strategy` / `project-context-and-docs`
- `frontend-stack` → `docs/development/frontend-stack.md`
- `icon-library` → `docs/development/icon-library.md`
- `team-escalation`
