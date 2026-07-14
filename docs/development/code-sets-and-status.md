# CodeSet、状态机与字符串常量

## 三类数据边界

- 固定 CodeSet：由代码注册，保存稳定 machine code、i18n key、semantic、Iconify 图标和排序，不提供写 API。
- 领域状态机：只声明允许的状态转换。权限、guard、事务和副作用仍由 Service/Action 执行。
- 租户业务字典：未来独立使用 `tenant_dictionary_types/items`，用于客户等级、来源渠道等可配置参考数据。不得承载系统状态、Action、permission、事件或 Job 类型，也不应塞入 Setting JSON。

## 所有权与命名

- Go 使用自定义 `string` 类型和 `const`；TypeScript 使用 `as const` 与推导联合类型。
- 框架 key 放在对应 `framework/*` 包；领域状态和 Action/Menu/i18n key 放在所属模块。禁止无边界的全局 `constants.go`。
- CodeSet key 使用小写点分，如 `common.activation_status`；item value 使用小写 snake_case，如 `awaiting_payment`。
- 后端协议是跨端事实源；前端只在 `constants/` 或 API 类型中集中镜像，不得在页面复制 options。

## 展示与业务规则

- API/数据库只保存 machine value。用户可见文本使用 i18n key，固定 CodeSet 的翻译不写数据库。
- Semantic 仅允许 `default/info/success/warning/danger/muted`；前端映射主题组件，业务代码不得根据颜色、label 或 icon 判断状态。
- 图标遵循 Iconify `collection:name`，默认 `mdi`。禁止 emoji、URL 和裸图标名。
- `CodeTag` 对未知值显示原始 machine value 并降低视觉权重；`CodeSelect` 按 order 排序并禁用 deprecated 项。

## 状态变化

- 普通资料 Update 不得隐式修改状态；生命周期变化使用明确 Action。
- `ActivationStatus` 只用于语义确实为启用/停用的模型。订单、Job、Outbox、通知投递、附件等即使值同名，也必须定义独立类型和 CodeSet。
- 终态不得有出边；Transition 引用的状态、Action 和 permission 在注册时校验。

## API 与缓存

- `GET /api/v1/codesets` 返回当前 realm 可见的全部 CodeSet；重复 `key` 查询参数可批量筛选。
- `GET /api/v1/codesets/:key` 返回指定 CodeSet；未知 key 返回稳定业务错误。
- 响应按 `Accept-Language` 本地化 label/description，并提供 Registry version 与 `ETag`。客户端语言变化后重新加载。

## 新增检查清单

1. 定义领域类型、常量、CodeSet 与全部语言 i18n。
2. 需要生命周期时定义显式 Action，再注册 StateMachine。
3. 数据库存储状态时同步版本化 CHECK 迁移。
4. 前端复用 `CodeTag`/`CodeSelect`，只提交 machine value。
5. 运行 `scripts/check-code-strings.sh`、后端测试、前端 typecheck/build。

允许直接字符串的范围仅为语言/框架语法、序列化 Tag、SQL/格式模板和专门验证非法输入的测试。例外必须在调用点说明原因。
