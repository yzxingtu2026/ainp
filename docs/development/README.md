# 开发规范

- [AI 模型供应商与模型目录](./ai-model-providers.md)：Provider Type、Secret、Discovery、Deployment 能力、Alias 与 Resolver 边界。
- [AI 模型统一调用协议](./ai-model-runtime.md)：Generate/Stream、Adapter、调用账本与安全边界。
- [AI Conversation 与上下文装配](./ai-conversations.md)：固定 Agent Version、追加式消息、幂等、预算装配与 Snapshot 边界。
- [AI 上下文压缩](./ai-context-compaction.md)：模型感知预算、确定性缩减、结构化摘要、质量校验与版本化 Snapshot。
- [AI Agent Runtime](./ai-agent-runtime.md)：Run/Step 状态机、lease、Outbox→Job、工具确认暂停与可靠恢复。
- [统一 Tool Runtime](./tool-runtime.md)：渐进式工具发现、Scope、Handle、确认与执行安全边界。
- [MCP OAuth 与远程传输](./mcp-oauth.md)：Streamable HTTP、PKCE、独立 audience、Grant 与实时授权边界。
- [AI 工具开发指南](./ai-tool-development.md)：新增 Action Tool、Builtin/Plugin/MCP/HTTP Provider 与 Executor 的操作手册。

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
- [租户数据访问](./tenant-data-access.md)：TenantDB、Registry、Callback、Repository、受控 SQL、复合约束与架构门禁。
- [统一认证与会话](./authentication-and-session.md)
- [审计日志（最小集）](./audit.md)
- [后端目录与模块边界](./backend-layout.md)
- [CodeSet、状态机与字符串常量](./code-sets-and-status.md)
- [租户角色与权限管理](./role-management.md)
- [时间展示与本地化](./datetime-display.md)
- [统一设置系统](./settings-system.md)
- [统一对象存储](./object-storage.md)
- [租户隔离附件系统](./attachments.md)
- [前端统一附件组件](./frontend-attachments.md)
- [统一通知协议与站内通知](./notifications.md)
- [事务性 Outbox](./transactional-outbox.md)
- [幂等与并发控制](./idempotency-concurrency.md)
- [后台 Job](./background-jobs.md)
- [Copilot PR 语义审查](./copilot-code-review.md)
- [发布版本与构建身份](./release-versioning.md)
- [容器部署基线](../deployment/containers.md)

新增或调整公共能力时，请同步更新本目录或 `.agent/rules/`、`.agent/skills/`、`.agent/team/`，运行 `agent-rules sync`，并在 Issue/PR 中说明规范位置。
