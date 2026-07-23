# MCP OAuth 与远程传输

AINP 的远程 MCP 入口使用官方 Go MCP SDK 提供 Streamable HTTP。业务不得手写 JSON-RPC、SSE 或 session 状态机。

## 第一期边界

- MCP endpoint 固定为 `/mcp`，OAuth metadata 位于标准 `/.well-known/` 路径。
- authorization endpoint `/oauth/authorize` 是前端浏览器页面；它使用网页登录态访问 `/api/v1/mcp/oauth/authorize` 查询并确认授权，避免页面路由与协议 API 冲突。
- Public Client 通过动态注册获得 `client_id`，不依赖 client secret；redirect URI 必须精确匹配，非回环地址必须使用 HTTPS。
- 授权只支持 Authorization Code + PKCE S256。授权码 5 分钟内单次消费，并绑定 Client、redirect URI、Grant 和 challenge。
- MCP Access Token audience 固定为 `ainp:mcp`，与网页 `ainp:tenant-console` 双向隔离。
- Refresh Token 使用 opaque token，数据库只存 SHA-256；每次刷新轮换，旧 token 重放会撤销整个 Grant。
- Grant 固定 tenant、user、client、scope 和 `auth_version`。每次 MCP 请求重新加载 Grant，并通过 `AuthorizationProvider` 实时校验用户、租户和授权版本。
- 第一期只建立 transport 与授权边界，不投影 Action Tool；`tools/list` / `tools/call` 属于第二期。

## Scope

| Scope | 授权页默认值 | 语义 |
|---|---|---|
| `actions:read` | 选中且必选 | 允许后续发现和调用只读 Action |
| `actions:write` | 不选中 | 普通写的持续授权，不扩大实时 permission |
| `actions:sensitive` | 不选中 | 只允许发起敏感操作，仍需逐次确认 |

Scope 只缩小能力。任何阶段都不得绕过 realm、tenant、实时 permission、Action 状态或敏感操作确认。

## 部署

- `mcp.base_url` / `MCP_BASE_URL` 必须是客户端可访问的规范公网地址；生产必须为 HTTPS。
- `/mcp` 拒绝未知 Origin，并限制请求体为 1 MiB。反向代理必须覆盖 Host、Origin、连接和读取超时门禁。
- Code、Access/Refresh Token、PKCE verifier、Cookie 和完整授权 Body 禁止进入日志、审计或错误响应。

个人中心的“MCP 连接”页签通过 authenticated API 读取规范 MCP URL，并提供桌面 Agent 配置指南。指南只展示名称、`streamable_http` 类型和 URL；OAuth 客户端的 Bearer 环境变量及自定义 Header 必须留空，禁止引导用户复制 Token。

## 数据

`mcp_oauth_clients` 是协议级全局 Client；Grant 是 tenant-owned 用户委托；授权码和 Token family 只能通过受控 SystemDB 协议仓储访问。Grant 撤销独立于网页 Session，网页 Token 和 MCP Token 不得跨入口使用。
