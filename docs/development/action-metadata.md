# Action / Capability / Metadata / 菜单注册规范

Agent 只读要点：`.agent/rules/action-metadata.md`。本文档为 Phase 1 字段设计与注册约定。

## 目标

- Capability、Action、Menu 职责分离，后端代码定义导航与权限。
- HTTP / 工作流 / Agent / MCP 共用同一 Action 入口。
- 与图标（Iconify）、API 信封、运行日志 / 审计规范对齐。

## AI 暴露分类

所有非 `demo` 的生产 Action 在注册处必须完成且只能完成一种 AI 分类：

- `exposed`：`Metadata.AI.ExposeTool=true`，并完整声明风险、side effect、输入 Schema、用户说明和结果摘要；可稳定建模时补充输出 Schema。
- `excluded`：`Metadata.AI.ExposeTool=false`，并通过 `ExclusionReason` 声明非空、稳定的原因代码。统一使用 `action.ExcludeFromAI(...)` 和框架原因常量。

两种状态互斥。暴露项同时填写排除原因、排除项缺少原因、或生产 Action 完全缺少 `AI` 元数据，注册与架构测试都会 fail closed。Action Registry 是唯一事实源，禁止另建 Action ID switch 或平行 AI 暴露清单。

`risk=high|critical`，或 `side_effect=write|external|paid|destructive` 的暴露 Action 必须显式设置 `RequiresConfirmation=true`，否则注册失败。即使运行时会根据风险和副作用兜底触发确认，注册元数据仍必须完整表达安全契约。

包含密码、Token 或 Provider 密钥的现有请求 DTO 不能直接用作 Tool 输入 Schema。必须先设计不让敏感值进入模型上下文、确认参数和调用日志的安全 DTO；完成前使用 `sensitive-credential-boundary` 排除。例如当前用户创建和密码重置 Action 均包含明文密码，因此不参与第二批投影。

运维状态变更 Action 必须使用最小引用输入：Job 取消/重试只接受 `id`，Outbox 重试只接受 `event_id`，通知投递重试只接受 `id`。禁止把原始 Job/Event payload、metadata、任务类型或通知正文放入 mutation 输入 Schema；重试必须由领域服务读取并复用已持久化事实。Job 状态变更至少为 `medium/write`，可能重新发布事件或触达外部渠道的重试使用 `high/write|external`，全部强制单次确认。运维只读 Action 仍需保持租户隔离，不能因 Tool 投影扩大日志、事件、投递或审计记录的可见范围。

所有暴露 Action 的业务模块必须在 AI 模块刷新 Tool Registry 之前完成注册。生产启动顺序保持 `system → operations → ai`；请求路径不得通过再次 Refresh 临时补注册，新增后置业务模块时必须先调整显式启动聚合顺序。

当前稳定排除原因包括：

- `pending-safety-review`：等待对应批次完成强类型 DTO、风险和确认评审；
- `public-authentication-entry`：登录、挑战、刷新等公共认证入口；
- `installation-operation`：安装状态、环境检查、数据库测试和初始化；
- `internal-runtime-operation`：Tool / Agent / 索引 / 恢复等内部运行时能力；
- `sensitive-credential-boundary`：会接收或处理密码、Token、Provider 密钥等敏感凭证。

新增原因必须表达稳定的架构边界，不能使用临时 Issue 编号或个人备注。`platform/tests/action_ai_architecture_test.go` 扫描生产模块的元数据声明，并按模块汇总漏分类位置；`demo` 与测试夹具按明确规则不进入生产清单。

## 分层关系

```text
Capability
  └── Action（唯一执行入口）+ Metadata
Menu（独立注册）
  ├── directory  目录
  ├── menu       页面叶子
  └── button     按钮级权限（无独立路由）
```

- 不是每个 Action 都有菜单（如审批动作）。
- 不是每个菜单都有 Action（如目录节点）。
- 按钮通过 `permission`（通常 = 某 `action.id`）控制显隐/可点。

## Capability

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 能力域，如 `purchase` |
| `name` | string | 是 | 展示名 |
| `description` | string | 否 | 人或 AI 可读说明 |
| `module` | string | 否 | 应用模块，如 `erp` |
| `order` | int | 否 | 排序，默认 0 |

```go
RegisterCapability(Capability{
  ID: "purchase", Name: "采购", Module: "erp",
})
```

## Action

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 全局唯一，见命名约定 |
| `handler` | func | 是 | 执行函数 |
| `meta` | Metadata | 是 | 至少含 `capability` |

### `action.id` 命名（已拍板）

格式：`{capability}.{verb}`

- `capability`：与 Capability.id 一致，单段小写（可含下划线，如 `purchase`）。
- `verb`：动作名；需要资源语义时用**下划线**，**不要**再叠一层点分。

合法：

```text
purchase.create
purchase.order_create
purchase.order_approve
inventory.stock_in
```

非法：

```text
purchase.order.create    # 禁止多层点分
Purchase.Create          # 禁止大写
purchase_order_create    # 禁止缺少 capability 点分前缀（除非 capability 本身如此且已注册）
```

### 权限默认值（已拍板）

- Action 的 `access` 只有 `public`、`authenticated`、`permission` 三种；省略时默认 `permission`，保持 fail-closed。
- `public` 允许匿名调用；`authenticated` 要求有效登录上下文但不校验角色 permission；`permission` 要求有效登录且通过 permission 校验。
- `access` 属于 Action 语义，与 HTTP/MCP 等执行入口无关；禁止在 `HTTPMeta` 维护第二套公开访问配置。
- `permission` 模式的**默认权限码 = `action.id`**。
- 仅在确需聚合/别名时填写 `meta.permission` 覆盖；一般不覆盖。
- `public` / `authenticated` Action 不得声明可分配 permission，也不进入角色权限目录。

## Metadata（挂在 Action 上）

### 核心

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `capability` | string | 是 | 所属 Capability.id |
| `surface` | string | 否 | `tenant` / `platform`；省略兼容为 `tenant` |
| `permission` | string | 否 | 默认等于 `action.id` |
| `access` | enum | 否 | `public` / `authenticated` / `permission`，默认 `permission` |
| `permission_group` | string | 否 | 权限树功能组稳定 key，如 `system.role_management` |
| `permission_group_name` / `permission_group_name_key` | string | 否 | 功能组 fallback 名称与 i18n key |
| `name` | string | 否 | 展示名 |
| `description` | string | 否 | OpenAPI / AI 说明 |
| `audit` | bool | 否 | 建议写操作 `true`；只读 list/get 可 `false`；默认建议 `true` |
| `http` | HTTPMeta | 否 | 暴露 HTTP 时填 |
| `ai` | AIMeta | 否 | Action Tool 的唯一暴露声明；`nil` 默认不暴露 |

**不在** Metadata 内嵌菜单树；菜单一律 `RegisterMenu`。

权限目录的展示层级固定为 `Capability → PermissionGroup → Permission/Action`。同一功能的一组 Action 应声明相同的 `permission_group`；未声明时进入 Capability 的 `other` 组，禁止前端根据 action.id 文本临时猜分组。

菜单按用户工作场景组织，PermissionGroup 按授权边界组织，两者不要求一一对应。配置类权限与运行运维类权限必须拆分，例如通知模板配置与通知投递运维即使位于相邻导航，也不得共用权限组。禁止将 Job、Outbox、通知投递合并为单一宽泛的运维权限组。

应用壳级特殊入口仍由菜单 Metadata 驱动。`placement=global_ai_launcher` 表示该 `menu` 不进入普通滚动菜单，而由应用壳在垂直侧栏底部、水平顶栏或移动导航中自适应投影；它仍必须通过 `permission` / `bind_action` 服从后端权限过滤。`immersive=true` 表示路由隐藏普通页签、内容边距和 Footer。前端禁止根据菜单 ID、标题或 URL 猜测这些行为。

安全敏感只读能力可显式启用审计，例如审计日志列表/详情使用独立 `system.operations.audit` PermissionGroup 且 `audit=true`；普通高频列表仍应显式 `audit=false`，避免审计噪声。

### HTTPMeta

| 字段 | 类型 | 说明 |
|------|------|------|
| `method` | string | `GET` / `POST` / `PUT` / `PATCH` / `DELETE` |
| `path` | string | 如 `/api/v1/purchase/orders`；统一 `/api/v1` 前缀 |
| `enabled` | bool | 默认 true |

### AIMeta

`Metadata.AI` 是 Action Tool 暴露的唯一事实源。`AI=nil` 默认不暴露；只有 `ExposeTool=true` 才由 `ActionProvider` 自动投影，并继续复用 Action permission。禁止维护独立 Action ID 暴露清单。

| 字段 | 类型 | 说明 |
|------|------|------|
| `expose_tool` | bool | 是否作为 Agent/MCP Tool |
| `risk` | string | `low` / `medium` / `high` / `critical` |
| `side_effect` | string | `none` / `read` / `write` / `external` / `paid` / `destructive` |
| `requires_confirmation` | bool | 是否强制单次确认 |
| `summary` | string | 面向模型的安全用途描述 |
| `human_description` | string | 面向用户的工具说明 |
| `tags` | []string | 用于检索的稳定业务词 |
| `result_summary` | string | 不含参数与正文的安全结果摘要 |
| `input_schema` | JSON Schema | 由 `action.InputSchemaFor[Request]()` 生成；字段 required/optional 必须显式声明 |
| `output_schema` | JSON Schema | 由 `action.SchemaFor[Response]()` 生成 |

### 预留（Phase 1 不强制）

`workflow`、`ui`、更细的 MCP 开关等：结构可先占位，业务可不填。

## Menu（独立注册）

目录节点只表达导航结构，原则上不声明额外 permission。目录是否可见由权限过滤后是否仍有可见子菜单决定；无可见子菜单时必须隐藏整个目录。侧栏最多两级，新增顶层工作区时直接注册其二级页面，不通过系统管理制造三级菜单。

与 `docs/development/icon-library.md` 一致。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 稳定键，如 `erp.purchase`、`erp.purchase.order_list` |
| `parent_id` | string | 否 | 空为顶级 |
| `title` | string | 是 | 标题 |
| `title_key` | string | 否 | i18n；有则优先 |
| `icon` | string | 否 | Iconify `collection:name` |
| `local_icon` | string | 否 | 本地 SVG；有则覆盖 `icon` |
| `order` | int | 否 | 同级排序 |
| `type` | string | 是 | `directory` \| `menu` \| `button` |
| `path` | string | 条件 | `menu` 叶子建议必填（前端路由） |
| `component` | string | 否 | 动态路由组件约定 |
| `permission` | string | 否 | permission Action 的可见/可点权限；authenticated 基础入口不得填写 |
| `bind_action` | string | 否 | 关联主 Action（文档/审计）；按钮常与 `permission` 相同 |
| `hide_in_menu` | bool | 否 | 默认 false；按钮通常不出现在侧栏 |
| `keepalive` | bool | 否 | 仅 `menu` 有意义 |
| `module` | string | 否 | 如 `erp` |
| `surface` | string | 否 | `tenant` / `platform`；省略兼容为 `tenant` |

Action 执行时先校验 AuthContext realm 与 surface，再按 access 执行认证/permission 策略。菜单与 CodeSet 接口为 authenticated 基础能力，但返回内容仍先按 surface、再按业务 permission 过滤。`Module` 不得用于推断 realm。

### 类型约定

| type | 用途 | path / component | permission |
|------|------|------------------|------------|
| `directory` | 侧栏目录 | 无 | 可选（有子且可见时由子决定，或目录本身要求） |
| `menu` | 页面 | 建议有 path | 建议有，控制菜单可见 |
| `button` | 页内按钮/操作 | 无 path | **必填**，等于可执行的 `action.id` |

按钮不进入侧栏树展示（`hide_in_menu=true` 或组装菜单树时过滤 `type=button`）；permission Action 的按钮按权限列表显隐，authenticated Action 的按钮只依赖有效登录状态并必须 `bind_action`。

### 示例

```go
RegisterMenu(Menu{
  ID: "erp.purchase", Type: "directory", Title: "采购",
  Icon: "mdi:cart-outline", Module: "erp", Order: 10,
})

RegisterMenu(Menu{
  ID: "erp.purchase.order_list", ParentID: "erp.purchase", Type: "menu",
  Title: "采购单", Icon: "mdi:file-document-outline",
  Path: "/erp/purchase/orders", Component: "erp/purchase/order-list",
  Permission: "purchase.order_list", BindAction: "purchase.order_list", Order: 1,
})

RegisterMenu(Menu{
  ID: "erp.purchase.order_create_btn", ParentID: "erp.purchase.order_list",
  Type: "button", Title: "新建采购单",
  Permission: "purchase.order_create", BindAction: "purchase.order_create",
  HideInMenu: true,
})
```

```go
RegisterAction(Action{
  ID: "purchase.order_create",
  Handler: CreatePurchaseOrder,
  Meta: Metadata{
    Capability: "purchase",
    Audit: true,
    HTTP: &HTTPMeta{Method: "POST", Path: "/api/v1/purchase/orders"},
    AI: &AIMeta{
      ExposeTool: true, Risk: "medium", SideEffect: "write",
      RequiresConfirmation: true,
      Summary: "创建采购订单", HumanDescription: "创建一张新的采购订单",
      ResultSummary: "采购订单已创建",
      InputSchema: action.InputSchemaFor[OrderCreateRequest](),
      OutputSchema: action.SchemaFor[OrderView](),
    },
  },
})
```

## 注册顺序建议

1. `RegisterCapability`
2. `RegisterAction`（及其 Metadata）
3. `RegisterMenu`（directory → menu → button）

启动时校验：`action.meta.capability` 已存在；`menu.permission` / `bind_action` 若填写则 Action 已注册；`icon` 格式合法。

## 检查清单

- [ ] Action.id 为 `{capability}.{verb}`，无多层点分
- [ ] 未随意覆盖 permission（默认即 action.id）
- [ ] 菜单独立注册；按钮有 permission 且对应 Action
- [ ] 图标为 Iconify `collection:name`
- [ ] 需审计的写操作 `audit=true`

## 变更

调整字段或命名规则时，同步更新本文档与 `.agent/rules/action-metadata.md`，并 `agent-rules sync`。
