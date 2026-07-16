# Action / Metadata / 菜单

- 三套注册：`RegisterCapability` / `RegisterAction` / `RegisterMenu`；菜单独立，不挂在 Action.Meta 里。
- `action.id` 格式：`{capability}.{verb}`，动词可用下划线；Access 仅 `public` / `authenticated` / `permission`，省略默认 `permission`（fail-closed）。
- Access 属于 Action 语义，不放在 HTTP Metadata；`permission` 默认权限码 = action.id，`public` / `authenticated` 禁止声明可分配 permission。
- 菜单 `type`：`directory` | `menu` | `button`；`icon` 为 Iconify `collection:name`（见 `icon-library`）。
- Phase 1 必填核心字段；`input_schema` / `ui` / 细粒度 MCP 等预留不强制。按钮级权限 Phase 1 就要做。
- permission Action 按钮必须声明 permission；authenticated 基础按钮不得伪造 permission，必须绑定对应 Action。
- Action/Menu 的 `surface` 仅 `tenant` / `platform`，省略默认 tenant；执行和菜单返回均先匹配当前 realm，禁止用 `module == system` 推断域。
- 可分配权限树只包含 `access=permission` Action，固定为 `Capability → PermissionGroup → Permission/Action`；功能组由 Action Metadata 稳定声明。

详规：`docs/development/action-metadata.md`
