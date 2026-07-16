# AI 模型供应商与模型目录

## 边界

业务与 Agent 只能引用稳定 Alias，禁止直接保存供应商模型 ID、Endpoint 或 API Key。解析链固定为：

`Alias -> ModelResolver -> Deployment -> Provider -> ProviderTypeRegistry / CredentialCipher`

本阶段只允许模型列表发现请求，不提供 Generate、Stream、Embedding、图片、音频、Tool Calling、RAG 或 Agent Runtime。

## Provider Type

Provider Type 由代码注册，`protocol` 是稳定契约。注册项包含配置 Schema、是否支持 discovery，以及 Adapter 已实现的能力上限。空 protocol、重复 protocol、非法 Schema、未知 modality/feature 必须在启动注册时失败。

新增协议时：

1. 注册新的 `aimodel.ProviderType`，不得在业务中按模型字符串分支。
2. 如支持发现，实现 `ModelDiscoveryProvider`，输出标准 `RemoteModelPage`。
3. 明确 Adapter 已实现能力。Deployment 勾选不能扩大此上限。
4. 添加 Registry、发现响应限制、错误脱敏和 SSRF 测试。

## Provider 与加密凭证

Provider 按 AuthContext 的 `tenant_id` 隔离。管理员提交的供应商凭证通过 AES-256-GCM 加密后保存到 `credential_ciphertext`，供应商凭证不支持 `env:*` 引用。平台级 `ai.credential_encryption_key` 必须是 32 字节随机密钥的 Base64，生产通过 `AI_CREDENTIAL_ENCRYPTION_KEY` 注入，不得与 JWT Secret 共用。

每次加密使用随机 nonce，并以 `tenant_id + provider_id` 作为 AAD，密文不能跨 Provider 搬用。API 只返回 `credential_configured`；不返回明文或密文。编辑 Provider 时凭证留空表示保留，填写表示重新加密。解密后的凭证只能存在于单次服务端发现/解析请求内存中，禁止进入 API、日志、审计或普通租户设置。

自定义 Endpoint 填写服务根地址且结尾不带 `/v1`，例如 `https://api.example.com`；发现 Adapter 统一追加 `/v1/models`。Endpoint 仅接受无 userinfo 的 HTTPS 公网地址；localhost、环回、私网、链路本地和不安全重定向均拒绝。发现请求固定超时、分页上限、响应体上限，错误只返回脱敏摘要。

## Deployment 与能力

Deployment 是管理员确认后的本地模型资产。远端发现结果只是候选，不得自动创建或启用 Deployment；远端缺少类型/能力时 UI 必须要求管理员确认。手工模型同样必须声明类型、能力和限制。

稳定 model type：`chat`、`embedding`、`rerank`、`image`、`speech`、`transcription`。`vision` 是 chat 输入能力，不是 model type。有效能力始终为：

`Provider Adapter 已实现能力 ∩ Deployment 声明能力`

Resolver 再验证调用方 `CapabilityRequirements` 是否为有效能力的子集。非法 modality、feature 或负数 limit 拒绝保存。

## Alias 与解析

Alias 是租户内唯一的一对一映射，同一个 Deployment 可被多个 Alias 引用。Alias 在模型目录内联管理，不注册独立菜单。`Resolve(ctx, alias, requirements)` 依次检查租户、Alias、Deployment 状态、Provider 状态、model type、能力、Provider Type 与加密凭证，任一失败都不降级、不跨租户查找。

未来 fallback、权重和容灾应新增目标/策略模型，禁止把逗号列表或路由 JSON 塞进 Alias。

## 示例

```go
resolved, err := app.ModelResolver.Resolve(ctx, "general-chat", aimodel.CapabilityRequirements{
    ModelType: aimodel.ModelTypeChat,
    Features: []aimodel.Feature{aimodel.FeatureStreaming, aimodel.FeatureToolCalling},
})
```

下一 Issue 才能基于 `ResolvedModel` 实现统一模型请求和流式事件协议。
