# 前端技术栈（Soybean Admin + Naive UI）

本文档是 AINP 管理端与 AI Workspace 的前端选型详规。Agent 只读要点：`.agent/rules/frontend-stack.md`。

管理后台下游装配、页面注册、模块裁剪与升级见 [frontend-downstream-composition.md](frontend-downstream-composition.md)。该契约只适用于 Vue 管理后台，不适用于 Taro、React Native 或其他客户端。

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

## Select 空值契约

业务单选优先使用 `AppSelect`，可选值在 UI 模型中使用 `null` 表示未选择；API 请求边界再根据 DTO 需要显式转换为 `undefined` 或 `''`。禁止使用 `value || fallback` 转换 Select 值，因为 `0` 与合法空字符串都可能是业务值。

`AppSelect` 默认使用 `empty-string-policy="auto"` 兼容历史模型：仅当当前 options 中不存在合法 `value: ''` 时，视图才把 `''` 作为未选择并展示 placeholder，且初次渲染不会回写业务模型。options 包含空字符串时保留其选项语义；异步 options 到达后自动重新判断。确需在无对应 option 时保留空字符串显示语义，可显式设置 `empty-string-policy="preserve"`。多选数组、数字 `0`、非空历史值均不归一化。

`CodeSelect` 与 `RemoteSelect` 必须复用 `AppSelect`。筛选器中的“全部”若使用 `{ label: '全部', value: '' }`，保持该 option 即可，不得在全局请求层或组件层无条件把空字符串转换为 `null`。新增或迁移表单时必须测试初始化、清空、异步 options、合法空字符串和 API DTO 转换。

`frontend/src` 的业务页面、公共业务组件和应用壳设置禁止直接导入或渲染 Naive UI `NSelect`；只有 `components/common/app-select.vue` 及其白盒测试可以访问底层组件。`pnpm test:app-select-architecture` 会扫描源码并阻止旁路，新增 Select 场景必须通过 `AppSelect`、`CodeSelect` 或 `RemoteSelect` 接入。

## 请求错误展示边界

统一 Axios 客户端默认是 HTTP/业务信封失败的唯一全局展示方。后端信封的已本地化 `message` 优先；信封没有安全文案时使用前端 `request.failed`，禁止把 `Request failed with status code ...` 等 Axios `error.message` 展示给用户。401 等身份失效仍执行会话清理，错误展示模式不能绕过安全处理。

每个请求通过 `errorMode` 明确展示所有权：

| 模式 | 展示方 | 调用方约束 |
|------|--------|------------|
| `global` | 请求层全局 toast（默认） | 收到 `{ data, error }` 后只终止流程或回滚状态，不再提示错误 |
| `silent` | 无 | 仅用于明确允许静默降级的后台刷新、探测等场景 |
| `inline` | 页面内联区域 | 请求必须显式声明；使用 `getRequestErrorMessage(error, fallback)` 获取安全文案 |
| `custom` | 页面或公共组件 | 请求必须显式声明；本地 toast/dialog 与请求层不能同时展示 |

```ts
const { data, error } = await request<Item[]>({ url: '/api/v1/items' });
if (error) return;

const result = await request<Item[]>({ url: '/api/v1/items', errorMode: 'inline' });
if (result.error) {
  inlineError.value = getRequestErrorMessage(result.error, $t('request.failed'));
}
```

原生 `fetch`、附件 Transfer、SSE 等非统一请求链路可以本地展示，但必须通过 `getStandaloneErrorMessage(error, fallback)` 过滤 Axios 技术文案。统一请求失败不得经 `throw error` 后在外层 catch 再次显示；必须区分 Axios 请求错误与本地异常，确保只有一个展示方。

`pnpm test:request-error-boundary` 同时执行错误模式单测、静态扫描正反例和全仓扫描。门禁会追踪 `await` 返回的解构 `error`、别名以及 `result.error`，阻止其 `.message` 流入 toast 或内联状态；无需展示文案的默认失败分支直接返回即可。

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

普通 API 请求使用统一 Axios 客户端，由客户端处理 Service Base URL。必须使用原生 `fetch` 的流式或特殊请求，通过 `src/utils/service.ts` 的 `buildServiceURL()` 构造地址；调用方只传受控的应用内绝对路径，查询参数使用 `URLSearchParams`，禁止用模板字符串直接拼接 Base URL、API 路径或未编码查询参数。

开始前端任务前确认：

- [ ] 未引入其他后台脚手架作为主壳
- [ ] 页面组件基于 Naive UI，而非其他主 UI 库
- [ ] 菜单图标为 Iconify `collection:name`（默认 `mdi`），见 [图标库规范](./icon-library.md)
- [ ] 未在业务页复制 Layout / 请求 / 权限逻辑
- [ ] 原生 `fetch` 使用 `buildServiceURL()`，未直接拼接 Service Base URL
- [ ] 统一请求失败只有一个展示方，接管时已显式声明 `errorMode`，未展示 Axios 原始文案
- [ ] 复杂表格优先 TanStack Table
- [ ] 用户可见时间使用 `formatDateTime` / `DateTimeText`，见 [时间展示规范](./datetime-display.md)
- [ ] 新增公共 UI 约定已写入本文档或 `.agent/rules/frontend-stack.md`

## 变更流程

更换脚手架或主 UI 库前，须同步更新：

1. `.agent/rules/frontend-stack.md`
2. 本文档
3. `README.md` 技术选型
4. 运行 `agent-rules sync` 刷新本地 Agent 规则
