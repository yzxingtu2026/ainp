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
# 按需修改 database.dsn、auth.jwt.secret
```

3. 启动（首次会 AutoMigrate 系统表并写入种子数据）：

```bash
cd platform
go run ./cmd/ainp -config configs/config.yaml
```

## 种子账号

| 项 | 值 |
|----|-----|
| 租户 | `tenant_demo`（演示租户） |
| 用户名 | `admin` |
| 密码 | `Admin@123` |
| 角色 | `admin`（含 `demo.*` 与 `system.auth_*` 权限） |

种子幂等：已存在 `tenant_demo` 时跳过。

## 登录与鉴权示例

```bash
# 登录（成功 code=0，含 access_token；错误密码为 HTTP 200 + code=20002）
curl -s -X POST localhost:8080/api/v1/system/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Admin@123"}'

# 当前用户
curl -s localhost:8080/api/v1/system/auth/me \
  -H "Authorization: Bearer <access_token>"

# 权限列表
curl -s localhost:8080/api/v1/system/auth/permissions \
  -H "Authorization: Bearer <access_token>"

# 菜单（按权限过滤 directory/menu/button）
curl -s localhost:8080/api/v1/menus \
  -H "Authorization: Bearer <access_token>"

# 受保护 Action
curl -s -X POST localhost:8080/api/v1/demo/hello \
  -H "Authorization: Bearer <access_token>" \
  -H 'Content-Type: application/json' \
  -d '{"name":"AINP"}'
```

约定：

- `tenant_id` 只来自 Token，不信任 Body/Query。
- 无 Token → HTTP 401 / `code=10002`；无权限 → HTTP 403 / `code=10003`。
- 登录成功/失败均写审计（`system.auth_login`，`meta.audit=true`）；失败另打运行日志。

## 关闭开发态 Token 桩

配置项 `auth.dev_token_enabled`：

- 示例配置默认 `false`。
- **生产必须关闭**。
- 开启时可用：`Authorization: Bearer dev:<tenant_id>:<user_id>:<perm1,perm2,...>`
- 正式 JWT 与 `dev:` 格式互斥；非 `dev:` 前缀一律按 JWT 校验。

```yaml
auth:
  dev_token_enabled: false
  refresh_ttl: 720h
  jwt:
    secret: "<生产随机密钥>"
    issuer: ainp
    ttl: 15m
```

## 目录

见仓库 `docs/development/backend-layout.md`。

后端单元测试与集成测试统一位于 `tests/`，执行方式见 `tests/README.md`。

## VS Code / Cursor 一键开发

仓库已提供 `.vscode/tasks.json`：

1. 命令面板（`Cmd+Shift+P`）→ **Tasks: Run Task**
2. 选择：
   - `dev: all` — 后端 + 前端（均热重载）
   - `dev: backend` — 仅后端（Air 监听 `.go` 变更自动重启）
   - `dev: frontend` — 仅前端（Vite HMR）
3. 或打开 **运行和调试**，选择 `Dev: Backend + Frontend（热重载）`

前端默认：`http://127.0.0.1:9527`；后端：`http://127.0.0.1:8080`。
