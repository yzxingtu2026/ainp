# AI 能力域包边界与迁移契约

## 目的与边界

本契约冻结 `modules/ai` 的目标能力域、单向依赖、根 Facade 兼容窗口和逐文件迁移位置。迁移只改变代码归属，不改变模型调用、Tool、Agent、Conversation Runtime、知识库、记忆、导航或个性化协议。

逐文件事实源为 `platform/tests/testdata/ai_capability_migration.tsv`。架构测试要求每个 AI 根目录 Go 实现文件（`register.go` 除外）恰好存在一条映射，并拒绝过期映射。

## 能力域与所有权

| 目标包 | 所有权 | 稳定公开面 |
| --- | --- | --- |
| `ai/modelruntime` | Provider、Deployment、Alias、模型目录、调用事实、运行时调用服务 | 模型/Embedding 解析与调用 contracts、模型管理 DTO、Invocation 构造器 |
| `ai/tooling` | Tool Registry 投影、搜索/加载/执行、确认、目录索引 | Tool DTO、handle/确认协议、Tool Service 与目录事件/Job ID |
| `ai/agent` | Agent Contract/Version/Binding、管理服务、Slot | Agent/Slot DTO、不可变 Version 与 Slot 解析 contracts |
| `ai/conversation` | Conversation/Message/Snapshot、Agent Run/Step/Event、压缩与 runtime budget | Conversation/Run DTO、状态/事件/Job ID、runtime/compaction contracts |
| `ai/knowledge` | Knowledge Base/Document/Chunk、绑定、解析、摄取与检索 | Knowledge DTO、resource type、摄取事件/Job ID、检索与来源 contracts |
| `ai/memory` | 时态记忆事实、提取、治理、投影、召回、合并与管理 | Memory DTO、事件/Job ID、召回/治理 contracts |
| `ai/experience` | 导航语义检索与助手个性化 | 导航索引、个性化设置/事件/Job ID 与投影 DTO |
| `ai/internal/actionmeta` | 各能力注册器共用的私有 Action AI metadata 构造 | 不公开；不得被 `modules/ai` 外部导入 |

模型必须归业务事实的唯一 owner。现有 `model.go` 在迁移时拆为 `modelruntime/model`、`tooling/model` 与 `experience/navigation/model`，禁止创建新的横切 `ai/model` 巨型包。

跨 Knowledge、Tooling 与 Navigation 使用的 PostgreSQL vector GORM 值类型归 `framework/database.PGVector`；各域模型只引用该数据库技术类型，根 `ai.PGVector` 继续通过 alias 兼容。它不承载 Embedding 业务协议或模型选择逻辑。

## 依赖图

```text
ai root Register / Facade
├── modelruntime
├── tooling
├── agent
├── conversation ──> agent, modelruntime, tooling
├── knowledge ──> agent
├── memory ──> conversation
└── experience ──> agent

all capabilities ──> framework contracts
all capabilities ──> ai/internal/actionmeta (private leaf, when needed)
```

未列出的能力间直接 import 一律禁止。优先把跨域需求表达为 `framework/aimodel`、`aimodelruntime`、`aitool`、`aiagent`、`aiconversation`、`aiknowledge`、`aimemory`、`runtimecontext` 等 Provider-neutral contracts，并由根 `Register` 注入实现。能力包不得 import `modules/ai` 根包；根包只向下装配。

`help` 是迁移期共享资源包，不代表可新增的能力节点；原 `tools` 已在 #221 归档到各 owner 与 `tooling`。架构门禁拒绝恢复横切 Tool Slot 包，也拒绝其他未在本契约声明的 AI 一级包。

允许的直接依赖有明确方向：Conversation 固定 Agent Version 并由 Runtime 编排模型调用与 Tool 执行；Knowledge 管理 Agent 绑定；Memory 从 Conversation 事实提取候选；Personalization 只读解析 Agent Slot。反向需求必须通过 framework contract、事件或根装配回调解决，不能增加反向 import。

## 公开符号与启动装配盘点

当前根公开 import surface 的使用者为 `cmd/ainp`、`internal/boot`、数据库 migration runner 和集中测试。公开符号按稳定性分为：

| 分组 | 当前代表符号 | 迁移策略 |
| --- | --- | --- |
| 启动与迁移 | `Register`、`AutoMigrate`、各 `Register*` | 根 `Register` 永久保留；细分注册器迁移后由根委托，迁移 runner 继续调用根 Facade |
| 稳定协议 | Action/Menu/Setting/Event/Job ID、resource type、状态常量 | 根常量 alias 长期保留，不改变字符串值 |
| framework 实现 | Resolver、Provider、Runtime、Tool/Knowledge/Memory contracts 的实现 | contract 留在 framework；具体实现迁入 owner，根构造器委托 |
| Service/Repository/DTO | `Service`、`AgentService`、`ConversationService`、`ToolService` 等 | 已知调用方迁移期间用 type alias/薄构造器 Facade；Repository 仅在确有外部调用时兼容 |
| GORM model | Provider、Agent、Conversation、Knowledge、Memory 等 | 迁入 owner 的 `model` 子包；根包 type alias 保证 migration 与下游编译兼容 |
| 私有注册辅助 | request adapter、AI metadata、未导出 handler | 随 owner 迁移，不进入根 Facade |

启动装配顺序继续由根 `Register` 持有：注册 capability/setting/model runtime → Tool 与目录索引 → Agent/Slot → Conversation/Runtime → Knowledge → Memory → Navigation/Personalization → help/menu。子能力不得自行导入并启动其他能力。

## `ai -> system` 治理

| 当前依赖 | 现状 | 治理方案 |
| --- | --- | --- |
| `register.go -> modules/system` | 创建附件知识来源 resolver | 在 Knowledge 迁移批次将 `aiknowledge.DocumentSourceResolver` 由 boot/root 注入；AI 子能力不 import system |
| `agent_service.go -> system/model` | 读取用户展示信息 | 抽取/复用 access 的最小只读身份 contract，由根装配注入；不得依赖兼容 model alias |
| `knowledge/service.go` | 使用固定 `TenantRaw` 附件元数据投影 | 不 import System model；附件内容仍由根注入的 `aiknowledge.DocumentSourceResolver` 解析 |

迁移完成条件是 `modules/ai/<capability>` 对 `modules/system`、`modules/system/model` 和 `modules/ai` 根包的 import 均为零。根 `register.go` 的临时 system 依赖可在对应注入边界落地前保留在精确架构基线中。

## Facade 生命周期

1. 每批迁移先在 owner 包建立实现与 contracts，再在根包添加类型 alias、常量 alias 和薄构造器/注册委托。
2. Action ID、Job/Event type、Setting key、resource type、持久化表名和 JSON 协议不因包移动改变。
3. 根 `Register`、稳定协议常量及 framework contract 实现入口不设删除期限。
4. Service/DTO/model/repository 兼容出口仅在仓库及已知下游完成 import 迁移、经过至少一个发布周期且无引用后删除；删除必须单独 Issue/PR 并附 `rg` 与编译证据。
5. 禁止在 Facade 中复制业务逻辑、持有状态或引入新的横切模型；Facade 只能 alias、委托和装配。

## 分批迁移顺序

1. `modelruntime`：先稳定模型/Embedding contracts，拆分 `model.go` 所属类型。
2. `tooling`：依赖 framework runtime contracts，不直接依赖 modelruntime 实现。
3. `agent`：迁移 Agent 与 Slot，并以 contracts 接收模型/Tool 能力。
4. `conversation`：在前三域稳定后迁移 Conversation、Runtime、Compaction。
5. `knowledge`：通过 Agent contract 和注入的附件/身份 resolver 收口 system 依赖。
6. `memory`：只依赖 Conversation 事实 contract，模型调用走 framework runtime。
7. `experience`：迁移 Navigation/Personalization，最后清理根帮助与剩余基线。

每批必须独立提交，更新 TSV 实际位置/兼容状态、架构基线和集中测试；不得用一次大规模移动跳过中间可编译状态。

## 迁移进度

### Issue #219：Model Runtime 与 Tooling

- 已迁移到 `modelruntime`：Provider/Deployment/Alias/ModelInvocation、模型管理 Service/Repository、InvocationService/Repository、RuntimeBudgetProvider。
- 已迁移到 `tooling`：Tool Catalog/Search/Scope/Confirmation/Invocation 模型、ToolService/Repository、目录投影、Embedding Job 与向量检索。
- `model.go` 已按 owner 拆分；Navigation 仅移动 `SearchDocument` 数据模型，不迁移导航 Service 行为。
- 根 `model_facade.go`、`modelruntime_facade.go`、`tooling_facade.go` 保留 deprecated alias/薄委托。新代码直接 import owner；兼容出口按一个发布周期与零引用条件删除。
- `runtime_budget.go` 依据 #219 的明确范围随底层 runtime provider 迁入 `modelruntime`；后续 Conversation Runtime 只依赖其 `aiagentruntime.BudgetProvider` contract，不反向持有模型实现。

### Issue #220：Agent、Conversation Runtime 与 Personalization

- `agent` 持有 Agent/Version/Tool Binding、Slot 模型与管理服务；Conversation 通过 owner 的只读查询契约固定版本、读取运行态 Agent 与 Tool Binding。
- `conversation` 持有 Conversation/Message/Stream/Snapshot、Run/Step/Event、Context Compaction 与 Runtime；聚合内用文件分责，不建立互相引用的父子 Go 包。
- `experience/personalization` 持有助手个性化快照、生成与注册；跨 Memory/Run/Conversation 的读取使用最小只读投影，不借用其他能力的 GORM owner model。
- Run terminal 事件名由 `framework/aiagentruntime.EventRunTerminal` 稳定；Memory Recall 由根装配转换为 `conversation.MemoryRecallFunc`，Conversation 不反向依赖 Memory 实现。
- Agent 头像校验由根 composition root 注入，`agent` 能力不再 import `modules/system/model`。
- 根 `agent_facade.go`、`conversation_facade.go`、`personalization_facade.go` 保留兼容 aliases、构造器与注册委托；新代码直接 import owner 包。

### Issue #221：Knowledge、Memory 与 Navigation

- `knowledge` 持有知识库模型、Repository、Service、摄取、检索、Job/Action 注册与 Knowledge Tool Slot；附件元数据通过固定租户投影读取，内容解析继续由根注入 Provider-neutral resolver。
- `memory` 持有时态事实、提取、治理、投影、管理、召回、合并、Job/Action 注册与 Memory Tool Slot；只依赖 `conversation` 事实，Agent Version 通过固定租户投影读取冻结策略。
- `experience/navigation` 持有导航投影、词法/语义检索、Embedding Job、Navigation/Help Tool Slot；Tool ID、Job type、Action ID、索引表和授权投影语义保持不变。
- 通用 Tool Slot 聚合入口迁入 `tooling.RegisterBuiltins`，原迁移期 `ai/tools` 包已归档；共享帮助文档仍由 `ai/help` 嵌入并在根装配。
- 根 `knowledge_facade.go`、`memory_facade.go`、`navigation_facade.go` 保留兼容 aliases 与薄委托；`ai_capability_migration.tsv` 和 module root implementation 基线清零。
