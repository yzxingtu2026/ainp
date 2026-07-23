# MCP OAuth

- 远程 MCP transport 必须使用成熟 SDK；禁止手写 JSON-RPC、SSE 或 session 状态机。
- Action Registry 是唯一工具事实源；禁止建立 MCP 专用业务工具清单。
- MCP Action Adapter 只投影 tenant realm、`AIMeta.ExposeTool=true` 的 Action；list/call 均实时求 Action 状态、realm、permission、scope 交集并只经 `ActionRegistry.Invoke` 执行。目录上限 100，普通写需要 `actions:write`，敏感 Action 在逐次确认链完成前不可见、不可执行。
- Tool name 必须由 Action ID 确定性映射并检测冲突；原 Action ID/RuntimeRef、permission refs、tenant 与未授权数量不得对外泄露。
- MCP `_meta` 幂等键只写入可信 AuthContext；审计记录 `actor_type/client=mcp`、client/grant、Action、permission、result/replay，禁止记录参数、Token 或原始结果。
- MCP 必须使用独立 `ainp:mcp` audience、Grant 和 Refresh family；不得复用网页 Token。
- OAuth 只允许 Authorization Code + PKCE S256；redirect URI 精确匹配，授权码单次短 TTL。
- Grant 固定 tenant/user/client/scope/auth_version；每次请求通过 AuthorizationProvider 实时校验。
- `actions:read` 默认选中；write/sensitive 默认关闭；sensitive scope 永不替代逐次确认。
- OAuth Code、Token、PKCE verifier、Cookie 和敏感参数不得进入日志或审计摘要。
- 详规见 `docs/development/mcp-oauth.md`。
