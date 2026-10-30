# 项目背景与文档规范

- 开发前读 `README.md`；专题规范读 `docs/`（前端、图标、日志等见 `docs/development/`）。文档与代码冲突时先澄清再扩大实现。
- 人工文档在 `docs/`（如 `architecture/`、`api/`、`development/`、`guides/`、`decisions/`）；根目录只留 `README.md`。
- `.agents/rules/` 只写 Agent 硬约束要点，详细说明引用 `docs/`，禁止规则与文档双份堆砌。
- 新增公共约定：详规进 `docs/development/`，要点进对应 rule，并 `agent-rules sync`。
