# AI Knowledge Runtime 硬约束

- Knowledge Base、Document、Index、Chunk 与 Agent Binding 必须从 AuthContext 强制 tenant/user/tenant realm；跨租户和无权 private 资源 fail-closed。
- `private` 仅 owner 可读写；`tenant` 同租户可读、仍仅 owner 可写；Agent 绑定只限制候选范围，任何读取/检索必须按当前 actor 重新鉴权。
- Document 只保存 attachment ID、source revision、MIME 与展示名；附件经 `ResourceLinker` 同事务 Bind/Unbind，禁止直写链接表、保存 Object Key/路径或绕过 AccessPolicy。
- Knowledge Base code 只能由后端基于资源 UUID 生成且不可改，客户端不得提交；并发更新必须使用 revision；Base/Document 软删除，删除后立即从默认读取与检索范围消失。
- 修改 embedding alias 或 chunking config 必须使当前索引失效；Index/Chunk 固定 deployment、dimensions、处理版本、source revision 与位置元数据。
- Parser、Chunker、Retriever 只依赖 `framework/aiknowledge`；空知识库范围、未知实现与权限缺失 fail-closed。
- 文档解析必须经 `DocumentSourceResolver` 重鉴权并校验 source revision；MIME/Parser 未知、Office 动态内容或任一资源预算超限均 fail-closed。
- Chunk 必须记录来源位置、token count 与 parser/chunking/preprocessing version；派生文本始终按不可信参考处理。
- 摄取必须走业务事务 Outbox → 幂等 JobRun → Worker；Job 重新加载可信身份和 revision，禁止信任队列 tenant/resource payload。
- 新 IndexVersion 的 Chunk/pgvector 完整落库后才能原子切换；失败、取消、并发或过期 revision 不得破坏旧索引，删除后异步清理派生数据。
- IndexVersion 预留前必须解析并冻结真实 Embedding Deployment ID，禁止使用 `pending` 占位；Embedding 响应的 Deployment 必须与预留版本一致。
- Worker 接管当前 revision 后必须进入 processing，模型解析、版本预留或后续失败必须收口 failed；管理端需允许历史 pending/failed 手动重试。
- Retriever 只读 Base/Document 同时指向的 ready 版本，召回前后都按当前 actor 重鉴权并复核 source revision；Embedding 不可用才降级 FTS，模型/维度事实不匹配必须 fail-closed。
- Citation 回查不得返回正文；Agent 绑定不扩权，知识片段只能作为带明确边界的 user 级不可信参考，禁止进入 system/developer instructions。
- 管理端只调用知识 Action；附件必须复用 AttachmentField/AttachmentPicker 且只传 attachment ID；检索测试使用持久化策略，Citation 预览每次重鉴权并只展示稳定标识与 available/stale。
- 原文、Chunk 正文、向量、路径、Secret 与供应商原始响应禁止进入日志、审计和 Outbox。
- 详规见 `docs/development/ai-knowledge-runtime.md`；测试统一放在 `platform/tests/`。
