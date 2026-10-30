# 统一认证与会话

Agent 只读要点：`.agent/rules/authentication-and-session.md`。

## Principal Kind 与 Client Surface

- 主体使用 `auth.PrincipalRef{Kind, ID}`；JWT `sub` 始终等于 `PrincipalRef.ID`，`principal_kind` 必须已注册。
- AINP 内置 `ainp.account` 与 `ainp.tenant-console`。产品主体使用 owner 命名空间，并通过 `App.RegisterPrincipalProvider` 注册。
- Client Surface 是服务端安全事实，包含稳定 ID、唯一 audience、HTTP path prefix、允许 Kind 和认证方法；通过 `App.RegisterClientSurface` 注册。
- Client Surface 可选配置会话策略：`AbsoluteTTL` 覆盖该访问面新会话的全局 `auth.refresh_ttl`，`IdleTTL` 在刷新和受保护请求时校验空闲时间，`MaxActiveSessions` 限制同一租户、主体和访问面的活动会话数，`OverflowPolicy` 可取 `reject` 或 `revoke_oldest`。
- 会话策略零值保持兼容：两个 TTL 为 `0` 时不增加对应限制，`MaxActiveSessions=0` 表示不限制；设置并发上限但省略 `OverflowPolicy` 时按 `reject` 处理。策略不改变 `auth.jwt.ttl`，只影响新建会话及服务端会话校验。
- 空闲、绝对到期和溢出撤销分别返回稳定错误 key，并记录 `system.auth_session_policy_revoke` 审计。升级迁移会增加 `auth_sessions.revoked_reason` 和活动会话查询索引。
- Host 在模块完成 `Prepare/Register` 后校验并封存注册表。未知 Kind、Provider、认证方法、Action 引用以及重复 audience/prefix 都阻止启动。
- HTTP 按最长 path prefix 解析 Surface，再按目标 audience 验签。未知路径、跨 audience、Token surface 与路径不一致、Kind/Surface 不兼容均 fail-closed。
- 独立 Bearer 协议入口只能通过显式精确路径跳过 Web Session Auth，并由目标 Handler 校验自己的 audience、授权事实和实时权限；不得使用前缀放行，未知路径仍须 fail-closed。当前仅 `/mcp` 使用该机制。
- Web Session Auth 在正式验签失败后只能使用 `session_id` / `auth_version` 等会话独占 claim 判断请求是否携带失效 Session Token；`principal_kind`、`surface` 等跨协议身份事实不得单独作为会话分类依据。被分流给附件等专用 Handler 的 Bearer Token 不会获得 Session AuthContext，仍必须由目标 Handler 完整验签并执行自己的授权协议。
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

运行时依赖 Integration、Credential 或租户配置的认证器可额外实现可选接口 `Available(context.Context) bool`。统一认证服务会在 methods、challenge 和 authenticate 三条公开入口使用同一检查；返回 false 或检查 panic 均 fail-closed，对外只返回通用 method unavailable，不泄露配置、数据库或 credential 细节。未实现该接口的认证器按静态可用处理。

`Capabilities` 同时是受信任的静态展示元数据边界：redirect method 必须声明 `challenge=true`，可提供 `label` / `label_key`、Iconify 风格的固定 `icon`（例如 `simple-icons:dingtalk`）和非负 `order`。注册时校验格式，目录按 order、method 确定性排序；这些字段不得承载动态 import、远程组件、HTML 或 Provider 配置。

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

## 浏览器 WebSocket 握手

浏览器原生 `WebSocket` 不能设置 `Authorization`。宿主需要长连接时，必须在模块 `Prepare` 阶段通过 `App.RegisterWebSocketHandshakeTarget` 注册静态精确路径；动态参数、通配符、Query 和未命中 Client Surface 的路径会阻止启动或注册。注册目标只授权认证握手，不注册业务路由、Hub、订阅或 Origin allowlist。

已认证页面向当前 Surface 的 `POST .../auth/websocket-grants` 提交 `{"target_path":"/api/v1/customer/events"}`。服务端签发 30 秒有效的高熵 opaque grant，数据库只保存 SHA-256 摘要以及 tenant、Principal、Session、Surface、audience、auth version 和目标路径。浏览器只通过响应的 `protocols` 建立连接，不把 access token 或 grant 放入 URL：

```js
const issued = await api.post('/api/v1/customer/auth/websocket-grants', {
  target_path: '/api/v1/customer/events',
})
const socket = new WebSocket('wss://example.test/api/v1/customer/events', issued.protocols)
```

宿主必须先使用自己的生产 allowlist 校验 `Origin`，再调用 `auth.AuthenticateWebSocketHandshake`；校验成功后用返回请求中的可信 `auth.Context` 执行业务授权并 Upgrade。服务端只选择并回显稳定协议 `ainp.ws.v1`，不得回显凭据协议。以下示例省略业务 Hub：

```go
func websocketHandler(c *gin.Context) {
    if !allowedOrigin(c.Request.Header.Get("Origin")) {
        c.AbortWithStatus(http.StatusForbidden)
        return
    }
    request, err := auth.AuthenticateWebSocketHandshake(c.Request, application.WebSocketHandshakeGrants)
    if err != nil {
        c.AbortWithStatus(http.StatusUnauthorized)
        return
    }
    c.Request = request
    upgrader.Subprotocols = []string{auth.WebSocketHandshakeProtocol}
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    // Hub、订阅、保活、断线恢复和连接期 AuthorizationProvider 复核均由宿主持有。
}
```

消费以数据库条件更新原子完成；过期、篡改、重放、并发第二次消费、跨目标路径、跨 Surface 或 audience 均 fail-closed。消费成功后仍调用 `AuthorizationProvider`，因此已撤销 Session、AuthVersion 变化和主体停用不会恢复 AuthContext。`Sec-WebSocket-Protocol`、grant 和 access token 均属于敏感凭据，日志与审计不得记录原值。

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
| `POST /api/v1/system/auth/websocket-grants` | authenticated | 为已注册目标签发短时单次 WebSocket 握手凭据 |
| `POST /api/v1/system/profile/password/update` | authenticated | 校验当前密码后修改本人密码，并使全部旧会话失效 |

每个下游 Client Surface 还会在其 `PathPrefix + /auth/...` 自动投影 methods、challenge、authenticate、refresh、logout、session 和 websocket-grants 路由。公共认证请求的 `client` 必须等于请求路径解析出的 Surface；refresh token 也只能从原 Surface 路径轮换，WebSocket grant 只能签发给同一 Surface 已注册的精确目标路径。

对话运行面由 Host 把白名单 Action 的 `/api/v1/...` 路径投影到同一 Surface `PathPrefix`，例如 `/api/v1/community/ai/conversations/create-for-slot`。Token audience 必须匹配请求路径解析出的 Surface；社区 Token 调用未投影的 `/api/v1/ai/*`、后台 Token 调用社区投影路径均 fail-closed，且不得泄露内部路由。MCP 公共入口仍只允许 `ainp.account + ainp.tenant-console`。

Admin 登录页只消费 `/auth/methods` 中通过校验且 `redirect=true + challenge=true` 的条目。点击后调用统一 challenge 获取 `authorize_url`，不拼 Provider URL。Provider callback 固定进入平台 `/auth-callback` 页面，由页面调用统一 authenticate 保存 Session/Token；成功后只恢复登录前的单斜杠站内路径，协议相对、跨域与带凭据 URL 一律拒绝。callback 在成功或失败后都 replace 清理 code、state、error 等 query；刷新或重复提交因一次性临时上下文和下游 state 消费规则安全失败并返回密码登录。

异步 Outbox Event 与 JobRun 同样持久化 `principal_kind + actor/created_by + client_surface + audience`。Dispatcher/Worker 恢复 AuthContext 时必须使用这些字段；旧事件/任务仅有 Account actor 时按 `ainp.account` 兼容，禁止默认切换到其他主体或 tenant-console Surface。

审计记录保存 Actor ID 的同时必须保存 Principal Kind、Client Surface 与 audience；幂等唯一键包含 Principal Kind，避免不同主体类型使用相同 ID 和 key 时互相重放或冲突。旧审计与幂等记录按 `ainp.account` 回填，不能据此推断其他业务主体。

通知请求同样保存 Principal Kind、Client Surface 与 audience；Outbox Handler 和 Delivery Job 必须从同一份可信 Envelope 恢复，通知幂等唯一键还必须隔离主体 ID、Surface 和 audience。MCP OAuth Grant、Access Token、Refresh、Confirmation 也必须绑定来源 Principal Ref、Surface 和 audience；当前 MCP 公共入口仅允许 `ainp.account + ainp.tenant-console`，任何伪造或跨 Surface 复用均 fail-closed。

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
- [ ] methods / challenge / authenticate 复用同一 Available 边界
- [ ] redirect callback 使用平台固定路由并清理敏感 query
- [ ] challenge 有 TTL、单次消费和次数上限
- [ ] WebSocket 目标显式注册；Origin 先校验；grant 不进 Query/日志并且并发只能消费一次
- [ ] 敏感凭据未进入日志、审计、响应或普通设置

变更协议时同步本文、`auth-context.md` 与 Agent 规则，并运行 `agent-rules sync`。
