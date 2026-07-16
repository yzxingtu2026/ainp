# 后端目录与模块边界

- 仓库：`platform/`（Go）与 `frontend/` 平级；module path：`github.com/yzxingtu2026/ainp/platform`。
- Phase 1 modules 包含 `system`（配置与身份）、`operations`（运行态运维）和 `demo`；业务域稍后。
- 依赖：`modules → framework`；`framework` 不得 import `modules`；module 间默认不互引。
- 骨架即接 GORM + Postgres；配置用 YAML。
- 运行时业务设置在 `framework/setting` 注册；租户覆盖值在业务表，见 `settings-system`。
- 对象存储在 `framework/storage`（ObjectStore）；禁止业务拼本地路径，见 `object-storage`。
- Transactional Outbox 在 `framework/outbox`；业务只经 EventWriter 写事件，见 `transactional-outbox`。
- 后台 Job 在 `framework/job` + `asynqadapter`；业务禁止直连 Asynq，见 `background-jobs`。
- Job / Outbox / 通知投递管理 Action、API、菜单统一归 `modules/operations` 与 `operations.*` 语义；领域服务通过接口注入，`operations` 禁止反向依赖 `system`。
- 后端 `_test.go` 统一放在 `platform/tests/`，使用外部测试包通过公开 API 测试；禁止散落到 `framework/*`、`modules/*`。

详规：`docs/development/backend-layout.md`
