# 统一认证与会话

- 新业务主体注册 owner 命名空间 `PrincipalKind`、`PrincipalProvider`、Authenticator 和 Client Surface；禁止任意 Kind 与未知路径回落。
- Token 绑定 `sub`、Kind、audience、surface、session、auth version，并按请求 path 对应 audience 验签；禁止跨客户端复用。
- Provider 只做主体/状态/授权复核，不签 Token、不写可信 AuthContext、不借用 Account RBAC。
- 新认证型 Action 显式声明 `PrincipalKinds` 与 `ClientSurfaces`；Account 默认仅用于历史兼容。

- 新登录方式注册 `Authenticator`；Provider 只验证凭据并返回 `VerifiedIdentity`，禁止自行签 Token、加载权限或写 AuthContext。
- Authenticator 只产出凭据身份/主体候选；Principal Provider 实时复核主体、状态与版本，Session Service 复核后才可签 Token。
- access token 必须含 `session_id` 和 `auth_version`，不得携带/信任 roles/permissions；请求仍经过 `AuthorizationProvider`。
- refresh token 使用高熵 opaque 值，数据库只存摘要；事务轮换，旧 token 重放撤销对应会话族。
- 会话事实使用 tenant + Principal Ref + Client Surface + audience；仅 `ainp.account` 可兼容关联 `account_id`，业务主体禁止复用。
- refresh 固定 Session 原 Kind/Surface/Audience 并在行锁内轮换；重放撤销 token family，Provider 停用/版本变化后禁止刷新。
- 会话列表/撤销限定当前 tenant、Principal Ref 和 Surface；认证 body 的 client 必须匹配请求路径解析出的 Surface。
- 后台 Account RBAC 事实源仍是 `TenantAccount/tenant_accounts`；业务主体由下游 Provider 持有，不得继承 Account RBAC。
- 本人修改密码必须校验当前密码、复用统一强度规则并递增 `auth_version`，使全部旧会话失效；不得复用管理员重置密码权限接口。
- challenge 短 TTL、单次消费、限制失败次数；入口经统一 `RateLimiter`，多实例部署替换共享实现。
- 密码、验证码、Token、challenge secret 禁止进入日志、审计、响应或普通租户设置。
- 生物认证只允许 Passkey/WebAuthn；禁止采集、上传、存储原始指纹或人脸。
- 新具体渠道单开 Issue，复用统一认证/身份/会话主链。

详规：`docs/development/authentication-and-session.md`
