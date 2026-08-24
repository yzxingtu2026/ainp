# Issue #296 升级与发布说明

## 兼容策略

- 旧 Account Session、Token、Outbox/Job、审计、幂等和附件记录按 `ainp.account` 兼容回填。
- 通知请求按 `ainp.account + ainp.tenant-console + ainp:tenant-console` 回填；新的幂等唯一键包含主体 Kind、主体 ID、Surface 和 audience。
- MCP 旧 Grant/Confirmation 记录按 Account 和 tenant console 回填。旧 MCP access token 因缺少来源 Kind/Surface/audience 声明而失效；未撤销的 refresh token 仍可在 Grant 与实时授权复核通过后换发。为避免旧确认密文的 AAD 被错误解释，仍处于 `pending/approved` 的确认在迁移时撤销，用户需重新发起确认。
- 新主体不得写入 Account 专属关联或继承 Account RBAC；跨 Kind、Surface、audience 的复用必须拒绝。
- API、embedded Worker 与独立 Worker 必须从同一 Host Module 清单完成注册和 `SealAuthentication` 启动校验；独立 Worker 从 `job_runs` 恢复原始 Principal Ref、Surface 与 audience，只有历史 Account 记录可补齐 tenant-console 默认值。

## 发布步骤

1. 在 `platform` 运行 PostgreSQL 集成测试、`go test ./...`、`go vet ./...`，并验证独立下游示例。
2. 完成人工安全测试后先 approve，再为认证安全边界 PR 添加 `tested-pass`。
3. 在 root 与 `platform` 同一提交创建不可变 SemVer Tag，更新镜像和部署配置到完整 Tag 或 digest。
4. 验证 `/healthz` 的版本、commit 和构建时间与 Tag 一致；下游只消费该 Tag，不消费 `main`、分支或伪版本。
5. 关闭 Issue #296；后续 HOA/ERP 源码脱壳、Schema ownership 和生产切换由各自下游 Issue 完成。
