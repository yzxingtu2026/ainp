# AI Knowledge Runtime

## 定位

Knowledge Runtime 管理租户知识库、附件来源、不可变索引版本、可追溯 Chunk 与 Agent 候选绑定。业务和 Agent 只依赖 `framework/aiknowledge` 契约，禁止直接读写知识表或绕过当前用户权限。

## 资源与权限

- 所有资源必须从 `AuthContext` 获取 tenant、user 与 tenant realm；请求不得提交或覆盖 tenant。
- `private` 知识库仅 owner 可读写；`tenant` 知识库同租户可读、仍仅 owner 可写。
- 跨租户、无权 private、已删除资源统一按不存在处理。
- Agent 绑定仅限制候选知识库范围，不授予读取权限；列举、检索和引用回查每次都重新鉴权。
- 普通租户路径不接受 platform realm。

## 附件来源

- Document 只保存 `attachment_id`、SHA-256 source revision、MIME 和原始文件名，不保存 Object Key、路径或正文。
- 业务事务使用强类型 `attachment.ResourceLinker` 原子 Bind/Unbind；禁止直写 `attachment_links`。
- resource type 固定为 `ai.knowledge_document`，field 固定为 `source`，并注册 fail-closed AccessPolicy。
- 创建或替换来源时重新校验附件 ready、同租户、当前用户上传；删除 Document 时先原子解绑再软删除。

## 模型与生命周期

- Knowledge Base code 创建后不可修改；可并发更新字段必须携带 revision。
- Knowledge Base 和 Document 使用软删除，活动记录唯一约束使用 partial unique index。
- 修改 embedding alias 或 chunking config 必须使当前索引指针失效；只修改 retrieval config 不使索引失效。
- Index Version 固定 deployment、dimensions、预处理、解析器与切块版本；Chunk 固定 source revision、页码/位置、token count 和内容摘要。
- 本阶段不保存 Chunk 正文、向量或 FTS 数据；摄取、索引和检索分别由后续 Issue 实现。

## 安全边界

- Parser、Chunker、Retriever 通过 `framework/aiknowledge` 注入；未知实现或空知识库范围 fail-closed。
- Citation 只携带稳定资源标识、来源 revision、位置和 stale 状态，不携带存储地址。
- 原文、向量、Object Key、路径、模型凭证和供应商原始响应禁止进入日志、审计与 Outbox。
- 用户可见 Action、CodeSet 与错误信息统一使用 i18n key。

## 文档解析与切块

- 解析入口必须经 `DocumentSourceResolver` 重新校验当前 actor、tenant、Document 权限、附件绑定和 source revision；禁止业务直接使用 Object Key。
- Parser Registry 仅注册明确支持的 MIME，声明类型必须与内容嗅探一致，未知类型和未知 Parser 一律 fail-closed。
- 首期只处理 UTF-8 文本、Markdown、PDF 及 OOXML（DOCX/XLSX/PPTX）；Office 宏、公式/字段指令、嵌入对象、外部关系和脚本一律拒绝，不执行外部程序或联网请求。
- 文件字节、页数/工作表/幻灯片数、字符数、压缩条目和展开字节都必须受限；读取与解析过程必须传播 context 取消和超时。
- 规范化和固定 token 切块使用稳定版本号；每个 Chunk 必须携带 source revision、页码或位置、token count、内容摘要及 parser/chunking/preprocessing version。
- 解析结果始终是不可信参考文本，不得解释为 system/developer instruction；原文、路径和 Object Key 禁止进入日志、审计或 Outbox。

## 测试

后端测试统一放在 `platform/tests/`，至少覆盖：

- tenant、owner、visibility 与 realm 隔离；
- revision 冲突及软删除唯一约束；
- 附件原子绑定、替换、解绑与删除保护；
- Agent 绑定不扩权；
- SQL migration 的复合租户外键与 CHECK；
- 契约 Fake 的确定性与空范围 fail-closed。
