# 统一列表查询

- 分页请求固定 `page/page_size`，响应固定 `items/total/page/page_size`；默认值和最大值只由 `framework/pagination` 维护。
- List Action 使用 `action.JSON[T]` 和嵌入 `pagination.Request` 的领域 DTO；禁止手写 JSON Decode、分页默认值或分页 map。
- 筛选字段必须强类型且枚举合法；排序只接受 DTO 明确开放的后端白名单映射。
- 禁止任意字段/操作符/SQL Filter DSL；tenant scope、权限和资源边界不得弱化。
- 前端表格复用 `hooks/common/table.ts`；筛选提交、重置及页大小变化回到第一页。
- 禁止新增 `records/rows/list/current/size` 平行分页协议。

详规：`docs/development/list-query.md`
