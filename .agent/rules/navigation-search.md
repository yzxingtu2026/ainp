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

详规：`docs/development/navigation-search.md`
