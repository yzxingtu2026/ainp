# 后端目录与模块边界

- 仓库：`platform/`（Go）与 `frontend/` 平级；module path：`github.com/yzxingtu2026/ainp/platform`。
- Phase 1 modules 包含 `system`（配置与身份）、`operations`（运行态运维）和 `demo`；业务域稍后。
- 依赖：`modules → framework`；`framework` 不得 import `modules`；module 间默认不互引。
- tenant-owned 依赖固定为 `Service → Tenant Repository/Domain Query → GORM`；业务 Service 禁止持有可直接执行 tenant CRUD 的裸 DB，详见 `tenant-data-access`。
- 骨架即接 GORM + Postgres；配置用 YAML。
- 运行时业务设置在 `framework/setting` 注册；租户覆盖值在业务表，见 `settings-system`。
- 对象存储在 `framework/storage`（ObjectStore）；禁止业务拼本地路径，见 `object-storage`。
- Transactional Outbox 在 `framework/outbox`；业务只经 EventWriter 写事件，见 `transactional-outbox`。
- 后台 Job 在 `framework/job` + `asynqadapter`；业务禁止直连 Asynq，见 `background-jobs`。
- 通用请求结构校验在 `framework/validation`；领域、数据库、权限与租户规则保留在 `modules/*` Service。
- 通用分页 Request/Result 在 `framework/pagination`；领域筛选 DTO 留在 `modules/*`，禁止万能查询 DSL。
- Job / Outbox / 通知投递管理 Action、API、菜单统一归 `modules/operations` 与 `operations.*` 语义；领域服务通过接口注入，`operations` 禁止反向依赖 `system`。
- 后端 `_test.go` 统一放在 `platform/tests/`，使用外部测试包通过公开 API 测试；禁止散落到 `framework/*`、`modules/*`，由架构门禁扫描防回流。
- `modules/<module>` 根包只保留 `register.go`、命名明确的稳定 Facade、contracts 与必要常量；实现进入按能力域或技术角色组织的按需子包，禁止空目录和 `common`/`utils`/`misc` 等兜底包。
- `modules/A` 禁止 import `modules/B`，不再接受迁移基线或依赖白名单；跨模块协作必须通过 `framework` contract 和 composition root 注入。
- AINP 与下游的 composition root 是公开 `platform/host`；官方 cmd 与下游均不得导入 `platform/internal/boot`。
- 公开 Host 默认不装配 Demo；官方 API/Worker 必须在 composition root 显式传 `host.WithDemo()`，禁止环境变量隐式启用或路由黑名单。
- 每个模块必须在根目录提供返回 `error` 的 `Register` 入口；禁止空目录、根目录实现文件、子包反向 import 模块根包及 `common`/`utils`/`misc` 等兜底包。
- `system` 根包只保留 `Register`、Facade、contracts 与常量；能力归档为 `access`、`settings`、`attachments`、`notifications`、`setup`，子包禁止反向 import 根包。
- `system/access` 内部 canonical 术语为 `TenantAccount`、`AccountRole` 和 `Account*` Service/DTO；仅 `system` 根 Facade 与 `system/model` 可暂留带弃用说明的 `User*` aliases，外部 Action/HTTP/JSON/AuthContext 兼容字段保持不变。
- 依赖方向固定为 `system root Facade → access → access/model`、`root → settings`、`root → attachments → attachments/model|jobs`、`root → notifications → notifications/jobs`、`root → setup → access`。
- `system/model` 与 `system/jobs` 仅保留临时兼容 alias；已知下游迁移并经过一个发布周期后删除，稳定协议 ID/key 不删除。
- 迁移测试要合并重复覆盖并记录替代位置；禁止为测试新增生产导出 API、测试桥接或其他边界绕过。

详规：`docs/development/backend-layout.md`
