# 前端技术栈（Soybean Admin + Naive UI）

本文档是 AINP 管理端与 AI Workspace 的前端选型详规。Agent 只读要点：`.agent/rules/frontend-stack.md`。

## 选型结论

| 项 | 选择 | 原因 |
|----|------|------|
| 脚手架 | [Soybean Admin](https://github.com/soybeanjs/soybean-admin) | 起壳快、目录浅、对编程智能体更友好；原生 Naive UI |
| UI 库 | Naive UI | MIT、现代化、易主题扩展，与 AI Workspace 产品形态匹配 |
| 基础 | Vue 3、TypeScript、Vite、Pinia、Vue Router | 与 README 技术选型一致 |

未选 Vben Admin 的主要原因：monorepo 与适配层更重，Agent 改页面成本更高；AINP 菜单/权限将由 Metadata 驱动，不宜深绑厚脚手架。

## 标准技术组合

```text
Soybean Admin（壳）
├── Vue 3 + TypeScript + Vite
├── Pinia + Vue Router
├── Naive UI
├── Iconify（菜单图标，默认 mdi）
├── TanStack Table（复杂表格）
├── Monaco Editor / Tiptap / ECharts（按模块接入）
└── 业务菜单 ← 平台 Metadata / 后端动态路由（icon 为 Iconify 字符串）
```

## 起壳与裁剪建议

1. 以 Soybean Admin 的 Naive UI 版为起点初始化 `frontend/`。
2. 保留：Layout、登录/鉴权、主题、动态路由、请求客户端、基础错误页。
3. 移除或停用：与 AINP 无关的演示页、示例 Mock 业务。
4. 业务路由以**后端动态路由 / Metadata** 注入为主；Elegant Router 仅作本地开发辅助，不作为权限菜单的唯一真相源。
5. 后续公共组件、主题 token、请求拦截等抽到 `frontend/packages/`（或等价公共包），业务 app 只消费公共能力。

## Agent / 开发者检查清单

分页表格统一使用 `hooks/common/table.ts`：API 请求为 `page/page_size`，响应为 `items/total/page/page_size`；筛选提交、重置或页大小变化回到第一页。类型化筛选定义只描述控件和序列化，不包含 SQL 字段、操作符或授权逻辑。

业务页面通过 `src/composables/page-context.ts` 的 `usePageContextProvider()` 注册当前页面的受限语义；全局 AI 发送时只调用 `capturePageContext()` 获取一次快照。禁止全局组件读取 DOM/组件实例或按路由字符串维护业务分支，禁止 Provider 上传未提交表单、密码、Token 或隐藏状态。Provider 必须正确处理 mounted/activated/deactivated/unmount 生命周期。完整协议见 `page-context.md`。

智能体形象统一使用 `AgentAvatar`：只传 `avatar_attachment_id`、名称与稳定尺寸。组件经附件 download prepare/transfer 加载，按 attachment ID 去重并限制并发，在最后一个消费者卸载时回收 Object URL；业务页面禁止直接拼图片地址。

全局 AI 侧边栏与 Workspace 必须复用应用级 `useAiConversationController()` 和 `AiConversationSurface`；应用壳 Store 只管理面板与二级视图，不持有第二套 Conversation/Run/SSE 状态。完整边界见 `global-ai-assistant.md`。

开始前端任务前确认：

- [ ] 未引入其他后台脚手架作为主壳
- [ ] 页面组件基于 Naive UI，而非其他主 UI 库
- [ ] 菜单图标为 Iconify `collection:name`（默认 `mdi`），见 [图标库规范](./icon-library.md)
- [ ] 未在业务页复制 Layout / 请求 / 权限逻辑
- [ ] 复杂表格优先 TanStack Table
- [ ] 用户可见时间使用 `formatDateTime` / `DateTimeText`，见 [时间展示规范](./datetime-display.md)
- [ ] 新增公共 UI 约定已写入本文档或 `.agent/rules/frontend-stack.md`

## 变更流程

更换脚手架或主 UI 库前，须同步更新：

1. `.agent/rules/frontend-stack.md`
2. 本文档
3. `README.md` 技术选型
4. 运行 `agent-rules sync` 刷新本地 Agent 规则
