# 发布版本与构建身份

AINP 正式版本以不可变 Git SemVer Tag 为唯一事实源。`package.json`、Go 源码常量和页面文案都不得作为正式版本源，也不得为发布直接修改 `main`。

## BuildInfo 协议

backend、worker 与 web 必须由同一 Tag、Commit 和 UTC RFC3339 构建时间生成，并暴露以下字段：

- `version`：正式构建使用完整 Tag（如 `v0.2.0`），本地默认为 `dev`。
- `commit`：可追溯 Git SHA，本地默认为 `unknown`。
- `build_time`：UTC RFC3339 时间，本地默认为 `unknown`。

Go 构建通过 `-ldflags -X` 注入 `framework/buildinfo` 的私有变量；`ainp` 与 `ainp-worker` 必须使用完全相同的 flags。前端通过 `VITE_BUILD_VERSION`、`VITE_BUILD_COMMIT`、`VITE_BUILD_TIME` 注入，并且只允许 `src/utils/build-info.ts` 读取这些变量。

`/healthz` 返回服务状态及三个 BuildInfo 字段。部署验证必须同时检查 `data.status=ok` 和 `data.version` 等于目标 Tag，不能只检查 HTTP 200。

## CI 与后续发布阶段

`.github/workflows/ci.yml` 是 PR 和 `main` 的测试事实源。Tag 发布工作流应依赖相同门禁结果，不复制一套测试定义；发布、镜像和部署任务不得使用会取消运行中任务的 concurrency 配置。

后续阶段的容器镜像必须写入对应 OCI `version`、`revision`、`created`、`source` labels，并只允许不可变 SemVer Tag 或 digest 进入正式 Compose。禁止以 `latest` 作为部署版本。

Phase 2 已提供 backend/web 两个多阶段镜像、同源 Web 代理、GHCR Tag 构建和演示 Compose 基线，具体使用方式见 [容器部署基线](../deployment/containers.md)。migration runner、服务器备份/部署/回滚和 Draft Release 状态机仍属于后续阶段，当前 Compose 不应被描述为自动升级方案。
