# 前端技术栈硬约束

管理端与 AI Workspace 前端统一采用以下栈，编程智能体与开发者均须遵守，不得擅自更换或并行引入替代方案。

## 必选栈

| 层级 | 选型 | 说明 |
|------|------|------|
| 管理端脚手架 | Soybean Admin | 布局、登录、主题、动态路由、请求封装等壳层以此为基准 |
| UI 组件库 | Naive UI | 业务页面与公共组件统一使用 |
| 框架 | Vue 3 + TypeScript + Vite | 与仓库技术选型一致 |
| 状态 / 路由 | Pinia + Vue Router | 与 Soybean Admin 惯例一致 |

配套能力（已定，按需接入，不替换上述底座）：

- TanStack Table（复杂表格）
- Monaco Editor、Tiptap、ECharts
- **Iconify**（菜单与 UI 图标；默认集合 `mdi`，详见 `icon-library`）

## 禁止事项

- **禁止**引入或迁移到 Vben Admin、vue-element-plus-admin、Pure Admin 等其他后台脚手架作为主壳。
- **禁止**以 Element Plus、Ant Design Vue、Arco Design、Vuetify、PrimeVue、shadcn-vue 等作为主 UI 组件库（局部临时 POC 须先书面确认并限期拆除）。
- **禁止**在业务模块内另起一套布局、主题、权限路由或请求封装；优先扩展 Soybean 壳层与项目公共包。
- **禁止**为「看起来更快」而新建第二套前端工程或并行组件体系。

## 落地约定

1. **壳层用法**：保留 Soybean 的 Layout、登录、主题、动态路由与请求封装；删除或停用与业务无关的示例页。
2. **路由**：业务菜单与权限以平台 Metadata / 后端动态路由为主；勿把 Elegant Router 静态文件路由当成长期唯一来源。
3. **组件**：优先用 Naive UI；复杂表格用 TanStack Table，不与 Naive 表格能力重复造轮子。
4. **图标**：菜单 `icon` 使用 Iconify `collection:name`（默认 `mdi`），与后端 Metadata 一致；详见 `icon-library`。
5. **扩展**：新增通用 UI 封装放公共包或约定目录，并同步更新 `docs/development/frontend-stack.md`。
6. **选型变更**：更换脚手架或主 UI 库须先更新本规则与 `docs/development/frontend-stack.md`，再改代码。

## 详规文档

面向开发者的说明与裁剪清单见 `docs/development/frontend-stack.md`。
