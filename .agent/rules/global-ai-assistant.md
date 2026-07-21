# 全局 AI 助手

- Header 在所有 layout mode 提供受动态路由权限控制的侧边栏入口；Sider 入口只进入完整 Workspace，禁止混用行为。
- Conversation/Message/SSE/Run/确认/附件/幂等/Page Context 只由应用级单例 controller 管理；Workspace 与 Sidebar 必须复用同一 Conversation Surface，不得复制协议或合并逻辑。
- 面板挂载于应用壳，关闭/跨路由不清 controller；展开 Workspace 保留 Conversation、草稿和 Run。刷新只持久化 Conversation ID 并从服务端恢复事实。
- 登出、认证失效、身份/realm 变化或入口撤权必须 abort 流并清空草稿、附件、Conversation、Run 和恢复引用。
- Sidebar 仅含顶部栏、消息/空状态、Composer 与 history/run 二级视图；禁止 Page Context 条、内部 resource/capability/tag/Tool ID 和嵌套 Drawer。
- 桌面宽度、移动全屏、长内容 overflow、焦点返回、Escape、ARIA、Tooltip 与 reduced-motion 必须验证。

详规：`docs/development/global-ai-assistant.md`
