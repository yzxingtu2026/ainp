# Issue 自动编码

工作流 `.github/workflows/issue-auto-coding.yml` 运行在 GitHub 托管的临时 runner，保留两种入口：

- `issues: assigned/labeled`：重新确认 Issue 为 open、包含 `auto-coding` 标签且分配给指定账号；固定分支 `feat/issue-<编号>-auto-coding` 已有任何状态的 PR 时跳过，否则要求 Codex 实施并创建 PR。
- `issue_comment: created`：监听 Issue 和 PR 的普通评论；只接受提及 `AUTO_CODING_ASSIGNEE` 且评论者有本仓库 write/admin 权限的评论，并排除代理自己的评论。Issue 必须仍为 open、带 `auto-coding` 标签并分配给代理；PR 还须是代理创建的同仓库 open PR，其分支为对应 Issue 的固定分支。评论入口提供仓库、评论所在 Issue/PR 的类型与编号、关联 Issue 编号及评论 ID（Issue 评论使用当前编号，PR 评论从固定分支名解析关联 Issue）；Codex 据此读取评论与讨论后决定实施、修改 PR、解释或不操作，不强制创建新 PR。PR 代码行的审查评论不属于 `issue_comment`。

同一目标编号的任务串行；GitHub concurrency 最多保留一个 pending 任务，新的 pending 会替换旧的。GitHub 仓库中的 Issue 和 PR 共用编号序列。评论入口在默认分支上的工作流生效后才可触发。关闭的 Issue/PR、不符合范围或未提及代理的评论均跳过。失败且无 PR 时可以重新分配、重贴标签，或由有权成员在符合条件的 Issue 评论提及代理重试；Codex 必须根据实际状态决定下一步。

## 配置

在仓库 Actions variables 中设置：

- `AUTO_CODING_ASSIGNEE`：指定 GitHub login。
- `OPENAI_API_BASE`：runner 可访问的 HTTPS Responses API 基础地址，不含凭据。
- `OPENAI_MODEL`：网关支持的编码模型名称。

在 Actions secrets 中设置 `OPENAI_API_KEY` 和 `AUTO_CODING_GITHUB_TOKEN`。后者应为专用 GitHub App installation token 或细粒度 PAT，能够读取代码、读写 Issues、读写 Pull requests（含请求评审）和写入 Contents，且对本仓库有效；按组织策略限制持有人和有效期。PR 是否触发现有 CI 取决于凭据类型及 GitHub 和组织策略；不要假设 `GITHUB_TOKEN` 创建的 PR 会触发常规 CI。启用 Actions、确认组织允许所需操作并保留分支保护及人工评审。工作流不自动批准、合并或标记测试通过。

在安装 Codex 前，工作流通过 `gh api user` 核对 token 身份，并查询该账号对本仓库的有效角色，必须是 write/admin，否则提前失败。这能发现账号未加入仓库或只有只读权限；只读查询无法证明细粒度 PAT 的 Contents、Issues、Pull requests 各项写入权限，应在配置时逐项核对，并通过真实的推送、评论和 PR 测试验证。

工作流每次从 npm 安装最新版 `@openai/codex`，版本不固定，CLI 更新可能导致配置或行为变化；遇到兼容问题时检查 Actions 日志中的版本并重新验证。工作流以临时用户级 `CODEX_HOME/config.toml` 配置 Responses provider，`env_key` 引用环境变量，不写入 API key。令牌通过 `GH_TOKEN` 和 `gh auth setup-git` 用于 Git/gh，不写入仓库。安装和网络访问依赖 npm、GitHub 和模型网关可达。禁止在配置或 Issue 中放置密钥。

## 安全与排障

Codex 使用无交互、无沙箱模式，能读取运行时凭据并执行任意命令。评论触发前必须完成提及、评论者权限、目标 Issue/PR 状态验证；评论正文不直接拼入 shell 命令，只把评论 ID 交给 Codex 自行读取。仍只应由可信成员控制分配、标签、Issue 内容及评论；模型供应商、依赖和 runner 均处于信任边界内，不能保证恶意任务不会外传凭据。不要在长期运行或共享的 self-hosted runner 使用此工作流。

评论所在的 PR 分支会检出供 Codex 继续；Issue 评论入口若已有固定远端分支则检出，否则从默认分支检出。runner 不会在下次运行自动挂载旧工作区，未推送的本地提交无法自动恢复；本工作流未配置 artifact 保存。配置缺失、安装或认证失败、权限不足、网关不可达都会使任务失败；检查 Actions 日志、凭据有效期、权限及网关可达性，避免输出密钥。Codex 遇阻应按指令在原讨论说明；若它未能成功执行，评论不保证送达。初始 Issue 入口正常退出但未创建固定分支 PR 时工作流失败；评论入口由 Codex 自主判断，不要求新 PR。自动检查不证明实现、评审人、测试或 CI 成功。推送成功但创建 PR 失败时，人工检查远端分支后再决定如何重试。

启用前用测试 Issue 验证两种标签/分配顺序、关闭与不满足条件、重复触发、权限不足预检、Issue 和 PR 提及、无权限评论、自评论、阻碍留言、请求评审和 CI 实际触发；检查日志及 Git 历史无密钥。工作流不会启动本地数据库或浏览器作为初始任务要求，不改变现有 PR CI。
