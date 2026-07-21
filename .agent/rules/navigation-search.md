# Navigation Search

- Menu Registry 是页面 ID、层级、路径、图标、realm 与 permission 的唯一事实源；Navigation Catalog 只能实时派生，禁止平行路由清单或调用方提供 path。
- Navigation 元数据必须附着 `menu.Menu` 并通过注册期预算/i18n key/稳定路径校验；只索引无参数 `menu`，directory/button/空路径/参数化路径永不返回。
- 普通菜单默认可搜索；`HideInMenu` 默认不可搜索，必须显式 `Searchable: true` 且能形成安全稳定路径；普通菜单可显式关闭。
- API、AI Tool 与其他入口必须复用 `menu.SearchService`，每次从当前 AuthContext 做 realm/permission/language 过滤；缓存和历史结果不得绕过重鉴权。
- 返回 path 只能来自当前 Registry 投影；日志禁止记录原始查询和目录正文，只记录长度、数量、匹配类型与耗时。
- Navigation Catalog 与 Action Tool Catalog 分离；词法阶段禁止提前引入数据库镜像、Embedding 或模型依赖。

详规：`docs/development/navigation-search.md`
