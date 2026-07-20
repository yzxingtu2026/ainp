# 模型提示词管理

- 模型长指令与运行时模板统一放在 `platform/prompts/`，Go 代码通过 `framework/prompt` 引用，禁止重新硬编码。
- 相对 Markdown 路径即 prompt key；新增必需 key 必须同步加入 `boot.LoadPrompts` 启动校验。
- 动态内容使用 `prompt.Render` 的结构化模板数据注入，并保留可信/不可信内容边界。
- 热重载必须完整校验后原子替换；失败时保留上一份有效快照。
- 详细规范见 `docs/development/prompt-management.md`。
