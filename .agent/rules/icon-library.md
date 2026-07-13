# 图标库硬约束

业务菜单由后端 Metadata / 代码注册，前端按字符串渲染图标。图标协议必须前后端统一，编程智能体与开发者均须遵守。

## 必选方案

| 项 | 选型 | 说明 |
|----|------|------|
| 图标协议 | [Iconify](https://iconify.design/) | 与 Soybean Admin 原生能力一致 |
| 浏览与检索 | [Icônes](https://icones.js.org/) | 选定图标名称的唯一推荐入口 |
| 菜单默认集合 | `mdi`（Material Design Icons） | 企业后台与 ERP 领域覆盖面大，便于 Agent 选型 |
| 渲染 | Soybean `SvgIcon` / `@iconify/vue` | 禁止另起 Font Awesome、iconfont 全量引入等第二套主方案 |

## 字符串格式（写入后端菜单元数据）

统一使用 Iconify 规范名（**带集合前缀与冒号**）：

```text
{collection}:{name}
```

示例：

- `mdi:home-outline`
- `mdi:package-variant`
- `mdi:cart-outline`
- `mdi:cash-multiple`
- `mdi:account-group-outline`

后端 `Menu` / Metadata 字段建议：

- `icon`：Iconify 名称（必填优先走此字段）
- `local_icon`（可选）：仅品牌/定制 SVG；有值时前端忽略 `icon`

禁止写入：

- 无集合前缀的裸名（如 `home`、`mdi-home` 连字符旧写法）
- URL、emoji、CSS class、base64
- 未在本文档「允许集合」内的随意集合（临时 POC 须限期改为 `mdi` 或登记扩集）

## 允许的 Iconify 集合

| 集合 | 用途 | 是否默认 |
|------|------|----------|
| `mdi` | 系统菜单、业务模块、通用操作 | 是（首选） |
| `carbon` | 仅当 `mdi` 无合适语义且需更克制线型时 | 否 |
| 本地 SVG | Logo、品牌、无公开等价物的定制标 | 否（`local_icon`） |

新增集合须先更新 `docs/development/icon-library.md` 与本规则，再在代码中使用。

## 禁止事项

- **禁止**以 Element / Ant Design 图标包、iconfont 项目、Font Awesome Kit 作为菜单图标主来源。
- **禁止**在后端存前端组件名或 import 路径。
- **禁止**同一语义在多处使用不同集合的近似图标（如首页同时用 `mdi:home` 与 `carbon:home`）。
- **禁止**为单个菜单引入整包图标字体以「省事」。

## 落地约定

1. 后端注册菜单时只写 Iconify 字符串；前端动态路由/`meta.icon` 原样透传。
2. 前端按需或离线装载 `@iconify-json/mdi`（及已批准集合），保证内网/离线可渲染。
3. 本地 SVG 放在约定目录（Soybean：`src/assets/svg-icon`），`local_icon` 为不含扩展名的文件名。
4. 缺省菜单图标使用统一占位（如 `mdi:menu`），避免空白或裂图。
5. 变更默认集合或协议格式时，先改文档与本规则，再批量迁移元数据。

## 详规文档

命名约定、推荐映射与检查清单见 `docs/development/icon-library.md`。
