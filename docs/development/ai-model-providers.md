# AI 模型供应商与模型目录

## 边界

业务与 Agent 只能引用稳定 Alias，禁止直接保存供应商模型 ID、Endpoint 或 API Key。解析链固定为：

`Alias -> ModelResolver -> Deployment -> Provider -> ProviderTypeRegistry / SecretResolver`

本阶段只允许模型列表发现请求，不提供 Generate、Stream、Embedding、图片、音频、Tool Calling、RAG 或 Agent Runtime。

## Provider Type

Provider Type 由代码注册，`protocol` 是稳定契约。注册项包含配置 Schema、是否支持 discovery，以及 Adapter 已实现的能力上限。空 protocol、重复 protocol、非法 Schema、未知 modality/feature 必须在启动注册时失败。

新增协议时：

1. 注册新的 `aimodel.ProviderType`，不得在业务中按模型字符串分支。
2. 如支持发现，实现 `ModelDiscoveryProvider`，输出标准 `RemoteModelPage`。
3. 明确 Adapter 已实现能力。Deployment 勾选不能扩大此上限。
4. 添加 Registry、发现响应限制、错误脱敏和 SSRF 测试。

## Provider 与 Secret

Provider 按 AuthContext 的 `tenant_id` 隔离，`credential_ref` 只保存引用。首期仅支持 `env:VARIABLE_NAME`，`EnvSecretResolver` 找不到环境变量或收到其他格式时 fail-closed。Secret 解析值只能存在于单次服务端请求内存中，禁止进入 API、日志、审计、设置或数据库。

自定义 Endpoint 仅接受无 userinfo 的 HTTPS 公网地址；localhost、环回、私网、链路本地和不安全重定向均拒绝。发现请求固定超时、分页上限、响应体上限，错误只返回脱敏摘要。

## Deployment 与能力

Deployment 是管理员确认后的本地模型资产。远端发现结果只是候选，不得自动创建或启用 Deployment；远端缺少类型/能力时 UI 必须要求管理员确认。手工模型同样必须声明类型、能力和限制。

稳定 model type：`chat`、`embedding`、`rerank`、`image`、`speech`、`transcription`。`vision` 是 chat 输入能力，不是 model type。有效能力始终为：

`Provider Adapter 已实现能力 ∩ Deployment 声明能力`

Resolver 再验证调用方 `CapabilityRequirements` 是否为有效能力的子集。非法 modality、feature 或负数 limit 拒绝保存。

## Alias 与解析

Alias 是租户内唯一的一对一映射，同一个 Deployment 可被多个 Alias 引用。Alias 在模型目录内联管理，不注册独立菜单。`Resolve(ctx, alias, requirements)` 依次检查租户、Alias、Deployment 状态、Provider 状态、model type、能力、Provider Type 与 credential reference，任一失败都不降级、不跨租户查找。

未来 fallback、权重和容灾应新增目标/策略模型，禁止把逗号列表或路由 JSON 塞进 Alias。

## 示例

```go
resolved, err := app.ModelResolver.Resolve(ctx, "general-chat", aimodel.CapabilityRequirements{
    ModelType: aimodel.ModelTypeChat,
    Features: []aimodel.Feature{aimodel.FeatureStreaming, aimodel.FeatureToolCalling},
})
```

下一 Issue 才能基于 `ResolvedModel` 实现统一模型请求和流式事件协议。
