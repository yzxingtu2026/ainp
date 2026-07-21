# 前端技术栈

- 脚手架：**Soybean Admin**；UI：**Naive UI**；基础：Vue 3 / TypeScript / Vite / Pinia / Vue Router。
- 禁止另起后台脚手架或更换主 UI 库；业务菜单走后端 Metadata / 动态路由。
- 复杂表格用 TanStack Table；图标见 `icon-library`。
- 用户可见时间展示见 `datetime-display`（禁止直接渲染后端原始时间串）。
- 租户业务设置见 `settings-system`（动态表单由协议驱动，禁止第二套设置表）。
- CRUD/运维分页表格复用 `hooks/common/table.ts`，API 统一 `page/page_size` 与 `items/total/page/page_size`；筛选变化回第一页。
- 页面语义只经统一 Page Context Provider Registry 注册；全局 AI 禁止读取 DOM/组件实例或按路由维护业务分支，Provider 禁止上传表单草稿和敏感状态。

详规：`docs/development/frontend-stack.md`
