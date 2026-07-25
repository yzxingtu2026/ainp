# 后端统一日志规范

Agent 只读要点：`.agent/rules/logging.md`。本文档为运行日志与审计边界的详规。

## 目标

- 全平台一套 Logger 门面，便于排障、采集与 Agent 稳定生成代码。
- 与**审计日志**分离，避免用 Info 日志冒充合规追溯。
- 与 HTTP、Action 执行链、异步任务等入口自动关联上下文。

## 两类日志

| 类型 | 用途 | 落点 | API |
|------|------|------|-----|
| 运行日志 | 排障、观测、告警 | stdout + 可轮转 JSONL 文件 | 框架 `Logger`（`slog`） |
| 审计日志 | 谁、何租户、对何 Action、结果如何 | DB 或专用存储，可查询与保留 | 框架 `Audit`（另文/后续包） |

业务代码不得用运行日志替代审计；审计实现未就绪前，也不得用 `fmt.Println` 临时代替运行日志。

## 技术选型

| 项 | 选择 |
|----|------|
| 门面 | 项目封装（建议包路径 `platform/framework/logging` 或等价） |
| 实现 | Go 标准库 `log/slog` |
| 生产格式 | stdout 与文件均为 JSON；文件固定单行 JSONL |
| 本地开发 | text 或 JSON，由配置切换 |
| 替换策略 | 业务只依赖门面；Handler 日后可换（如 zap 适配），不改调用方 |

禁止：业务直接依赖 zap/logrus/zerolog；禁止零散 `print` / `fmt.Print*` / 标准库 `log` 包作为业务日志。

生产配置可组合 `outputs: [stdout, file]`。文件 Sink 使用并发安全轮转 Writer，支持 `max_size_mb`、`max_backups`、`max_age_days` 和 `compress`；文件初始化或写入失败只向 stderr 输出 `logging ... degraded` 并继续服务。API、Worker 必须使用不同路径，并通过 `service`、`instance_id` 区分进程和副本。进程退出尽力关闭 Writer。

```yaml
logging:
  level: info
  format: json
  outputs: [stdout, file]
  file:
    path: /var/lib/ainp/logs/api.jsonl
    max_size_mb: 100
    max_backups: 14
    max_age_days: 14
    compress: true
```

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
| `service` / `instance_id` | 进程类型与副本标识 |

`request_id` 表示一次 HTTP/RPC/Job 执行，`trace_id` 表示跨 HTTP、Outbox、Job、Agent Run 和 Tool Invocation 的链路。异步执行必须生成自己的 `request_id` 并继承原始 `trace_id`。这些高基数字段只作为 JSON 字段，不作为集中日志 label。

## 内部诊断错误

`framework/diagnostic.Error` 将对外 `AppError` 与内部诊断分离，提供稳定 `error_category`、`error_stage`、`safe_summary`、白名单 `attributes` 和可 `errors.Is/As/Unwrap` 的 cause。分类描述原因，stage 描述位置；业务不得通过字符串解析类型或创建同义分类。公共最终边界负责 Error 日志，Service/Adapter 只包装诊断元数据，避免逐层重复输出。

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
- URL query/fragment、请求/响应正文、SQL 参数和连接串

Logger 对敏感键统一替换为 `***`，普通字符串属性最多 1024 字节；诊断属性还必须通过固定白名单。外部 URL 只记录 host 与无 query/fragment 的路径。

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
- 当前 demo 同时持久化 `/var/lib/ainp/logs/api.jsonl` 与 `worker.jsonl`，不引入 Loki。宿主机使用 `deploy/demo/search-logs.sh request_id <id>` 或 `trace_id <id>` 检索当前及 `.gz` 历史轮转文件；脚本只读取 `.env` 声明的 `AINP_LOG_DIR`。
- 容量上限需按 `service × instance × max_size_mb × (max_backups + 1)` 估算；部署负责人必须监控日志目录磁盘空间。压缩会降低历史占用，但不能替代容量告警。
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
- 当前阶段不注册日志中心 Action、HTTP API、权限组或菜单；只保留 `framework/logcenter` Provider 协议、脱敏能力与前端页面骨架。恢复产品入口必须另开 Issue，并重新评估可信 Provider、租户隔离与审计边界。

### 关联跳转协议

保留页面骨架 `/operations/logs` 仅用于未来恢复开发；当前没有菜单与后端 API，不属于可用产品入口。未来恢复时，URL query 预填参数仍必须经过服务端白名单和范围校验。

## 检查清单

- [ ] 新代码只通过框架 Logger 打运行日志
- [ ] 无 `print` / 业务直连第三方日志库
- [ ] HTTP / Action / 任务入口已带上下文字段
- [ ] 无密钥与明文证件；AI 载荷已截断或脱敏
- [ ] 需合规追溯的操作有 Audit 规划，而非仅 Info

## 变更

调整门面、字段表或级别语义时，同步更新本文档与 `.agent/rules/logging.md`，并 `agent-rules sync`。
