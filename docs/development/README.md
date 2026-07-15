# 开发规范

本目录用于沉淀面向开发者的公共规范详规。`.agent/rules/` 仅保留 Agent 硬约束要点并引用本文档，避免双份堆砌。

- 通用封装、统一组件、公共工具的适用范围和调用方式。
- 跨模块约定、目录边界、命名规范。
- 运行、测试、发布、观测等工程实践。

## 已沉淀规范

- [前端技术栈（Soybean Admin + Naive UI）](./frontend-stack.md)
- [图标库（Iconify + mdi）](./icon-library.md)
- [后端统一日志](./logging.md)
- [API 响应与错误](./api-response.md)
- [Action / Metadata / 菜单注册](./action-metadata.md)
- [多租户与鉴权上下文](./auth-context.md)
- [审计日志（最小集）](./audit.md)
- [后端目录与模块边界](./backend-layout.md)
- [CodeSet、状态机与字符串常量](./code-sets-and-status.md)
- [租户角色与权限管理](./role-management.md)
- [时间展示与本地化](./datetime-display.md)
- [统一设置系统](./settings-system.md)

新增或调整公共能力时，请同步更新本目录或 `.agent/rules/`、`.agent/skills/`、`.agent/team/`，运行 `agent-rules sync`，并在 Issue/PR 中说明规范位置。
