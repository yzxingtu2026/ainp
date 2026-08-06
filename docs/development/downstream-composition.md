# 下游业务模块装配与版本消费

AINP 下游业务仓库通过 `github.com/yzxingtu2026/ainp/platform/host` 公开契约组合 API、Worker 与迁移入口。下游业务保留在自己的 Go Module 中，不复制或导入 `platform/internal/*`，也不修改 AINP 内置模块。

## 版本固定

正式消费只使用子模块 Tag：

```bash
go get github.com/yzxingtu2026/ainp/platform@v0.2.0
go mod download
go mod verify
```

Git 中对应 Tag 为 `platform/v0.2.0`。同一次发布的根 Tag `v0.2.0` 与子模块 Tag 必须指向同一 Commit；发布脚本和 Tag workflow 对此 fail-closed。运行时 BuildInfo 描述下游最终二进制的构建身份，不等同于依赖模块版本；下游构建清单应同时保留两者。

禁止正式环境长期跟随 `main`、分支或伪版本。安全修复发布后，下游应优先升级到包含修复的最小正式版本并执行完整验证。

## 模块契约与顺序

每个模块实现稳定 `ID()` 与 `Register(*app.App) error`。可选契约包括：

- `Prepare(*app.App) error`：内置 system 注册前注入 Provider-neutral Provider，例如 Authenticator。
- `AutoMigrate(*gorm.DB) error`：仅用于新安装创建当前下游 Schema。
- `MigrationSource() migrations.Source`：为安装 baseline、`status` 和后续 `up` 提供 forward-only SQL。

确定顺序为：校验并按模块 ID 排序、创建 framework runtime、`Prepare`、内置 setup/system/operations/ai 注册、按显式选项注册 Demo、`Register`、全局注册表与帮助/AI 覆盖校验。非法或重复模块 ID、重复 Provider/Action/Job/Setting 等由相应注册表拒绝并终止启动。API 与 Worker 必须由同一个 `[]host.Module` 和同一组装配选项声明创建。

```go
modules := []host.Module{orders.Module{}, membership.Module{}}
api, err := host.OpenAPI(ctx, cfg, host.WithModules(modules...))
worker, err := host.OpenWorker(ctx, cfg, profile, host.WithModules(modules...))
composition, err := host.New(host.WithModules(modules...))
status, err := composition.Migrations().Up(db)
```

## Demo 安全默认值

公开 Host 默认不装配 `platform/modules/demo`。生产下游调用 `host.New()`、`host.OpenAPI()`、`host.OpenInstalled()` 或 `host.OpenWorker()` 时无需传入禁用配置；默认结果不包含 Demo Capability、Action、HTTP 路由、菜单、Page Help、附件策略、Job、Outbox Handler、权限目录或演示租户种子数据。

AINP 官方演示应用或下游本地 showcase 必须在 composition root 显式传入 `host.WithDemo()`：

```go
options := []host.Option{
    host.WithModules(modules...),
    host.WithDemo(),
}
api, err := host.OpenAPI(ctx, cfg, options...)
worker, err := host.OpenWorker(ctx, cfg, profile, options...)
```

`WithDemo()` 是代码级 opt-in，不得由环境变量或路由黑名单替代。API、embedded worker 与独立 Worker 必须消费同一份不可变选择，避免 Demo Action、Job 与 Outbox Handler 漂移。`Composition.DemoEnabled()` 仅公开最终装配事实，Registry 仍是验证具体 Capability、Action、Menu 与 Job 是否存在的稳定事实源。

调用方负责配置和日志初始化、OS 信号以及最终调用 `Close`；Host 负责 App、Outbox、Job、Worker runner 与关闭顺序。最小独立模块见 `platform/examples/downstream`。

## Runtime Assets

AINP 正式 Prompt 与后端 i18n 通过 `embed.FS` 随 `platform/vX.Y.Z` 发布，默认 Host 不读取 cwd，也不要求下游或容器复制 `prompts/`、`i18n/`。API、安装模式、embedded worker 与独立 Worker 均由同一 `Composition` 加载完全相同的资源源。

下游 Module 可实现 `host.RuntimeAssetProvider`，以 `host.RuntimeAssets{Prompts: fs.FS, I18n: fs.FS}` 注册不可变资源。owner 固定为 Module ID；Prompt key 必须位于 `{module-id}/...`，翻译 key 必须以 `{module-id}.` 开头。资源源按 Module ID 排序，重复 owner、非法 key、空内容、解析失败或任何跨 owner 冲突均在启动期 fail-closed，默认不允许覆盖 AINP 安全基线。

`Platform.RuntimeAssets` 提供 owner、key、类型、locale/version 与 SHA-256 摘要，供 API/Worker 和构建清单核对；不包含 Prompt 或翻译正文。下游发布清单应同时记录 AINP Module 版本与该清单的确定性摘要。

外置开发目录不是生产配置。只有显式传 `host.WithDevelopmentRuntimeAssets(promptDir, i18nDir)`（官方命令对应 `-dev-runtime-assets`）时才替换 `ainp` 内嵌源并允许 reload；目录缺失或内容非法会阻止启动，reload 失败保留上一份有效快照。

## 下游迁移

每个来源的 `Owner` 必须等于模块 ID，文件采用 `NNNN_name.sql`。同一 owner 内版本连续递增；全局执行顺序为 AINP 内置迁移优先，之后按 owner 字典序、再按 version 升序。状态身份是 `owner + version + name + SHA-256`。

AINP 已发布历史继续保存在 `schema_migrations`，不得插入、重排或改写。下游历史保存在 `schema_module_migrations`；未知记录、名称变化或 checksum 变化均拒绝 status/up。新安装先由 `AutoMigrator` 创建当前 Schema，再 baseline 内置和下游历史；已有环境只执行 SQL migration。所有迁移 forward-only，不提供 down migration。

## 所有权与升级

- 下游仓库拥有业务模型、Action、页面、迁移和临时业务补丁；AINP 拥有通用框架、安全边界与内置模块。
- 可复用或安全修复先以 Issue/PR 回提 AINP。上游发布后，下游升级正式版本、验证并删除临时补丁，禁止形成永久 fork。
- 升级前审阅公开契约与迁移，备份数据库，运行独立模块测试、`go mod verify`、migration status/up、API/Worker smoke test。
- 应用失败时回退下游二进制版本；数据库依赖升级前备份和向后兼容 Schema，不执行 down migration。破坏性公开契约变化遵循 SemVer 和迁移窗口。

下游模块仍遵循 owner contracts + composition root 注入、`modules/A` 不依赖 `modules/B`、集中测试和零架构豁免。公开 Host 不授予额外数据库、租户或权限能力。
