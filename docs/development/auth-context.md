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

经框架写入 `context.Context`，业务只读：`auth.FromContext(ctx)`。普通登录 Token 使用 audience `ainp:tenant-console`；Token 同时校验 issuer、audience 与 realm。该协议启用后旧 Token 需要重新登录。

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
验 Token/会话 → AuthContext
  → 租户状态（启用等）
  → permission（默认 action.id）
  → Handler(ctx, input)
  → 审计（meta.audit）
```

| 失败 | HTTP / code |
|------|-------------|
| 未认证 | 401 / `10002` |
| 无权限 | 403 / `10003` |
| 租户停用等 | 403 或 `2xxxx`（如 `20001`） |

前端按钮按 permission 显隐；**后端 Action 必须再鉴权**。

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
- [ ] Tenant 模型已预留域名字段（即使暂未使用）

## 变更

调整隔离或权限模型时，同步更新本文档与 `.agent/rules/auth-context.md`，并 `agent-rules sync`。
