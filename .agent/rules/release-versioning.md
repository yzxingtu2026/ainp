# 发布版本协议

- 正式版本唯一事实源是不可变 Git SemVer Tag；禁止发布时修改多个版本文件或移动已有 Tag。
- backend、worker、web 必须注入相同 version、commit、UTC RFC3339 build_time；本地默认 `dev/unknown`。
- 正式产物固定为 backend/web 两个镜像；API 与独立 Worker 必须来自同一 backend 镜像，Compose 中共享 state 与附件持久卷。
- Web 镜像必须同源代理 API、MCP/OAuth metadata、附件与 AI SSE；正式 Compose 只能引用完整 SemVer Tag 或 digest，禁止 `latest`。
- Tag 镜像工作流必须复用同一提交已成功的 CI 事实；仅镜像构建 Job 可申请 `packages: write`，并记录 OCI labels 与 digest。
- 正式环境关闭启动时 AutoMigrate，只允许统一 runner 按编号和校验和 forward-only 执行；新安装自动 baseline，旧环境 baseline 必须显式确认。
- 演示部署必须 flock 串行、迁移前备份、记录目标 digest，并以 healthz 状态和目标 Tag 双重校验；失败只回滚应用镜像，不自动 down migration。
- 后端统一读取 `framework/buildinfo`，前端统一读取 `src/utils/build-info.ts`，页面不得硬编码正式版本。
- 部署健康校验必须同时验证 `status=ok` 与目标版本；正式部署禁止依赖 `latest`。
- PR/main CI 是测试事实源；发布工作流复用其门禁，不复制另一套测试协议。
- 详细规范见 `docs/development/release-versioning.md`。
