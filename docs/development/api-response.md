# API 响应与错误规范

Agent 只读要点：`.agents/rules/api-response.md`。本文档定义 HTTP 信封、错误码分段与 Action 错误映射。

## 目标

- SPA、OpenAPI、后续 MCP/Agent 适配器共用同一套成功/失败语义。
- 业务禁止在 Gin Handler 里手写互不一致的 JSON。
- 错误可映射到日志 `request_id` 与后续审计。

## 响应信封

所有 JSON API（除文件下载、SSE 等另有约定的端点）使用：

```json
{
  "code": 0,
  "message": "成功",
  "message_key": "common.ok",
  "data": {},
  "request_id": "req_xxx"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | `0` 成功；非 `0` 为平台/业务错误码 |
| `message` | string | 已按请求语言本地化的短文案；失败时由 `message_key` 解析填充 |
| `message_key` | string | 稳定机器键（omitempty）；如 `auth.invalid_credentials`；客户端可用于精确分支 |
| `data` | any / null | 成功载荷；失败时为 `null`，或带结构化细节 |
| `request_id` | string | 与运行日志一致，中间件注入 |

字段名固定 **snake_case**，不得改为驼峰。

### message_key 约定

- 格式：`domain.reason`，小写点分，如 `auth.invalid_credentials`、`common.validation_failed`。
- 与 `code` 是多对一关系：同一错误码可对应多个 key。
- 未显式传 key 时，框架按 `code` 自动映射默认 key（见 `framework/errs/codeDefaultKey`）。
- `message_key` 为 omitempty：成功响应也携带；旧客户端仅读 `message` 仍可用。

## 语言协商与多语言体系

### 翻译文件

- AINP 基础翻译为独立 YAML，存放在 `platform/i18n/` 并通过 `embed.FS` 随 Go Module 版本发布。
- 文件名即语言标识：`zh-CN.yaml` → `zh-CN`，`en.yaml` → `en`。
- 扁平 key 结构：`domain.reason` 小写点分，便于 Agent 逐行增删。
- 正式 Host 启动时通过 owner-aware `i18n.LoadSources()` 加载内嵌基线与下游 Module `fs.FS`；下游 key 必须使用 `{module-id}.` 前缀，重复 owner、跨 owner 冲突、空值或非法 YAML 均 fail-closed。

### 语言协商

- 中间件 `middleware.AcceptLanguage()` 解析 `Accept-Language` 请求头，将协商结果写入 `AuthContext.Language`。
- 支持：`zh-CN` / `zh` → `zh-CN`；`en` / `en-US` / `en-GB` → `en`；其他回退到默认语言。
- 默认语言：`config.I18n.DefaultLanguage`，未配置时使用 `zh-CN`。
- 响应时 `message` 由 `i18n.T(lang, message_key)` 解析；缺翻译时回退 `zh-CN` → key 本身。

### 覆盖范围

所有后端用户可见字符串均通过 i18n key 本地化：

| 场景 | 字段 | key 格式 |
|------|------|----------|
| API 错误文案 | `message` | `auth.invalid_credentials` |
| Capability 名称 | `NameKey` | `capability.system.name` |
| Capability 描述 | `DescriptionKey` | `capability.system.description` |
| Action 名称 | `NameKey` | `action.system.auth_login.name` |
| Action 描述 | `DescriptionKey` | `action.system.auth_login.description` |
| 菜单标题 | `TitleKey` | `menu.system.root.title` |
| 业务响应文案 | 自定义 | `demo.hello_greeting` |

### Reload API

正式内嵌资源不可变，调用重载端点会拒绝操作。仅受信运维人员以 `-dev-runtime-assets` 显式启动 API/Worker 时，`config.i18n.dir` 才作为开发目录生效，调用重载端点可原子读取该目录；普通租户无法注册资源源或选择目录：

```
POST /api/v1/system/i18n/reload
```

返回：

```json
{
  "code": 0,
  "message": "成功",
  "data": { "langs": ["en", "zh-CN"], "key_count": 38 },
  "request_id": "req_xxx"
}
```

### 业务 handler 中使用

```go
lang := i18n.FromCtx(ctx)
greeting := i18n.T(lang, "demo.hello_greeting")
```

## HTTP 状态与 `code` 分工

| 场景 | HTTP | `code` |
|------|------|--------|
| 成功 | 200 | `0` |
| 业务可预期失败（含领域资源不存在、业务规则拒绝） | 200 | 非 `0` |
| 未认证 | 401 | `10002`（或等同通用码） |
| 无权限 / 租户不允许 | 403 | `10003` |
| 限流 | 429 | 通用限流码 |
| 路由不存在（非领域资源） | 404 | 可选固定码 |
| 未归类系统错误 | 500 | `50000` 段 |

**写死约定：**

- 领域资源不存在 → **HTTP 200 + `code = 10004`**（不要用 HTTP 404 表示业务实体缺失）。
- 前端：先看 HTTP 处理登录过期/网络；再看 `code` 处理业务分支。
- 禁止「同一类业务错误有时 200 有时 4xx」的混用。

## 分页（在 `data` 内）

列表请求统一使用 `page` / `page_size`；缺省值和上限由 `framework/pagination.Request.Normalize` 维护。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "total": 100,
    "page": 1,
    "page_size": 20
  },
  "request_id": "req_xxx"
}
```

列表接口统一使用 `pagination.Result[T]` 产生 `items` / `total` / `page` / `page_size`，禁止再发明 `list`/`rows`/`records`/`current`/`size` 等多种键名。分页结果不负责 HTTP 输出，外层仍由 `response.OK` 统一包装。

## 错误码分段

整数常量，集中定义（如 `platform/framework/errs` 或等价包）：

| 区间 | 含义 |
|------|------|
| `0` | 成功 |
| `1xxxx` | 通用：参数、认证、权限、不存在、限流等 |
| `2xxxx` | 租户 / 组织 / 账号 |
| `3xxxx` | Action / Metadata / 权限引擎 |
| `4xxxx` | 业务域（按 capability 再细分） |
| `5xxxx` | 系统 / 依赖 / 内部错误 |

通用码示例（落地时可微调编号，但须进常量文件）：

| code | 含义 |
|------|------|
| `10001` | 参数校验失败 |
| `10002` | 未认证 |
| `10003` | 无权限 |
| `10004` | 资源不存在 |
| `20001` | 租户停用 |
| `20002` | 账号或密码错误 |
| `20003` | 用户停用 |
| `30001` | Action 未注册 |
| `50000` | 内部错误 |

新增业务码：在对应分段内追加，并在模块文档或错误码表中登记，禁止魔法数。

## 校验失败 `data` 形态

```json
{
  "code": 10001,
  "message": "参数校验失败",
  "message_key": "common.validation_failed",
  "data": {
    "fields": [
      { "field": "role_ids[0]", "message": "UUID 格式不正确", "message_key": "validation.uuid" }
    ]
  },
  "request_id": "req_xxx"
}
```

- 顶层错误固定为 `CodeValidation`（`10001`）与 `common.validation_failed`。
- `fields` 一次返回全部结构错误；`field` 使用 JSON 路径，不暴露 Go 字段名。
- 未知字段使用 `validation.unknown_field`；非法 JSON、类型错误和多个 JSON 值使用 `validation.invalid_json`。
- 字段 `message` 由 `response.Fail` 按 `message_key` 和请求语言本地化。

## Action 与 `AppError`

- Action Handler 返回 `(data, error)`；可预期失败返回框架错误类型，例如：

```text
AppError { Code, MessageKey, Message, Fields, Cause }
```

- Gin / 未来 MCP、Agent 适配器将 `AppError` **统一映射**为上述信封（或 Tool 错误），不各自翻译。
- Handler **禁止**直接 `c.JSON` 拼信封；统一 `response.OK(ctx, data)` / `response.Fail(ctx, err)`（包名以落地为准）。
- `Cause` 只写运行日志，不进入对外 `message`（5xx / 内部错误对外固定短句）。

## 安全与日志

- 5xx 对外不暴露内部堆栈或 SQL 细节。
- 失败响应带同一 `request_id`，并打运行日志（业务失败多为 Warn，系统错误为 Error）。
- 需合规追溯的失败另走 Audit，不靠响应体充当审计。

## 非 JSON 端点

文件下载、SSE、WebSocket 等可不使用本信封，但须在接口文档标明；若失败可转为 JSON 信封，须仍带 `request_id`。

## 检查清单

- [ ] 响应含 `code` / `message` / `message_key` / `data` / `request_id`
- [ ] `message` 已按请求语言本地化；`message_key` 为稳定机器键
- [ ] 业务失败为 200 + 非 0；未混用随意 HTTP 4xx
- [ ] 资源不存在为 `10004` + 200
- [ ] 错误来自 `AppError` 常量码，无魔法数
- [ ] 无手写不一致 JSON；内部错误未泄露 Cause

## 变更

调整信封字段、HTTP 分工或码段时，同步更新本文档与 `.agents/rules/api-response.md`，并 `agent-rules sync`。
