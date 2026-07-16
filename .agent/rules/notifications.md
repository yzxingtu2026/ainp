# 通知协议规范

## 核心规则

1. **业务禁止直连渠道 SDK**：所有通知必须通过 `framework/notification` 协议发送，不得直接调用邮件、短信、Webhook 等 SDK。

2. **必须注册 Definition**：每种通知类型必须注册 `Definition`，包含：
   - `key`（格式 `{domain}.{verb}`，如 `user.password_reset`）
   - `VariableSchema`（声明所有变量、required/sensitive/ephemeral 标记）
   - `AllowedChannels` / `DefaultChannels`
   - 至少一个 channel+locale 模板

3. **可靠通知走 Outbox → Orchestrator → Job**：
   - 业务事务中只写 Outbox Event（`notification.requested.v1`）
   - Outbox Handler 调用 Orchestrator 创建 request + deliveries
   - 每个 Delivery 通过 `notification.deliver` Job 执行投递

4. **幂等**：使用 `tenant_id + definition_key + idempotency_key` 保证不重复创建。

## 安全约束

5. **模板渲染安全**：
   - 禁止执行任意代码、访问 DB/文件/网络/环境变量
   - 模板函数仅允许白名单：dateFormat, truncate, upper/lower, currency 等
   - `missingkey=error`：缺少必填变量时渲染失败

6. **变量脱敏**：
   - `sensitive` 变量在日志和审计中脱敏
   - `ephemeral` 变量不持久化到 `variables_json`（仅用于渲染）

7. **ActionDescriptor 安全**：
   - 仅允许内部路由（`/path`）或已知 action ID
   - 禁止 `javascript:` 和外部 URL（`http://`、`https://`）

## 权限与隔离

8. **Tenant 隔离**：所有通知操作必须有 tenant scope。

9. **用户只能访问自己的通知**：管理员不能仅凭同租户默认读取其他用户站内通知。

10. **管理 Action** 按授权边界拆分：模板归 `system.notification_configuration`，投递归 `system.operations.notification_delivery`，均需显式授权。

11. **个人通知 Action** 使用 authenticated，不进入角色权限目录；必须固定使用 AuthContext tenant/user scope。

## Provider Adapter 边界

12. **ChannelProvider 接口**：实现 `Name()`, `Channel()`, `Send()` 方法。
    - `Send` 返回 `ProviderReceipt`（provider/external_id/accepted_at/retryable）
    - 错误分为 Retryable（瞬时）和 Permanent（不可重试）
    - 新 Adapter 不需要修改业务模块或 Orchestrator

13. **InApp Provider**：
    - 创建 `user_notifications` 记录
    - 使用 `(delivery_id, user_id)` 唯一约束保证幂等
    - 站内通知与 `window.$notification` toast 分离：toast 用于临时 UI 提示

## 文件组织

14. **Framework 层**：`platform/framework/notification/` — 类型、接口、合同
15. **Module 层**：`platform/modules/system/notification_*.go` — 具体实现
15. **Job**：`platform/modules/system/jobs/notification_deliver.go` — 一个 Job Type 一个文件
16. **Fake Provider**：`platform/framework/notification/fake/` — 测试用
