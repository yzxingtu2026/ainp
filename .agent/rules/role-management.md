# 租户角色与权限管理

- Role/UserRole/RolePermission 操作必须从 AuthContext 取 tenant，并同时校验资源 tenant；禁止信任请求 tenant_id。
- permission 只来自 tenant realm、`access=permission` 的 Action Registry；按 permission 去重，public/authenticated 不展示、不写角色权限，Menu ID 不作为权限。
- 角色 code 创建后不可改；权限替换、启停和删除必须事务化，权限/状态变化同事务调用 #14 授权版本失效入口。
- 启用的内置 `admin` 不可删除/停用/编辑权限；授权投影动态授予当前全部 tenant permission，前端只读展示，普通角色仍使用持久化权限集合。
- 权限目录、角色写入、管理员最低权限子集与超级管理员投影复用同一 tenant `access=permission` 判定；authenticated 不得进入角色权限。
- 用户角色分配/停用须在事务中保证租户至少保留一名 enabled 内置 admin；只能分配当前租户 enabled 角色。
- 初始管理员、内置角色及绑定只由 Setup 创建（Demo Seed 仅限开发）；内置管理员权限运行时投影，Schema Migration/AutoMigrate 禁止初始化或回填业务权限数据。
- 前端权限选择复用 `Capability → PermissionGroup → Permission/Action` 三级折叠树，保留父级全选/半选与叶节点说明，不允许自由输入权限字符串。

详规：`docs/development/role-management.md`
