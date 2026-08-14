# AINP 下游 Go 模块架构

本规则适用于外部仓库的 Go 业务模块。规则来源必须取自该仓库 `go.mod` 固定的同一 `github.com/yzxingtu2026/ainp/platform@vX.Y.Z` 发布物，禁止从 `main` 复制后与依赖版本混用。

- `modules/<module>` 根包只允许 `register.go`、`facade.go` / `*_facade.go`、`contracts.go` / `*_contracts.go`、`constants.go` / `*_constants.go`。
- 根 `register.go` 定义 `type Module`，并实现 `ID() string` 与 `Register(*app.App) error`；`Prepare`、`MigrationSource`、`AutoMigrate`、`RuntimeAssets` 按需实现。
- Action、Service、Repository、Model、HTTP 和领域流程进入 owner 明确的能力子包，禁止空目录与 `common`、`utils`、`misc`、`shared` 等兜底包。
- `modules/A` 禁止 import `modules/B`；能力子包禁止反向 import 自身模块根包。跨模块协作通过 framework contract 和 composition root 注入。
- 模块树内禁止 `_test.go`；测试集中到仓库指定的 `tests` 目录，并通过公开 API 验证行为。
- 禁止用基线、通配豁免或自定义白名单绕过架构失败。

每次提交前以及 CI 中，在下游 Go Module 根目录执行：

```bash
go run github.com/yzxingtu2026/ainp/platform/cmd/ainp-archcheck \
  --profile downstream \
  --modules-root ./modules \
  --tests-root ./tests
go vet ./...
go test ./...
```

如实际目录不是 `./modules` 或 `./tests`，显式传对应目录。检查器默认从最近的 `go.mod` 推导 module import prefix；只有特殊工具场景才使用 `--module-path` 显式传入。
