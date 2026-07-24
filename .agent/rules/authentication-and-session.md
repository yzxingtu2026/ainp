# 统一认证与会话

- 新登录方式注册 `Authenticator`；Provider 只验证凭据并返回 `VerifiedIdentity`，禁止自行签 Token、加载权限或写 AuthContext。
- Identity 在可信 tenant 边界内按 `provider + subject` 解析到唯一 user；禁止信任 Body/Query `tenant_id`。
- access token 必须含 `session_id` 和 `auth_version`，不得携带/信任 roles/permissions；请求仍经过 `AuthorizationProvider`。
- refresh token 使用高熵 opaque 值，数据库只存摘要；事务轮换，旧 token 重放撤销对应会话族。
- 会话操作限定 AuthContext 当前 tenant/user；停用用户/租户或授权版本变化后禁止刷新。
- 本人修改密码必须校验当前密码、复用统一强度规则并递增 `auth_version`，使全部旧会话失效；不得复用管理员重置密码权限接口。
- challenge 短 TTL、单次消费、限制失败次数；入口经统一 `RateLimiter`，多实例部署替换共享实现。
- 密码、验证码、Token、challenge secret 禁止进入日志、审计、响应或普通租户设置。
- 生物认证只允许 Passkey/WebAuthn；禁止采集、上传、存储原始指纹或人脸。
- 新具体渠道单开 Issue，复用统一认证/身份/会话主链。

详规：`docs/development/authentication-and-session.md`
