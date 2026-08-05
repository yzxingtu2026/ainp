# 统一认证与会话

Agent 只读要点：`.agent/rules/authentication-and-session.md`。

## 主链路

```text
Client → Authentication API → Authenticator Registry → Identity Resolver
       → TenantAccount / Account Policy → Session Service → Token Service
       → AuthorizationProvider → AuthContext / RBAC
```

- Authenticator 只验证凭据并返回 `VerifiedIdentity`，禁止签发 Token、加载权限或写 AuthContext。
- Identity 由 `tenant_id + provider + subject` 唯一标识并通过 `account_id` 映射到一个 `TenantAccount`；Body/Query `tenant_id` 不是可信来源。
- `TenantAccount` 是租户后台的凭据、角色、权限和审计主体。当前保持一账号一租户；普通业务 `User/users` 不属于本认证域。
- Session 管理刷新与撤销；AuthorizationProvider 仍实时加载用户、角色和权限。

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

- access token 是短期 JWT，包含 `tenant_id + user_id + realm + auth_version + session_id`，不包含 roles/permissions。
- refresh token 是高熵 opaque token，数据库只保存 SHA-256 摘要。
- refresh 在事务与行锁内轮换；旧 token 重放撤销该 session token family。
- 会话表使用 `account_id` 关联 `TenantAccount`；会话操作只能作用于 AuthContext 当前 tenant/user。`AuthContext.UserID` 和 JWT `user_id` 是兼容协议字段，值为当前 `TenantAccount.ID`。
- AuthorizationProvider 校验 session 未撤销、未过期且 auth_version 一致；用户/租户停用或授权版本变化后不得刷新。
- Web access token 延续现有 local storage；refresh token 仅放 tab 级 session storage。高安全部署可替换为 HttpOnly Secure SameSite Cookie/BFF。

新增 `sid` 后旧 JWT 不兼容，升级需要重新登录。会话基础迁移见 `platform/migrations/0012_authentication_sessions.sql`；账号领域原位重命名见 `0054_tenant_accounts.sql`，该迁移保留既有 identity、session、refresh family 和授权版本。

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
- [ ] Identity / Session 仅以 `account_id` 关联 `tenant_accounts`，会话操作强制当前 tenant/user scope
- [ ] methods 只暴露启用方法和非敏感元数据
- [ ] challenge 有 TTL、单次消费和次数上限
- [ ] 敏感凭据未进入日志、审计、响应或普通设置

变更协议时同步本文、`auth-context.md` 与 Agent 规则，并运行 `agent-rules sync`。
