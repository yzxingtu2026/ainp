# 强类型请求验证

- JSON Action 使用独立强类型 DTO，并经 `action.JSON[T]` 固定执行 Decode → Normalize → `validation.Struct` → `Validate(ctx)` → Service。
- Create/Update/Reset 等场景禁止字符串 scene、超大通用 DTO 和直接复用数据库 Model。
- 标签只表达纯输入结构规则；验证器禁止查库、鉴权、判断 tenant scope、资源存在性或状态机。
- 验证失败必须转换为 `CodeValidation` + `common.validation_failed` + `AppError.Fields`；字段使用 JSON 路径，规则使用稳定 `validation.*` 多语言键。
- 公共规则只在 `framework/validation` 初始化时注册；业务模块禁止修改全局 Validator 或依赖 go-playground 错误类型。

详规：`docs/development/validation.md`
