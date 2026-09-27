# 发布版本协议

- 正式版本唯一事实源是不可变 Git SemVer Tag；禁止发布时修改多个版本文件或移动已有 Tag。
- `platform` Tag 同时锁定内嵌 Prompt/i18n；API、Worker 与下游构建应记录不含正文的 Runtime Assets 摘要。
- 正式 API、Worker 与 migration runner 必须保留公开 Host 提供的内嵌 IANA timezone 数据；发布门禁需验证 Windows 独立 EXE 包含 `time/tzdata`。
- backend、worker、web 必须注入相同 version、commit、UTC RFC3339 build_time；本地默认 `dev/unknown`。
- 正式 Linux 产物为同版本 API/Worker/migrate 二进制 + 静态 Web，私有 OSS 不可变包；通用/AI Worker 复用同一 worker，独立于持久化 state/附件。
- Nginx 必须同源精确代理 API、MCP/OAuth metadata、附件与 AI SSE；正式部署引用完整 SemVer/commit/SHA256，禁止 latest。
- Tag 二进制工作流复用同提交 CI 门禁（含 linux-release）；仅发布 Job 通过 OIDC/STS 写 OSS，不申请 packages:write，不自动操作生产。
- 正式环境关闭启动时 AutoMigrate，只允许统一 runner 按编号和校验和 forward-only 执行；新安装自动 baseline，旧环境 baseline 必须显式确认。
- 主机部署必须串行、有维护窗口、迁移前持久化备份，校验 installed/healthz/version/commit/Web/进程；失败保留维护状态，显式应用回滚且不自动 down migration。
- 本地 release.py 必须按 deploy/linux/project.json 校验自身仓库、同步默认分支和 PR CI；AINP 原子推送根 `vX.Y.Z` 与 `platform/vX.Y.Z` annotated Tag（同 Commit），普通下游仅推应用 Tag；Release 必须 OSS 完整性校验上传→Draft→Published；部署为独立显式动作，已发布 Tag rerun 禁止重建或覆盖产物。
- AI Release Notes 只读受限 commit/PR 事实并返回受 Schema 约束的分类摘要；Tag、产物、SHA256、Compare 与发布状态由模板固定，未知引用/非法输出/模型失败必须降级且不阻塞发布。
- 后端统一读取 `framework/buildinfo`，前端统一读取 `src/utils/build-info.ts`，页面不得硬编码正式版本。
- 部署健康校验必须同时验证 `status=ok` 与目标版本；正式部署禁止依赖 `latest`。
- PR/main CI 是测试事实源；发布工作流复用其门禁，不复制另一套测试协议。
- 详细规范见 `docs/development/release-versioning.md` 与 `docs/development/binary-deployment.md`。
