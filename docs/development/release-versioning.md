# 发布版本与构建身份

AINP 正式版本以不可变 Git SemVer Tag 为唯一事实源。`package.json`、Go 源码常量和页面文案都不得作为正式版本源，也不得为发布直接修改 `main`。

`platform` 是独立 Go 子模块。每次根版本 `vX.Y.Z` 必须同时发布 `platform/vX.Y.Z`，且两个 annotated Tag 指向同一 Commit。`scripts/release.py` 在干净、与 `origin/main` 一致且 CI 全绿时一次推送两个 Tag；任一 Tag 已存在或 Tag workflow 检测到 Commit 漂移都必须中止。下游只在 `go.mod` 固定正式模块版本，不长期消费 `main` 或伪版本。

AINP 内置 Prompt 与后端 i18n 属于 `platform` 版本内容，通过 `embed.FS` 编译进 API/Worker；发布包和下游二进制不得依赖复制工作目录资源。构建清单除 Module 版本外应记录 Runtime Assets owner/key/type 摘要，用于核对 API/Worker 一致性，禁止记录资源正文。

公开 Host 还内嵌 IANA timezone 数据。正式 API、Worker 与 migration runner 必须保留 `time/tzdata` 依赖；CI 交叉构建 Windows/amd64 三个独立 EXE 并检查嵌入符号，同时在 Windows Runner 的无效 `ZONEINFO`/`GOROOT` 环境中执行时区行为测试，不能只依赖安装 Go SDK 或系统 zoneinfo 的开发机结果。以 Go 1.26.5、Windows/amd64、`CGO_ENABLED=0`、`-trimpath` 的本仓库基线测量，API 增加 418,304 字节（408.5 KiB）、Worker 增加 412,160 字节（402.5 KiB）、Migrate 增加 410,112 字节（400.5 KiB），属于可接受的自包含成本；Go 版本、链接参数或依赖变化后应重新测量。

## BuildInfo 协议

API、Worker、migration runner 与 Web 必须由同一 Tag、Commit 和 UTC RFC3339 构建时间生成，并暴露以下字段：

- `version`：正式构建使用完整 Tag（如 `v0.2.0`），本地默认为 `dev`。
- `commit`：可追溯 Git SHA，本地默认为 `unknown`。
- `build_time`：UTC RFC3339 时间，本地默认为 `unknown`。

Go 构建通过 `-ldflags -X` 注入 `framework/buildinfo` 的私有变量；`ainp`、`ainp-worker` 与 `ainp-migrate` 必须使用完全相同的 flags。前端通过 `VITE_BUILD_VERSION`、`VITE_BUILD_COMMIT`、`VITE_BUILD_TIME` 注入，并且只允许 `src/utils/build-info.ts` 读取这些变量。

`/healthz` 返回服务状态及三个 BuildInfo 字段。部署验证必须同时检查 `data.status=ok` 、`data.version` 等于目标 Tag、`data.commit` 等于目标 SHA，并确认持久 state 已 installed，不能只检查 HTTP 200。

## CI 与发布阶段

`.github/workflows/ci.yml` 是 Pull Request 测试事实源，不在合入 `main` 后重复运行完整构建。正式 Tag 工作流从对应的已合并 PR head 读取 backend、frontend、repository-gates、linux-release 的成功检查；linux-release 聚合原生 Ubuntu/CJK/打包门禁。Windows timezone 门禁保留。发布任务 `cancel-in-progress: false`，不能中途取消已开始的上传。

正式 Linux 产物为同一包内的 API/Worker/migrate 二进制 + 静态 Web，私有 OSS 保存不可变版本对象。通用/AI Worker 共用同一可执行文件。`release-binaries.yml` 替换旧镜像发布工作流；不依赖 ACR/GHCR，不再自动 SSH 部署演示/生产。

发布顺序：同提交 CI 验证 → 原生构建（同一 commit 时间生成 build_time）→ 私有 OSS 包及 SHA256 核验上传 → reference.json 完成标记 → GitHub Draft → Published。已 Published 的版本重跑不修改，部分上传重试只允许相同字节，不能移动 Tag、覆盖正式对象或用 latest 部署。

GitHub Release 只附元数据，不公开应用包或环境秘密。中文发布说明基于受限 commit/PR 事实，模型不可编辑发布元数据；异常降级不阻塞发布。

`scripts/release.py` 仍从干净、同步 main 推送配对 Tag，但会先将 `main` 合并提交追溯到唯一的已合并 PR，并检查该 PR head 的必需 CI；同时检查 OSS 发布配置而非 DEMO_URL。生产更新/回滚是管理员单独显式动作，与创建 Release 解耦。

部署命令、RAM/OIDC、基础设施准备、SSL 生命周期与下游接入见 [原生部署 SOP](binary-deployment.md)。分支保护须将旧 `containers` required check 改为 `linux-release`；旧 [容器部署基线](../deployment/containers.md) 仅用于存量兼容，不是新项目主路径。
