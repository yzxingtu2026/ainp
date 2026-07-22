# 全局 AI 助手

全局 AI 助手是应用壳级 Conversation 入口。Header 图标打开右侧面板，Sider 底部入口仍进入完整 `/ai/workspace`。两者复用同一服务端 Conversation/Run 协议、同一 controller 和同一 Conversation Surface。

## 所有权

- `useAiConversationController()` 是应用生命周期单例，唯一负责 Conversation、Message、SSE、Run cursor、轮询、确认、附件、幂等和 Page Context 发送时捕获。
- `AiConversationSurface` 统一渲染消息、流式/thinking、工具活动/确认、错误、空状态和 Composer，只提供 workspace/sidebar 密度变体。
- `ai-assistant` Pinia Store 只管理面板 visible 与 conversation/history/run 二级视图，不复制协议状态。
- Workspace、全局侧边栏只负责布局。禁止在任一容器新增第二套 send/watch/reconnect/message merge。

## 应用壳与入口

Header launcher 在所有 layout mode 中显示，但仍以动态路由中 `shellPlacement=global_ai_launcher` 为权限事实；点击只开关面板，不修改当前业务路由。Sider launcher 始终跳转 Workspace。

桌面面板宽度限制为 400px 至 `min(640px, 48vw)`，固定在应用壳右侧；窄屏为全屏分层页。面板没有 Page Context 区域，不展示 resource/capability/tag/Tool ID。Escape、关闭和展开后恢复触发入口焦点，reduced-motion 禁用转场。

首页只提供进入 Workspace 的明确命令，不承载 Prompt 输入、Conversation 或待发送消息。点击“开始工作”只执行可信路由跳转；首页不得创建第二个 controller，不得通过 URL、Hash 或浏览器历史传递对话内容。

首页模块宫格只投影 `routeStore.menus` 中具有可见子菜单的一级目录，目标取稳定顺序下首个授权叶子路由。标题、图标和路径均继承实时 Menu Registry 投影；权限、身份或菜单变化后不得使用平行缓存或硬编码模块列表。

“继续工作”只聚合当前用户待确认的 AI 工具调用与 Tab Store 中仍打开的授权页面。页面入口每次渲染均重新匹配当前菜单投影；权限撤销或菜单下线后立即消失。框架没有通用待办协议时禁止使用示例待办、伪造截止时间或业务动态占位。

首页专属机器人只表达页面交互状态：首次进入打招呼，默认坐下，长时间无操作冥想，聚焦“开始工作”时思考，点击后短暂跳动并导航。动画使用固定画布，后台标签页暂停空闲计时，`prefers-reduced-motion` 使用静态姿态；该组件不得替换 Workspace 的 `AiMascot` 状态协议。

## 生命周期与恢复

关闭面板或切换路由只卸载视图，controller、草稿、Conversation 和 Run 保留。应用只在 sessionStorage 保存当前 Conversation ID；刷新后重新读取服务端 Message、最新 Run 和 cursor，不持久化草稿、消息、Prompt 或 Page Context。

登出、认证失效或 AI launcher 权限撤销调用 `resetStore()`：abort SSE、停止轮询并清空 Conversation、草稿、附件、Run 和 session Conversation ID，禁止跨身份泄漏。

## 个性化启动快照

首页、Workspace 与侧边栏读取同一用户、`workspace.default` Slot 的物化快照。读取立即返回 `fresh`、`stale`、`missing` 或 `disabled`：stale 返回旧值并经 Outbox 去重刷新，missing/disabled 返回确定性 fallback；请求关键路径不得等待模型。

生成只使用统一 Model Runtime 的非流式严格结构化输出，不创建 Conversation、Message、Agent Run 或 Tool Invocation。用户输入、点击推荐或绑定 Conversation 后，本次 Surface 冻结欢迎区；身份变化必须清除前端共享快照。

全局快照输入只使用预算化可信投影：Runtime Context 提供称呼、locale、时区和当前本地时间；Agent Slot Resolver 提供当前发布版本；Memory Current Profile 只保留受限字符串字段；近期工作最多读取 5 个已完成 Run 对应的会话标题、页面类型和完成时间。另可读取最近一个 Workspace 会话结尾最多 4 条 committed user/assistant 纯文本，总预算 1200 字符；该片段必须标记 untrusted，禁止投影附件、工具参数、系统消息、完整历史、Page Context 正文或内部资源标识。

完成 Run、Memory Current Profile 重建以及 `workspace.default` Slot 绑定/个人偏好变化后，将当前用户快照标记为 stale 并通过 Outbox 延迟合并刷新。同一当前 generation 的连续事件使用相同下一 revision 幂等键，避免 Run 结束和画像重建连续调用模型。

时间段问候属于读取时确定性派生值，不属于模型快照。模型只保存稳定 greeting body；读取接口按快照 locale、用户时区和当前服务器 UTC 时间动态组合早上/中午/下午/晚上问候，并兼容清理旧快照中的时段前缀。禁止依赖快照生成时间判断当前时段。

## 二级视图

History 复用 `ConversationPanel`，支持新建、选择、重命名、归档与恢复。Run 详情在面板内部展示安全状态和 Step 时间线，不嵌套 Drawer。展开 Workspace 复用当前 controller，保留 Conversation、草稿和可恢复 Run。

## 测试

- 架构门禁验证单 controller/Surface、入口行为、身份清理、禁止 Context 暴露和仅持久化 Conversation ID。
- typecheck/build 验证所有布局共享组件编译。
- 手工联调覆盖所有 layout mode、业务表单/滚动保持、桌面/移动、Page Context 发送、SSE/停止/重连、历史/Run、刷新恢复、权限撤销与登出。
