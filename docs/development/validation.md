# 请求验证规范

Agent 只读要点：`.agent/rules/validation.md`。本文定义 JSON Action 的强类型请求生命周期与验证边界。

## 固定生命周期

```text
JSON Decode → Normalize → validation.Struct → Validate(ctx) → Service
```

- `action.JSON[T]` 默认严格拒绝未知字段、多个 JSON 值和类型不匹配。
- 空 Body 按 `{}` 处理，由 DTO 的 `required` 规则产生字段错误。
- `Normalize` 仅做无歧义归一化，例如 `TrimSpace`；不得修复非法业务值。
- 结构标签只描述纯输入规则；跨字段纯规则可实现 `Validate(context.Context)`。
- Service 继续负责权限、租户、资源存在性、数据库唯一性、状态机和事务规则。

## DTO 与规则

每个 Action 使用独立请求 DTO，Create、Update、Reset 不得通过字符串 scene 共用超大结构体。禁止直接把 GORM Model 暴露为请求 DTO。

公共规则由 `framework/validation` 只读注册：`required`、`min`、`max`、`len`、`oneof`、`email`、`uuid`、`url`、`locale`、`timezone` 以及数组 `dive`。业务模块不得运行时修改全局 Validator。

## 字段错误

验证器统一返回 `CodeValidation`、`common.validation_failed` 和全部 `errs.FieldError`。`field` 使用 JSON 路径，例如 `role_ids[0]`、`profile.email`；`message_key` 使用稳定的 `validation.*` 键，禁止返回 Go 字段名或验证库原始错误文本。

非法 JSON 和类型错误使用 `validation.invalid_json`；未知字段使用 `validation.unknown_field`。

## 与列表协议的关系

列表分页和筛选 Request DTO 嵌入 `pagination.Request`，并复用 `action.JSON[T]` 与本验证器，不得建立第二套解析或验证逻辑。完整协议见 `list-query.md`。

## 测试

验证规则、嵌套路径、生命周期和 Action 执行链测试统一放在 `platform/tests/`。领域规则继续由对应 Service 集成测试覆盖。
