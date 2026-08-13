# AI 会话硬约束

- Conversation 必须固定明确 `agent_version_id`；禁止运行时追随 Agent current version。
- Conversation、Message、Agent Run 对所有 tenant 已登录用户开放，不要求角色额外分配 permission；仍必须强制 tenant/owner/visibility，且不得放宽业务工具权限。
- Conversation 必须声明 `purpose`（`workspace` / `builder_preview`）；列表默认只返回 workspace，Workspace 禁止钉 `kind=preview` Version。
- Message 是不可变追加事实；客户端只能创建 user 消息，system/tool/assistant 只能由受控服务端流程产生。
- 会话、消息、Snapshot 必须从 AuthContext 强制 tenant/owner/visibility，跨租户和无权 private 资源 fail-closed。
- 消息创建强制 `Idempotency-Key`，sequence 必须事务分配并受数据库唯一约束；分页使用 sequence cursor。
- Content Part 复用统一多模态协议；图片只存 image ID + revision，文档只存 file ID + revision，使用时重新执行 AccessPolicy/ready/MIME/大小/revision，客户端禁止提交 `derived_text`。
- ContextAssembler 只装配并报告预算，不静默裁剪、不调用模型；压缩交给 ContextCompactor。
- ContextSnapshot 不覆盖 canonical Message；必须固定 conversation revision、Agent Version、模型、来源范围与策略版本。
- 文档正文只在 Run 内存展开；Snapshot retained messages 落库前还原为 canonical `input_file`，恢复时重新授权解析。
- 日志、审计与 Outbox 禁止携带消息正文、附件内容或 Snapshot payload。
- Runtime Context 不是 canonical Message；Snapshot 只保存 run/context/policy/timezone/evaluated_at refs，不保存动态 Prompt 或私人画像正文。
