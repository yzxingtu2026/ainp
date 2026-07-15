# 数据库迁移

- 开发启动仍由 `modules/system.AutoMigrate` 创建基础表。
- 已发布环境按编号执行本目录 SQL，不得只依赖 GORM Tag 改变存量结构。
- `0002_users_auth_version.sql` 为存量用户补齐授权版本；执行后旧 JWT 需要重新登录。
- `0003_role_management.sql` 只为角色补充描述与内置保护字段；初始管理员、内置角色和权限由安装向导创建。
