# 下游装配

- Vue 管理后台的编译期装配另见 `frontend-downstream-composition`；本文件其余条目描述 Go Host。

- 下游只能通过公开 `platform/host` 组合 API、Worker 和 migration runner，禁止导入或复制 `platform/internal/*`。
- API 与 Worker 复用同一稳定模块声明；模块 ID、阶段顺序、重复注册与迁移 checksum 全部 fail-closed。
- 公开 Host 默认不装配 Demo；仅官方演示或本地 showcase 可在 composition root 显式传 `host.WithDemo()`，API 与 Worker 必须复用同一选择，禁止环境变量隐式启用或路由黑名单。
- AINP Prompt/i18n 默认内嵌；下游 Module 仅以 owner-scoped `fs.FS` 注册自身命名空间，冲突 fail-closed，API/Worker 共享清单，正式环境禁止 cwd 资源和开发 reload。
- Provider 仅在 `Prepare` 注入，业务能力在 `Register` 注册；新安装 Schema 使用 `AutoMigrator`，已有环境只走 namespaced forward-only migration。
- 下游 migration owner 必须等于模块 ID；不得改写 AINP 已发布迁移历史或恢复任何架构豁免。
- 正式消费固定 `platform/vX.Y.Z`；根 Tag 与模块 Tag 必须同版本、同 Commit。
