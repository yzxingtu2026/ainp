# 统一认证与会话

Agent 只读要点：`.agent/rules/authentication-and-session.md`。

## Principal Kind 与 Client Surface

- 主体使用 `auth.PrincipalRef{Kind, ID}`；JWT `sub` 始终等于 `PrincipalRef.ID`，`principal_kind` 必须已注册。
- AINP 内置 `ainp.account` 与 `ainp.tenant-console`。产品主体使用 owner 命名空间，并通过 `App.RegisterPrincipalProvider` 注册。
- Client Surface 是服务端安全事实，包含稳定 ID、唯一 audience、HTTP path prefix、允许 Kind 和认证方法；通过 `App.RegisterClientSurface` 注册。
- Host 在模块完成 `Prepare/Register` 后校验并封存注册表。未知 Kind、Provider、认证方法、Action 引用以及重复 audience/prefix 都阻止启动。
- HTTP 按最长 path prefix 解析 Surface，再按目标 audience 验签。未知路径、跨 audience、Token surface 与路径不一致、Kind/Surface 不兼容均 fail-closed。
- 多主体使用 `TokenService.IssueFor/ParseFor`；旧 `Issue/Parse` 仅是 Account + tenant-console 兼容入口。

`PrincipalProvider` 只复核主体、会话版本和实时授权投影，不签 Token、不写 AuthContext，也不得让业务主体继承 Account RBAC。

## 主链路

```text
Client → Authentication API → Authenticator Registry → Identity Resolver
       → Principal Provider → Principal Snapshot → Session Service → Token Service
       → Principal Provider recheck → AuthContext / live authorization
```

- Authenticator 只验证凭据并返回 `VerifiedIdentity`，禁止签发 Token、加载权限或写 AuthContext。
- Authenticator 返回凭据身份；自定义主体必须同时返回 `PrincipalRef` 与候选 `AuthVersion`，随后由对应 Principal Provider 实时复核。Body/Query `tenant_id` 不是独立可信来源。
- 内置 password identity 继续按 `tenant_id + provider + subject` 映射 `TenantAccount`；该映射不适用于下游主体。
- `TenantAccount` 是租户后台的凭据、角色、权限和审计主体。当前保持一账号一租户；普通业务 `User/users` 不属于本认证域。
- Session Service 在创建和刷新前都重新调用 Principal Provider，只有 Session Service 可以签发 AINP Token。

## 扩展认证器

认证器注册到 `framework/authentication.Registry`，method 使用稳定小写标识。直接认证实现 `Complete`；验证码、Passkey、OIDC 等声明 challenge 并实现 `Begin + Complete`。redirect 必须同时声明 challenge。

```go
type Authenticator interface {
    Method() string
    Capabilities() Capabilities
    Begin(context.Context, BeginRequest) (*Challenge, error)
    Complete(context.Context, CompleteRequest) (*VerifiedIdentity, error)
}
```

Provider secret 使用启动安全配置或 Secret 管理，不进入普通租户设置 JSON。短信、邮件、Passkey、OIDC、微信或钉钉 Adapter 必须单开 Issue，并复用统一身份、会话、限流和审计链路。

## 密码认证

`password` 是首个内置 Authenticator。兼容入口 `/auth/login` 与统一入口 `/auth/authenticate` 共用同一主链路。首次成功登录为存量用户补建 password identity，不复制密码摘要。账号不存在与密码错误使用相同语义，避免枚举。

## 会话协议

- access token 是短期 JWT，包含 tenant、`sub`、Principal Kind、realm、auth version、session、audience 和 Client Surface，不包含 roles/permissions。
- refresh token 是高熵 opaque token，数据库只保存 SHA-256 摘要；已使用摘要进入 `auth_refresh_token_uses`，不保存 Token 原文。
- refresh 在事务与行锁内轮换；任意已使用 token（不只上一枚）重放都会撤销该 session token family。
- 会话唯一主体事实是 `tenant_id + principal_kind + principal_id`，并固定 `client_surface + audience + token_family_id`。仅 `ainp.account` 兼容保留可空 `account_id` 外键；其他 Kind 必须为 NULL。
- 列表和撤销同时限定当前 tenant、Principal Ref 和 Client Surface，不能跨 Kind、跨主体或跨访问面操作。
- refresh 在行锁内按 Session 固定 Kind/Surface/Audience 调用 Principal Provider；停用、版本变化、Provider 缺失或注册漂移均拒绝刷新。
- Web access token 延续现有 local storage；refresh token 仅放 tab 级 session storage。高安全部署可替换为 HttpOnly Secure SameSite Cookie/BFF。

会话基础迁移见 `0012_authentication_sessions.sql`，账号领域原位重命名见 `0054_tenant_accounts.sql`。`0058_principal_sessions.sql` 原地回填旧 Account Session 的 Kind、Surface、Audience 和 family，旧 Account access/refresh 继续兼容，无需因本迁移重新登录；新主体不得写入 `account_id`。升级前已经消费且早于 `previous_refresh_token_hash` 的历史 Token 没有可恢复摘要；升级后的每次轮换开始记录完整使用历史。

MCP 使用独立的 OAuth Grant、Token family 和 `ainp:mcp` audience，不复用网页 Session Token。其 Refresh 重放检测沿用本节原则，但撤销 MCP Grant 不影响网页会话，详见 `mcp-oauth.md`。

## API

| API | Access | 说明 |
|---|---|---|
| `GET /api/v1/system/auth/methods` | public | 启用方法与非敏感能力元数据 |
| `POST /api/v1/system/auth/challenges` | public | 创建认证挑战 |
| `POST /api/v1/system/auth/authenticate` | public | 完成认证并建立会话 |
| `POST /api/v1/system/auth/refresh` | public | 原子轮换 refresh token |
| `POST /api/v1/system/auth/logout` | authenticated | 撤销当前会话 |
| `GET /api/v1/system/auth/sessions` | authenticated | 当前用户有效会话 |
| `POST /api/v1/system/auth/sessions/revoke` | authenticated | 撤销自己的指定会话 |
| `POST /api/v1/system/auth/sessions/revoke-all` | authenticated | 撤销自己的全部会话 |
| `POST /api/v1/system/profile/password/update` | authenticated | 校验当前密码后修改本人密码，并使全部旧会话失效 |

每个下游 Client Surface 还会在其 `PathPrefix + /auth/...` 自动投影 methods、challenge、authenticate、refresh、logout 和 session 路由。公共认证请求的 `client` 必须等于请求路径解析出的 Surface；refresh token 也只能从原 Surface 路径轮换。

异步 Outbox Event 与 JobRun 同样持久化 `principal_kind + actor/created_by + client_surface + audience`。Dispatcher/Worker 恢复 AuthContext 时必须使用这些字段；旧事件/任务仅有 Account actor 时按 `ainp.account` 兼容，禁止默认切换到其他主体或 tenant-console Surface。

审计记录保存 Actor ID 的同时必须保存 Principal Kind、Client Surface 与 audience；幂等唯一键包含 Principal Kind，避免不同主体类型使用相同 ID 和 key 时互相重放或冲突。旧审计与幂等记录按 `ainp.account` 回填，不能据此推断其他业务主体。

## Challenge、限流与审计

Challenge 只保存摘要，必须具备短 TTL、单次消费、失败次数上限和并发条件更新。具体认证器负责写入并消费，不得存验证码明文。

入口经 `authentication.RateLimiter` 统一限流；默认进程内实现提供单实例基线，多实例部署应替换为 Redis 等共享实现。限流键不得包含密码、验证码或 Token 原文。

登录、challenge、refresh、退出、撤销和本人密码修改使用统一 Action 审计。本人修改密码必须校验当前密码、复用统一强度规则并递增 `auth_version`；密码、验证码、Token、challenge secret 和完整认证 Body 禁止进入日志与审计。

## Passkey 边界

设备生物认证必须使用 Passkey/WebAuthn。指纹、人脸只在设备安全区本地验证，服务端仅保存 credential ID、公钥及计数器；禁止采集、上传或存储原始生物特征。

## 检查清单

- [ ] 新认证器不签 Token、不加载权限
- [ ] Identity 受可信 tenant 约束并 fail-closed
- [ ] refresh 只存摘要、事务轮换、重放撤销
- [ ] Token 权限仍由 AuthorizationProvider 实时加载
- [ ] Session 以 Principal Ref + Surface 为事实；仅 Account 兼容关联 `account_id`
- [ ] Session Service 签发前再次调用 Provider，refresh 固定原 Kind/Surface/Audience
- [ ] methods 只暴露启用方法和非敏感元数据
- [ ] challenge 有 TTL、单次消费和次数上限
- [ ] 敏感凭据未进入日志、审计、响应或普通设置

变更协议时同步本文、`auth-context.md` 与 Agent 规则，并运行 `agent-rules sync`。
