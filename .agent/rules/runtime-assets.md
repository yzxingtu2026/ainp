# 运行时资源装配

- AINP 内置 Prompt 与后端 i18n 必须通过 `embed.FS` 随 `platform` 版本发布；正式 Host 禁止依赖 cwd 或复制资源目录。
- 下游资源只通过 Module 的 `host.RuntimeAssetProvider` 注册；owner 固定为 Module ID，Prompt 使用 `{module-id}/`、i18n 使用 `{module-id}.` 命名空间。
- 重复 owner、跨 owner key 冲突、空内容、非法模板/YAML 均启动期 fail-closed；默认禁止覆盖 AINP 安全基线。
- API、安装模式、embedded worker 与独立 Worker 必须消费同一 Composition 资源声明；诊断只暴露 owner/key/type/locale/version/SHA-256，禁止正文。
- 外置目录和 reload 仅限显式 `WithDevelopmentRuntimeAssets` / `-dev-runtime-assets` 受信开发模式；正式容器禁止启用，reload 失败保留上一有效快照。
- 变更同步 `docs/development/downstream-composition.md`、Prompt/i18n、部署与发布文档，并运行 `agent-rules sync/doctor`。
