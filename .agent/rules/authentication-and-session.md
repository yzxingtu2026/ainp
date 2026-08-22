# 统一认证与会话

- 新业务主体注册 owner 命名空间 `PrincipalKind`、`PrincipalProvider`、Authenticator 和 Client Surface；禁止任意 Kind 与未知路径回落。
- Token 绑定 `sub`、Kind、audience、surface、session、auth version，并按请求 path 对应 audience 验签；禁止跨客户端复用。
- Provider 只做主体/状态/授权复核，不签 Token、不写可信 AuthContext、不借用 Account RBAC。
- 新认证型 Action 显式声明 `PrincipalKinds` 与 `ClientSurfaces`；Account 默认仅用于历史兼容。

- 新登录方式注册 `Authenticator`；Provider 只验证凭据并返回 `VerifiedIdentity`，禁止自行签 Token、加载权限或写 AuthContext。
- Identity 在可信 tenant 边界内按 `provider + subject` 解析到唯一 `TenantAccount`，持久化外键使用 `account_id`；禁止信任 Body/Query `tenant_id`。
- access token 必须含 `session_id` 和 `auth_version`，不得携带/信任 roles/permissions；请求仍经过 `AuthorizationProvider`。
- refresh token 使用高熵 opaque 值，数据库只存摘要；事务轮换，旧 token 重放撤销对应会话族。
- 会话操作限定 AuthContext 当前 tenant/user；停用账号/租户或授权版本变化后禁止刷新。`AuthContext.UserID` / JWT `user_id` 是兼容字段，值为 `TenantAccount.ID`。
- 后台认证唯一事实源是 `TenantAccount/tenant_accounts`；业务 `User/users` 不得接入该身份、会话或 RBAC 链路。
- 本人修改密码必须校验当前密码、复用统一强度规则并递增 `auth_version`，使全部旧会话失效；不得复用管理员重置密码权限接口。
- challenge 短 TTL、单次消费、限制失败次数；入口经统一 `RateLimiter`，多实例部署替换共享实现。
- 密码、验证码、Token、challenge secret 禁止进入日志、审计、响应或普通租户设置。
- 生物认证只允许 Passkey/WebAuthn；禁止采集、上传、存储原始指纹或人脸。
- 新具体渠道单开 Issue，复用统一认证/身份/会话主链。

详规：`docs/development/authentication-and-session.md`
