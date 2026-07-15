# 前端技术栈

- 脚手架：**Soybean Admin**；UI：**Naive UI**；基础：Vue 3 / TypeScript / Vite / Pinia / Vue Router。
- 禁止另起后台脚手架或更换主 UI 库；业务菜单走后端 Metadata / 动态路由。
- 复杂表格用 TanStack Table；图标见 `icon-library`。
- 用户可见时间展示见 `datetime-display`（禁止直接渲染后端原始时间串）。
- 租户业务设置见 `settings-system`（动态表单由协议驱动，禁止第二套设置表）。

详规：`docs/development/frontend-stack.md`
