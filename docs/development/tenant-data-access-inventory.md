# 租户数据访问迁移库存

本文档是 #112 的迁移基线。统计范围为 `platform/framework`、`platform/modules`、HTTP、Worker/Job 与 Outbox Handler；最终门禁与完整规范由 #113 收口。

## 基线

| 类别 | 初始数量 | 处理原则 |
|---|---:|---|
| 显式 tenant-owned GORM Model | 44 | 注册后只允许 TenantDB/SystemDB |
| 含手写顶层 tenant 条件的生产 Go 文件 | 29 | 改为 Repository/领域 Query，保留 owner/status/revision 条件 |
| Raw/Exec/Table/JOIN 调用点 | 37 | 业务查询迁移到受控出口；DDL/migration 精确登记 |

## 分批迁移

| 批次 | 范围 | 状态 | 验证 |
|---|---|---|---|
| #111 样板 | Agent、AgentVersion、AgentToolBinding 及直接 Runtime 消费方 | 已完成 | Agent/Builder Preview/PostgreSQL |
| Demo 模板 | RevisionDoc | 已完成 | 默认测试链与跨租户 Callback |
| System/Framework | Setting、Notification Request/Delivery/TemplateOverride/UserNotification（含 User 展示名与详情 JOIN）、Attachment/AttachmentLink（含 Service、管理查询、User 展示名与 Thumb Job）、Idempotency Record、Audit、Outbox、Job Run、AuthSession、Auth pre-auth/授权加载、Role 与 User 已迁移；Job Worker/Outbox dispatcher 保留为精确可信恢复例外 | 已完成 | 各模块 PostgreSQL 集成测试 |
| AI 治理与 Tool | Tool FTS Raw 已迁移；Provider/Deployment/Alias/Invocation/Tool/Scope/Confirmation ORM 待迁移 | 迁移中 | AI Model/Tool Runtime 测试 |
| AI 会话与 Runtime | Context Compaction 与 Snapshot 外部引用查询已迁移；Conversation/Message/Stream/Snapshot 写链及 Run/Step/Event 待迁移 | 迁移中 | Conversation/Agent Runtime 测试 |
| Knowledge | Retrieval/Citation/Agent Binding Raw JOIN 已迁移；Base/Document/Index/Chunk/Payload ORM 与 Ingestion Worker 待迁移 | 迁移中 | Knowledge 全链路测试 |
| Operations | Audit/Outbox/Job/Log 管理查询 | 待迁移 | Operations Center 测试 |

## 精确例外

| 位置 | 类型 | 原因 | 约束 |
|---|---|---|---|
| `*/migrate.go`、`modules/*/model.go`、`system/seed.go` | Exec/DDL | schema migration、约束和索引维护 | 只在启动/安装阶段；不得接受请求 tenant |
| `RevisionDocService.AutoMigrate` | AutoMigrate | Demo 表启动建表 | 不执行 tenant-owned 业务 CRUD |
| `framework/revision`、`framework/dblock` | Table/Exec | 通用并发基础设施 | 调用方必须来自 scoped transaction；本 Issue 后续迁移 |
| `framework/audit.GormRecorder` | mixed-realm write | `audit_logs` 同时记录 tenant 与 platform realm | tenant 管理查询只走 TenantRaw；写入 tenant/realm 来自已认证 Action AuthContext |
| `framework/outbox.Dispatcher` | trusted dispatcher | Dispatcher 在请求 AuthContext 外跨租户 claim/publish | tenant 管理查询与手动重试走 TenantRaw；dispatcher 能力在后续批次独立收敛 |
| `framework/job.GetTrusted`、`RunStore.UpdateFields/ListPendingEnqueue` | trusted worker | Worker 首次加载 JobRun 时尚未恢复业务 AuthContext，并需跨租户恢复 pending enqueue | tenant 列表/详情/取消走 TenantRaw；worker 能力在后续批次独立收敛 |

## 已识别契约风险

- Notification OverrideProvider 原接口缺少 `context.Context`，已升级为上下文透传并迁移到 TenantDB；禁止恢复仅凭 tenantID 的 context-free 查询。
- AuthSession refresh-token lookup 发生在 tenant 身份建立前，只能走 system 模块私有的 `preAuthStore` 固定查询；该能力不暴露通用 DB/SQL，也不能伪造 platform AuthContext。
- RolePermission 不含 tenant 字段，运行时读写必须通过 `roles.tenant_id` 归属约束的 TenantRaw/TenantExec；不得把它错误注册为可独立 tenant scope 的 Model。
- Idempotency cleanup 已收敛为仅 platform operator 可获取的 SystemDB；普通请求 CRUD 只接受 AuthContext tenant scope，方法参数 tenant 仅做冲突校验。
- Notification Delivery 已拍板：空 `status` 表示当前租户全部投递状态；异常队列使用显式 `failed`/`dead` 筛选，重试仍只允许这两种状态。旧测试期望已统一。

## 当前边界

- tenant 只来自 AuthContext；业务 DTO、Query 和消息 payload 不得构造数据库 tenant scope。
- Model 只有在其全部业务消费方迁移后才启用强制 Registry，避免把未迁移路径伪装为安全路径。
- owner、visibility、permission、revision、lease 和状态机仍是领域条件，不能因自动 tenant scope 删除。
- Knowledge Raw/JOIN 在迁移到受控领域 Query 前不得登记为已完成。
