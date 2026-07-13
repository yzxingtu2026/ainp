# 前端技术栈（Soybean Admin + Naive UI）

本文档是 AINP 管理端与 AI Workspace 的前端选型说明。Agent 行为硬约束见 `.agent/rules/frontend-stack.md`。

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
├── TanStack Table（复杂表格）
├── Monaco Editor / Tiptap / ECharts（按模块接入）
└── 业务菜单 ← 平台 Metadata / 后端动态路由
```

## 起壳与裁剪建议

1. 以 Soybean Admin 的 Naive UI 版为起点初始化 `frontend/`。
2. 保留：Layout、登录/鉴权、主题、动态路由、请求客户端、基础错误页。
3. 移除或停用：与 AINP 无关的演示页、示例 Mock 业务。
4. 业务路由以**后端动态路由 / Metadata** 注入为主；Elegant Router 仅作本地开发辅助，不作为权限菜单的唯一真相源。
5. 后续公共组件、主题 token、请求拦截等抽到 `frontend/packages/`（或等价公共包），业务 app 只消费公共能力。

## Agent / 开发者检查清单

开始前端任务前确认：

- [ ] 未引入其他后台脚手架作为主壳
- [ ] 页面组件基于 Naive UI，而非其他主 UI 库
- [ ] 未在业务页复制 Layout / 请求 / 权限逻辑
- [ ] 复杂表格优先 TanStack Table
- [ ] 新增公共 UI 约定已写入本文档或 `.agent/rules/frontend-stack.md`

## 变更流程

更换脚手架或主 UI 库前，须同步更新：

1. `.agent/rules/frontend-stack.md`
2. 本文档
3. `README.md` 技术选型
4. 运行 `agent-rules sync` 刷新本地 Agent 规则
