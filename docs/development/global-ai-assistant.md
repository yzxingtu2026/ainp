# 全局 AI 助手

全局 AI 助手是应用壳级 Conversation 入口。Header 图标打开右侧面板，Sider 底部入口仍进入完整 `/ai/workspace`。两者复用同一服务端 Conversation/Run 协议、同一 controller 和同一 Conversation Surface。

## 所有权

- `useAiConversationController()` 是应用生命周期单例，唯一负责 Conversation、Message、SSE、Run cursor、轮询、确认、附件、幂等和 Page Context 发送时捕获。
- `AiConversationSurface` 统一渲染消息、流式/thinking、工具活动/确认、错误、空状态和 Composer，只提供 workspace/sidebar 密度变体。
- 共享 Composer 支持图片和 PDF/XLSX/DOCX/TXT/MD 文档：从“我的附件”选择或在 Picker 中直接上传，MIME 与扩展名必须严格匹配，单项最多 10 MiB，图片与文档合计最多 4 个。图片粘贴仍只在检测到 PNG/JPEG/GIF/WebP 时阻止默认粘贴并走统一上传队列；不增加 Office 剪贴板特殊逻辑。上传中禁止发送，切换会话取消未完成队列，普通文本粘贴保持浏览器默认行为；Workspace 与 Sidebar 不得分别实现。
- 用户消息头像优先使用认证态中的当前用户头像 attachment ID，并通过统一附件 prepare/download 协议加载；未设置、无权访问或加载失败时降级为账户图标。Workspace 与全局侧边栏必须共享这一渲染逻辑。
- `ai-assistant` Pinia Store 只管理面板 visible 与 conversation/history/run 二级视图，不复制协议状态。
- Workspace、全局侧边栏只负责布局。禁止在任一容器新增第二套 send/watch/reconnect/message merge。

## 应用壳与入口

Header launcher 在所有 layout mode 中显示，并以动态路由中的 `shellPlacement=global_ai_launcher` 为入口事实；该路由与默认 Slot 解析、本人会话/消息/Run、工具运行时确认及“我的附件”对话上传链路均为 authenticated 自助能力，不要求角色额外分配 AI 权限。点击只开关面板，不修改当前业务路由。Sider launcher 始终跳转 Workspace。

已登录可用只免除对话基础设施自身的角色 permission，不改变任何资源边界：会话、消息、Run、附件仍强制当前 tenant/user/owner；Agent 管理、Slot 管理、模型、知识库管理仍需角色权限；Agent 实际调用的业务 Action 必须在 Tool Search/Load/Execute 和执行前按当前用户实时权限过滤，禁止借 AI 助手越权。

桌面面板宽度限制为 400px 至 `min(640px, 48vw)`，固定在应用壳右侧；窄屏为全屏分层页。面板没有 Page Context 区域，不展示 resource/capability/tag/Tool ID。Escape、关闭和展开后恢复触发入口焦点，reduced-motion 禁用转场。

侧边栏必须高于应用壳普通内容，但低于 Naive UI 的 Modal、Drawer、Dialog 等统一浮层；从 Composer 打开的附件选择器及侧边栏内其他模态交互必须完整覆盖侧边栏和遮罩整个页面。禁止通过给单个业务弹窗持续叠加更高 z-index 修补层级。

首页只提供进入 Workspace 的明确命令，不承载 Prompt 输入、Conversation 或待发送消息。点击“开始工作”只执行可信路由跳转；首页不得创建第二个 controller，不得通过 URL、Hash 或浏览器历史传递对话内容。

首页模块宫格只投影 `routeStore.menus` 中具有可见子菜单的一级目录，目标取稳定顺序下首个授权叶子路由。标题、图标和路径均继承实时 Menu Registry 投影；权限、身份或菜单变化后不得使用平行缓存或硬编码模块列表。

“继续工作”只聚合当前用户待确认的 AI 工具调用与 Tab Store 中仍打开的授权页面。页面入口每次渲染均重新匹配当前菜单投影；权限撤销或菜单下线后立即消失。框架没有通用待办协议时禁止使用示例待办、伪造截止时间或业务动态占位。

首页专属机器人只表达页面交互状态：首次进入打招呼，默认坐下，长时间无操作冥想，聚焦“开始工作”时思考，点击后短暂跳动并导航。动画使用固定画布，后台标签页暂停空闲计时，`prefers-reduced-motion` 使用静态姿态；该组件不得替换 Workspace 的 `AiMascot` 状态协议。

## 生命周期与恢复

关闭面板或切换路由只卸载视图，controller、草稿、Conversation 和 Run 保留。应用只在 sessionStorage 保存当前 Conversation ID；刷新后重新读取服务端 Message、最新 Run 和 cursor，不持久化草稿、消息、Prompt 或 Page Context。

登出、认证失效或 AI launcher 权限撤销调用 `resetStore()`：abort SSE、停止轮询并清空 Conversation、草稿、附件、Run 和 session Conversation ID，禁止跨身份泄漏。

草稿与历史消息中，图片显示受控缩略图，文档显示类型图标和当前授权读取的安全文件名。附件详情或 revision 校验失败时降级为通用不可用状态，不显示 attachment ID、路径、object key 或下载 URL。

## 个性化启动快照

首页、Workspace 与侧边栏读取同一用户、`workspace.default` Slot 的物化快照。读取立即返回 `fresh`、`stale`、`missing` 或 `disabled`：stale 返回旧值并经 Outbox 去重刷新，missing/disabled 返回确定性 fallback；请求关键路径不得等待模型。

生成只使用统一 Model Runtime 的非流式严格结构化输出，不创建 Conversation、Message、Agent Run 或 Tool Invocation。用户输入、点击推荐或绑定 Conversation 后，本次 Surface 冻结欢迎区；身份变化必须清除前端共享快照。

全局快照输入只使用预算化可信投影：Runtime Context 提供称呼、locale、时区和当前本地时间；Agent Slot Resolver 提供当前发布版本；Memory Current Profile 只保留受限字符串字段；近期工作最多读取 5 个已完成 Run 对应的会话标题、页面类型和完成时间。另可读取最近一个 Workspace 会话结尾最多 4 条 committed user/assistant 纯文本，总预算 1200 字符；该片段必须标记 untrusted，禁止投影附件、工具参数、系统消息、完整历史、Page Context 正文或内部资源标识。

完成 Run、Memory Current Profile 重建以及 `workspace.default` Slot 绑定/个人偏好变化后，将当前用户快照标记为 stale 并通过 Outbox 延迟合并刷新。同一当前 generation 的连续事件使用相同下一 revision 幂等键，避免 Run 结束和画像重建连续调用模型。

时间段问候属于读取时确定性派生值，不属于模型快照。模型只保存稳定 greeting body；每次读取均重新构建当前 Trusted Runtime Context，按当前 locale、有效时区和可信 UTC 时刻动态组合问候，并兼容清理旧快照中的时段前缀。首页在接口返回前只显示无时间段的中性文案，不得使用浏览器本地时间；时区或 locale 变更后的下一次读取立即生效，无需重建快照。

## 二级视图

History 复用 `ConversationPanel`，支持新建、选择、重命名、归档与恢复。Run 详情在面板内部展示安全状态和 Step 时间线，不嵌套 Drawer。展开 Workspace 复用当前 controller，保留 Conversation、草稿和可恢复 Run。

## 测试

- 架构门禁验证单 controller/Surface、入口行为、身份清理、禁止 Context 暴露和仅持久化 Conversation ID。
- typecheck/build 验证所有布局共享组件编译。
- 手工联调覆盖所有 layout mode、业务表单/滚动保持、桌面/移动、Page Context 发送、SSE/停止/重连、历史/Run、刷新恢复、权限撤销与登出。
