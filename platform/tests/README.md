# Platform 测试

后端测试统一放在本目录，使用外部测试包 `tests`，通过公开 API 验证 `framework` 与 `modules` 的行为。不要在业务包目录中散落 `_test.go` 文件，也不要为测试新增生产导出 API。

`test_placement_architecture_test.go` 扫描 `platform/framework` 与 `platform/modules`，阻止散落测试回流。历史测试迁移或合并必须留下原文件到集中覆盖的对照记录；Issue #164 的首轮记录见 `issue_164_test_migration.md`。

## 执行

```bash
go test ./...
```

需要 PostgreSQL 的集成测试通过 `AINP_TEST_DATABASE_DSN` 显式启用；测试使用随机临时 schema，并在结束时清理：

```bash
AINP_TEST_DATABASE_DSN='host=127.0.0.1 user=ainp password=ainp dbname=ainp port=5433 sslmode=disable' \
  go test ./tests -run TestUserTenantUsernameMigration -v
```
