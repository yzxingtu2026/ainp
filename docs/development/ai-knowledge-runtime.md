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

- Knowledge Base code 由后端基于资源 UUID 统一生成，客户端不得提交；创建后不可修改，可并发更新字段必须携带 revision。
- Knowledge Base 和 Document 使用软删除，活动记录唯一约束使用 partial unique index。
- 修改 embedding alias 或 chunking config 必须使当前索引指针失效；只修改 retrieval config 不使索引失效。
- Index Version 固定 deployment、dimensions、预处理、解析器与切块版本；Chunk 固定 source revision、页码/位置、token count 和内容摘要。
- 本阶段不保存 Chunk 正文、向量或 FTS 数据；摄取、索引和检索分别由后续 Issue 实现。

## 安全边界

- Parser、Chunker、Retriever 通过 `framework/aiknowledge` 注入；未知实现或空知识库范围 fail-closed。
- Citation 只携带稳定资源标识、来源 revision、位置和 stale 状态，不携带存储地址。
- 原文、向量、Object Key、路径、模型凭证和供应商原始响应禁止进入日志、审计与 Outbox。
- 数据库 Logger 必须启用参数化查询，错误 SQL 也不得展开 Chunk 正文、向量或其他参数值。
- 用户可见 Action、CodeSet 与错误信息统一使用 i18n key。

## 文档解析与切块

- 解析入口必须经 `DocumentSourceResolver` 重新校验当前 actor、tenant、Document 权限、附件绑定和 source revision；禁止业务直接使用 Object Key。
- Parser Registry 仅注册明确支持的 MIME，声明类型必须与内容嗅探一致，未知类型和未知 Parser 一律 fail-closed。
- 首期只处理 UTF-8 文本、Markdown、PDF 及 OOXML（DOCX/XLSX/PPTX）；Office 宏、公式/字段指令、嵌入对象、外部关系和脚本一律拒绝，不执行外部程序或联网请求。
- PDF 文本提取必须解析嵌入字体及字符映射，禁止直接把内容流字形码作为正文；解析器版本变化后，历史文档必须重新摄取才能生成新索引。
- XLSX 必须解析工作表顺序、名称、共享字符串与单元格显示值；按非空行生成带工作表、行号和表头映射的独立 Section，禁止直接向量化 worksheet XML 中的共享字符串索引。
- 文件字节、页数/工作表/幻灯片数、字符数、压缩条目和展开字节都必须受限；读取与解析过程必须传播 context 取消和超时。
- 规范化和固定 token 切块使用稳定版本号；每个 Chunk 必须携带 source revision、页码或位置、token count、内容摘要及 parser/chunking/preprocessing version。
- 文本规范化必须在 Parser 和 Chunker 边界移除 NUL、替换无效 UTF-8，并清理非空白控制字符；向量输入与 PostgreSQL 正文必须使用同一份规范化文本。
- 解析结果始终是不可信参考文本，不得解释为 system/developer instruction；原文、路径和 Object Key 禁止进入日志、审计或 Outbox。

## 摄取与索引发布

- Document 创建、更新、重试与删除只在业务事务内写版本化 Outbox 事件；Dispatcher 再生成幂等 JobRun，禁止业务事务直连 Redis/Asynq。
- Outbox/Job payload 仅携带 Document ID、source revision、document revision 和受控 operation，不得包含正文、Chunk、向量、路径或供应商错误。
- Worker 必须按 JobRun ID 重新加载可信 tenant/realm/actor/payload，再读取当前 Document、附件授权和 revision；队列 Envelope 不是资源事实源。
- 每次构建冻结整库活动 Document revision 并使用独立 IndexVersion；解析和 Embedding 在发布事务外完成，只有完整 Chunk/pgvector 数据落库且 Base 配置未变化后，才能在同一事务原子更新所有 Document/Base 指针。
- 预留 IndexVersion 前必须通过统一模型 Resolver 解析 Embedding Alias，并写入真实 Deployment ID；禁止使用 `pending` 等不满足模型部署外键的占位值，Embedding 响应必须与预留 Deployment 一致。
- 重复事件、重复 Job 和并发 Worker 通过 source revision 唯一约束及行锁幂等；过期 revision 静默退出，不得覆盖新索引。
- 失败、取消或维度/模型版本漂移只标记候选版本失败，保留旧指针；错误只存稳定安全分类，不保存供应商原始响应。
- Worker 接管当前 revision 后先将 Document 标记为 processing；模型解析、版本预留或后续处理任一失败都必须收口为 failed，禁止遗留不可重试的 pending/processing。管理端对历史 pending 和 failed 允许手动重试，对 ready 文档允许主动重新索引；同 source revision 仅在 Parser/Chunker 版本一致时幂等跳过。
- Document 更新立即清空其当前索引指针；删除后软删除事实立即阻止检索，并通过独立清理 Job 最终删除 Chunk、正文和向量。

## 混合检索与 Citation

- `KnowledgeRetriever` 的候选 Knowledge Base 由调用方限定，但每次召回前仍按当前 AuthContext 重验 Base；每条结果返回前再次重验 Document 权限和 source revision。
- 只读取 Base 与所有活动 Document 同时指向的 `ready` IndexVersion；旧版本、过期 revision、软删除或指针不一致均不可返回正文。
- exact/FTS 与 pgvector 分路受限召回，按固定权重融合并以 Chunk ID 稳定打破同分；`top_k`、候选倍数、Document filter 与上下文 token 总量都有硬上限。
- Query Embedding 必须经统一 Embedding Runtime，deployment、dimensions 与 IndexVersion 完全匹配；Runtime 不可用时可降级 FTS，返回的模型事实异常则 fail-closed。
- Citation 只包含稳定 Base/Document/Chunk/Index、source revision、来源位置、标题与 attachment ID；回查重新鉴权并复核当前指针，不返回正文，来源变化后标记 stale。
- Agent 知识库绑定仅提供候选 Base ID；`knowledge.search` 还必须在 Agent Builder 中作为 Tool 有效绑定。Agent Run 默认采用 `tool_only` 策略，不得在每个 Model Step 前固定召回；只有工具已绑定、至少一个候选库实时可用且模型调用 `knowledge.search` 时，才按当前 actor 执行 Retriever。
- `knowledge.search` 未指定 Base 时在全部当前可用绑定库内统一召回，指定 Base/Document 只能与绑定和权限范围求交；全局 TopK、稳定去重、Score Threshold 与上下文 Token Budget 不得按库数量线性扩大。
- Tool Result 正文必须包装为 `UNTRUSTED KNOWLEDGE REFERENCES`，禁止拼入 system/developer instructions。空结果返回结构化事实，模型可继续回答、澄清或改写查询，不得伪造引用。
- 同一 Run 对完全相同的 actor、auth version、绑定/索引版本、query、filters 和检索配置复用结果；每次复用前仍重验候选与权限。单 Run 知识检索另设硬上限，并同时计入 Tool Call、Step 与 Token 预算。
- Invocation、Step、事件、审计与 Outbox 只保存查询次数、候选/返回/Citation 数、耗时、降级、缓存命中及稳定失败分类，不保存 Chunk 正文、向量或存储路径。

## 管理端页面

- 知识库管理页只调用已注册 Action，不在前端实现权限判断、检索排序、解析或索引状态机。
- Agent Builder 通过 `agent_knowledge_list/bind/unbind` 管理知识库绑定；选择器只展示当前可见、已启用且索引 ready 的候选，绑定只限定 Agent 候选范围且不得扩张当前用户权限。
- 状态使用知识库 CodeSet/`CodeTag`/`CodeSelect`，时间使用 `DateTimeText`，列表接入统一分页语义。
- 文档来源必须通过 `AttachmentPicker`/`AttachmentField` 选择并只提交 `attachment_id`；禁止拼 Object Key、缓存 transfer token 或自行生成附件下载地址。
- 检索测试固定使用知识库持久化 `RetrievalConfig`；无 Embedding 时明确展示降级状态，不在浏览器侧模拟向量结果。
- 管理端检索测试必须显式校验 Base 已启用且当前 IndexVersion 为 ready；未就绪返回可翻译冲突错误，禁止静默返回空结果。
- Citation 预览每次调用回查 Action 重新鉴权，只展示稳定标识、位置与 available/stale；撤权或 stale 后不得保留或展示受限正文与附件访问能力。
- 页面必须覆盖 loading、empty、error、permission revoked、failed/retry、stale 与移动端布局，并在交付验收中验证主流程和安全退化。

## 测试

后端测试统一放在 `platform/tests/`，至少覆盖：

- tenant、owner、visibility 与 realm 隔离；
- revision 冲突及软删除唯一约束；
- 附件原子绑定、替换、解绑与删除保护；
- Agent 绑定不扩权；
- SQL migration 的复合租户外键与 CHECK；
- 契约 Fake 的确定性与空范围 fail-closed。
- 管理端 CRUD、附件关联、失败重试、检索测试、Citation stale/撤权和无 Embedding 退化。
