# 发布版本协议

- 正式版本唯一事实源是不可变 Git SemVer Tag；禁止发布时修改多个版本文件或移动已有 Tag。
- backend、worker、web 必须注入相同 version、commit、UTC RFC3339 build_time；本地默认 `dev/unknown`。
- 后端统一读取 `framework/buildinfo`，前端统一读取 `src/utils/build-info.ts`，页面不得硬编码正式版本。
- 部署健康校验必须同时验证 `status=ok` 与目标版本；正式部署禁止依赖 `latest`。
- PR/main CI 是测试事实源；发布工作流复用其门禁，不复制另一套测试协议。
- 详细规范见 `docs/development/release-versioning.md`。
