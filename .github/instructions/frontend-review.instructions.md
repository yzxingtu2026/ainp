---
applyTo: "frontend/**/*.{ts,tsx,js,jsx,vue}"
---

# 前端审查路由

审查前端变更时，除仓库级指令外，读取并应用：

- `.agent/rules/frontend-stack.md`
- `.agent/rules/frontend-i18n.md`
- `.agent/rules/icon-library.md`
- 涉及附件时读取 `.agent/rules/frontend-attachments.md`
- 涉及时间展示时读取 `.agent/rules/datetime-display.md`

重点检查是否绕过现有 API Client、状态管理、统一组件或 i18n 机制，是否新增硬编码用户可见文案，以及是否引入规则禁止的重复封装。
