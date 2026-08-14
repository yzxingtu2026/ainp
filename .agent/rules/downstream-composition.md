# 下游装配

- Vue 管理后台的编译期装配另见 `frontend-downstream-composition`；本文件其余条目描述 Go Host。

- 下游只能通过公开 `platform/host` 组合 API、Worker 和 migration runner，禁止导入或复制 `platform/internal/*`。
- API 与 Worker 复用同一稳定模块声明；模块 ID、阶段顺序、重复注册与迁移 checksum 全部 fail-closed。
- 公开 Host 默认不装配 Demo；仅官方演示或本地 showcase 可在 composition root 显式传 `host.WithDemo()`，API 与 Worker 必须复用同一选择，禁止环境变量隐式启用或路由黑名单。
- AINP Prompt/i18n 默认内嵌；下游 Module 仅以 owner-scoped `fs.FS` 注册自身命名空间，冲突 fail-closed，API/Worker 共享清单，正式环境禁止 cwd 资源和开发 reload。
- 下游默认助手能力通过 Module 的 `AgentContractContributions` 显式贡献到 `system.operations@1.0.0`；API/Worker 共用聚合结果，禁止自动扩权或独立幽灵 Contract。
- Provider 仅在 `Prepare` 注入，业务能力在 `Register` 注册；新安装 Schema 使用 `AutoMigrator`，已有环境只走 namespaced forward-only migration。
- 下游 migration owner 必须等于模块 ID；不得改写 AINP 已发布迁移历史或恢复任何架构豁免。
- 正式消费固定 `platform/vX.Y.Z`；根 Tag 与模块 Tag 必须同版本、同 Commit。
- 下游模块根包只保留薄 `Module` 生命周期、稳定 Facade/contracts/constants；Action、Service、Repository、Model、HTTP 与领域流程进入 owner 明确的能力子包，测试集中到模块树外的 `tests`。
- 下游 CI 必须在 `go vet/go test` 前显式运行依赖版本提供的 `cmd/ainp-archcheck --profile downstream`；不接受架构基线、通配豁免或本地白名单。
- 发布物中的精简规则来源是 `platform/rules/downstream-module-architecture.md`。下游升级后仍须复制或同步同版本规则、登记自身 `.agent/rules/main.md` 与 `.agent/adapters/rules.json`，再运行 `agent-rules sync/doctor`；保存文档快照或只升级 Go 依赖不会自动启用规则。
