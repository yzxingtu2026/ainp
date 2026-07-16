# AI 模型治理

- 业务/Agent 只引用 Alias；禁止直写供应商模型 ID、Endpoint、协议分支或 API Key。
- Provider Type 由 `framework/aimodel` 代码注册；新协议必须声明 Adapter 能力上限和可选 discovery。
- Provider、Deployment、Alias 必须从 AuthContext 强制 tenant scope，缺租户 fail-closed。
- 供应商凭证只接受 API 写入并以 AES-256-GCM 密文落库，不兼容 `env:*`；API 只返回 `credential_configured`。
- 数据加密主密钥由 `AI_CREDENTIAL_ENCRYPTION_KEY` 注入，禁止与 JWT Secret 共用；明文和密文均禁止进入日志、审计与响应。
- 远端模型仅是候选，管理员确认类型、能力、限制并保存为 enabled Deployment 后才可解析。
- 有效能力不得超过 Adapter 与 Deployment 声明的交集；Resolver 不满足要求时禁止静默降级。
- Alias 在模型目录内联管理，不建立独立菜单；fallback/权重需新增策略模型。
- Discovery 必须限制 HTTPS 公网 Endpoint、重定向、超时、分页/数量和响应体，并脱敏错误。
- 详规见 `docs/development/ai-model-providers.md`。
