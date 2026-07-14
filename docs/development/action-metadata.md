# Action / Capability / Metadata / 菜单注册规范

Agent 只读要点：`.agent/rules/action-metadata.md`。本文档为 Phase 1 字段设计与注册约定。

## 目标

- Capability、Action、Menu 职责分离，后端代码定义导航与权限。
- HTTP / 工作流 / Agent / MCP 共用同一 Action 入口。
- 与图标（Iconify）、API 信封、运行日志 / 审计规范对齐。

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

- **默认权限码 = `action.id`**。
- 仅在确需聚合/别名时填写 `meta.permission` 覆盖；一般不覆盖。

## Metadata（挂在 Action 上）

### 核心

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `capability` | string | 是 | 所属 Capability.id |
| `surface` | string | 否 | `tenant` / `platform`；省略兼容为 `tenant` |
| `permission` | string | 否 | 默认等于 `action.id` |
| `name` | string | 否 | 展示名 |
| `description` | string | 否 | OpenAPI / AI 说明 |
| `audit` | bool | 否 | 建议写操作 `true`；只读 list/get 可 `false`；默认建议 `true` |
| `http` | HTTPMeta | 否 | 暴露 HTTP 时填 |
| `ai` | AIMeta | 否 | 暴露 Agent/MCP 时填 |

**不在** Metadata 内嵌菜单树；菜单一律 `RegisterMenu`。

### HTTPMeta

| 字段 | 类型 | 说明 |
|------|------|------|
| `method` | string | `GET` / `POST` / `PUT` / `PATCH` / `DELETE` |
| `path` | string | 如 `/api/v1/purchase/orders`；统一 `/api/v1` 前缀 |
| `enabled` | bool | 默认 true |

### AIMeta（Phase 1 可简化）

| 字段 | 类型 | 说明 |
|------|------|------|
| `expose_tool` | bool | 是否作为 Agent/MCP Tool |
| `risk` | string | `low` / `medium` / `high`（high 预留人工确认） |
| `summary` | string | 给模型的一句话；可复用 description |

### 预留（Phase 1 不强制）

`workflow`、`input_schema`、`output_schema`、`ui`、更细的 MCP 开关等：结构可先占位，业务可不填。

## Menu（独立注册）

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
| `permission` | string | 否 | 可见/可点所需权限；**按钮建议必填**且等于某 `action.id` |
| `bind_action` | string | 否 | 关联主 Action（文档/审计）；按钮常与 `permission` 相同 |
| `hide_in_menu` | bool | 否 | 默认 false；按钮通常不出现在侧栏 |
| `keepalive` | bool | 否 | 仅 `menu` 有意义 |
| `module` | string | 否 | 如 `erp` |
| `surface` | string | 否 | `tenant` / `platform`；省略兼容为 `tenant` |

Action 执行时先校验 AuthContext realm 与 surface，再校验 permission。菜单接口先按 surface 过滤，再按 permission 过滤。`Module` 不得用于推断 realm。

### 类型约定

| type | 用途 | path / component | permission |
|------|------|------------------|------------|
| `directory` | 侧栏目录 | 无 | 可选（有子且可见时由子决定，或目录本身要求） |
| `menu` | 页面 | 建议有 path | 建议有，控制菜单可见 |
| `button` | 页内按钮/操作 | 无 path | **必填**，等于可执行的 `action.id` |

按钮不进入侧栏树展示（`hide_in_menu=true` 或组装菜单树时过滤 `type=button`）；前端按 `permission` / 权限列表控制按钮显隐。

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
    AI:   &AIMeta{ExposeTool: true, Risk: "medium"},
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
