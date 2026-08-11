# AI 上下文压缩硬约束

- 压缩只创建不可变 ContextSnapshot，禁止覆盖、删除或改写 canonical Message。
- 预算必须来自目标模型窗口并扣除输出、固定指令、工具 Schema、附件与安全余量；未知时 fail-closed。
- Run 剩余预算接近收尾预留时可用 `run_budget_pressure` 提前压缩；目标必须受剩余 Run 预算约束，不得等待 context window 超限后才处理。
- 先确定性缩减，再结构化摘要；近期消息、完整 tool interaction 和安全事实不得被切断或交给摘要改写。
- 摘要必须通过强类型 Schema、来源范围、opaque ID 和 Token 下降校验；失败不得发布 Snapshot。
- Snapshot 固定 conversation revision、Agent Version、模型、parent、策略/Prompt/Schema 版本、来源范围和压缩深度。
- 连续压缩深度必须受限；抖动、单项超窗或压缩不降返回 cannot_fit，不得无限重试。
- 摘要模型只走统一 Model Runtime；Provider 可替换但不得绑定供应商 opaque compaction。
- Agent Run 触发的摘要 Invocation 必须关联 run，实际 Usage 纳入同一累计 Token；Snapshot/计数复用必须幂等，恢复不得重复计费。
- 日志、审计、Outbox 和诊断禁止携带消息正文、摘要正文、敏感工具结果、附件内容或 Secret。
