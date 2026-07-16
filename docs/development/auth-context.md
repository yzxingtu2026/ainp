# 多租户与鉴权上下文规范

Agent 只读要点：`.agent/rules/auth-context.md`。本文档定义租户隔离、身份注入与权限模型（含已拍板约定）。

## 目标

- Action / HTTP / Job / AI·MCP 均在明确的租户与身份上下文中执行。
- 与日志字段、`action.id` 权限、菜单按钮权限、审计边界一致。

## 已拍板结论

1. **租户只来自 Token/会话**，不信任请求体或 Query 中的 `tenant_id`。
2. **Org / Dept 仅预留字段**；行级数据权限后续按需扩展。
3. **Phase 1 禁止普通业务跨租户**；仅显式 System 入口可做平台级操作，且须审计。
4. **权限模型**：Role ↔ permission 字符串（默认等于 `action.id`）；先不做 ABAC/策略引擎。
5. **一用户一租户**（一个登录身份只隶属一个 `tenant_id`）。
6. **预留**租户绑定**自定义域名**（Phase 1 可不实现解析，模型留字段）。
7. **Realm 只有 `tenant` / `platform`**；当前登录固定为 `tenant`，平台入口后续独立建设。
8. 普通租户 Scope 缺失 `tenant_id` 必须报错；跨租户访问只允许显式 System Scope，且要求 platform realm。

## 主体模型

```text
System
  └── Tenant
        └── Organization（预留）
              └── Department（预留）
                    └── User（一用户一租户）
```

| 概念 | Phase 1 |
|------|---------|
| Tenant | 强制隔离边界 |
| User | 强制；系统任务可无 user |
| Role / Permission | 强制；permission 多为 `action.id` |
| Organization / Department | 字段预留，不做行级权限 |
| 自定义域名 | Tenant 上预留，见 §域名 |

## AuthContext

经框架写入 `context.Context`，业务只读：`auth.FromContext(ctx)`。普通登录 Token 使用 audience `ainp:tenant-console`；Token 同时校验 issuer、audience、realm 与正整数 `auth_version`。JWT 只表达已签名的 `tenant_id + user_id + realm + auth_version`，不携带或信任角色/权限快照。该协议启用后旧 Token 需要重新登录。

| 字段 | 必填 | 说明 |
|------|------|------|
| `request_id` | 是 | 入口生成 |
| `trace_id` | 是 | 可与 request 相同 |
| `tenant_id` | 是 | 当前租户（System 入口另规） |
| `realm` | 是 | 当前执行域：`tenant` 或 `platform` |
| `user_id` | 条件 | 用户请求必填；job 可空 |
| `org_id` | 否 | 预留 |
| `dept_id` | 否 | 预留 |
| `roles` | 否 | 角色列表（日志/展示） |
| `session_id` | 否 | 会话 |
| `client` | 否 | `web` / `mcp` / `agent` / `job` |
| `is_system_operator` | 否 | 仅 System 入口可为 true |

**禁止**业务代码自行 `WithValue` 篡改 `tenant_id` / `user_id` / `is_system_operator`。

## 信任边界

| 来源 | 做法 |
|------|------|
| Web | `Authorization` Bearer（或约定 Session）→ 验签/查会话 → 得到唯一 `user_id` + `tenant_id` |
| MCP / Agent | 服务端校验后的用户或服务凭证 → 同样注入上下文 |
| Job | 入队时从当时上下文**拷贝** `tenant_id`（及可选 `user_id`）；消费端只信任该拷贝 |

硬规则：

- 不存在「请求里带 tenant_id 就切换租户」。
- 因一用户一租户，**Phase 1 不做租户切换 API**。
- 仓储/GORM：默认按 `tenant_id` Scope；缺失租户上下文直接报错，不得退化为无条件查询。
- Action 内取租户：只从 AuthContext。

## 鉴权流程

```text
验 Token/会话 → AuthorizationProvider（tenant + user + realm + auth_version）
  → 实时校验租户/用户状态与版本
  → 加载当前 roles / permissions → AuthContext
  → Action access（public / authenticated / permission）
  → permission 模式下校验 permission（默认 action.id）
  → Handler(ctx, input)
  → 审计（meta.audit）
```

| 失败 | HTTP / code |
|------|-------------|
| 未认证 | 401 / `10002` |
| 无权限 | 403 / `10003` |
| 租户停用等 | 403 或 `2xxxx`（如 `20001`） |

正式 JWT 的 Token 无效、缺少授权版本、版本过期、用户停用或租户停用均按未认证处理，避免继续使用旧会话；已经通过身份校验但缺少 Action permission 才返回 forbidden。

前端按钮按 permission 显隐；**后端 Action 必须按 access 再鉴权**。`authenticated` 仍必须经过正式 Token/会话、AuthorizationProvider、租户/用户状态与 `auth_version` 校验，只跳过额外角色 permission。

## 授权版本与失效

- `users.auth_version` 从 `1` 开始，只能递增；登录签发 JWT 时写入当前版本。
- 每个正式请求验签后，`AuthorizationProvider` 必须按 `tenant_id + user_id` 实时读取用户、租户、有效角色和权限，并比较版本。
- 用户启停、密码重置、用户角色变化使用单用户版本递增入口。
- 角色权限或状态变化使用按租户角色批量递增入口。
- 版本递增必须与对应业务变更共用一个数据库事务，禁止 Handler 散落 SQL 或提交后再补失效。
- Phase 1 以数据库实时读取保证正确性；未来缓存必须具备精准失效，并保持 Provider 契约不变。
- `dev:` Token 仅在 `dev_token_enabled=true` 时使用，不经过正式 JWT Provider 流程，不得据此推断生产安全语义。

## 租户用户管理

- 用户属于单一租户；`(tenant_id, username)` 联合唯一，username 创建后不可修改。
- 管理 Action：`system.user_list|get|create|update|status_update|roles_update|password_reset`；状态变更与资料编辑分离。
- 角色全量替换并事务化：校验角色属于当前租户且 enabled → 替换 UserRole → `BumpAuthVersion`。
- 不提供物理删除；使用 `disabled` 保留审计主体。当前用户不可自停用；租户须保留至少一名 enabled 内置 `admin` 用户。
- OrgID/DeptID 仅预留字段，本阶段不实现组织架构选择。

## 密码强度

- 统一由 `ValidatePassword` 校验（安装向导、创建用户、密码重置共用）：长度 8–72，至少包含一个字母和一个数字。
- 密码只接收明文用于 bcrypt；API/日志/审计/响应禁止回显明文或 `PasswordHash`。
- 前端仅做一致提示；后端为最终边界。

## 系统级操作

- 普通 Action：`is_system_operator=false`，强制单租户。
- 平台超管 / 运维入口：必须是已认证 platform realm，并通过显式 System Scope/Repository，设 `is_system_operator`，**强制审计**，禁止复用普通业务 Action 做全租户扫描。
- 多租户批处理：按租户循环 `WithTenant`，禁止无租户上下文写库。

## 租户自定义域名（预留）

Phase 1 可不实现解析与证书，但 Tenant 模型预留例如：

| 字段 | 说明 |
|------|------|
| `primary_domain` | 可选，租户主域名 |
| `custom_domains` | 可选列表，绑定的自定义域名 |
| `domain_verify_status` | 预留：待验证/已验证等 |

后续扩展时：

- 按 `Host` 解析到 `tenant_id`，再与 Token 内 `tenant_id` **交叉校验**（不一致则拒绝）。
- 仍不得仅凭 Host 在无凭证时执行写操作。
- 规范细化时可单开 `docs/development/tenant-domain.md`。

## 与日志 / 审计

运行日志自动带 `tenant_id`、`user_id`、`request_id`、`action_id` 等（见 `logging.md`）。  
审计事件须含租户与操作者等最小集（见 `audit.md`）。

## 前端

- 只携带 Token；业务 API 不传「当前 tenant_id」。
- 菜单/按钮 permission 对齐 `action.id`。

## 检查清单

- [ ] 上下文仅由框架注入；未信任 Body 中的 tenant_id
- [ ] 一用户一租户；未实现违规的跨租户切换
- [ ] Action 鉴权使用 permission 字符串
- [ ] DB 查询带租户 Scope
- [ ] 正式 JWT 不携带/信任 roles 与 permissions，授权来自当前 Provider 快照
- [ ] 权限相关写操作与 `auth_version` 递增处于同一事务
- [ ] Tenant 模型已预留域名字段（即使暂未使用）

## 变更

调整隔离或权限模型时，同步更新本文档与 `.agent/rules/auth-context.md`，并 `agent-rules sync`。
