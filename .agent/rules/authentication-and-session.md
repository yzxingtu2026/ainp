# 统一认证与会话

- 新业务主体注册 owner 命名空间 `PrincipalKind`、`PrincipalProvider`、Authenticator 和 Client Surface；禁止任意 Kind 与未知路径回落。
- Token 绑定 `sub`、Kind、audience、surface、session、auth version，并按请求 path 对应 audience 验签；禁止跨客户端复用。
- 使用独立 Bearer 协议的入口只能按精确路径委托给专用认证处理器；禁止前缀放行或未知路径回落，专用处理器必须完整校验自身 audience、授权事实与实时权限。
- 未验签 Token 的协议分流只能用 `session_id/auth_version` 等会话独占 claim；`principal_kind/surface` 是跨协议事实，不得单独判定为 Session Token。分流不产生 AuthContext，专用 Handler 仍须完整验签授权。
- Provider 只做主体/状态/授权复核，不签 Token、不写可信 AuthContext、不借用 Account RBAC。
- 新认证型 Action 显式声明 `PrincipalKinds` 与 `ClientSurfaces`；Account 默认仅用于历史兼容。
- 对话运行面白名单由 Host 按 Surface `PathPrefix` 投影 `/api/v1/...`；交叉 audience/path fail-closed，不泄露未投影内部路由。

- 新登录方式注册 `Authenticator`；Provider 只验证凭据并返回 `VerifiedIdentity`，禁止自行签 Token、加载权限或写 AuthContext。
- Authenticator 只产出凭据身份/主体候选；Principal Provider 实时复核主体、状态与版本，Session Service 复核后才可签 Token。
- access token 必须含 `session_id` 和 `auth_version`，不得携带/信任 roles/permissions；请求仍经过 `AuthorizationProvider`。
- refresh token 使用高熵 opaque 值，数据库只存摘要；事务轮换，旧 token 重放撤销对应会话族。
- 会话事实使用 tenant + Principal Ref + Client Surface + audience；仅 `ainp.account` 可兼容关联 `account_id`，业务主体禁止复用。
- Client Surface 会话策略必须保持零值兼容：`AbsoluteTTL` 仅覆盖新会话的 refresh/absolute lifetime，`IdleTTL` 在 refresh 与受保护请求路径共同执行，`MaxActiveSessions=0` 表示无限制，设置上限但省略 `OverflowPolicy` 时按 `reject`。
- 并发会话上限必须在同一事务和主体+Surface 粒度数据库锁内检查与创建；`revoke_oldest` 按创建时间稳定排序。空闲、绝对到期和溢出撤销必须原子写入稳定 reason 与审计，不得改变 access token TTL。
- refresh 固定 Session 原 Kind/Surface/Audience 并在行锁内轮换；重放撤销 token family，Provider 停用/版本变化后禁止刷新。
- 会话列表/撤销限定当前 tenant、Principal Ref 和 Surface；认证 body 的 client 必须匹配请求路径解析出的 Surface。
- Outbox/Job 持久化并恢复 Principal Kind、Surface 和 audience；异步恢复不得丢失主体事实或默认切换主体类型。
- 审计保留 Principal Kind/Surface/audience；幂等作用域必须包含 Principal Kind，禁止跨 Kind 共享重放记录。
- 通知和 MCP Grant/Token/Confirmation 保留来源 Principal Kind、Surface、audience；当前公共 MCP 授权只允许 Account tenant-console，跨 Kind/Surface 复用 fail-closed。
- 后台 Account RBAC 事实源仍是 `TenantAccount/tenant_accounts`；业务主体由下游 Provider 持有，不得继承 Account RBAC。
- 本人修改密码必须校验当前密码、复用统一强度规则并递增 `auth_version`，使全部旧会话失效；不得复用管理员重置密码权限接口。
- challenge 短 TTL、单次消费、限制失败次数；入口经统一 `RateLimiter`，多实例部署替换共享实现。
- 浏览器 WebSocket 只使用已认证 Bearer 请求签发的短时 opaque grant；目标路径须在 Host 显式注册并绑定原 Principal/Session/Surface/Audience/AuthVersion。grant 通过 `Sec-WebSocket-Protocol` 携带，禁止 Query Token；Origin 校验先于原子单次消费，消费后必须经 AuthorizationProvider 实时复核并恢复 AuthContext。
- 密码、验证码、Token、challenge secret 禁止进入日志、审计、响应或普通租户设置。
- 生物认证只允许 Passkey/WebAuthn；禁止采集、上传、存储原始指纹或人脸。
- 新具体渠道单开 Issue，复用统一认证/身份/会话主链。
- 运行时配置型 Authenticator 实现可选 `Available(ctx) bool`；methods/challenge/authenticate 必须复用同一检查并 fail-closed，禁止泄露 Integration/Credential 细节。
- redirect 展示只使用受校验的静态 label/label_key/icon/order；Admin 固定 `/auth-callback` 经统一 authenticate 建 Session，清理 code/state，登录后仅允许站内单斜杠 redirect。

详规：`docs/development/authentication-and-session.md`
