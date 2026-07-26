# 租户角色与权限管理规范

## 边界

- 角色属于单一租户，所有查询与写入必须同时约束 `tenant_id + role_id`。
- `code` 在租户内唯一且创建后不可修改；不同租户可使用相同 code。
- permission 只来自 tenant realm 下 `access=permission` 的 Action Registry；HTTP 是否暴露不改变可分配性，Menu ID 不是 permission。
- `public` / `authenticated` Action 不进入权限目录、管理员最低权限集合或 `role_permissions`；存量基础 permission 可保留，但运行时不依赖、目录不展示，也不要求批量回填或清理。
- 权限目录、角色写入校验、管理员最低权限不变量和超级管理员授权投影必须复用同一 tenant `access=permission` 判定。
- 多个 Action 可共享一个 permission，权限目录按 permission 去重并保留关联 Action ID。
- `system.assignable_permissions` Dynamic Reference Resolver 是可分配权限候选与版本摘要的唯一事实源；权限目录、角色创建、角色权限替换和 HTTP/Tool 最终校验都复用它。AI 调用 `permissions[]` 前必须在同一 Scope/Run 读取 `system.permission_catalog`，不得猜测通配符或历史权限代码。
- `system.permission_catalog` 的 HTTP 管理端保持完整 rich 树；可信 Tool Invocation 自动获得有界 compact 投影，不暴露由模型选择 rich/compact 的开关。关键词只用于定位稳定 Capability/PermissionGroup，最终权限集合必须按 capability/group 代码确定性枚举；超限响应按层提供下一步可用 capability/group 与固定 hint，不使用 cursor 或平行权限清单。
- `system.user_roles_update.role_ids[]` 使用 `system.assignable_roles` Resolver 与 `system.role_list` Discovery Action 作为第二 adopter，仅验证公共框架契约，不改变用户管理 UI。

## 写操作

- 角色资料、状态、权限替换和删除都在 Service 事务中完成，Handler 不散落 SQL。
- 权限替换先完整校验目录，再删除旧值并写入新值；成功后在同一事务按角色递增关联用户 `auth_version`。
- 角色启停同样在事务中触发关联用户授权失效。
- 有 UserRole 关联的角色拒绝删除，必须先由用户管理解除分配。

## 内置管理员保护

- 安装向导创建 `is_builtin=true && code=admin` 的角色，该角色不可删除、停用、修改 code 或编辑权限。
- 服务端授权投影以当前租户启用的内置 `admin` 角色事实为准，动态授予 Action Registry 中全部 tenant `access=permission` 权限；不得信任客户端角色 code，也不得绕过租户、用户、角色、会话或 `auth_version` 状态校验。
- 内置管理员读取时返回展开后的当前权限集合，保证 HTTP、菜单、Page Context、MCP/Tool 使用同一授权快照；新增权限在重新认证或刷新授权后生效，不依赖 `role_permissions` 快照。
- `adminMinimumPermissions` 仅保留架构保护与契约检查用途，且必须始终是当前 tenant 可分配权限集合的子集；authenticated 基础能力不得进入该集合。
- 安装向导负责创建初始租户、管理员、内置角色和绑定；Demo Seed 使用相同内置管理员授权语义，仅额外负责开发演示数据。
- Schema Migration 只维护字段、索引与约束，不创建或修补角色、管理员及权限等业务数据；未来存量升级另设版本化业务数据升级机制。
- 用户管理解除角色或停用用户时，须在同一事务中校验租户至少保留一个 enabled 的内置 `admin` 用户。
- 用户角色分配只能选择当前租户 enabled 角色；跨租户 role_id 注入必须失败。

## 前端

- 页面只提交 role ID、可编辑字段和权限目录返回的 permission，不允许自由输入权限字符串。
- 内置管理员明确展示“超级管理员 / 全部权限”只读状态，不渲染可编辑权限树或保存入口；有关联用户的角色禁用删除入口。
- 权限使用 `Capability → PermissionGroup → Permission/Action` 三级可折叠树展示，例如“系统 → 权限管理 → 角色列表/创建/编辑/删除”；父节点支持全选/半选，叶节点展示 permission 与说明，仅当关联 Action ID 不同于 permission 时额外展示 Action，避免默认同值重复，并支持搜索、已选数量和失败反馈。

## 验证

- PostgreSQL 测试覆盖两租户同 code、跨租户 ID、权限目录过滤、非法 permission、内置保护、有关联用户删除和 `auth_version` 递增。
- 运行后端测试、前端 typecheck/build、Agent 规则同步、静态检查与 `git diff --check`。
