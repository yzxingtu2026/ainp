# 租户角色与权限管理规范

## 边界

- 角色属于单一租户，所有查询与写入必须同时约束 `tenant_id + role_id`。
- `code` 在租户内唯一且创建后不可修改；不同租户可使用相同 code。
- permission 只来自 tenant realm 下已启用、非 Public 的 Action Registry；Menu ID 不是 permission。
- 多个 Action 可共享一个 permission，权限目录按 permission 去重并保留关联 Action ID。

## 写操作

- 角色资料、状态、权限替换和删除都在 Service 事务中完成，Handler 不散落 SQL。
- 权限替换先完整校验目录，再删除旧值并写入新值；成功后在同一事务按角色递增关联用户 `auth_version`。
- 角色启停同样在事务中触发关联用户授权失效。
- 有 UserRole 关联的角色拒绝删除，必须先由用户管理解除分配。

## 内置管理员保护

- 安装向导创建 `is_builtin=true` 的 `admin` 角色，该角色不可删除、停用或修改 code。
- 内置管理员必须保留角色管理、权限目录、当前用户与权限读取的最小权限集合。
- 安装向导负责创建初始租户、管理员、内置角色和初始权限；Demo Seed 仅负责开发演示数据。
- Schema Migration 只维护字段、索引与约束，不创建或修补角色、管理员及权限等业务数据；未来存量升级另设版本化业务数据升级机制。
- 用户管理解除角色或停用用户时，须在同一事务中校验租户至少保留一个 enabled 的内置 `admin` 用户。
- 用户角色分配只能选择当前租户 enabled 角色；跨租户 role_id 注入必须失败。

## 前端

- 页面只提交 role ID、可编辑字段和权限目录返回的 permission，不允许自由输入权限字符串。
- 内置角色明确标识，并禁用删除和停用；有关联用户的角色禁用删除入口。
- 权限使用 `Capability → PermissionGroup → Permission/Action` 三级可折叠树展示，例如“系统 → 权限管理 → 角色列表/创建/编辑/删除”；父节点支持全选/半选，叶节点展示 permission 与说明，仅当关联 Action ID 不同于 permission 时额外展示 Action，避免默认同值重复，并支持搜索、已选数量和失败反馈。

## 验证

- PostgreSQL 测试覆盖两租户同 code、跨租户 ID、权限目录过滤、非法 permission、内置保护、有关联用户删除和 `auth_version` 递增。
- 运行后端测试、前端 typecheck/build、Agent 规则同步、静态检查与 `git diff --check`。
