# Issue 自动编码

工作流 `.github/workflows/issue-auto-coding.yml` 仅运行在 GitHub 托管的临时 runner。`assigned`、`labeled` 事件触发后，以 GitHub API 重新确认 Issue 为 open、包含 `auto-coding` 标签且分配给指定账号。同一 Issue 的任务串行；执行前检查固定分支 `feat/issue-<编号>-auto-coding` 是否已有任何状态的 PR，存在则跳过。GitHub concurrency 最多保留一个 pending 任务，新的 pending 会替换旧的；失败且无 PR 时可通过重新分配或重新贴标签重试。关闭的 PR 也会阻止自动重试，需人工处理。

## 配置

在仓库 Actions variables 中设置：

- `AUTO_CODING_ASSIGNEE`：指定 GitHub login。
- `OPENAI_API_BASE`：runner 可访问的 HTTPS Responses API 基础地址，不含凭据。
- `OPENAI_MODEL`：网关支持的编码模型名称。

在 Actions secrets 中设置 `OPENAI_API_KEY` 和 `AUTO_CODING_GITHUB_TOKEN`。后者应为专用 GitHub App installation token 或细粒度 PAT，能够读取代码、读写 Issues、读写 Pull requests（含请求评审）和写入 Contents，且对本仓库有效；按组织策略限制持有人和有效期。PR 是否触发现有 CI 取决于凭据类型及 GitHub 和组织策略；不要假设 `GITHUB_TOKEN` 创建的 PR 会触发常规 CI。启用 Actions、确认组织允许所需操作并保留分支保护及人工评审。工作流不自动批准、合并或标记测试通过。

工作流每次从 npm 安装最新版 `@openai/codex`，版本不固定，CLI 更新可能导致配置或行为变化；遇到兼容问题时检查 Actions 日志中的版本并重新验证。工作流以临时用户级 `CODEX_HOME/config.toml` 配置 Responses provider，`env_key` 引用环境变量，不写入 API key。令牌通过 `GH_TOKEN` 和 `gh auth setup-git` 用于 Git/gh，不写入仓库。安装和网络访问依赖 npm、GitHub 和模型网关可达。禁止在配置或 Issue 中放置密钥。

## 安全与排障

Codex 使用无交互、无沙箱模式，能读取运行时凭据并执行任意命令。仅允许可信成员控制分配、标签、Issue 内容及评论；模型供应商、依赖和 runner 均处于信任边界内，不能保证恶意任务不会外传凭据。不要在长期运行或共享的 self-hosted runner 使用此工作流。

配置缺失、安装或认证失败、权限不足、网关不可达都会使任务失败；检查 Actions 日志、凭据有效期、权限及网关可达性，避免输出密钥。Codex 遇阻应按指令评论 Issue；若它未能成功执行，评论不保证送达。Codex 正常退出但未创建固定分支 PR 时工作流失败并提示检查日志和评论；非零退出同样失败。自动检查仅确认对应 PR 存在，不证明实现、评审人、测试或 CI 成功。推送成功但创建 PR 失败时，人工检查远端分支后再决定如何重试。

启用前用测试 Issue 验证两种标签/分配顺序、关闭与不满足条件、重复触发、阻碍留言、请求评审和 CI 实际触发；检查日志及 Git 历史无密钥。工作流不会启动本地数据库或浏览器作为任务要求，不改变现有 PR CI。
