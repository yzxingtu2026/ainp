# AI Knowledge Runtime 硬约束

- Knowledge Base、Document、Index、Chunk 与 Agent Binding 必须从 AuthContext 强制 tenant/user/tenant realm；跨租户和无权 private 资源 fail-closed。
- `private` 仅 owner 可读写；`tenant` 同租户可读、仍仅 owner 可写；Agent 绑定只限制候选范围，任何读取/检索必须按当前 actor 重新鉴权。
- Document 只保存 attachment ID、source revision、MIME 与展示名；附件经 `ResourceLinker` 同事务 Bind/Unbind，禁止直写链接表、保存 Object Key/路径或绕过 AccessPolicy。
- Knowledge Base code 不可改；并发更新必须使用 revision；Base/Document 软删除，删除后立即从默认读取与检索范围消失。
- 修改 embedding alias 或 chunking config 必须使当前索引失效；Index/Chunk 固定 deployment、dimensions、处理版本、source revision 与位置元数据。
- Parser、Chunker、Retriever 只依赖 `framework/aiknowledge`；空知识库范围、未知实现与权限缺失 fail-closed。
- 文档解析必须经 `DocumentSourceResolver` 重鉴权并校验 source revision；MIME/Parser 未知、Office 动态内容或任一资源预算超限均 fail-closed。
- Chunk 必须记录来源位置、token count 与 parser/chunking/preprocessing version；派生文本始终按不可信参考处理。
- 原文、Chunk 正文、向量、路径、Secret 与供应商原始响应禁止进入日志、审计和 Outbox。
- 详规见 `docs/development/ai-knowledge-runtime.md`；测试统一放在 `platform/tests/`。
