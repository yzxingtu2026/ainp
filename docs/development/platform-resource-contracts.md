# 下游平台资源契约

`framework/platformresource` 是 AINP 面向独立下游模块的稳定最小契约，system 模块负责实现，Host 通过 `app.App.PlatformResources` 注入。下游不创建平行 system Service，不直接访问 AINP 内置表。

## 附件

- Owner：`system.attachments`。
- 前置：可信 tenant/principal AuthContext；调用者提供资源类型、资源 ID 和可选字段，服务端重新执行 AccessPolicy、tenant scope、绑定查询和 `ready`/variant 状态检查。
- `Open` 返回 `AttachmentMetadata` 与受控 `io.ReadCloser`；元数据不含 object key、provider、transfer token 或 GORM Model。`BindingExists` 与 `ListBindings` 只返回最小绑定投影。
- `OpenOwnedUpload` 仅打开 tenant realm 中当前 `Principal kind + id` 上传的 `ready` 原始附件，供下游在绑定前校验 MIME、大小、页数与摘要。调用方必须显式提供正数 `MaxBytes`；服务端在打开对象前后校验元数据、对象实际大小与预算，并返回受限 `io.ReadCloser`。该能力不要求附件已有绑定，也不授予任意附件查询或 object key 访问。
- 未绑定、跨租户、策略拒绝、非 ready、variant 不存在、对象缺失和缺少依赖都返回不可用/拒绝错误，不透露资源是否存在。

## 主体/账号

- Owner：`system.access`；当前版本仅实现 `ainp.account`。
- `Get` 只返回主体引用、显示名、启用状态和认证版本 revision。
- `Invalidate` 必须由可信 tenant/actor 调用；目标不是当前主体时需显式权限，使用 revision 条件更新并写统一审计。跨租户、未知主体、并发 revision 冲突均 fail-closed。
- 调用方不得把客户端传入的 tenant 当作授权依据，不得直接更新 `tenant_accounts.auth_version` 或 `auth_sessions`。

### Account Identity Binding

- `AccountIdentities` 仅服务可信 tenant AuthContext 中的当前 `ainp.account`；`List`、`Bind`、`Unbind` 均不接收 tenant ID、account ID、角色或权限字段，账号或租户停用时 fail-closed。
- `Bind` 只接收下游 OAuth/企业身份适配器在服务端完成凭据验证后生成的规范化小写 Provider、稳定 Subject 与 `normalized_identifier`；这些字段不是凭据或绑定证明，禁止把该 DTO 直接暴露为客户端“声明身份”接口。内置 `password` Provider 不开放绑定；服务端在同一 `auth_identities` 唯一事实源上强制 `(tenant_id, provider, subject)` 和 `(tenant_id, account_id, provider)` 唯一，冲突及并发重复统一拒绝。
- `Unbind` 只接收公共稳定 identity ID，并重新校验当前 tenant/account 所有权；未知、跨租户、其他账号或内置密码身份均按不可用处理，不泄露记录存在性。
- 返回投影仅含 identity ID、Provider、Subject、规范化标识与验证时间，不暴露 GORM Model、Metadata、Token、Session、裸 DB 或 SQL 条件。绑定/解绑写统一审计，摘要不含 Provider、Subject、标识、OAuth code/state/token/secret。
- 下游 Authenticator 仍只返回 `authentication.VerifiedIdentity`；登录前解析由 system access 私有 store 完成，绑定服务不创建账号、Session、JWT 或权限事实。

## 通知接收人

通知 Resolver 通过同一主体投影解析账号接收人；不得恢复直接查询 `tenant_accounts` 的下游实现。API、Outbox 与 Worker 共享这一个 Port 实现。

## 版本策略

contract 在同一主版本内只做兼容追加；删除或改名必须先完成下游迁移，并随正式 platform Tag 发布。模块根 Facade 的兼容别名只服务迁移窗口，不得暴露 repository、裸 DB、SQL 条件或内部 Model。
