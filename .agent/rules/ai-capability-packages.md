# AI 能力域包边界

- `modules/ai` 根包只保留稳定 `Register`、Facade、contracts 与常量；实现目标域为 `modelruntime`、`tooling`、`agent`、`conversation`、`knowledge`、`memory`、`experience`。
- `help` 是迁移期共享资源包；原 `tools` 已在 #221 归档到各 owner 与 `tooling`，禁止恢复横切 Tool Slot 包。除冻结能力域和 `internal` 外，新增 AI 一级包必须先更新架构契约与门禁。
- 逐文件迁移事实源是 `platform/tests/testdata/ai_capability_migration.tsv`；新增或删除 AI 根实现文件必须同步更新映射，禁止无目标文件。
- 能力包禁止 import `modules/ai` 根包；未批准的能力间 import 由 AST 门禁拒绝。
- 仅允许 `conversation → agent|modelruntime|tooling`、`knowledge → agent`、`memory → conversation`、`experience → agent`；其他跨域协作通过 framework contract、事件或根装配注入。
- `ai/internal/actionmeta` 仅为 AI 域内私有叶子，禁止模块外导入；不得新建横切巨型 `model`、`service`、`repository` 包。
- 根 Facade 只允许 alias、薄委托和装配，禁止复制业务逻辑；稳定 Action/Job/Event/Setting/resource ID 与协议值不得因迁移改变。
- Service/DTO/model/repository 兼容出口须在已知下游迁移并经过至少一个发布周期后，另开 Issue 删除；根 `Register` 和稳定协议常量长期保留。
- `ai` 子能力不得依赖 `system` 或其兼容 model；附件与身份能力通过 framework/owner contracts 注入。
- PostgreSQL vector GORM 值类型统一使用 `framework/database.PGVector`；禁止各能力复制 Scan/Value 实现或借用 Knowledge 模型类型。
- #219 后模型运行时与 Tooling 新代码直接 import owner 包；根 deprecated Facade 仅用于兼容，禁止继续扩大。
- #220 后 Agent/Slot、Conversation/Runtime/Compaction、Personalization 新代码直接 import owner 包；Run terminal 事件引用 `framework/aiagentruntime.EventRunTerminal`，禁止复制协议值或反向 import Memory/System 实现。
- #221 后 Knowledge、Memory、Navigation 与对应 Tool Slot/Job/注册实现直接归 owner；根迁移基线必须保持为空，跨域事实读取使用 framework contract 或固定租户投影。

详规：`docs/development/ai-capability-packages.md`
