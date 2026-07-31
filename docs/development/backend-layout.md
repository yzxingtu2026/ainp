# 后端目录与模块边界

Agent 只读要点：`.agent/rules/backend-layout.md`。本文档为已拍板的 Go 骨架布局。

## 已拍板结论

1. **`platform/` 与 `frontend/` 平级**（前端不放在 platform 内）。
2. **Go module path**：`github.com/yzxingtu2026/ainp/platform`。
3. **Phase 1 modules**：`system`（配置与身份）、`operations`（运行态运维）+ `demo`；采购等业务域稍后。
4. **骨架第一步即接 GORM + PostgreSQL**。
5. **配置格式**：YAML（提供 `configs/config.example.yaml`）。
6. **后端测试统一归档**：所有 `_test.go` 放入 `platform/tests/`，使用外部测试包通过公开 API 验证行为；迁移时合并重复覆盖并记录替代位置，禁止仅为测试导出生产私有实现。

## 仓库顶层

```text
ainp/
├── platform/                 # Go module
│   ├── go.mod
│   ├── cmd/
│   ├── framework/
│   ├── modules/
│   ├── tests/                # 统一后端单元/集成测试
│   ├── configs/
│   └── migrations/           # 迁移文件（随库表演进）
├── frontend/                 # Soybean（后续）
├── docs/
└── .agent/
```

## Phase 1 目录

```text
platform/
├── go.mod
├── cmd/
│   └── ainp/
│       └── main.go           # 组装与启动
├── configs/
│   └── config.example.yaml
├── migrations/
├── framework/
│   ├── app/                  # Register*、Boot、Run
│   ├── config/               # YAML 加载
│   ├── logging/              # slog 门面
│   ├── response/             # API 信封
│   ├── errs/                 # AppError 与码段
│   ├── validation/           # 强类型 DTO 结构校验与字段错误映射
│   ├── pagination/           # 统一分页 Request、默认值与泛型 Result
│   ├── auth/                 # AuthContext
│   ├── tenant/               # TenantDB/SystemDB、Registry、Callback、Repository
│   ├── capability/           # Capability 注册表
│   ├── action/               # Action 注册表与执行链
│   ├── menu/                 # Menu 注册表（独立）
│   ├── setting/              # 设置 Category/Definition/Field 注册与 Provider
│   ├── storage/              # ObjectStore 接口、Local Provider、Key 生成
│   ├── outbox/               # Transactional Outbox（Writer/Transport/Dispatcher）
│   ├── job/                  # 后台 Job Contract（Enqueuer/Handler/RunStore）
│   ├── asynqadapter/         # Asynq Adapter（业务禁止直连）
│   ├── permission/           # Role ↔ permission
│   ├── audit/                # 审计写入
│   ├── httpserver/           # Gin、健康检查
│   ├── middleware/           # request_id、recover、auth
│   └── database/             # GORM + Postgres
├── internal/
│   └── boot/                 # HTTP / Worker 共享组装
├── modules/
│   ├── system/               # 可选：健康/版本类 Action
│   ├── operations/           # Job、Outbox、通知投递等运行态运维 Action/Menu
│   └── demo/                 # 示例 Capability/Action/Menu/Button
│       └── register.go
└── tests/                    # 外部测试包；单元/集成测试统一入口
```

### 延后再建（Phase 1 不建空包）

`workflow`、`scheduler`、`ai/**`、`plugin`、`search`、`notification`，以及 `purchase` 等业务 module。

> 说明：`framework/outbox`（#20）、`framework/job` + `asynqadapter`（#21）已落地；原延后 `queue`/`event` 由上述包承接。

### 注册表 vs 业务管理

| 位置 | 职责 |
|------|------|
| `framework/menu`、`permission` | 代码注册与运行时校验 |
| 后续 `modules/*` 管理端 | 若做角色/菜单 CRUD 再引入；Phase 1 以代码 `Register*` 为准 |

## 依赖方向

```text
cmd/ainp     → framework + modules
modules/*    → framework
framework    ❌ modules
modules/A    ❌ modules/B（默认；共享进 framework）
tests        → framework + modules（仅测试依赖）
```

## Module 包架构

`platform/modules/<module>` 的根包是模块对外公开面，不是业务实现的默认落点：

- 根包仅保留 `register.go`、稳定 Facade、跨包 contracts 和必要常量；对应文件使用 `facade.go` / `*_facade.go`、`contracts.go` / `*_contracts.go`、`constants.go` / `*_constants.go` 命名。
- Action 注册、Service、Repository、模型和领域流程进入子包。大模块优先按能力域拆分（例如 `identity`、`attachment`、`knowledge`），能力包内部再按复杂度组织；小模块可直接采用 `model`、`repository`、`service` 等简单角色分层。
- `model`、`repository`、`service`、`pagecontext`、`jobs`、`help`、`tools` 均为按需目录，禁止为了满足布局检查创建空目录。
- 禁止新增 `common`、`commons`、`util`、`utils`、`misc`、`shared`、`helpers` 等兜底包；无法明确归属的跨模块抽象应先审查其所有权，再放入具体能力包或 `framework` 的稳定契约包。
- 模块内子包可以依赖同模块的根包或其他子包，但必须避免循环依赖；根包 Facade 只做稳定编排和委托，不重新承载实现。

### 跨模块依赖

`modules/A` 默认禁止 import `modules/B`（包括 B 的子包）。共享机制应下沉 `framework`，业务协作应通过由所有者定义的最小 contract 注入。确需跨模块依赖时，必须在 `platform/tests/testdata/module_architecture_baseline.tsv` 中登记精确源文件、目标 import 和审查原因；该清单是迁移基线，不是永久白名单。

### 渐进式门禁

`platform/tests/module_architecture_test.go` 使用 Go parser 扫描源文件和 import，不依赖文本匹配。门禁规则如下：

1. 新增模块根目录实现文件失败，并给出建议目标位置。
2. 新增未登记的跨模块 import 失败。
3. 新增兜底包失败。
4. 存量违规按精确基线暂时放行；代码迁移后若基线未同步删除，以 stale baseline 失败。

基线变更必须与实际迁移在同一 PR 中完成并说明原因；禁止用宽泛目录、通配符或无原因条目绕过检查。

### 试点迁移兼容出口

`operations` 与 `demo` 的实现分别迁入 `center`（运维中心）和 `showcase`（演示能力）包后，根包继续提供稳定 `Register`、公开 ID 常量和必要 contracts。旧的 Service 构造器与 DTO 仅通过类型别名/委托 Facade 兼容现有调用方；当仓库及已知下游不再从模块根包引用对应 Service/DTO，且完成一个发布周期的迁移窗口后，应删除这些非注册类兼容出口。稳定注册入口和协议 ID 不在该删除范围。

tenant-owned 数据依赖固定为：

```text
Action/Handler → Service → Tenant Repository / Domain Query → GORM
```

- 通用 CRUD 使用 TenantDB Repository。
- 领域专用 ORM 查询使用 GuardedDatabase，不能保存可直接绕过 Callback 的根 DB。
- Raw/JOIN 使用 TenantRaw/TenantExec；平台跨租户使用 SystemDB。
- transaction 内继续使用 scoped transaction，禁止回退 Service 根 DB。
- 详规见 [`tenant-data-access.md`](tenant-data-access.md)。

- 业务执行入口只有 **Action**；Gin 只做 HTTP 适配。
- `operations` 不反向依赖 `system`；通知等领域能力通过最小接口注入运维注册器。
- `framework/validation` 仅依赖通用错误体系，不依赖 Gin 或业务 module；查库、权限、租户、资源与状态规则保留在 Service。
- `framework/pagination` 只定义传输无关的分页请求和结果；领域筛选 DTO 留在 `modules/*`，不得建立万能查询 DSL。
- 禁止 module 内手写响应 JSON；走 `response` + `errs`。
- 禁止业务 `print`；走 `logging`。
- `_test.go` 不放在 `framework/*` 或 `modules/*`；统一放入 `platform/tests/`，由架构测试扫描防回流。
- 迁移白盒测试时改写为注册表、Service、Action、状态转换或稳定输出等公开行为；不得增加 test-only 导出函数、测试桥接文件或绕过边界的链接技巧。
- 与集中测试重复的断言应合并，并在迁移记录或 PR 中注明替代覆盖位置，不得静默删除有效覆盖。

## 启动与执行链

```text
main → config → logging → database → app.New
    → demo.Register / system.Register
    → MountHTTP（按 Meta.HTTP + /healthz）→ Run
```

```text
HTTP → request_id → AuthContext → permission(action.id)
    → Handler → Audit(meta.audit) → 信封响应
```

## 与规范文档映射

| 规范 | 包 |
|------|-----|
| `logging.md` | `framework/logging` |
| `api-response.md` | `framework/response`、`errs` |
| `action-metadata.md` | `capability`、`action`、`menu` |
| `auth-context.md` | `auth`、`tenant`、`permission` |
| `audit.md` | `audit` |

## Phase 1「骨架完成」标准

1. `go run ./cmd/ainp` 可启动（需可用 Postgres）。
2. `GET /healthz` 返回统一信封。
3. demo：≥1 Capability、Action、directory + menu + button。
4. 鉴权：无 Token → 401；无权限 → 403；成功 `code=0` + `request_id`。
5. `audit=true` 时有审计记录。
6. 配置来自 YAML；示例无密钥。

## 变更

调整目录或依赖规则时，同步更新本文档、`.agent/rules/backend-layout.md` 与 `README.md` 规划目录，并 `agent-rules sync`。
