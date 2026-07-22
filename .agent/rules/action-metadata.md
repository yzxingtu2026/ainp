# Action / Metadata / 菜单

- 三套注册：`RegisterCapability` / `RegisterAction` / `RegisterMenu`；菜单独立，不挂在 Action.Meta 里。
- `action.id` 格式：`{capability}.{verb}`，动词可用下划线；Access 仅 `public` / `authenticated` / `permission`，省略默认 `permission`（fail-closed）。
- Access 属于 Action 语义，不放在 HTTP Metadata；`permission` 默认权限码 = action.id，`public` / `authenticated` 禁止声明可分配 permission。
- 菜单 `type`：`directory` | `menu` | `button`；`icon` 为 Iconify `collection:name`（见 `icon-library`）。
- Phase 1 必填核心字段；`input_schema` / `ui` / 细粒度 MCP 等预留不强制。按钮级权限 Phase 1 就要做。
- permission Action 按钮必须声明 permission；authenticated 基础按钮不得伪造 permission，必须绑定对应 Action。
- Action/Menu 的 `surface` 仅 `tenant` / `platform`，省略默认 tenant；执行和菜单返回均先匹配当前 realm，禁止用 `module == system` 推断域。
- 可分配权限树只包含 `access=permission` Action，固定为 `Capability → PermissionGroup → Permission/Action`；功能组由 Action Metadata 稳定声明。
- 菜单按工作场景组织，PermissionGroup 按授权边界组织；配置与运行运维必须分组，Job、Outbox、通知投递不得合并为单一宽泛运维组。
- 目录不制造额外 permission，由可见子菜单决定显示；无可见子菜单时隐藏目录，侧栏最多两级。
- 应用壳特殊入口使用受控 `placement` 元数据；`global_ai_launcher` 仅适用于 `menu`，仍须绑定 Action 权限。沉浸式内容使用 `immersive=true`，禁止前端按菜单 ID、标题或路径散落硬编码。
- 非 `demo` 生产 Action 注册时必须在 `Metadata.AI` 明确二选一：完整 `ExposeTool=true`，或 `ExcludeFromAI(稳定原因代码)`；两者互斥，禁止空原因和平行 AI 暴露清单。
- 暴露 Action 必须完整声明风险、side effect、输入 Schema、用户说明和结果摘要；写入、外部、付费、破坏性或高风险操作必须进入确认机制。
- 暴露 Action 输入统一使用 `action.InputSchemaFor[Request]()` 并通过显式字段存在语义门禁；普通 `SchemaFor` 仅用于响应等非输入 Schema。

详规：`docs/development/action-metadata.md`
