# 数据库迁移

- 开发启动仍由 `modules/system.AutoMigrate` 创建基础表。
- 已发布环境按编号执行本目录 SQL，不得只依赖 GORM Tag 改变存量结构。
- `0002_users_auth_version.sql` 为存量用户补齐授权版本；执行后旧 JWT 需要重新登录。
