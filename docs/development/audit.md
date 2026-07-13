# 审计日志规范（最小集）

Agent 只读要点：`.agent/rules/audit.md`。本文档定义审计与运行日志的边界，以及 Phase 1 字段最小集。

## 目标

- 回答：谁、在哪个租户、对哪个 Action、结果如何。
- 与 `logging`、`auth-context`、`action-metadata`（`meta.audit`）对齐。
- 不替代排障用的运行日志。

## 与运行日志分工

| | 运行日志 | 审计 |
|--|----------|------|
| 用途 | 排障、观测 | 合规、追责、安全追溯 |
| 落点 | stdout / 采集 | DB 或专用存储（可查询、保留策略） |
| 内容 | 可含 debug、耗时、错误栈（脱敏后） | 稳定业务事实，字段固定 |

禁止用 `Info` 日志冒充审计。

## 何时写入

- Action 的 `meta.audit == true` 时，在 **Action 执行链统一写入**（成功与失败都写）。
- 写操作建议默认 `audit=true`；只读 list/get 可 `false`。
- 鉴权失败（已能识别身份或至少有 request）：建议记一条 `result=failure`，便于审计「谁试图做什么」。
- 业务 Handler **禁止**自行插入审计表；只通过框架 Audit API。

## 字段最小集（必填）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 审计记录唯一 ID |
| `occurred_at` | time | 发生时间，存 UTC |
| `tenant_id` | string | 租户；System 操作可空或平台约定值，须可区分 |
| `user_id` | string | 操作者；系统任务可空 |
| `actor_type` | string | `user` / `system` / `agent` / `job` |
| `action_id` | string | 如 `purchase.order_create` |
| `result` | string | `success` / `failure` |
| `request_id` | string | 与 API 信封、运行日志关联 |

## 强烈建议（Phase 1 建议具备）

| 字段 | 说明 |
|------|------|
| `permission` | 实际校验的权限码（通常 = `action_id`） |
| `error_code` | 失败时的业务错误码；成功为空 |
| `resource_type` | 如 `purchase_order` |
| `resource_id` | 业务主键 |
| `client` | `web` / `mcp` / `agent` / `job` |
| `summary` | 短句摘要，无密钥与大段敏感正文 |

## 禁止写入审计的内容

- 密码、Token、Authorization 全量
- 完整 Request/Response Body、Prompt/Tool 全文
- Stack trace、SQL 原文

上述走运行日志并按 `logging.md` 脱敏/截断。

## 与 AuthContext 的关系

审计字段从执行时的 `AuthContext` 与 Action 结果填充，不信任客户端上报的「审计内容」。

## 保留与查询（原则）

- 须支持按 `tenant_id`、`action_id`、`user_id`、`occurred_at`、`request_id` 查询。
- 保留周期与归档策略后续在运维/合规文档中定；实现上预留按租户清理的能力。

## 检查清单

- [ ] `meta.audit=true` 的 Action 有统一审计写入
- [ ] 最小集字段齐全；失败含 `result=failure`
- [ ] 无敏感大字段；未用 Logger 冒充审计
- [ ] 业务代码未直接写审计表

## 变更

调整字段或写入时机时，同步更新本文档与 `.agent/rules/audit.md`，并 `agent-rules sync`。
