# 模型提示词管理

## 适用范围

发送给模型的长指令、基础系统指令和运行时上下文模板统一存放在 `platform/prompts/`，由该目录内的 `embed.FS` 随 `platform/vX.Y.Z` 编译发布；业务 Go 代码只通过 `framework/prompt` 按 key 引用。

## 文件与 key

- 文件使用 UTF-8 Markdown，可带 `description`、`version` 等 YAML frontmatter；被 Agent Contract 使用的基础指令必须声明语义化 `version`。
- 相对路径去掉 `.md` 后即为 key，例如 `ai/memory-extraction.md` 对应 `ai/memory-extraction`。
- 正文允许使用 Go `text/template` 变量；动态数据必须作为结构化模板参数传入，禁止在业务代码中重新拼接长提示词。

## 加载与失败边界

- 正式 Host 启动阶段通过 `prompt.LoadSources` 加载 owner 为 `ainp` 的内嵌基线及下游 Module 资源，和当前工作目录无关；必需 key 缺失、正文为空、模板语法错误、重复 owner 或 key 冲突必须阻止启动。
- 下游 Prompt key 必须使用 `{module-id}/...` 前缀，默认不得覆盖 AINP 或其他 Module 的 Prompt。
- 正式内嵌源不可 reload。仅 API/Worker 显式传入 `-dev-runtime-assets` 时，才以受信目录替换 `ainp` 源并允许 `prompt.Reload()`；完整校验成功后原子替换，失败继续使用上一份有效快照。
- 固定提示词使用 `prompt.MustGet`，模板使用 `prompt.Render`。仅在“提示词确实可选”时使用 `prompt.Get`。
- 修改 Markdown 后需重启进程或由受控管理入口调用 `prompt.Reload()`；业务模块不得自行维护缓存。

## 安全要求

- 用户输入、工具输出和长期记忆等不可信内容必须保持明确边界，并只通过模板数据注入。
- 提取提示词不得削弱原有鉴权、租户隔离、工具确认、来源标识或结构化输出约束。
- 新增必需提示词时，同步加入 `host` 的启动校验清单并添加测试。
- `prompt.Manifest()` 和 Host Runtime Assets 清单只暴露 owner、key、类型、版本和 SHA-256，不得输出正文。

## Agent Contract 版本治理

- Contract 托管的 BaseInstructions 属于运行时安全行为。Prompt frontmatter `version` 是基础指令内容版本，Contract Version 是能力与兼容性协议版本，两者不得靠相等关系隐式绑定；Contract 注册代码必须显式声明并校验预期 Prompt Version，不一致则阻止启动。
- 只修改角色文案、稳定工作方式等基础指令内容时升级 Prompt Version，并更新 Contract 注册处的显式映射；只有能力、托管字段、必需工具或兼容边界变化时才升级 Contract Version，并同步默认 Slot 引用和存量 Definition 升级方案。
- Setup 创建当前 Contract Version；显式 builtin reconcile 只在 Contract Version 确实升级时更新 Definition 的托管字段，不复制 Base 到 `tenant_instructions`，不覆盖 Builder 草稿，也不修改历史 AgentVersion。
- 旧 Definition 在 reconcile 前引用的 Contract 无法解析时必须 fail-closed；不得退化为只使用租户补充指令。
- 若已发布版本曾错误写入非正式 Contract Version，可注册范围明确的临时兼容别名，确保历史 Conversation 和 `UpgradeCompatible` Slot 可运行；兼容别名必须显式测试，未知 key/version 仍 fail-closed，并由 builtin reconcile 将 Definition 规范化回正式版本。
