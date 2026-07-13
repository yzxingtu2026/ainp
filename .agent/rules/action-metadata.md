# Action / Metadata / 菜单

- 三套注册：`RegisterCapability` / `RegisterAction` / `RegisterMenu`；菜单独立，不挂在 Action.Meta 里。
- `action.id` 格式：`{capability}.{verb}`，动词可用下划线（如 `purchase.order_create`）；**默认权限码 = action.id**。
- 菜单 `type`：`directory` | `menu` | `button`；`icon` 为 Iconify `collection:name`（见 `icon-library`）。
- Phase 1 必填核心字段；`input_schema` / `ui` / 细粒度 MCP 等预留不强制。按钮级权限 Phase 1 就要做。

详规：`docs/development/action-metadata.md`
