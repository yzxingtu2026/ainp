# 模型提示词管理

- 模型长指令与运行时模板统一放在 `platform/prompts/` 并通过 `embed.FS` 随版本发布，Go 代码通过 `framework/prompt` 引用，禁止重新硬编码。
- 相对 Markdown 路径即 prompt key；新增必需 key 必须同步加入 `host` 启动校验。
- 动态内容使用 `prompt.Render` 的结构化模板数据注入，并保留可信/不可信内容边界。
- 正式内嵌源不可 reload；仅显式受信开发源可热重载，完整校验后原子替换，失败保留上一份有效快照。
- Contract Base Prompt Version 与 Agent Contract Version 分别表示内容版本和能力兼容版本；注册代码必须显式绑定并校验预期 Prompt Version，只有 Contract 兼容边界变化才同步升级默认 Slot。
- 详细规范见 `docs/development/prompt-management.md`。
