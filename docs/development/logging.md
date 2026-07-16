# 后端统一日志规范

Agent 只读要点：`.agent/rules/logging.md`。本文档为运行日志与审计边界的详规。

## 目标

- 全平台一套 Logger 门面，便于排障、采集与 Agent 稳定生成代码。
- 与**审计日志**分离，避免用 Info 日志冒充合规追溯。
- 与 HTTP、Action 执行链、异步任务等入口自动关联上下文。

## 两类日志

| 类型 | 用途 | 落点 | API |
|------|------|------|-----|
| 运行日志 | 排障、观测、告警 | stdout（JSON）/ 日志采集 | 框架 `Logger`（`slog`） |
| 审计日志 | 谁、何租户、对何 Action、结果如何 | DB 或专用存储，可查询与保留 | 框架 `Audit`（另文/后续包） |

业务代码不得用运行日志替代审计；审计实现未就绪前，也不得用 `fmt.Println` 临时代替运行日志。

## 技术选型

| 项 | 选择 |
|----|------|
| 门面 | 项目封装（建议包路径 `platform/framework/logging` 或等价） |
| 实现 | Go 标准库 `log/slog` |
| 生产格式 | JSON，写 stdout |
| 本地开发 | text 或 JSON，由配置切换 |
| 替换策略 | 业务只依赖门面；Handler 日后可换（如 zap 适配），不改调用方 |

禁止：业务直接依赖 zap/logrus/zerolog；禁止零散 `print` / `fmt.Print*` / 标准库 `log` 包作为业务日志。

## 上下文与字段

经 `context.Context` 注入，中间件 / Action 链自动合并到日志属性。字段名统一 **snake_case 英文**：

| 字段 | 说明 |
|------|------|
| `request_id` | 单次 HTTP/RPC 请求 |
| `trace_id` | 跨服务/异步追溯（可与 request 相同或 W3C trace） |
| `tenant_id` | 租户 |
| `user_id` | 操作者，可空（系统任务） |
| `action_id` | 如 `purchase.create` |
| `capability` | 能力域 |
| `module` | 子系统或包名 |
| `err` | 错误对象/消息（Error 级） |
| `duration_ms` | 耗时（请求/Action 结束时） |

业务可追加领域字段，但不得覆盖上表语义，不得使用驼峰或中文 key。

## 级别

| 级别 | 何时用 |
|------|--------|
| Debug | 开发细节；生产默认关闭 |
| Info | 关键路径里程碑（请求/Action 成功摘要） |
| Warn | 可恢复异常、降级、重试 |
| Error | 失败需关注；带 `err`，保留堆栈信息（按门面约定） |

默认不打每一笔成功 SQL；慢查询、失败查询按配置提升到 Warn/Error。

## 必须接入的入口

1. Gin（或统一 HTTP）中间件：请求开始/结束、状态码、`duration_ms`
2. Action 执行链：开始、成功、失败（含 `action_id`）
3. Asynq 任务 / gocron 任务：任务名、重试次数、成败
4. 外部依赖（LLM、MCP、重要 HTTP 客户端）：摘要级 Info/Warn，失败 Error
5. Panic 恢复：统一 Error + stack

## 脱敏与禁止输出

禁止写入运行日志：

- 密码、刷新令牌、API Key、Authorization 头全量
- 身份证、银行卡等明文证件号
- 大段用户隐私正文（按需哈希或截断）

AI 相关：

- Prompt / Tool 入参默认**截断**（长度阈值在配置，如 512～2048 字符）
- 全量载荷仅 Debug 且仅非生产，或走专用调试开关
- 审计若需留存 AI 操作，走 Audit，并单独定义保留与脱敏策略

## 调用约定（示意）

```go
import "your.module/platform/framework/logging"

logging.Info(ctx, "purchase created", "purchase_id", id)
logging.Error(ctx, "purchase create failed", err, "purchase_id", id)
```

- 第一个参数为 `ctx`，以便合并租户/请求字段。
- 消息用简短英文或稳定中文短句，同一事件保持文案稳定，便于检索。
- 成对 key-value 追加字段，避免把结构化数据塞进消息字符串。

包名以仓库落地为准；实现合并后更新本节路径。

## 审计最小预期（与 Logger 对齐）

审计事件字段与写入时机见 `docs/development/audit.md`。本规范仅要求：**不得用运行日志冒充审计**。

## 日志中心查询架构

- 运行日志继续输出结构化 stdout，由 Loki/OpenSearch 等外部系统采集；AINP 业务数据库不得作为运行日志主存储。
- 框架层 `framework/logcenter` 定义只读 Provider、来源 Registry、统一 `Entry/Query/Page` 和错误语义；业务模块不得按 Provider 类型分支，也不得透传 Loki/SQL 等原生 DSL。
- 日志中心不读取任意服务器文件，不返回 Provider 凭证、服务器路径，不提供 Shell、SQL 或命令执行能力。
- 操作审计、运行日志、JobRun、Outbox Event 各自保持独立事实源，关联只使用稳定 ID，不复制日志正文。

### 租户与平台边界

- tenant realm 只列出声明 `TenantFilter=true` 的 Provider，并强制将 AuthContext `tenant_id` 注入查询；Provider 返回其他租户数据时 fail-closed。
- PostgreSQL 原始日志、混合租户 Worker 日志等无法证明租户隔离的来源不得暴露给 tenant Action。
- platform realm 的跨租户入口、权限和 Provider 由后续平台控制台单独实现，不得借用 tenant 入口降级开放。

### 统一查询限制

- 默认最近 15 分钟，最大跨度 24 小时，分页最多 100 条，Provider 超时 5 秒，单次结构化结果最多 1 MiB。
- 白名单筛选：level、service、component、keyword、request/trace/action ID、job type/run ID、event ID、source。
- 查询端统一对 password、Token、Authorization、连接串、API Key、证件/银行卡字段脱敏；消息最多 4096 字节，属性值最多 1024 字节。
- `system.log_source_list/query/get` 使用 `system.operations.logs` 权限组并强制审计。

### 关联跳转协议

日志中心使用 `/operations/logs`，允许后续页面通过 URL query 预填：`source`、`start_at`、`end_at`、`request_id`、`trace_id`、`action_id`、`job_type`、`job_run_id`、`event_id`。这些参数仍经过服务端白名单和范围校验。

Loki Provider 的启用、凭据环境变量、labels 与采集 pipeline 见 `docs/deployment/loki.md`。前端不得提交原始 LogQL；应用与 Worker Provider 由服务端固定附加 tenant/source selector。

## 检查清单

- [ ] 新代码只通过框架 Logger 打运行日志
- [ ] 无 `print` / 业务直连第三方日志库
- [ ] HTTP / Action / 任务入口已带上下文字段
- [ ] 无密钥与明文证件；AI 载荷已截断或脱敏
- [ ] 需合规追溯的操作有 Audit 规划，而非仅 Info

## 变更

调整门面、字段表或级别语义时，同步更新本文档与 `.agent/rules/logging.md`，并 `agent-rules sync`。
