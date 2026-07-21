# 多租户与鉴权上下文

- 租户/用户只从 Token/会话解析写入 `AuthContext`；**禁止**信任 Body/Query 的 `tenant_id`。
- **一用户一租户**；Action Access 为 public/authenticated/permission，空值默认 permission；permission 模式使用 Role ↔ permission 字符串（默认 `action.id`）。
- authenticated 仍须完成 Token/会话、AuthorizationProvider、租户/用户状态、realm 与 `auth_version` 校验，只跳过额外角色 permission。
- 正式 JWT 只表达可信身份并携带 `auth_version`；角色/权限必须由服务端 `AuthorizationProvider` 实时加载，禁止信任 Token 权限快照。
- 用户状态、密码或角色变化以及角色权限/状态变化，必须在同一事务调用统一授权版本递增入口；旧 Token 下一请求失效。
- 租户用户管理：username 创建后不可改；状态与资料编辑分离；角色全量替换；不提供物理删除；禁止自停用；须保留至少一名 enabled 内置 admin。
- 密码统一强度校验（8–72，含字母与数字）；明文/哈希不得进入响应、日志或审计。
- Dev Token 仅用于显式启用的开发环境，不代表生产 Token 失效语义，也不得进入生产 JWT 路径。
- Phase 1：Org/Dept 仅预留；禁止普通业务跨租户；tenant-owned DB 访问必须走 TenantDB/GuardedDatabase 强制边界。
- Realm 仅 `tenant` / `platform`；当前登录固定 tenant。缺 tenant 必须报错，跨租户只走经 platform realm + system operator 校验的 SystemDB。
- 预留租户绑定自定义域名（Phase 1 可不实现）。
- tenant 日志查询只使用声明可信 tenant filter 的 Provider，tenant_id 由 AuthContext 注入；混合租户或数据库原始日志必须 fail-closed。
- `agent_run_id` / `tool_invocation_id` 仅由 Agent/Tool Runtime 可信注入，用于审计和观测，不参与授权，也不接受客户端、模型或工具参数上报。
- tenant-owned/global/可信基础设施 Model 必须显式登记或分类；已登记 tenant-owned Model 只允许经 `TenantDB` Repository 或 `GuardedDatabase` 访问。裸 DB、缺失上下文、冲突 TenantID 必须 fail-closed；详见 `tenant-data-access`。

详规：`docs/development/auth-context.md`
