# 统一通知协议与站内通知

通知请求必须保存来源 `PrincipalRef`、Client Surface 和 audience。同步调用从可信 AuthContext 取值，Outbox 调用从 Envelope 恢复；Delivery Job 必须校验 Job Envelope 与请求记录一致，禁止退化为默认 Account。幂等作用域包含 tenant、definition、Principal Kind/ID、Surface、audience 和业务 key。

## 概述

`platform/framework/notification/` 提供统一通知协议，业务模块只提交 Notification Request，不直接调用任何渠道 SDK。

## 核心概念

| 概念 | 说明 |
|------|------|
| **Definition** | 通知定义：key、category、变量 Schema、允许的渠道、模板 |
| **Template** | 渠道模板：按 channel+locale 组织，支持 in_app/email/sms/webhook/im |
| **VariableSchema** | 变量声明：required/sensitive/ephemeral 标记，校验与脱敏 |
| **Renderer** | 模板渲染：Go text/template + 白名单函数 + missingkey=error |
| **TemplateResolver** | 三级解析：tenant override → code default → locale fallback |
| **Orchestrator** | 核心编排：校验 → 幂等 → 变量脱敏 → 渠道策略 → 模板解析 → 入队 |
| **ChannelProvider** | 渠道接口：Send → ProviderReceipt（retryable/permanent 语义） |
| **InApp Provider** | 站内通知 Provider：创建 user_notifications 记录 |

## 文件结构

```
platform/framework/notification/
  types.go          — Channel, Priority, RecipientSpec, ProviderReceipt, ActionDescriptor
  definition.go     — Definition struct
  template.go       — TemplateDefinition, ChannelTemplate, TemplateVariant
  schema.go         — VariableSchema: 变量声明与校验
  renderer.go       — Renderer: Go text/template 渲染
  renderer_funcs.go — 受控模板函数白名单
  provider.go       — ChannelProvider 接口, ProviderInput/RenderedContent/ProviderError
  recipient.go      — RecipientResolver 接口
  status.go         — NotificationRequestStatus / NotificationDeliveryStatus
  registry.go       — DefinitionRegistry: 注册与校验
  resolver.go       — TemplateResolver: 三级解析
  orchestrator.go   — Orchestrator: 核心编排
  fake/
    fake_providers.go — Email/SMS/Webhook/IM Fake Provider

platform/modules/system/
  notification_models.go              — GORM 模型
  notification_repo.go                — Repository CRUD
  notification_register.go            — Action/Menu 注册 (12 Actions)
  notification_inapp_provider.go      — InApp Provider
  notification_service.go             — NotificationService + Outbox Handler
  notification_override_provider.go   — OverrideProvider 桥接
  jobs/notification_deliver.go        — notification.deliver Job

platform/migrations/
  0010_notifications.sql              — 4 张表迁移

frontend/src/
  service/api/notification.ts         — API 调用
  typings/api/notification.d.ts       — 类型定义
  views/notification/list/index.vue   — 通知列表页
  layouts/.../notification-bell.vue   — Bell 按钮 + Badge + 下拉
```

## 注册通知定义

```go
app.RegisterNotification(notification.Definition{
    Key:      "user.password_reset",
    Category: "security",
    Surface:   notification.SurfaceTenant,
    Variables: notification.VariableSchema{
        Fields: []notification.VariableField{
            {Name: "user_name", Type: "string", Required: true},
            {Name: "reset_link", Type: "string", Required: true, Ephemeral: true},
        },
    },
    AllowedChannels:  []notification.Channel{notification.ChannelInApp, notification.ChannelEmail},
    DefaultChannels:  []notification.Channel{notification.ChannelInApp},
    Priority:         notification.PriorityHigh,
    Templates: &notification.TemplateDefinition{
        Version: 1,
        Variants: map[string]notification.ChannelTemplate{
            "in_app:zh": {Subject: "密码重置", Body: "{{.user_name}}，您的密码已重置"},
        },
    },
})
```

## 发送通知

通过 Outbox 可靠投递（推荐）：

```go
outbox.Write(ctx, tx, outbox.Envelope{
    EventType: "notification.requested.v1",
    Payload:   json.RawMessage(`{
        "definition_key": "user.password_reset",
        "recipients": [{"type":"user","id":"user-123"}],
        "variables": {"user_name":"张三","reset_link":"/reset?token=xxx"},
        "idempotency_key": "pw-reset-user123-20240101"
    }`),
})
```

## 渠道策略

| 字段 | 说明 |
|------|------|
| `AllowedChannels` | Definition 允许的渠道范围 |
| `DefaultChannels` | 默认投递渠道 |
| `Mandatory` | 强制渠道（不可被收件人偏好覆盖） |

## 模板解析优先级

1. **Tenant Override** — 租户在 DB 中的模板覆盖
2. **Code Default** — 代码中 Definition.Templates 的精确匹配
3. **Locale Fallback** — 去掉 region 后匹配（zh-CN → zh, en-US → en）

## 幂等

使用 `tenant_id + definition_key + idempotency_key` 唯一约束。重复请求复用已有 request/delivery，不重复创建。

## 数据表

| 表 | 说明 |
|----|------|
| `notification_requests` | 通知请求（含幂等唯一索引） |
| `notification_deliveries` | 投递记录（按渠道/收件人） |
| `notification_template_overrides` | 租户模板覆盖 |
| `user_notifications` | 站内通知（用户可见） |

## 站内通知前端

- **Bell 按钮**：Header 右侧，30s 轮询未读数，下拉显示最近 8 条
- **通知列表页**：`/notification/list`，支持分页、未读/归档筛选、标记已读、全部已读、归档/取消归档
- **受控导航**：action_json 仅允许内部路由，禁止 javascript:/外部 URL

## 错误码

| 码 | 说明 |
|----|------|
| 60001 | 通知定义不存在 |
| 60002 | 通知模板缺失 |
| 60003 | 通知收件人无效 |
| 60004 | 通知投递失败 |
| 60005 | 重复的通知请求 |

## Fake Provider

测试用 Fake Provider（email/sms/webhook/im）位于 `framework/notification/fake/`，支持：
- 记录 Send 调用
- 自定义 StubError（retryable/permanent）
- 自定义 StubReceipt
- Contract Tests 验证接口一致性

## 安全约束

- 模板渲染禁止执行任意代码、访问 DB/文件/网络/环境变量
- 模板函数白名单：dateFormat, truncate, upper/lower, currency 等
- 敏感变量标记 `sensitive`/`ephemeral`，不持久化到 variables_json
- ActionDescriptor 禁止 javascript: 和外部 URL
- 用户只能访问自己的通知
- 个人通知 Action 使用 `access=authenticated`，不进入角色权限目录；服务端固定使用 AuthContext 的 tenant/user scope
