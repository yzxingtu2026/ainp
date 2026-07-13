# AINP Frontend

基于 [Soybean Admin](https://github.com/soybeanjs/soybean-admin)（Naive UI）的管理端壳，对接 AINP `platform` 统一 API 信封、登录 JWT、动态菜单与按钮权限。

规范见：

- `docs/development/frontend-stack.md`
- `docs/development/icon-library.md`
- `docs/development/api-response.md`

## 环境要求

- Node.js `>= 20.19`
- pnpm `>= 10.5`（必须用 pnpm，勿用 npm/yarn）

## 快速开始

1. 启动后端（Postgres + platform），见 `platform/README.md`。
2. 安装并启动前端：

```bash
cd frontend
cp .env.example .env.test   # 若本地尚无 env；仓库已带默认联调配置时可直接 pnpm i
pnpm i
pnpm dev
```

默认开发地址：`http://localhost:9527`（Vite 代理到 `http://127.0.0.1:8080`）。

也可在 Cursor/VS Code：**Tasks: Run Task** → `dev: frontend` 或 `dev: all`（见仓库 `.vscode/tasks.json`）。

## 联调账号

与后端种子一致（详见 `platform/README.md`）：

| 项 | 值 |
|----|-----|
| 用户名 | `admin` |
| 密码 | `Admin@123` |

登录页已预填演示账号；也可点「演示管理员」一键填入。

## 对接说明

| 能力 | 后端接口 |
|------|----------|
| 登录 | `POST /api/v1/system/auth/login` |
| 当前用户 | `GET /api/v1/system/auth/me` |
| 菜单/按钮 | `GET /api/v1/menus` |
| Demo | `POST /api/v1/demo/hello`（页内按钮权限 `demo.hello`） |

- 请求层解析信封 `code` / `message` / `data` / `request_id`；`code === 0` 为成功。
- HTTP 401 / 业务码 `10002` 会清理登录态并回到登录页。
- `VITE_AUTH_ROUTE_MODE=dynamic`：侧栏菜单以后端 Metadata 为主；本地保留 `home` 欢迎页。
- 图标为 Iconify `collection:name`（默认 `mdi`）。

## 常用脚本

```bash
pnpm dev        # 开发（mode=test）
pnpm build      # 生产构建
pnpm typecheck  # 类型检查
```
