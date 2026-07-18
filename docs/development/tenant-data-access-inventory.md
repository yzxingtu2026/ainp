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
| System/Framework | Setting、NotificationTemplateOverride 已完成；User/Role/Auth/Attachment/Delivery/UserNotification/Audit/Job/Outbox/Idempotency 待迁移 | 迁移中 | 各模块 PostgreSQL 集成测试 |
| AI 治理与 Tool | Provider/Deployment/Alias/Invocation/Tool/Scope/Confirmation | 待迁移 | AI Model/Tool Runtime 测试 |
| AI 会话与 Runtime | Conversation/Message/Stream/Snapshot/Run/Step/Event | 迁移中 | Conversation/Agent Runtime 测试 |
| Knowledge | Base/Document/Index/Chunk/Payload/Binding 及 Raw JOIN | 待迁移 | Knowledge 全链路测试 |
| Operations | Audit/Outbox/Job/Log 管理查询 | 待迁移 | Operations Center 测试 |

## 精确例外

| 位置 | 类型 | 原因 | 约束 |
|---|---|---|---|
| `*/migrate.go`、`modules/*/model.go`、`system/seed.go` | Exec/DDL | schema migration、约束和索引维护 | 只在启动/安装阶段；不得接受请求 tenant |
| `RevisionDocService.AutoMigrate` | AutoMigrate | Demo 表启动建表 | 不执行 tenant-owned 业务 CRUD |
| `framework/revision`、`framework/dblock` | Table/Exec | 通用并发基础设施 | 调用方必须来自 scoped transaction；本 Issue 后续迁移 |

## 已识别契约风险

- Notification OverrideProvider 原接口缺少 `context.Context`，已升级为上下文透传并迁移到 TenantDB；禁止恢复仅凭 tenantID 的 context-free 查询。
- AuthSession refresh-token lookup 与 Idempotency cleanup 发生在 tenant 身份建立前或跨租户维护阶段，需要独立的受控 pre-auth/maintenance capability，不能伪造 platform AuthContext。
- Notification Delivery 两个 PostgreSQL 测试对“空 status 的默认筛选”存在相反期望；在迁移 Delivery 前必须先拍板产品语义并统一测试。

## 当前边界

- tenant 只来自 AuthContext；业务 DTO、Query 和消息 payload 不得构造数据库 tenant scope。
- Model 只有在其全部业务消费方迁移后才启用强制 Registry，避免把未迁移路径伪装为安全路径。
- owner、visibility、permission、revision、lease 和状态机仍是领域条件，不能因自动 tenant scope 删除。
- Knowledge Raw/JOIN 在迁移到受控领域 Query 前不得登记为已完成。
