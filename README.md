# AINP

> AI Native Platform - 面向企业业务的 AI 原生 SaaS 能力平台。

AINP 是一个以业务能力、标准动作和元数据为核心的企业级应用平台。它的目标不是再造一个单体 ERP，而是提供一套可持续扩展的基础框架，让 ERP、CRM、OA、WMS、MES、SRM、BI、Agent Studio、Workflow Studio 和插件市场等产品能够共享统一的开发模型、运行时与治理体系。

ERP 是 AINP 的第一个业务应用，也是平台能力的首个验证场景。

## 项目愿景

传统企业软件通常围绕页面、接口和数据表组织功能，AI 只能作为外围助手接入。AINP 从一开始就把 AI 视为系统的一级参与者，并将企业业务抽象为 AI 和开发者都能理解、编排和执行的能力。

AINP 希望最终形成一个 **AI Native Enterprise SaaS Platform**：

```text
AINP
├── ERP
├── CRM
├── OA
├── WMS
├── MES
├── SRM
├── BI
├── AI Agent Studio
├── Workflow Studio
└── Plugin Marketplace
```

## 核心理念

平台围绕三个 Registry 构建统一的业务描述与运行机制。

### Capability Registry

Capability 描述企业“具备什么业务能力”，例如采购、库存、销售、审批、客户、工作流、知识库和 AI 对话。

Capability 不对应单个接口，而对应一个完整的业务领域：

```text
Purchase Capability
├── purchase.create
├── purchase.update
├── purchase.approve
├── purchase.cancel
└── purchase.list
```

AI 面向 Capability 理解和规划业务，而不是感知底层 HTTP 接口。

### Action Registry

Action 描述业务能力“如何执行”，是系统中所有操作的唯一标准入口，例如：

```text
purchase.create
purchase.cancel
inventory.in
inventory.out
customer.create
customer.update
```

HTTP API、工作流、Agent Tool 和 MCP Tool 最终都调用同一个 Action。AI 不直接访问数据库，也不绕过业务规则。

### Metadata Registry

Metadata 描述 Action 及 Capability 的治理和展示信息，包括：

- 权限与菜单
- 审计与日志
- OpenAPI 文档
- AI 与 MCP 暴露策略
- 工作流配置
- 输入、输出及界面描述

基于统一元数据，平台可以自动生成或装配 HTTP 路由、菜单、权限、OpenAPI、MCP、Agent Tool、工作流节点、审计和日志能力。

## 开发范式

业务开发者注册 Capability、Action 与独立 Menu（含按钮权限）：

```go
app.RegisterCapability(Capability{ID: "purchase", Name: "采购", Module: "erp"})

app.RegisterAction(Action{
    ID:      "purchase.order_create",
    Handler: CreatePurchaseOrder,
    Meta: Metadata{
        Capability: "purchase",
        Audit:      true,
        HTTP:       &HTTPMeta{Method: "POST", Path: "/api/v1/purchase/orders"},
        AI:         &AIMeta{ExposeTool: true, Risk: "medium"},
    },
})

app.RegisterMenu(Menu{ /* directory | menu | button，见 docs */ })
```

字段约定见 `docs/development/action-metadata.md`。

平台根据注册信息统一提供：

- HTTP API 与 OpenAPI 文档
- 菜单与权限控制
- MCP 和 Agent Tool
- 工作流节点
- 审计日志与可观测信息

## 整体架构

```text
                  Browser
        ┌────────────┴────────────┐
        │                         │
     Vue 3 SPA              AI Workspace
        │                         │
        └──────── API / SSE ──────┘
                  │
┌─────────────────────────────────────────────┐
│          AI Native Platform (Go)            │
├─────────────────────────────────────────────┤
│ Capability Registry                         │
│ Action Registry                             │
│ Metadata Registry                           │
├─────────────────────────────────────────────┤
│ HTTP API │ MCP │ Workflow │ Agent Runtime   │
│ Scheduler│ Queue │ Notification │ Audit      │
│ Search   │ Config Center │ Plugin System    │
├─────────────────────────────────────────────┤
│ System │ Tenant │ Organization │ Permission │
│ Approval │ Inventory │ Purchase │ Sales      │
│ Finance │ CRM │ OA │ Other Business Apps    │
├─────────────────────────────────────────────┤
│ PostgreSQL + pgvector │ Redis │ MinIO        │
│ Asynq │ gocron                              │
└─────────────────────────────────────────────┘
```

## AI Runtime

AINP 中的 Agent 不是附加在系统旁边的聊天框，而是理解和操作企业能力的运行时。

```text
User
  ↓
LLM
  ↓
Capability Planner
  ↓
Action Planner
  ↓
Workflow
  ↓
Tool Call
  ↓
Action
  ↓
Result
```

核心约束：

- AI 只通过已注册的 Action 执行业务操作。
- Action 统一应用鉴权、租户隔离、校验、审计和事务规则。
- 复杂任务通过 Workflow 编排，而非由模型自由串联数据库操作。
- 高风险操作必须支持审批、人工确认和完整追踪。

## Workflow

每个 Action 都可以成为 Workflow 节点。业务流程可以组合人工操作、规则判断、AI 分析、审批、通知与系统动作，例如：

```text
库存不足
  ↓
AI 分析
  ↓
生成采购建议
  ↓
人工审批
  ↓
创建采购单
  ↓
通知供应商
```

## 插件体系

插件可以为平台扩展：

- Action 与 Capability
- 菜单与权限
- Workflow
- AI Tool 与 Prompt
- Knowledge

后续将基于统一的插件规范建设 Plugin Marketplace，使业务模块能够独立开发、安装、升级和治理。

## 多租户模型

SaaS 与租户隔离是基础能力，不作为后期改造项：

```text
System
  ↓
Tenant
  ↓
Organization（预留）
  ↓
Department（预留）
  ↓
User（一用户一租户）
```

- 租户身份只来自 Token/会话；Role ↔ permission（默认 `action.id`）。
- 预留租户绑定自定义域名；Phase 1 可不实现解析。
- 约定详见 `docs/development/auth-context.md`。

所有业务数据、Action 执行、权限校验、审计记录和 AI 上下文都必须遵循租户边界。

## 技术选型

### 后端与基础设施

- Go
- Gin
- GORM
- PostgreSQL 17+
- pgvector
- Redis
- MinIO
- Asynq
- gocron
- **统一日志**：`log/slog` 门面（运行日志）；与审计日志分离，见 `docs/development/logging.md`
- **API 响应**：统一信封 `code` / `message` / `data` / `request_id`，见 `docs/development/api-response.md`
- **Action / 菜单**：Capability + Action + 独立 Menu 注册（含按钮权限），见 `docs/development/action-metadata.md`
- **多租户 / 鉴权**：Token 定租户、一用户一租户、Role↔permission，见 `docs/development/auth-context.md`

### 前端

- **脚手架**：Soybean Admin（管理端壳层；禁止改用 Vben 等其他后台脚手架）
- **UI**：Naive UI（主组件库；禁止改用 Element Plus / Ant Design Vue 等作为主库）
- Vue 3
- TypeScript
- Vite
- Pinia
- Vue Router
- TanStack Table
- Monaco Editor
- Tiptap
- ECharts
- **Iconify**（菜单/UI 图标协议；默认集合 `mdi`，格式 `collection:name`）

约束与裁剪说明见 `docs/development/frontend-stack.md`、`docs/development/icon-library.md`；Agent 硬约束见 `.agent/rules/frontend-stack.md`、`.agent/rules/icon-library.md`。

## AI 数据模型

AI 能力采用平台原生数据模型，初步包括：

```text
ai_conversation
ai_message
ai_embedding
ai_memory
ai_prompt
ai_tool_call
ai_workflow
ai_task
knowledge
knowledge_chunk
knowledge_embedding
```

具体表结构将在领域模型设计阶段确定，并统一纳入租户隔离、权限、审计和数据生命周期治理。

## AI Workspace

AI Workspace 是产品的核心交互区域，而不是临时弹窗或独立聊天页面。它与业务工作区长期并存，使用户可以在当前业务上下文中进行对话、发起任务、检查 Tool Call、处理审批和运行 Agent。

```text
┌──────────────────────────────────────────────────────────────┐
│ Logo / Navigation                              User / Notice │
├────────────┬──────────────────────────┬──────────────────────┤
│            │                          │ AI Workspace         │
│ Menu       │ Business Workspace       │                      │
│            │                          │ Conversation         │
│            │ Table / Form / Chart     │ Task / Tool Call     │
│            │                          │ Approval / Agent     │
└────────────┴──────────────────────────┴──────────────────────┘
```

## 规划目录

```text
platform/
├── cmd/
├── framework/
│   ├── action/
│   ├── capability/
│   ├── metadata/
│   ├── router/
│   ├── middleware/
│   ├── auth/
│   ├── tenant/
│   ├── menu/
│   ├── permission/
│   ├── workflow/
│   ├── scheduler/
│   ├── queue/
│   ├── audit/
│   ├── ai/
│   │   ├── runtime/
│   │   ├── registry/
│   │   ├── mcp/
│   │   ├── prompt/
│   │   ├── memory/
│   │   ├── embedding/
│   │   ├── tool/
│   │   ├── workflow/
│   │   └── llm/
│   ├── plugin/
│   ├── event/
│   ├── storage/
│   ├── config/
│   ├── notification/
│   └── search/
├── modules/
│   ├── system/
│   ├── organization/
│   ├── user/
│   ├── tenant/
│   ├── role/
│   ├── menu/
│   ├── permission/
│   ├── approval/
│   ├── workflow/
│   ├── inventory/
│   ├── purchase/
│   ├── sales/
│   ├── finance/
│   ├── crm/
│   └── oa/
└── frontend/
    ├── apps/
    └── packages/
```

该结构是项目启动阶段的设计草案，将随核心抽象验证逐步收敛。

## 路线图

### Phase 1 - Framework Core（预计 3-4 个月）

目标：形成可供团队稳定开发业务模块的平台核心，并以第一版 ERP 验证端到端能力。

核心范围：

- Capability、Action、Metadata Registry
- 基于 Gin 的路由与 Action 执行链
- GORM 数据访问封装
- PostgreSQL、pgvector、Redis、Asynq 与 gocron 集成
- RBAC、菜单自动生成与审计日志
- 多租户基础能力
- 基于 Soybean Admin + Naive UI 的 Vue 3 管理端与常驻 AI Workspace
- ERP 首批模块：库存、采购、销售

阶段成果：开发者可以通过注册 Capability、Action 与 Metadata 完成业务模块开发，并由平台统一暴露 HTTP、权限、菜单、审计及 AI 工具能力。

## 当前状态

项目处于架构设计与启动阶段。当前 README 用于明确产品方向和核心约束，具体 API、数据模型、插件协议及工程规范将在首个可运行骨架中持续验证和补充。

## 设计原则

1. **业务能力优先**：以 Capability 和 Action 组织系统，而不是以页面或接口堆叠功能。
2. **单一执行入口**：人、HTTP、Workflow、MCP 和 Agent 共享同一套 Action 语义与治理规则。
3. **AI 原生**：AI 是系统一级能力，但必须遵守权限、租户、审批与审计边界。
4. **元数据驱动**：减少菜单、权限、文档和工具定义之间的重复配置。
5. **多租户内建**：从第一天保证数据和执行上下文的租户隔离。
6. **模块化扩展**：业务模块和插件保持清晰边界，为独立交付与 Marketplace 做准备。
7. **先验证核心抽象**：优先完成可运行闭环，再扩展平台覆盖面。

## License

许可证尚未确定。项目正式开源前将补充 License 和贡献指南。
