# 全局 AI 助手

- Header 在所有 layout mode 提供 authenticated 全局 AI 入口；默认 Slot 解析、本人 Conversation/Message/Run、工具确认及对话所需“我的附件”链路不得要求额外角色权限。Sider 入口只进入完整 Workspace，禁止混用行为。
- authenticated 只适用于对话基础设施；Agent/Slot/模型/知识库管理仍需角色权限，业务工具必须按当前用户实时 Action 权限过滤和执行，禁止借 AI 越权。
- Conversation/Message/SSE/Run/确认/附件/幂等/Page Context 只由应用级单例 controller 管理；Workspace 与 Sidebar 必须复用同一 Conversation Surface，不得复制协议或合并逻辑。
- Composer 图片粘贴只接受受支持的图片 MIME，经统一上传队列加入当前草稿但不自动发送；上传中禁止发送，切换会话取消未完成上传，普通文本粘贴不得被拦截，Workspace 与 Sidebar 共用实现。
- 用户消息优先展示认证态当前用户头像 attachment ID，经统一附件下载协议加载；无头像或失败时降级账户图标，禁止保存对象 URL 或在容器层重复实现。
- 面板挂载于应用壳，关闭/跨路由不清 controller；展开 Workspace 保留 Conversation、草稿和 Run。刷新只持久化 Conversation ID 并从服务端恢复事实。
- 登出、认证失效、身份/realm 变化或入口撤权必须 abort 流并清空草稿、附件、Conversation、Run 和恢复引用。
- Sidebar 仅含顶部栏、消息/空状态、Composer 与 history/run 二级视图；禁止 Page Context 条、内部 resource/capability/tag/Tool ID 和嵌套 Drawer。
- Sidebar 层级只高于普通应用壳，必须低于统一 Modal/Drawer/Dialog 浮层；附件选择器等模态交互应覆盖整个侧栏，禁止按业务弹窗堆叠更高 z-index。
- 首页只提供欢迎、开始工作、继续工作与授权功能模块，不放置 Prompt 输入框或第二套会话 controller；开始工作统一进入 Workspace。
- 首页“继续工作”只能投影真实的待确认 AI 工具调用与当前仍有权限的已打开页面；框架期无通用待办模型时必须展示空状态，禁止伪造待办或项目数据。
- 首页机器人动作必须表达明确交互状态（欢迎、静坐、思考、冥想、跳动），保持固定画布避免漂移，并为 `prefers-reduced-motion` 提供静态图。
- 三个入口统一读取可重建 personalization snapshot；读取不得同步等待模型，disabled/missing 立即 fallback，输入、推荐点击或绑定 Conversation 后冻结本次欢迎区。
- 快照不得写入 Conversation/Memory；生成仅走 Model Runtime，不得创建 Agent Run、调用工具或回退未配置模型。
- 个性化输入只允许预算化 Runtime Context、Agent Slot Version、Memory Current Profile、近期 Run 摘要及最近 Workspace 会话结尾最多 4 条纯文本；会话片段必须标记 untrusted，禁止完整历史、工具参数、原始记忆与页面正文。
- 时段问候必须由服务端在每次读取时按用户时区动态计算；模型快照只保存稳定 greeting body，禁止固化“早上好/下午好/晚上好”。
- 桌面宽度、移动全屏、长内容 overflow、焦点返回、Escape、ARIA、Tooltip 与 reduced-motion 必须验证。

详规：`docs/development/global-ai-assistant.md`
