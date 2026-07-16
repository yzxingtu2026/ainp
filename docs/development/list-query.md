# 列表查询接入约束

统一分页、筛选与排序协议由 Issue #58 落地。本文件当前只固定其与请求验证器的依赖边界：列表 Request DTO 必须通过 `action.JSON[T]` 解码，并复用 `framework/validation` 的 Normalize、结构校验和 Validate 生命周期；不得在模块内另建 JSON 解析或字段错误体系。

具体字段、默认值、响应形态与前端表格适配应在 #58 中一次性定义并补充本文档。
