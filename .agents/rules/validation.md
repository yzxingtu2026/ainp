# 强类型请求验证

- JSON Action 使用独立强类型 DTO，并经 `action.JSON[T]` 固定执行 Decode → Normalize → `validation.Struct` → `Validate(ctx)` → Service。
- Create/Update/Reset 等场景禁止字符串 scene、超大通用 DTO 和直接复用数据库 Model。
- 标签只表达纯输入结构规则；验证器禁止查库、鉴权、判断 tenant scope、资源存在性或状态机。
- Action 输入 required 只由字段级精确 `validate:"required"` 决定；JSON `omitempty`、Go 非指针类型和 `dive` 后的元素规则不得参与字段 required 推导。
- AI 暴露 Action 输入必须使用 `action.InputSchemaFor[Request]()`；每个字段显式且互斥声明 `required` 或 `omitempty`，歧义在注册时 fail closed。响应 Schema 使用 `action.SchemaFor[Response]()`。
- 局部更新字段需要区分未传、清空、赋值时使用指针/Optional，Handler 只更新明确提交字段；合法零值且必须出现的字段用指针表达存在性。
- HTTP、Agent Tool 等入口复用同一 DTO、Schema 和 Handler，禁止 Agent 专用 DTO 或 Tool Runtime 参数修正。
- 验证失败必须转换为 `CodeValidation` + `common.validation_failed` + `AppError.Fields`；字段使用 JSON 路径，规则使用稳定 `validation.*` 多语言键。
- 跨渠道业务纠错仅使用受控 `errs.SafeDetails`，不得透传任意 map。动态引用 details 只允许字段、稳定 reason、调用者提交的受限非法值/计数/截断标记和 refresh Action；禁止返回合法目录、无权资源、数据库错误或堆栈。
- 公共规则只在 `framework/validation` 初始化时注册；业务模块禁止修改全局 Validator 或依赖 go-playground 错误类型。
- 列表 DTO 嵌入 `pagination.Request` 并遵循 `list-query`，不得另建分页解析或校验逻辑。

详规：`docs/development/validation.md`
