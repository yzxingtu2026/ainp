# 下游平台资源契约

`framework/platformresource` 是 AINP 面向独立下游模块的稳定最小契约，system 模块负责实现，Host 通过 `app.App.PlatformResources` 注入。下游不创建平行 system Service，不直接访问 AINP 内置表。

## 附件

- Owner：`system.attachments`。
- 前置：可信 tenant/principal AuthContext；调用者提供资源类型、资源 ID 和可选字段，服务端重新执行 AccessPolicy、tenant scope、绑定查询和 `ready`/variant 状态检查。
- `Open` 返回 `AttachmentMetadata` 与受控 `io.ReadCloser`；元数据不含 object key、provider、transfer token 或 GORM Model。`BindingExists` 与 `ListBindings` 只返回最小绑定投影。
- 未绑定、跨租户、策略拒绝、非 ready、variant 不存在、对象缺失和缺少依赖都返回不可用/拒绝错误，不透露资源是否存在。

## 主体/账号

- Owner：`system.access`；当前版本仅实现 `ainp.account`。
- `Get` 只返回主体引用、显示名、启用状态和认证版本 revision。
- `Invalidate` 必须由可信 tenant/actor 调用；目标不是当前主体时需显式权限，使用 revision 条件更新并写统一审计。跨租户、未知主体、并发 revision 冲突均 fail-closed。
- 调用方不得把客户端传入的 tenant 当作授权依据，不得直接更新 `tenant_accounts.auth_version` 或 `auth_sessions`。

## 通知接收人

通知 Resolver 通过同一主体投影解析账号接收人；不得恢复直接查询 `tenant_accounts` 的下游实现。API、Outbox 与 Worker 共享这一个 Port 实现。

## 版本策略

contract 在同一主版本内只做兼容追加；删除或改名必须先完成下游迁移，并随正式 platform Tag 发布。模块根 Facade 的兼容别名只服务迁移窗口，不得暴露 repository、裸 DB、SQL 条件或内部 Model。
