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

### 输入存在语义

- JSON tag 只定义字段名和序列化行为，`omitempty` 不决定输入是否必填。
- 暴露为 AI Tool 的 Action 输入必须使用 `action.InputSchemaFor[Request]()`；普通响应 Schema 继续使用 `action.SchemaFor[Response]()`。
- `InputSchemaFor` 要求每个输入字段精确声明且只能声明一种存在语义：`validate:"required"` 或 `validate:"omitempty,..."`。缺失、重复声明或模糊声明会在 Action 注册时 fail closed。
- required 只识别字段级的精确 `required` token；`required_if`、`required_without` 等条件规则不得误判为无条件必填。
- `dive` 后的规则约束集合元素，例如 `omitempty,max=20,dive,required` 中字段本身可选，元素必填；如集合字段也必须出现，应写为 `required,max=20,dive,required`。
- JSON Schema 的 required 列表只来自字段级 `validate:"required"`，不得根据 Go 非指针类型或 JSON `omitempty` 猜测。

更新 DTO 必须先确定是完整替换还是局部更新：

- 完整替换字段可使用普通类型；缺失字段按契约视为零值，但必须明确记录为 required 或 optional。
- 局部更新中需要区分“未提交、显式清空、设置新值”的字段使用指针或统一 Optional 类型。Handler 只能更新明确提交的字段。
- `0`、`false`、空数组等合法零值如果同时必须出现，使用指针表达字段存在性，再在 Handler 中解引用；禁止用普通数值/布尔字段配合验证器 `required`，避免合法零值被当作缺失。
- HTTP、Agent Tool 和其他 Action 入口必须复用同一 DTO、Schema 和 Handler，不得维护 Agent 专用输入结构或在 Tool Runtime 中修正参数。

公共规则由 `framework/validation` 只读注册：`required`、`min`、`max`、`len`、`oneof`、`email`、`uuid`、`url`、`locale`、`timezone` 以及数组 `dive`。业务模块不得运行时修改全局 Validator。

## 字段错误

验证器统一返回 `CodeValidation`、`common.validation_failed` 和全部 `errs.FieldError`。`field` 使用 JSON 路径，例如 `role_ids[0]`、`profile.email`；`message_key` 使用稳定的 `validation.*` 键，禁止返回 Go 字段名或验证库原始错误文本。

非法 JSON 和类型错误使用 `validation.invalid_json`；未知字段使用 `validation.unknown_field`。

## 与列表协议的关系

列表分页和筛选 Request DTO 嵌入 `pagination.Request`，并复用 `action.JSON[T]` 与本验证器，不得建立第二套解析或验证逻辑。完整协议见 `list-query.md`。

## 测试

验证规则、嵌套路径、生命周期和 Action 执行链测试统一放在 `platform/tests/`。领域规则继续由对应 Service 集成测试覆盖。

Mutation Action 至少覆盖最小合法载荷、字段未提交保持原值、显式清空、合法零值、HTTP/Tool 等价行为。AI Action 架构测试必须确认输入来自显式契约，内部契约标记不得投影到模型 JSON Schema。
