# Navigation Catalog 与词法搜索

Navigation Catalog 是从 Menu Registry 实时派生的授权搜索视图。Menu Registry 继续作为页面 ID、层级、路径、图标、realm 与 permission 的唯一事实源；禁止维护第二份路由清单、数据库菜单镜像或由调用方提供可点击路径。

## 注册元数据

页面用途、关键词、常见意图和帮助引用通过 `menu.Menu.Navigation` 附着在原菜单定义上。普通、无参数的 `menu` 节点默认可搜索；`directory`、`button`、空路径和参数化路径永不进入目录。`HideInMenu` 默认不可搜索，只有显式设置 `Searchable: true` 且路径稳定时才允许进入。

元数据同时支持直接文案和 i18n key。注册期校验描述长度、数组数量、单项长度、i18n key 格式和帮助引用长度。`Searchable` 使用可空布尔值，以区分普通菜单的默认行为和显式关闭搜索。

## 搜索与授权

`menu.SearchService` 每次查询均调用 `Registry.TreeForUser`，从当前 `AuthContext` 使用 realm、permission 和 language 构建目录；不缓存用户授权投影。因此权限撤销、realm 切换、模块下线或菜单注册变化会在下一次查询立即生效。

词法搜索覆盖本地化标题、完整面包屑、描述、关键词和 use case，执行大小写、标点和空白归一化，并按标题、关键词、面包屑、use case、描述的可信权重稳定排序。返回路径只能取自当前 Registry 菜单项。

受认证 API 为 `GET /api/v1/navigation/search?query=...&limit=...`。全局上限为 20；模型侧调用通过同一 Service 并设置 `MaxLimit: menu.MaxToolSearchLimit`（5），不得复制过滤或排序逻辑。空查询、超过 200 字符的查询及非法 limit 返回验证错误。

结构化日志仅记录查询字符数、结果数、匹配类型计数和耗时，不记录原始查询、菜单正文、业务数据或权限清单。

## 默认助手 Runtime

`builtin:navigation.search` 是显式注册的只读 Runtime Slot，使用 `BindingForbidden + LoadingDirect + DiscoveryDirect`。默认 `system.operations` contract 通过 `RequiredToolSelectors` 直接获得该能力；导航问题禁止先走 `tool.search -> tool.load`。Slot 的 query/limit 由模型提供，身份、realm、语言和权限只从可信 AuthContext 获取。

模型只可解释 Registry 返回的结果，不得生成、补全或修改 path。导航搜索不扩大 Action ToolScope，不自动 load/execute 页面对应 Action；失败或无结果时保持普通对话可用。

## 前端消费

全局搜索使用 300ms 防抖和单调请求序号；晚到的旧响应不得覆盖新查询。界面保留方向键、Enter、Esc、移动端交互，并展示 loading、empty、error、offline/degraded 状态。

动态路由为每个 Registry 菜单写入 `backendMenuId`。所有搜索结果和历史导航卡片在点击前必须校验当前 Vue Router 中的 `menu_id + path`；校验失败时禁用或提示不可用。跳转只调用 Vue Router，不刷新页面、不接受外部 URL，也不触发表单或业务 Action。

`NavigationResultCard` 只解析 canonical Tool Result 中 `builtin__navigation_search` 的结构化结果，不解析模型 Markdown URL。Sidebar 与 Workspace 均经共享 `ChatMessageList -> ToolActivityGroup` 使用同一组件；历史 Conversation 重放时仍按当前动态路由重新判断。

## 阶段边界

阶段一、二只使用内存派生目录与确定性词法排序，不引入数据库镜像或 Embedding。

## 独立语义索引

第三阶段使用 `navigation_search_documents`，不得复用 `ai_tool_search_documents`。索引源只来自 Menu Registry 元数据和已提交审核的 Page Help；表中不保存 path、权限清单、DOM、页面正文、用户数据或未提交表单。路径、层级与图标仍在每次返回时由当前 Menu Registry 投影。

索引与查询分别使用 `navigation_index`、`navigation_query` purpose，并记录 Alias、Deployment、维度、预处理、Schema Version、Catalog Revision 与内容哈希。一次 reconcile 将全部变化文档汇总进同一个 `ai.navigation_document.changed.v1` Outbox 事件，再由单个 `ai.navigation.embed` Job 通过一次批量 Embedding 请求构建向量；文档 ID + revision 仍逐项校验和幂等落库。禁止按文档创建独立 Job，避免菜单数量直接放大任务运行记录。下线文档标记 stale。Embedding 未配置、调用失败、维度或 provenance 不匹配、向量表不可用时，搜索必须保留词法结果并返回稳定降级原因。

`ai.navigation_search` 保存或重置后通过 `setting.ChangeRegistry` 立即执行当前租户 reconcile，不依赖后端重启。设置页调用受现有设置权限保护的状态与重建入口，展示 disabled/not_indexed/pending/ready/failed/degraded；pending 状态短轮询，手动重建强制提升 Catalog Revision 并重新进入同一 Outbox/Job 链路。

向量 SQL 必须先接收当前 AuthContext 经 Menu Registry 计算出的允许 `menu_id` 集合，并在同一查询中限制候选；禁止全局向量召回后再过滤。向量结果只提供候选分数，最终 DTO 和 path 仍由 `menu.SearchService` 生成。

## Page Help 规范

业务模块的操作帮助使用随代码提交的 Markdown，并通过 `embed.FS` 编译进 Go 二进制。frontmatter 只允许 `menu_id`、SemVer `version`、`locale`、`use_cases` 和 `permission_refs`；禁止声明 path。正文必须包含一级标题，单文件上限 32 KiB，同一菜单与 locale 不得重复。

`menu.HelpRegistry` 在启动注册期校验菜单存在、可搜索以及 permission ref 与菜单权限一致。帮助是 Git 审核后的事实源，不得从 DOM、组件实例或运行时页面抓取；AI 可以辅助起草，但生成内容必须经过代码评审后提交。

每个可搜索的生产 `menu` 必须声明非空 `module`，并在所属业务模块内提供默认 `zh-CN` 帮助文档，由该模块通过独立 `embed.FS` 注册。模块可调用 `ValidateModuleCoverage` 做局部快速校验；统一 boot 必须调用 `ValidateAllModulesCoverage`，从 Menu Registry 反推全部生产模块并执行全局门禁，新增模块无需额外接线。普通可见菜单默认属于覆盖范围；隐藏菜单只有显式 `Searchable: true` 时才要求文档。可选功能只加载当前实际注册菜单对应的文档，禁止留下引用未启用菜单的悬空文档。Demo 菜单不纳入生产模块覆盖门禁。

`builtin:navigation.help` 只接受 `navigation.search` 返回的稳定 `menu_id`、可选受信 use case 与 locale。执行时重新校验当前 realm、permission 和菜单状态，返回 Markdown、版本与来源引用。帮助只用于说明，不得自动点击、填写、提交或触发 Action。
