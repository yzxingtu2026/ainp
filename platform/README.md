# AINP Platform（后端）

Go module：`github.com/yzxingtu2026/ainp/platform`

## 快速开始

1. 准备 Postgres，创建库与用户（示例）：

```sql
CREATE USER ainp WITH PASSWORD 'ainp';
CREATE DATABASE ainp OWNER ainp;
```

2. 复制配置：

```bash
cp configs/config.example.yaml configs/config.yaml
# 按需修改 database.dsn
```

3. 启动：

```bash
cd platform
go run ./cmd/ainp -config configs/config.yaml
```

## 开发态鉴权桩

当 `auth.dev_token_enabled: true`：

```text
Authorization: Bearer dev:<tenant_id>:<user_id>:<perm1,perm2,...>
```

示例：

```bash
curl -s localhost:8080/healthz

curl -s localhost:8080/api/v1/system/ping

curl -s -X POST localhost:8080/api/v1/demo/hello \
  -H 'Authorization: Bearer dev:t1:u1:demo.hello' \
  -H 'Content-Type: application/json' \
  -d '{"name":"AINP"}'

curl -s localhost:8080/api/v1/menus \
  -H 'Authorization: Bearer dev:t1:u1:demo.hello'
```

生产必须关闭 `dev_token_enabled`。

## 目录

见仓库 `docs/development/backend-layout.md`。
