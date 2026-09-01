# 管理后台下游装配

## 稳定边界

AINP Vue 管理后台使用编译期静态装配。`frontend/src/composition/contract.ts`、`define-app.ts` 和 `index.ts` 是首期稳定契约；`application.ts` 是产品 Composition Root。Elegant Router 生成文件、平台 Store 和 `frontend/src` 其他实现仍属于 AINP 基线内部，不是公共 npm API。

下游从固定 SemVer Tag/Commit 初始化基线，只替换自有 `application.ts` 并在自有目录注册页面、i18n、Store 初始化器和 service。平台页面从稳定入口显式选择 `corePages`、`systemPages`、`operationsPages`、`aiPages`、`demoPages`；未引用的模块不进入依赖图。禁止深层导入 AINP 内部实现，禁止修改 `router/elegant/imports.ts`、`routes.ts`、`transform.ts`，也禁止 URL、文件路径拼接或后端字符串触发动态 import。

三类所有权边界：`frontend/packages/@sa/*` 是 Soybean 通用基础包，当前 workspace 包不承诺公共 npm 契约；`frontend/src/composition/*` 是稳定装配契约；其余 `frontend/src/*` 是 AINP 内部实现。AppSelect、请求、CodeSet、附件和时间展示仍按对应规范使用，不得复制分叉。

个人中心只开放 `profileExtensions` 的 `tab` placement。注册项 ID 必须使用 `owner.` namespace，提供 `label` 或下游 i18n `labelKey`、非负整数 `order`（默认 1000）以及编译期可解析的 lazy component；相同 order 按 ID 排序。`basic`、`preferences`、`memory`、`mcp`、`security` 是平台保留 ID。重复 ID、保留 ID、未知 placement、非法 owner/namespace、无效 label/component 会在 `defineAdminApp` 阶段立即失败。扩展只追加在平台内置 Tab 之后，不能替换内置 Tab、整页或注入 selector/DOM/文件路径。

扩展组件拥有自己的业务状态，只能从下游自有目录导入已注册 service、统一请求客户端和公开组件契约；它不会收到个人中心资料、密码、Session、MCP 状态，也不得深层导入平台 Store 或内部页面实现。合法 `?tab=<extension-id>` 可直接恢复；未知 query 会替换为 `basic`，异步组件加载失败只在该 Tab 内显示受控错误边界。

Application Shell 首期只开放 `shellExtensions` 的 `header.context` placement，位置固定在平台菜单/面包屑与右侧工具区之间。注册项必须提供 owner namespace ID、非负安全整数 `order` 和编译期可解析的 lazy component；相同 order 按 ID 排序。重复 ID、非法 owner/namespace、未知 placement、非法 order/component 在 `defineAdminApp` 阶段 fail-closed，解析后的数组与注册项冻结。契约不接受组件名字符串、URL、HTML、selector、DOM path 或整个 Header/Layout 替换。

平台 Header 只把已校验注册项交给隔离宿主，不向扩展传入 Auth Store、Router、Theme Store 或其他私有实例。扩展通过下游自有 Store、已注册 service 和公开组件工作；选择的公司、项目、门店等值只是业务请求候选上下文，后端必须按当前 actor/tenant 重新校验，不能改变平台认证或服务端授权。initializer 继续只负责状态初始化，不承担组件渲染。

桌面端扩展按确定顺序内联，并在受限区域内横向滚动，平台右侧搜索、通知、语言、主题和用户菜单固定不收缩。视口不大于 960px 时，扩展进入带可访问名称的“业务上下文”弹出入口，所有注册项仍可键盘到达，不得无替代地隐藏。扩展自行提供可访问名称、加载/空/错误状态和适配两种容器的宽度；平台分别隔离 lazy load 与 render 错误，单项失败不得破坏 Header、导航、用户菜单或登出。响应式容器切换会重新挂载扩展，跨布局状态应由下游 Store 管理。

Shell 扩展只随已登录 Base Layout 挂载，登出、账号/租户变化或离开壳层时正常卸载。没有注册项时不渲染额外入口，现有 Header 行为保持兼容。新增 placement 必须另行定义产品语义、布局、生命周期和安全边界，不能复用 `header.context` 绕过治理。

下游 service 必须复用基线的统一请求客户端及其错误展示契约，不得另建 Axios 拦截器或在业务页面补第二个全局 toast。默认 `errorMode=global` 时，页面只处理成功数据或在 `error` 时终止流程；静默降级、表单内联和页面自定义提示分别显式使用 `silent`、`inline`、`custom`，并由调用方独占展示。接管展示时只通过 `getRequestErrorMessage(error, fallback)` 读取后端信封安全文案；原生 `fetch`、下载、SSE 等独立链路使用 `getStandaloneErrorMessage`，任何 UI 都不得直接消费 Axios `error.message`。升级基线时必须运行 `pnpm test:request-error-boundary`。

认证登录属于平台固定外壳，不开放整页替换或动态组件注册。下游 redirect Provider 只需在后端注册 Authenticator，并通过受信任 `Capabilities` 提供 label/label_key、Iconify 固定 icon 与 order；Admin 会动态渲染入口并统一使用 `/auth-callback`。下游不得深层导入登录页、路由或 Auth Store，也不得让后端字符串触发动态 import。

## 生命周期与安全边界

`defineAdminApp` 按 ID 排序并校验所有注册。应用、页面、路由、i18n namespace、初始化器与 service 均携带 owner；重复键、未知页面、非法 path、远程品牌资源和非法模块组合立即失败。模块依赖为 `core` 必选、`system -> core`、`operations -> system`、`ai -> core`、`demo -> core`。

后端 Metadata 决定用户可见与可访问菜单，前端 page registry 只决定产物中存在的代码。动态菜单 component key 必须命中 registry；本地注册页面不产生权限。生产下游使用动态路由模式。Demo 仅在 Composition Root 显式加入 `demo` 模块及页面后进入产物。

应用启动顺序固定为：定义校验、品牌安装、平台插件、Pinia、Router、i18n、按 ID 排序的下游 initializer、挂载。initializer 只通过 context 获取 Vue app 与已注册 service，禁止依赖 import 副作用。

## 初始化、版本与升级

发布时执行 `pnpm baseline:generate`，清单记录前端版本、Commit、Node/pnpm、生成协议版本、模块和基线文件 SHA-256。Vite 会将该清单以 `admin-composition.json` 放入构建产物。根 Tag、`platform/v*` 和前端清单必须指向同一 Commit。

```bash
cd frontend
pnpm baseline:verify
node scripts/admin-baseline.mjs init ../my-admin
node scripts/admin-baseline.mjs preflight
```

`init` 遇到已有目标会停止，不静默覆盖。升级前固定新 Tag，在旧基线执行 `verify`；比较新旧清单，分开合并上游文件和下游目录。冲突时停止并逐文件记录临时补丁、上游 Issue 和删除条件。升级后验证 typecheck、lint、单测、生产构建、Metadata 页面映射、API/权限契约以及前后端 Commit。

通用改进优先回提 AINP。上游包含修复后删除临时补丁。失败回滚恢复旧 Tag、旧清单和旧锁文件，不修改已发布清单。

`frontend/examples/downstream-admin` 展示自定义品牌、页面/路由、i18n namespace、Store initializer、AINP 信封 service、个人中心 lazy Tab、通用业务上下文 Shell 扩展和可选 AI 模块，默认不包含 Demo。升级到包含扩展契约的基线时，应将 `profile-extension.ts`、`shell-extension.ts` 纳入清单，并重新执行 `pnpm test:composition`、`pnpm typecheck`、`pnpm lint`、生产/下游示例构建和 `pnpm baseline:verify`。
