# Navigation Catalog 与词法搜索

Navigation Catalog 是从 Menu Registry 实时派生的授权搜索视图。Menu Registry 继续作为页面 ID、层级、路径、图标、realm 与 permission 的唯一事实源；禁止维护第二份路由清单、数据库菜单镜像或由调用方提供可点击路径。

## 注册元数据

页面用途、关键词、常见意图和帮助引用通过 `menu.Menu.Navigation` 附着在原菜单定义上。普通、无参数的 `menu` 节点默认可搜索；`directory`、`button`、空路径和参数化路径永不进入目录。`HideInMenu` 默认不可搜索，只有显式设置 `Searchable: true` 且路径稳定时才允许进入。

元数据同时支持直接文案和 i18n key。注册期校验描述长度、数组数量、单项长度、i18n key 格式和帮助引用长度。`Searchable` 使用可空布尔值，以区分普通菜单的默认行为和显式关闭搜索。

## 搜索与授权

`menu.SearchService` 每次查询均调用 `Registry.TreeForUser`，从当前 `AuthContext` 使用 realm、permission 和 language 构建目录；不缓存用户授权投影。因此权限撤销、realm 切换、模块下线或菜单注册变化会在下一次查询立即生效。

词法搜索覆盖本地化标题、完整面包屑、描述、关键词和 use case，执行大小写、标点和空白归一化，并按标题、关键词、面包屑、use case、描述的可信权重稳定排序。返回路径只能取自当前 Registry 菜单项。

受认证 API 为 `GET /api/v1/navigation/search?query=...&limit=...`。全局上限为 20；未来模型侧调用必须通过同一 Service 并设置 `MaxLimit: menu.MaxToolSearchLimit`（5），不得复制过滤或排序逻辑。空查询、超过 200 字符的查询及非法 limit 返回验证错误。

结构化日志仅记录查询字符数、结果数、匹配类型计数和耗时，不记录原始查询、菜单正文、业务数据或权限清单。

## 阶段边界

第一阶段只使用内存派生目录与确定性词法排序，不引入数据库镜像、Embedding、模型调用、AI Tool、导航卡片或前端全局搜索改造。后续阶段必须继续复用本服务的授权投影，并保持 Navigation Catalog 与 Action Tool Catalog 独立。
