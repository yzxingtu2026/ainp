# 后端目录与模块边界

- 仓库：`platform/`（Go）与 `frontend/` 平级；module path：`github.com/yzxingtu2026/ainp/platform`。
- Phase 1 modules 仅 `system`（可选）+ `demo`；业务域稍后。
- 依赖：`modules → framework`；`framework` 不得 import `modules`；module 间默认不互引。
- 骨架即接 GORM + Postgres；配置用 YAML。

详规：`docs/development/backend-layout.md`
