# 下游业务模块装配与版本消费

AINP 下游业务仓库通过 `github.com/yzxingtu2026/ainp/platform/host` 公开契约组合 API、Worker 与迁移入口。下游业务保留在自己的 Go Module 中，不复制或导入 `platform/internal/*`，也不修改 AINP 内置模块。

认证扩展应在同一 Host Module 的 `Prepare` 注册 Authenticator、Principal Provider 和 Client Surface，在 `Register` 注册带 Kind/Surface 约束的 Action。API、embedded Worker 与独立 Worker复用同一模块清单，Host 启动时封存并校验注册结果。禁止复制 JWT、中间件、session 或 system module，也禁止 Provider 自行签 Token。可运行示例见 `platform/examples/downstream/modules/orders/catalog`。

Host 会按每个下游 Surface 的 path prefix 自动投影统一认证、refresh、logout 和 session 管理路由。客户端提交的 `client` 必须使用 Surface ID 并与实际请求路径一致；下游不得另建 TokenService 或复制 refresh 表。Provider 必须在无 Session 的首次认证和带 Session 的刷新/请求阶段都能复核同一 AuthVersion。

浏览器 WebSocket 目标在同一模块 `Prepare` 中通过 `App.RegisterWebSocketHandshakeTarget` 注册静态精确路径。Host 自动投影该 Surface 的 grant 签发接口；下游业务 Handler 在自己的 Origin allowlist 通过后调用 `auth.AuthenticateWebSocketHandshake`，再使用恢复的可信 AuthContext 执行 Upgrade。下游不得复制 grant 表/签发逻辑，不得使用 Query Token；业务 Hub、订阅、受众隔离、保活和断线恢复仍由下游持有。

Host 注册内置 system 模块时会统一挂载附件 Transfer Adapter：raw PUT、multipart POST 与 GET/HEAD 下载均自动可用。下游小程序只需在 upload prepare 选择 `transport=multipart` 并按响应调用 `wx.uploadFile` / `Taro.uploadFile`，不得复制 Gin Handler、另建附件表或绕过 Transfer Token/ObjectStore/ready 管线。代理请求体与超时上限须覆盖有效租户上传策略加协议开销；应用层限制仍必须保留。

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

公开 Host 同时统一内嵌 Go `time/tzdata`。下游通过 `host.OpenAPI`、`host.OpenWorker` 和 `host.New` 构建的 API、Worker 与 migration runner 自动获得相同的 IANA timezone 数据，不应在各 `main` 包重复添加空白导入，也不应要求 Windows 服务器安装 Go SDK、复制 `zoneinfo.zip` 或长期设置 `ZONEINFO`。Migrate 当前不直接执行 Runtime Context，但因其正式入口使用 `host.New`，仍纳入相同的产物契约和发布门禁，防止未来迁移路径增加时区依赖后产生漂移。

## PDF 中文解析构建契约

`github.com/gen2brain/go-fitz` 捆绑库不含 CJK 字体。未嵌字中文 PDF（如 STSong-Light / UniGB-UCS2-H）在默认捆绑库上会提取出大量 U+FFFD，但 `doc.Text` 仍返回 nil error。正式 Linux/musl 镜像和自行构建后端的下游必须使用外部完整 MuPDF，而不是 `go build -tags musl` 链接捆绑库。

固定契约：

- MuPDF 版本必须与头文件、静态库配套。当前官方镜像固定 **MuPDF 1.28.2**；go-fitz v1.28.2 包内头文件是 1.28.0，禁止把完整版 1.28.2 库和捆绑 1.28.0 头文件混用。
- 构建标签为 `extlib,static`，`CGO_ENABLED=1`，`CGO_CFLAGS` / `CGO_LDFLAGS` 指向同一 prefix 的 include 与 `libmupdf.a`。Alpine/musl 目标直接在 musl 工具链上编译该外部库，不要再依赖 go-fitz 的 `musl` 捆绑产物。
- 必须保留 CJK 字体，禁止 `TOFU` / `TOFU_CJK`。
- 可复用脚本：`docker/build-mupdf-cjk.sh`。官方 `docker/backend.Dockerfile` 在编译 API/Worker/Migrate 前会用合成样本 `platform/testdata/pdf/synthetic-unigb-receipt.pdf` 做 CJK 自检；下游 Dockerfile 必须做同等检查，覆盖实际发布的 API/Worker/AI Worker 架构。
- 本地 `go test` 在捆绑库上应 fail-closed（低质量提取，不返回乱码正文）。只有完整 MuPDF 才要求字段完全正确、零 U+FFFD。
- ERP 等下游使用自己的 Dockerfile 时，必须同步本契约后再升级 platform 基线。

## 严格模块包架构

官方示例使用同一 `composition.Modules` 装配 API 与 Worker，模块实现采用以下生产结构：

```text
platform/examples/downstream/
├── composition/
├── modules/
│   └── orders/
│       ├── register.go
│       ├── constants.go
│       └── catalog/
│           ├── model.go
│           ├── service.go
│           └── register.go
├── migrations/
├── prompts/
├── i18n/
└── tests/
```

根 `register.go` 只实现 `host.Module` 生命周期和资源装配；Action、Service、Model、Authenticator、Agent Contract 与 Job 归入 `catalog` owner 包。模块树内不放测试，`tests` 使用外部包和公开 API 验证 Host、迁移、运行时资源及 AI Action 契约。

下游 CI 必须在 `go vet`、`go test` 前运行由已固定 platform 版本提供的门禁：

```bash
go run github.com/yzxingtu2026/ainp/platform/cmd/ainp-archcheck \
  --profile downstream \
  --modules-root ./modules \
  --tests-root ./tests
```

检查器自动从最近的 `go.mod` 推导外部 module import prefix，不硬编码 AINP 仓库路径。失败输出包含稳定 rule ID、相对文件、目标 import/目录和修复方向；禁止复制 `platform/tests/module_architecture_test.go`，也不接受基线或白名单。

## Agent 规则接入

正式 `platform/vX.Y.Z` 发布物包含 `rules/downstream-module-architecture.md`。下游升级必须显式完成：

1. 将 `go.mod` 升级到正式 `platform/vX.Y.Z` 并验证依赖。
2. 从该版本发布物复制或同步精简规则到下游 `.agent/rules/`，在 `.agent/rules/main.md` 引用，并在 `.agent/adapters/rules.json` 增加 `source` 及 Qoder/Cursor 目标登记；随后运行下游自己的 `agent-rules sync` 和 `doctor`。
3. 在下游 backend quality job 中显式加入上述 `ainp-archcheck` 命令。

规则文件、检查器和 Go API 必须来自同一 platform 版本。`docs/upstream` 只读快照不会被 Agent 自动加载，升级 Go 依赖也不会自动改写下游目录、Agent adapter 或 CI。

## Runtime Assets

AINP 正式 Prompt 与后端 i18n 通过 `embed.FS` 随 `platform/vX.Y.Z` 发布，默认 Host 不读取 cwd，也不要求下游或容器复制 `prompts/`、`i18n/`。API、安装模式、embedded worker 与独立 Worker 均由同一 `Composition` 加载完全相同的资源源。

下游 Module 可实现 `host.RuntimeAssetProvider`，以 `host.RuntimeAssets{Prompts: fs.FS, I18n: fs.FS}` 注册不可变资源。owner 固定为 Module ID；Prompt key 必须位于 `{module-id}/...`，翻译 key 必须以 `{module-id}.` 开头。资源源按 Module ID 排序，重复 owner、非法 key、空内容、解析失败或任何跨 owner 冲突均在启动期 fail-closed，默认不允许覆盖 AINP 安全基线。

`Platform.RuntimeAssets` 提供 owner、key、类型、locale/version 与 SHA-256 摘要，供 API/Worker 和构建清单核对；不包含 Prompt 或翻译正文。下游发布清单应同时记录 AINP Module 版本与该清单的确定性摘要。

## 扩展默认系统运营助手

需要让默认 AI 工作台发现下游 Action 时，Module 实现 `host.AgentContractContributionProvider`，返回目标为 `system.operations@1.0.0` 的 `aiagent.ContractContribution`。`Owner` 可留空由 Host 固定为 Module ID；必须显式列出 capability。可选业务基础指令必须来自同 owner 的 Runtime Assets Prompt，并同时固定 key 与版本。

Host 在内置 AI 与全部下游模块注册后统一应用 contribution，再执行真实可达性门禁；Setup 初始化使用同一聚合 Contract。API 与 Worker 因此得到一致结果。禁止按 Action Registry 自动扩权，也禁止通过注册独立下游 Contract 来假装默认工作台可达。无 contribution 时内置 `system.operations` 的 capability、指令与工具集合保持不变。

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

## 平台资源契约

下游访问 AINP 平台资源只能依赖 `framework/platformresource`，并从 `app.App.PlatformResources` 获取 Host 注入的实现。附件使用最小元数据、授权后的 reader 和绑定投影；主体使用最小显示名/状态/认证版本投影及显式失效 Port。不得导入 `platform/internal/*`、`platform/modules/*/model` 或能力实现子包。

资源请求必须携带可信 tenant/principal AuthContext，API 与 Worker 使用同一注入实例。`AccountIdentities` 只管理当前 `ainp.account` 的外部身份绑定，不接受客户端 tenant/account 字段，也不替代 Authenticator、Session 或权限模型。缺少上下文、跨租户、策略拒绝、非 ready、variant/对象缺失统一 fail-closed，不返回 object key、Transfer Token、裸 DB 或 GORM Model。完整 owner、前置条件、错误边界和版本策略见 `platform-resource-contracts.md`。
