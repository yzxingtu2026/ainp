# 列表查询、分页与筛选规范

Agent 只读要点：`.agent/rules/list-query.md`。

## 固定协议

请求固定使用 `page` / `page_size`，响应固定使用 `items` / `total` / `page` / `page_size`。默认 `page=1`、`page_size=20`，最大 `page_size=100`，只在 `framework/pagination` 维护。禁止新增 `records`、`rows`、`list`、`current`、`size` 等平行协议。

## 后端职责

- 领域 List DTO 嵌入 `pagination.Request`，通过 `action.JSON[T]` 执行 Decode、Normalize 和验证。
- Service/Repository 只接收强类型查询；空字符串表示不启用可选筛选，模块可明确额外默认语义。
- 枚举筛选必须校验。排序只有 DTO 明确开放时才能使用，并映射到后端白名单 SQL 片段。
- 禁止任意字段、任意操作符或原始 SQL Filter DSL；tenant scope 由 TenantDB/GuardedDatabase 与 Callback 强制，列表 DTO 不接收 tenant。
- 返回 `pagination.Result[T]`；分页类型不依赖 Gin，也不负责 HTTP 输出。

## 前端职责

- 公共表格 Hook 内部可使用 Naive UI 的 `page/pageSize`，通过 `toListPageQuery` 转换为 API 字段。
- `defaultTransform` 统一消费 `items/total/page/page_size`。
- 筛选提交、重置及页大小变化后回到第一页；请求失败时结束 loading，由统一请求层展示错误。
- `defineTableFilters<Query>` 只描述搜索框、CodeSet 下拉和日期范围等控件及序列化，不承载数据库字段、权限或 SQL 操作符。

## 当前迁移范围

用户、角色、附件、任务运行、Outbox、通知投递和我的通知已统一使用本协议。非分页的简单选项列表不强制包装分页结果。

## 防回退

后端测试扫描业务模块，阻止重新引入 `json:"current"` 或 `"records":` 分页协议；租户数据架构门禁另行阻止裸 DB、手写 tenant 条件和未登记 Raw。新增列表还需覆盖默认值、上限、枚举筛选、空筛选和跨租户不可见。
