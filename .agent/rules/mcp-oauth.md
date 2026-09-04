# MCP OAuth

- 远程 MCP transport 必须使用成熟 SDK；禁止手写 JSON-RPC、SSE 或 session 状态机。
- Action Registry 是唯一工具事实源；禁止建立 MCP 专用业务工具清单。
- MCP Action Adapter 只投影 tenant realm、`AIMeta.ExposeTool=true` 的 Action；list/call 均实时求 Action 状态、realm、permission、scope 交集并只经 `ActionRegistry.Invoke` 执行。工具目录必须使用稳定排序和标准游标分页，单页上限 100，禁止无游标截断；游标绑定当前授权目录，目录或授权变化、游标无效或过期均 fail-closed 并要求从第一页重试。普通写需要 `actions:write`；敏感 Action 需要 `actions:sensitive` 才可见，首次调用必须返回逐次确认请求而非直接执行。
- Tool name 必须由 Action ID 确定性映射并检测冲突；原 Action ID/RuntimeRef、permission refs、tenant 与未授权数量不得对外泄露。
- MCP `_meta` 幂等键只写入可信 AuthContext；审计记录 `actor_type/client=mcp`、client/grant、Action、permission、result/replay，禁止记录参数、Token 或原始结果。
- MCP 必须使用独立 `ainp:mcp` audience、Grant 和 Refresh family；不得复用网页 Token。
- `/mcp` 必须按精确路径跳过 Web Session Auth，再由 MCP Handler 完整校验 Grant、Token、Principal、来源 Surface 与实时授权；禁止放行 `/mcp/` 前缀或其他未知路径。
- OAuth 只允许 Authorization Code + PKCE S256；redirect URI 精确匹配，授权码单次短 TTL。
- Grant 固定 tenant/user/client/scope/auth_version；每次请求通过 AuthorizationProvider 实时校验。
- `actions:read` 默认选中；write/sensitive 默认关闭；sensitive scope 永不替代逐次确认。
- 敏感确认优先使用客户端声明的标准 URL elicitation，不支持时返回结构化 URL；响应必须提供 opaque confirmation ID 和固定恢复工具。恢复工具只接受 confirmation ID，禁止接收 Action ID/业务参数，并在实时重鉴权后只经 `ActionRegistry.Invoke` 单次执行。确认绑定 tenant/user/client/grant、Action/Tool、Schema hash 与规范化参数 hash，参数只以 ownership-bound AEAD 短期密文保存并在消费、拒绝、过期或授权撤销时清空；参数或授权变化、过期、拒绝、撤销和重放全部 fail-closed。
- 用户可撤销单 Grant、某 Client 全部 Grant 或全部 Grant；撤销必须原子失效 Refresh family 与 pending/approved confirmation，且请求路径继续实时校验 Grant。
- OAuth Code、Token、PKCE verifier、Cookie 和敏感参数不得进入日志或审计摘要。
- 详规见 `docs/development/mcp-oauth.md`。
