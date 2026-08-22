# 身份字段分类与公开契约

后台身份必须按语义选择术语，禁止以 `User/user_id` 代指所有身份事实。

| 语义 | Go / JSON | 使用范围 |
|---|---|---|
| 租户后台账号实体 | `AccountID` / `account_id` | `tenant_accounts`、账号角色、认证身份；会话中仅作 `ainp.account` 可空兼容外键 |
| 当前认证主体 | `PrincipalRef` / `principal_kind + principal_id` | `auth.Context`、Session、授权请求/快照、运行时授权 Scope |
| 操作发起者事实 | `ActorID` / `actor_id` | 审计、Outbox、任务、通知、操作日志和不可变事件 |
| 业务用户 | 模块自定义 `UserID` / `user_id` | 仅业务域明确建模的普通用户、owner 或个人数据 |

框架与系统模块中的具体归类：

- Idempotency、Tool Scope/Confirmation/Invocation、Trusted Runtime Context 与短期 Tool/附件授权 Token 使用 `PrincipalID/principal_id`。
- MCP Grant/Confirmation、Knowledge Base owner、账号设置、助手个性化快照与站内通知收件人使用 `AccountID/account_id`。
- Audit、Outbox、Job、设置变更与运行日志中的操作发起者使用 `ActorID/actor_id`；运行日志当前认证上下文使用 `principal_id`。
- AI memory 的 `user_id` 是当前保留的业务域字段，不得据此向 framework 或 system access 新增同名账号字段。

## 后台账号公开面

- canonical Action：`system.account_list|get|create|update|status_update|roles_update|password_reset`。
- 权限组：`system.account_management`；菜单与 Page Context：`system.accounts`；资源：`system.account`；个人头像资源：`system.account_profile`。
- HTTP 使用 `/api/v1/system/accounts/*`，前端使用 `/system/accounts`、`Api.Account` 和“账号管理”。
- 旧 `system.user_*`、`system.users`、`system.user`、`system.user_profile` 与 `/system/users/*` 只能通过显式兼容 alias 解析，不得进入权限目录、菜单、Action/AI Discovery、Dynamic Reference 或前端 canonical 契约。

## Token 兼容

- JWT 的 canonical 主体字段是标准 `sub`。
- 兼容期签发时 `sub` 与旧 `uid` 同源双写；解析优先 `sub`，缺失时才回退 `uid`。
- `sub` 与 `uid` 同时存在但不一致必须拒绝，禁止产生双值身份。
- 附件传输 Token 同样先归一为单一 `PrincipalID`；兼容期可读取旧 `uid`，与 `sub/principal_id` 冲突时拒绝。

## 兼容窗口与删除条件

- 兼容入口从 `v0.1.7` 开始提供，预计在 `v0.3.0` 移除；实际移除不得早于连续两个发布周期完成监测。
- Action/HTTP alias 命中记录结构化日志 `identity compatibility alias used`，字段包含 `compatibility_kind=action`、`alias_id` 和 `canonical_id`。
- 仅含旧 `uid` 的租户 JWT 命中记录 `identity compatibility token used`，字段包含 `compatibility_kind=jwt_uid`；迁移后还应巡检旧权限码与 `system.user_profile` 资源链接存量。
- 删除旧 Action、HTTP、Page Context、附件资源 alias 和 JWT `uid` 前，必须同时满足：连续两个发布周期无兼容日志、所有环境已应用 `0055_identity_contract.sql`、旧权限与资源链接存量为零、已确认没有仍受支持的旧前端或外部调用方。
- 附件传输 Token 的旧 `uid` 读取可在上述条件满足且最长 Token TTL 已过后删除。

## 变更检查

- 新字段先判断是账号、当前主体、发起者事实还是业务用户，不做全仓机械替换。
- 历史持久化字段改名必须使用版本化 migration，并处理新旧值同时存在的冲突。
- 新兼容入口必须显式声明 alias 且有“不进入 Discovery”的测试。
- `platform/framework` 与 `platform/modules/system/access` 新增 `UserID/user_id` 会触发架构门禁；例外必须在测试 allowlist 中写明业务语义和删除条件。
