# 数据库迁移

- 开发启动仍由 `modules/system.AutoMigrate` 创建基础表。
- 已发布环境按编号执行本目录 SQL，不得只依赖 GORM Tag 改变存量结构。
- `0002_users_auth_version.sql` 为存量用户补齐授权版本；执行后旧 JWT 需要重新登录。
- `0003_role_management.sql` 只为角色补充描述与内置保护字段；初始管理员、内置角色和权限由安装向导创建。
- `0004_tenant_setting_values.sql` 创建租户设置覆盖值表；分类/字段 Schema 由代码注册，不在库中存 Definition。
- `0005_attachments.sql` 创建 `attachments` / `attachment_links` 与状态 CHECK；附件策略与 AccessPolicy 由代码注册。
- `0011_audit_log_indexes.sql` 为租户审计时间线、Action 与资源筛选增加复合索引。
- `0012_authentication_sessions.sql` 创建统一认证身份、challenge 与轮换会话表；旧 JWT 缺少 `sid`，升级后需重新登录。
