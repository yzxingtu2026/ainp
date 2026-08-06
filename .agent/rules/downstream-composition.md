# 下游装配

- 下游只能通过公开 `platform/host` 组合 API、Worker 和 migration runner，禁止导入或复制 `platform/internal/*`。
- API 与 Worker 复用同一稳定模块声明；模块 ID、阶段顺序、重复注册与迁移 checksum 全部 fail-closed。
- Provider 仅在 `Prepare` 注入，业务能力在 `Register` 注册；新安装 Schema 使用 `AutoMigrator`，已有环境只走 namespaced forward-only migration。
- 下游 migration owner 必须等于模块 ID；不得改写 AINP 已发布迁移历史或恢复任何架构豁免。
- 正式消费固定 `platform/vX.Y.Z`；根 Tag 与模块 Tag 必须同版本、同 Commit。
