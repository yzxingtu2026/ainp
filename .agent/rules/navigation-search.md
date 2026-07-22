# Navigation Search

- Menu Registry 是页面 ID、层级、路径、图标、realm 与 permission 的唯一事实源；Navigation Catalog 只能实时派生，禁止平行路由清单或调用方提供 path。
- Navigation 元数据必须附着 `menu.Menu` 并通过注册期预算/i18n key/稳定路径校验；只索引无参数 `menu`，directory/button/空路径/参数化路径永不返回。
- 普通菜单默认可搜索；`HideInMenu` 默认不可搜索，必须显式 `Searchable: true` 且能形成安全稳定路径；普通菜单可显式关闭。
- API、AI Tool 与其他入口必须复用 `menu.SearchService`，每次从当前 AuthContext 做 realm/permission/language 过滤；缓存和历史结果不得绕过重鉴权。
- 返回 path 只能来自当前 Registry 投影；日志禁止记录原始查询和目录正文，只记录长度、数量、匹配类型与耗时。
- Navigation Catalog 与 Action Tool Catalog 分离；词法阶段禁止提前引入数据库镜像、Embedding 或模型依赖。
- 默认助手通过显式 `builtin:navigation.search` Runtime Slot 直接获得导航能力，禁止多跳发现；模型不得生成或改写 Registry path，导航不得扩大 Action ToolScope 或自动执行操作。
- 全局搜索必须有防抖、取消/序号竞态保护和完整状态；导航卡片只消费 canonical Tool Result，Sidebar/Workspace 共享组件。
- 点击搜索结果或历史卡片前必须用当前动态路由复核 `menu_id + path`，只允许 Vue Router 内部跳转；撤权、路由卸载或结果过期时禁用。
- 语义检索必须使用独立 `navigation_search_documents` 与 `navigation_index/navigation_query` purpose；先由 Registry 计算授权 menu_id 集合，再在向量 SQL 内限制候选，异常时无损降级到词法检索。
- Page Help 必须是模块内版本化 Markdown，经 `embed.FS` 打包和注册期校验；每个可搜索生产菜单必须提供默认 `zh-CN` 审核文档，模块注册后执行覆盖门禁。frontmatter 禁止 path，内容禁止 DOM/运行时抓取。`navigation.help` 只接受可信 menu_id/use case，读取时重新鉴权且永不执行页面动作。
- `ai.navigation_search` 保存/重置必须通过 Setting Change Hook 立即 reconcile；状态与手动重建复用设置权限，前端明确展示 disabled/not_indexed/pending/ready/failed/degraded。
- 一次 Navigation reconcile 只允许产生一个批量 Outbox 事件和一个批量 Embedding Job；禁止按菜单/帮助切片创建独立任务，批内仍按 document ID + revision 幂等落库。

详规：`docs/development/navigation-search.md`
