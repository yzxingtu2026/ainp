# 模型提示词管理

## 适用范围

发送给模型的长指令、基础系统指令和运行时上下文模板统一存放在 `platform/prompts/`，业务 Go 代码只通过 `framework/prompt` 按 key 引用。

## 文件与 key

- 文件使用 UTF-8 Markdown，可带 `description`、`version` 等 YAML frontmatter。
- 相对路径去掉 `.md` 后即为 key，例如 `ai/memory-extraction.md` 对应 `ai/memory-extraction`。
- 正文允许使用 Go `text/template` 变量；动态数据必须作为结构化模板参数传入，禁止在业务代码中重新拼接长提示词。

## 加载与失败边界

- 启动阶段调用 `prompt.LoadDir("prompts")`，必需 key 缺失、正文为空或模板语法错误时必须阻止启动。
- `prompt.Reload()` 会完整读取并校验目录，成功后才原子替换内存快照；失败时继续使用上一份有效快照。
- 固定提示词使用 `prompt.MustGet`，模板使用 `prompt.Render`。仅在“提示词确实可选”时使用 `prompt.Get`。
- 修改 Markdown 后需重启进程或由受控管理入口调用 `prompt.Reload()`；业务模块不得自行维护缓存。

## 安全要求

- 用户输入、工具输出和长期记忆等不可信内容必须保持明确边界，并只通过模板数据注入。
- 提取提示词不得削弱原有鉴权、租户隔离、工具确认、来源标识或结构化输出约束。
- 新增必需提示词时，同步加入 `boot.LoadPrompts` 的启动校验清单并添加测试。
