# 容器部署基线

AINP 正式版本只使用 Git Tag 对应的两个 GHCR 镜像：`ainp-backend` 同时提供 API 与 Worker 二进制，`ainp-web` 提供静态页面和同源反向代理。部署不得引用 `latest`。

## 本地验证镜像

在仓库根目录执行：

```bash
docker build -f docker/backend.Dockerfile --build-arg VERSION=dev --build-arg COMMIT="$(git rev-parse HEAD)" --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" -t ainp-backend:dev .
docker build -f docker/web.Dockerfile --build-arg VERSION=dev --build-arg COMMIT="$(git rev-parse HEAD)" --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" -t ainp-web:dev .
```

基础镜像可通过 `GO_IMAGE`、`RUNTIME_IMAGE`、`NODE_IMAGE`、`NGINX_IMAGE` 构建参数覆盖，便于受限网络使用可信镜像代理；例如本地可传入 `docker.1ms.run/library/golang:1.25.6-alpine`。发布工作流默认使用官方镜像名，代理可用性与镜像摘要必须由部署方自行校验。

## 演示 Compose

复制 `deploy/demo/.env.example` 为 `deploy/demo/.env`，替换其中全部示例密钥，并将 `AINP_IMAGE_TAG` 固定为完整 SemVer Tag。然后运行：

```bash
docker compose --env-file deploy/demo/.env -f deploy/demo/compose.yaml config
docker compose --env-file deploy/demo/.env -f deploy/demo/compose.yaml up -d
```

`PUBLIC_BASE_URL` 必须是用户实际访问的 HTTPS 同源地址且不带尾部斜杠。公网入口应由宿主机 TLS 反向代理转发到 `WEB_BIND_ADDR:WEB_PORT`；PostgreSQL 和 Redis 不发布宿主端口。

持久化卷包括 PostgreSQL、Redis AOF、安装状态和附件对象目录。API 与独立 Worker 共享状态和附件卷，并固定 `jobs.enabled=true`、`jobs.embedded_worker=false`。

部署后的基础校验必须读取 `PUBLIC_BASE_URL/healthz`，同时确认 `data.status == "ok"` 且 `data.version == AINP_IMAGE_TAG`。仅 HTTP 200 不代表版本部署成功。

## GHCR 发布

推送格式为 `vMAJOR.MINOR.PATCH` 的 Tag 后，`release-images.yml` 先确认该提交的 `backend`、`frontend`、`repository-gates`、`containers` 四项 CI 成功，再构建镜像。每个镜像发布 `<tag>` 和 `sha-<short-sha>` 两个标签，Actions Summary 记录不可变 digest。

workflow 的构建 Job 仅使用仓库内置 `GITHUB_TOKEN` 和 `packages: write`。公开发布前，需要在 GitHub Package 设置中确认两个 Package 的可见性为 Public；部署服务器不应保存具有写权限的 GHCR Token。
