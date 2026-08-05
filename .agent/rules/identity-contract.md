# 身份契约

- 后台账号用 `Account/account_id`，当前认证主体用 `Principal/principal_id`，审计与事件发起者用 `Actor/actor_id`；业务 `User/user_id` 仅由业务模块按真实语义定义。
- 后台账号 canonical 公开面为 `system.account_*`、`system.account_management`、`system.accounts`、`system.account`、`system.account_profile`、`/api/v1/system/accounts/*` 与 `/system/accounts`。
- 旧 user 账号标识只能通过显式 alias 兼容，不得进入权限目录、菜单、AI Tool、Action Discovery、Dynamic Reference 或前端 canonical 契约。
- JWT canonical 主体使用 `sub`；兼容期 `uid` 只可同源双写或作为无 `sub` 旧 Token 的回退值，冲突必须拒绝。
- 禁止机械全仓替换业务 `User/user_id`；持久化改名必须走版本化 migration。
- framework 与 system access 的 `UserID/user_id` 仅允许进入带语义和删除条件的门禁 allowlist；新账号引用必须分类为 Account、Principal 或 Actor。
- 兼容入口从 `v0.1.7` 开始，预计 `v0.3.0` 移除；连续两个发布周期无兼容日志、0055 全环境应用且旧存量清零后才可删除。
