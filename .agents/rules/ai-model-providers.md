# AI 模型治理

- 业务/Agent 只引用 Alias；禁止直写供应商模型 ID、Endpoint、协议分支或 API Key。
- Provider Type 由 `framework/aimodel` 代码注册；新协议必须声明 Adapter 能力上限和可选 discovery。
- Provider 协议只描述传输、认证和消息映射；Deployment 单独声明模型实际能力，禁止把协议名称当作能力事实。
- Provider、Deployment、Alias 必须从 AuthContext 强制 tenant scope，缺租户 fail-closed。
- 供应商凭证只接受 API 写入并以 AES-256-GCM 密文落库，不兼容 `env:*`；API 只返回 `credential_configured`。
- 数据加密主密钥由平台 `CREDENTIAL_ENCRYPTION_KEY` 注入，AI 只复用 framework-owned Cipher Facade；旧 AI key 仅兼容一个弃用周期。禁止与 JWT Secret 共用；明文和密文均禁止进入日志、审计与响应。
- 远端模型仅是候选，管理员确认类型、能力、限制并保存为 enabled Deployment 后才可解析。
- 有效能力不得超过 Adapter 与 Deployment 声明的交集；Resolver 不满足要求时禁止静默降级。
- Enabled Embedding Deployment 的创建、Discovery 导入、更新、启用及 Alias 绑定必须复用统一能力契约；disabled 存量错误配置只允许诊断和修正，不得重新启用。
- Enabled Chat Deployment 的 `context_window` 与 `max_output_tokens` 必须是正整数，且扣除最大输出和 10% 安全余量后仍有正的输入预算；创建、更新、Discovery 导入、启用、Alias 与 Resolver 复用 `framework/aimodel` 公共预算函数。
- Discovery 缮信任 Limits 时不得猜测或填超大默认值；补齐前保持 disabled。历史非法 Enabled Chat 必须以 `model.context_budget_invalid` fail-closed，不得误报 Run 预算耗尽。
- Alias 在模型目录内联管理，不建立独立菜单；fallback/权重需新增策略模型。
- Discovery 必须限制 HTTPS 公网 Endpoint、重定向、超时、分页/数量和响应体，并脱敏错误。
- 详规见 `docs/development/ai-model-providers.md`。
