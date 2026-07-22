# 全局 AI 助手

- Header 在所有 layout mode 提供受动态路由权限控制的侧边栏入口；Sider 入口只进入完整 Workspace，禁止混用行为。
- Conversation/Message/SSE/Run/确认/附件/幂等/Page Context 只由应用级单例 controller 管理；Workspace 与 Sidebar 必须复用同一 Conversation Surface，不得复制协议或合并逻辑。
- 面板挂载于应用壳，关闭/跨路由不清 controller；展开 Workspace 保留 Conversation、草稿和 Run。刷新只持久化 Conversation ID 并从服务端恢复事实。
- 登出、认证失效、身份/realm 变化或入口撤权必须 abort 流并清空草稿、附件、Conversation、Run 和恢复引用。
- Sidebar 仅含顶部栏、消息/空状态、Composer 与 history/run 二级视图；禁止 Page Context 条、内部 resource/capability/tag/Tool ID 和嵌套 Drawer。
- 首页只提供欢迎、开始工作、继续工作与授权功能模块，不放置 Prompt 输入框或第二套会话 controller；开始工作统一进入 Workspace。
- 首页“继续工作”只能投影真实的待确认 AI 工具调用与当前仍有权限的已打开页面；框架期无通用待办模型时必须展示空状态，禁止伪造待办或项目数据。
- 首页机器人动作必须表达明确交互状态（欢迎、静坐、思考、冥想、跳动），保持固定画布避免漂移，并为 `prefers-reduced-motion` 提供静态图。
- 三个入口统一读取可重建 personalization snapshot；读取不得同步等待模型，disabled/missing 立即 fallback，输入、推荐点击或绑定 Conversation 后冻结本次欢迎区。
- 快照不得写入 Conversation/Memory；生成仅走 Model Runtime，不得创建 Agent Run、调用工具或回退未配置模型。
- 个性化输入只允许预算化 Runtime Context、Agent Slot Version、Memory Current Profile 和近期 Run 摘要；禁止完整对话、原始记忆与页面正文。画像重建完成或 Workspace Slot 变化后再失效刷新。
- 桌面宽度、移动全屏、长内容 overflow、焦点返回、Escape、ARIA、Tooltip 与 reduced-motion 必须验证。

详规：`docs/development/global-ai-assistant.md`
