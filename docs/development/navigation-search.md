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

阶段一、二只使用内存派生目录与确定性词法排序，不引入数据库镜像或 Embedding。第三阶段的语义索引仍必须复用本服务的授权投影，并保持 Navigation Catalog 与 Action Tool Catalog 独立。
