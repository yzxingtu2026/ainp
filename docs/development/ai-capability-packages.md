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

## 依赖图

```text
ai root Register / Facade
├── modelruntime
├── tooling
├── agent
├── conversation ──> modelruntime, tooling
├── knowledge ──> agent
├── memory ──> conversation
└── experience

all capabilities ──> framework contracts
all capabilities ──> ai/internal/actionmeta (private leaf, when needed)
```

未列出的能力间直接 import 一律禁止。优先把跨域需求表达为 `framework/aimodel`、`aimodelruntime`、`aitool`、`aiagent`、`aiconversation`、`aiknowledge`、`aimemory`、`runtimecontext` 等 Provider-neutral contracts，并由根 `Register` 注入实现。能力包不得 import `modules/ai` 根包；根包只向下装配。

`help` 与现有 `tools` 是迁移期遗留包，不代表可新增的能力节点；架构门禁允许它们继续存在直到对应批次归档，但拒绝其他未在本契约声明的 AI 一级包。

允许的三条直接依赖有明确方向：Conversation Runtime 编排模型调用与 Tool 执行；Knowledge 管理 Agent 绑定；Memory 从 Conversation 事实提取候选。反向需求必须通过 framework contract、事件或根装配回调解决，不能增加反向 import。

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
| `knowledge_service.go -> system/model` | 解析知识库 owner | 使用相同身份 contract 或 owner resolver；Knowledge 只持有接口 |

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
