# MCP OAuth 与远程传输

MCP OAuth Grant、Access Token、Refresh family 和 Confirmation 必须固定授权来源的 Principal Ref、Client Surface 与 audience。当前公共 MCP 授权仅接受 `ainp.account + ainp.tenant-console + ainp:tenant-console`；MCP 执行 Token 使用独立 `ainp:mcp` audience，但 Action 调用继续按来源 Kind/Surface 约束。Kind、Surface、来源 audience 或 Grant 任一不一致均按无效 Grant/不可用工具处理。

AINP 的远程 MCP 入口使用官方 Go MCP SDK 提供 Streamable HTTP。业务不得手写 JSON-RPC、SSE 或 session 状态机。

## OAuth 与传输边界

- MCP endpoint 固定为 `/mcp`，OAuth metadata 位于标准 `/.well-known/` 路径。
- `/mcp` 使用独立 Bearer 认证，由全局 HTTP 认证中间件按精确路径委托给 MCP Handler；MCP Handler 必须完整校验 `ainp:mcp` audience、Grant、Token、Principal、授权来源 Surface 与实时授权。委托不得使用路径前缀，`/mcp/` 及其他未知路径仍按 Web Surface 规则 fail-closed。
- authorization endpoint `/oauth/authorize` 是前端浏览器页面；它使用网页登录态访问 `/api/v1/mcp/oauth/authorize` 查询并确认授权，避免页面路由与协议 API 冲突。
- 未登录时进入统一登录页并把完整授权 URL 作为站内 redirect；登录后的账号切换清理不得覆盖该 redirect，且只接受以单 `/` 开头的站内目标。
- 授权成功后页面短暂展示成功状态并自动返回 Client callback；最终 callback 页面由 MCP 客户端控制，AINP 不得替换或延迟 Token 交换。
- Public Client 通过动态注册获得 `client_id`，不依赖 client secret；redirect URI 必须精确匹配，非回环地址必须使用 HTTPS。
- 授权只支持 Authorization Code + PKCE S256。授权码 5 分钟内单次消费，并绑定 Client、redirect URI、Grant 和 challenge。
- MCP Access Token audience 固定为 `ainp:mcp`，与网页 `ainp:tenant-console` 双向隔离。
- Refresh Token 使用 opaque token，数据库只存 SHA-256；每次刷新轮换，旧 token 重放会撤销整个 Grant。
- Grant 固定 tenant、user、client、scope 和 `auth_version`。每次 MCP 请求重新加载 Grant，并通过 `AuthorizationProvider` 实时校验用户、租户和授权版本。

## Action 自动投影与执行

- Action Registry 是唯一工具事实源。MCP 只投影 `source=action`、tenant realm、`AIMeta.ExposeTool=true` 且 enabled 的 Action；禁止 MCP 专用业务清单、DTO 或 Action ID switch。
- 每个新 MCP session 从已完成注册的 Action Registry 构建协议目录；`tools/list` 每次请求再按 Action 状态、tenant realm、实时 permission、OAuth scope 的顺序求交集。不可见项不返回；目录采用“有界单页 + 标准游标分页”，单页最多 100 项，存在后页时必须返回 `nextCursor`，禁止无游标截断。
- 分页目录的稳定顺序为 `ainp_confirmation_execute`（启用时固定首位），其余工具按 Action RuntimeRef 升序。游标必须 opaque、短期有效，并绑定 Principal、Grant、scope 与当前完整可见目录；后续请求仍实时重算授权交集。游标无效、过期、跨授权复用、锚点消失或目录变化时统一按协议 `Invalid params` fail-closed，客户端必须从第一页重新遍历。
- MCP tool name 由原 Action ID 的可读归一化片段和 SHA-256 稳定摘要组成。原 Action ID 只保留在服务端 `RuntimeRef`，名称冲突启动时 fail-closed。
- 输入 Schema、LLM/Human 描述和只读/破坏性/开放世界/幂等提示来自 Action Tool Definition。MCP 要求输出 Schema 顶层为 object；非 object 的 Action 输出 Schema 不声明为 MCP `outputSchema`，结果仍安全返回。
- `none/read` 需要 `actions:read`；普通 `write` 需要 `actions:write`。`external/paid/destructive/critical` 需要 `actions:sensitive` 才进入目录，并在每次执行前进入逐次确认链；scope 只允许发起确认，不能替代批准。
- `tools/call` 根据稳定名称定位原 Action，重新检查 Grant、scope、realm、permission、暴露状态和 Schema，并且只调用 `ActionRegistry.Invoke`。`_meta.idempotencyKey`（兼容 `idempotency_key`）写入可信 `AuthContext.IdempotencyKey`，复用 Action 的幂等策略。
- Action/校验错误映射为安全 MCP Tool error。鉴权与资源存在性统一返回 `tool is unavailable`；未知错误不返回堆栈、SQL、内部路径或 cause。
- Action 审计记录 `actor_type/client=mcp`、`mcp_client_id`、`mcp_grant_id`、Action、permission、结果和 replay。适配器预检拒绝也写入同一 Action 审计协议；参数和原始结果不进入审计摘要。

## Scope

| Scope | 授权页默认值 | 语义 |
|---|---|---|
| `actions:read` | 选中且必选 | 允许后续发现和调用只读 Action |
| `actions:write` | 不选中 | 普通写的持续授权，不扩大实时 permission |
| `actions:sensitive` | 不选中 | 只允许发起敏感操作，仍需逐次确认 |

Scope 只缩小能力。任何阶段都不得绕过 realm、tenant、实时 permission、Action 状态或敏感操作确认。

## 写操作确认与授权治理

- 普通写操作依赖 `actions:write` 持续授权和 Action 自身幂等策略；敏感操作不直接执行。客户端声明 URL elicitation 能力时由 MCP 协议发起浏览器确认；不支持时返回结构化 `confirmation_required`、浏览器确认 URL、恢复凭证和 10 分钟有效期。
- 确认绑定 tenant、user、Grant、Client、Action、Tool name、Schema hash 与规范化 arguments hash。首次响应同时返回 opaque `confirmation_id`、`resume_tool=ainp_confirmation_execute` 和固定恢复参数；Web 批准后，客户端调用该治理工具恢复一次原 Action。旧客户端仍可通过 `_meta.confirmationToken`（兼容 `confirmation_token`）执行同参数重试。
- `ainp_confirmation_execute` 只接受 confirmation ID，不接受 Action ID 或业务参数；执行前重新检查 Grant、scope、realm、实时 permission、Action 状态、Tool name、Schema 与参数 hash，并以 confirmation ID 派生幂等键，只经 `ActionRegistry.Invoke` 执行。参数、Schema、Grant、Client、用户变化，过期、拒绝、撤销或重复消费均 fail-closed。
- 数据库只保存恢复凭证 SHA-256；规范化 Action 参数使用服务端密钥派生的 AEAD 短期密文并绑定确认 ownership，消费、拒绝、过期或授权撤销时立即清空。确认页只展示服务端固化的 Human 描述、风险和副作用，不返回原始参数、密文或恢复凭证。确认链接未登录时复用统一登录 redirect，登录后必须返回原确认页。
- 个人中心列出当前用户的 MCP Grant，可撤销单次授权、某客户端全部授权或全部授权。撤销在同一事务把 Grant 标记为 revoked，并撤销关联 Refresh Token family 与 pending/approved confirmation；后续 Access Token 请求也因 Grant 实时校验立即失败。
- 待确认调用按 Action 审计协议记录 `result=failure` 和安全摘要；批准、拒绝及授权撤销另写治理审计。审计、日志和错误不得保存原始参数、恢复凭证、Code 或 Token。

## 部署

- `mcp.base_url` / `MCP_BASE_URL` 是 OAuth/MCP 后端规范公网地址；`mcp.web_base_url` / `MCP_WEB_BASE_URL` 是浏览器授权页地址。生产同域部署时两者可相同，本地前后端分端口时必须分别配置。
- `/mcp` 拒绝未知 Origin，并限制请求体为 1 MiB。反向代理必须覆盖 Host、Origin、连接和读取超时门禁。
- Code、Access/Refresh Token、PKCE verifier、Cookie 和完整授权 Body 禁止进入日志、审计或错误响应。

个人中心的“MCP 连接”页签通过 authenticated API 读取规范 MCP URL，并提供桌面 Agent 配置指南。指南只展示名称、`streamable_http` 类型和 URL；OAuth 客户端的 Bearer 环境变量及自定义 Header 必须留空，禁止引导用户复制 Token。

对于支持 `mcpServers` JSON 的客户端，页面生成 `type=streamable-http`、稳定 server key 与 URL 的最小配置并一键复制。不得把 `/mcp` 伪装成 `type=sse`；客户端若使用 `http` 等私有枚举，应提示用户只调整 type。客户端专用命令、Token、Header 或环境变量不得伪装成通用配置。

## 数据

`mcp_oauth_clients` 是协议级全局 Client；Grant 与 confirmation 是 tenant-owned 用户委托事实；授权码和 Token family 只能通过受控 SystemDB 协议仓储访问。Grant 撤销独立于网页 Session，网页 Token 和 MCP Token 不得跨入口使用。
