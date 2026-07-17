# 团队协作主规则

规则文件只写硬约束要点；细节以 `docs/` 为准，避免在此重复展开。

## 必守原则

1. **先 Issue 后代码**：可交付工作必须关联 Issue；范围/AC 以 Issue/PR 为准。
2. **禁止直改 main**：短分支 + PR 合入。
3. **小范围交付**：一分支一任务；超范围先拆 Issue。
4. **文档归位**：详规进 `docs/`；规则只留要点，变更同步沉淀。
5. **远端用 gh**：不得把 token 写入仓库。
6. **中文提交**：说明关联 Issue 与目的。
7. **团队信息**：查 `.agent/team/members.yml` / `roles.yml`。

## 开发硬约束

- 公共能力优先扩展，禁止业务内重复实现；新约定写入 `docs/development/`，Agent 要点写入 `.agent/rules/`。
- 业务逻辑下沉到服务/领域/公共模块，不堆在路由或 UI 事件里。
- 关键流程用统一日志，禁止零散 `print`（见 `logging`）。
- API 信封与错误码见 `api-response`。
- 强类型请求 DTO 与验证分层见 `validation`；列表查询接入边界见 `list-query`。
- Action / 菜单注册见 `action-metadata`。
- 多租户与鉴权上下文见 `auth-context`。
- 统一认证与会话见 `authentication-and-session`（Authenticator / Identity / Session / refresh 轮换；渠道禁止旁路签 Token）。
- 审计最小集见 `audit`。
- 后端目录与模块边界见 `backend-layout`。
- CodeSet、状态机与字符串常量见 `code-sets-and-status`。
- 租户角色、权限目录与管理员保护见 `role-management`。
- 后端多语言 i18n 见 `i18n`；前端多语言见 `frontend-i18n`。
- 用户可见时间展示见 `datetime-display`。
- 统一设置系统见 `settings-system`（管理端 `SettingService` Action；业务只读类型化 Provider，禁止散落 JSON/直写表）。
- 统一对象存储见 `object-storage`（`ObjectStore` / `BuildKey`；禁止业务拼路径或按 provider 分支）。
- 租户隔离附件见 `attachments`（元数据 + prepare/transfer token + AccessPolicy；禁止文件进 JSON、禁止未注册 resource_type）。
- 前端统一附件组件见 `frontend-attachments`（Client/队列/Field；禁止拼 key、禁止附件中心）。
- Transactional Outbox 见 `transactional-outbox`（同事务写事件；禁止直写 outbox 表 / 同步双写 Redis）。
- 后台任务 Job 见 `background-jobs`（经 Outbox 入队；禁止业务直连 Asynq）。
- 幂等、乐观锁、数据库不变量锁见 `idempotency-concurrency`（Mutation Action 必须评估幂等；并发更新资源必须用 revision；行锁必须经 dblock）。
- 统一通知协议见 `notifications`（业务禁止直连渠道 SDK；必须注册 Definition/变量 Schema/渠道模板；可靠通知走 Outbox → Orchestrator → Job；模板安全限制；站内通知与 toast 分离）。
- AI 模型治理见 `ai-model-providers`（业务只引用 Alias；Provider Type/能力交集/Secret/Discovery 均 fail-closed）。
- AI 模型调用见 `ai-model-runtime`（统一 Generate/Stream、Adapter Registry、Invocation 与敏感信息边界）。
- 前端栈、图标协议见关联规则。
- Copilot PR 语义审查见 `copilot-code-review`；它只负责中文语义反馈，不替代合并门禁与人工审批。

## 关联规则

- `github-issues-workflow` / `branching-strategy` / `project-context-and-docs`
- `frontend-stack` → `docs/development/frontend-stack.md`
- `datetime-display` → `docs/development/datetime-display.md`
- `settings-system` → `docs/development/settings-system.md`
- `object-storage` → `docs/development/object-storage.md`
- `attachments` → `docs/development/attachments.md`
- `frontend-attachments` → `docs/development/frontend-attachments.md`
- `transactional-outbox` → `docs/development/transactional-outbox.md`
- `background-jobs` → `docs/development/background-jobs.md`
- `idempotency-concurrency` → `docs/development/idempotency-concurrency.md`
- `notifications` → `docs/development/notifications.md`（Agent 要点见 `.agent/rules/notifications.md`）
- `ai-model-providers` → `docs/development/ai-model-providers.md`
- `ai-model-runtime` → `docs/development/ai-model-runtime.md`
- `icon-library` → `docs/development/icon-library.md`
- `logging` → `docs/development/logging.md`
- `api-response` → `docs/development/api-response.md`
- `validation` → `docs/development/validation.md`
- `list-query` → `docs/development/list-query.md`
- `action-metadata` → `docs/development/action-metadata.md`
- `auth-context` → `docs/development/auth-context.md`
- `authentication-and-session` → `docs/development/authentication-and-session.md`
- `audit` → `docs/development/audit.md`
- `i18n` → `docs/development/api-response.md`
- `frontend-i18n` → `docs/development/frontend-stack.md`
- `backend-layout` → `docs/development/backend-layout.md`
- `code-sets-and-status` → `docs/development/code-sets-and-status.md`
- `role-management` → `docs/development/role-management.md`
- `team-escalation`
- `copilot-code-review` → `docs/development/copilot-code-review.md`
