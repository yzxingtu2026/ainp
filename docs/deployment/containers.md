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

## 数据库迁移与自动更新

正式 Compose 已关闭启动时 GORM AutoMigrate。backend 镜像内置 `ainp-migrate status|up|baseline`，迁移状态保存在 `schema_migrations`，每个 SQL 文件通过编号、名称和 SHA-256 校验和锁定。新安装由安装向导在完整建表后自动 baseline；从旧版升级且尚无迁移账本时，维护者必须先核对 Schema，再执行一次：

```bash
docker compose --env-file deploy/demo/.env -f deploy/demo/compose.yaml run --rm --no-deps \
  --entrypoint /usr/local/bin/ainp-migrate api -config /app/config/config.yaml baseline
```

服务器将 `deploy/demo` 固定到低权限 deploy 用户目录，填写 `.env` 后执行 `./deploy-demo.sh <tag>`。脚本使用 `flock` 串行部署，拉取指定 Tag、记录镜像 digest、在迁移前创建 `pg_dump`、执行 forward-only migration、原子更新 Tag，并同时校验 `/healthz` 的状态和版本。启动或健康校验失败时恢复旧应用镜像；数据库不执行 down migration，依赖升级前备份和向后兼容 Schema。

部署日志与备份位于 `runtime/logs`、`runtime/backups`。备份按 `BACKUP_RETENTION_DAYS` 清理；失败日志和当次备份不会因为应用回滚而丢失。SSH、固定 host key 与 GitHub Environment 编排属于下一发布阶段。

## GHCR 发布

推送格式为 `vMAJOR.MINOR.PATCH` 的 Tag 后，`release-images.yml` 先确认该提交的 `backend`、`frontend`、`repository-gates`、`containers` 四项 CI 成功，再构建镜像。每个镜像发布 `<tag>` 和 `sha-<short-sha>` 两个标签，Actions Summary 记录不可变 digest。

workflow 的构建 Job 仅使用仓库内置 `GITHUB_TOKEN` 和 `packages: write`。公开发布前，需要在 GitHub Package 设置中确认两个 Package 的可见性为 Public；部署服务器不应保存具有写权限的 GHCR Token。
