# Loki 日志 Provider 部署与采集

AINP 只查询外部 Loki，不负责部署 Loki，也不从服务器文件直接读取日志。应用与 Worker 继续输出 JSON stdout，由 Grafana Alloy、Promtail 或容器平台采集。

## AINP 配置

```yaml
log_center:
  loki:
    enabled: true
    endpoint: "https://loki.example.com"
    timeout: 5s
    username_env: LOKI_USERNAME
    password_env: LOKI_PASSWORD
    bearer_token_env: LOKI_BEARER_TOKEN
    tenant_header_env: LOKI_TENANT_HEADER
    tenant_label: tenant_id
    source_label: process
    application_value: application
    worker_value: worker
    service_label: service
    component_label: component
```

YAML 只填写环境变量名称。按 Loki 认证方式设置实际环境变量；Bearer Token 优先于 Basic Auth。`LOKI_TENANT_HEADER` 对应 Loki 的 `X-Scope-OrgID`，它是 Loki 集群租户，不等同于 AINP 的 `tenant_id`。

## 标签规范

tenant realm 查询依赖 Loki label 中可信的 `tenant_id`，缺少该 label 的日志不得通过租户日志中心返回。

建议 labels：

- `tenant_id`：AINP 租户隔离条件。
- `process`：固定为 `application` 或 `worker`。
- `service`、`component`、`level`：低基数检索字段。

以下高基数字段必须留在 JSON 日志正文，不得设为 labels：`request_id`、`trace_id`、`user_id`、`action_id`、`job_run_id`、`event_id`。

Promtail/Alloy pipeline 的等价逻辑示例：

```yaml
pipeline_stages:
  - json:
      expressions:
        tenant_id: tenant_id
        level: level
        service: service
        component: component
  - labels:
      tenant_id:
      level:
      service:
      component:
```

`process` 应由应用/Worker 各自的采集任务或容器标签静态注入，避免信任日志正文切换来源。

## 安全边界

- 前端不能提交原始 LogQL、endpoint 或 Header；Provider 只根据白名单 DTO 构造查询。
- Loki 只读凭据不得写入仓库、API 响应、运行日志或审计记录。
- Provider 查询强制附加 `tenant_id` 与 `process` selector。
- `/ready` 只用于返回可用/不可用摘要，不向前端透出 Loki 响应正文或内部地址。
