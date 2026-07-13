# 图标库规范（Iconify + mdi）

菜单由图标字符串驱动：后端 Metadata / 代码定义 `icon`，前端 Soybean Admin 按 Iconify 渲染。Agent 只读要点：`.agent/rules/icon-library.md`。

## 为什么先定图标库

AINP 菜单与路由由后端注册，前端不再手写菜单树。若图标方案不统一，会出现：

- 后端随便写名字，前端无法解析；
- 多套图标库并存，视觉与包体积失控；
- 编程智能体无法稳定选图标。

因此在铺菜单元数据之前，先锁定 **Iconify**，菜单默认集合为 **`mdi`**。

## 协议

### 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `icon` | string | Iconify 名，格式 `collection:name` |
| `local_icon` | string，可选 | 本地 SVG 文件名（无后缀）；有值时优先生效 |

Go 侧示意：

```go
Menu: Menu{
    Title: "采购",
    Icon:  "mdi:cart-outline",
},
```

完整菜单字段与 `RegisterMenu` 见 `docs/development/action-metadata.md`。

### 合法示例

```text
mdi:view-dashboard-outline
mdi:package-variant-closed
mdi:truck-delivery-outline
mdi:domain
mdi:cog-outline
```

### 非法示例

```text
home                 # 缺集合
mdi-home             # 连字符旧写法，禁止
el-icon-menu         # 其他组件库
https://.../a.svg    # URL
🛒                   # emoji
```

## 选用顺序

1. 在 [Icônes](https://icones.js.org/) 的 **Material Design Icons** 中按语义搜索。
2. 菜单项优先 `*-outline` 线型，与后台侧栏风格一致；选中态由主题色处理，不必另存实心名。
3. `mdi` 确实无合适图标时，可用已批准的 `carbon:`；仍没有则加本地 SVG，并在模块说明中备注。

## 常见业务映射（推荐起点）

| 业务域 | 推荐 icon |
|--------|-----------|
| 工作台 / 首页 | `mdi:view-dashboard-outline` |
| 系统设置 | `mdi:cog-outline` |
| 组织 / 租户 | `mdi:domain` |
| 用户 | `mdi:account-outline` |
| 角色 / 权限 | `mdi:shield-account-outline` |
| 菜单管理 | `mdi:menu` |
| 库存 | `mdi:package-variant-closed` |
| 采购 | `mdi:cart-outline` |
| 销售 | `mdi:point-of-sale` |
| 财务 | `mdi:cash-multiple` |
| CRM / 客户 | `mdi:account-group-outline` |
| OA / 审批 | `mdi:clipboard-check-outline` |
| 工作流 | `mdi:sitemap-outline` |
| AI Workspace | `mdi:robot-outline` |
| 知识库 | `mdi:book-open-page-variant-outline` |
| 审计 / 日志 | `mdi:text-box-search-outline` |
| 通知 | `mdi:bell-outline` |

同一业务域在平台内复用上表，避免同义多图标。

## 与 Soybean Admin 的衔接

- 动态路由 `meta.icon` ← 后端 `icon`
- 动态路由 `meta.localIcon` ← 后端 `local_icon`
- 页面内静态图标同样走 Iconify 字符串，便于和菜单视觉一致
- 生产环境建议离线注册 `@iconify-json/mdi`，不依赖运行时拉 CDN

## 检查清单

- [ ] 菜单 `icon` 均为 `collection:name`
- [ ] 默认使用 `mdi`，未引入未批准集合
- [ ] 未使用 emoji / URL / 其他 UI 库图标类名
- [ ] 本地 SVG 仅用于品牌或无公开等价物场景
- [ ] 前端能在无外网时渲染 `mdi` 菜单图标

## 变更

更换默认集合或字段含义时，同步更新：

1. `.agent/rules/icon-library.md`
2. 本文档
3. 已注册菜单元数据
4. `agent-rules sync`
