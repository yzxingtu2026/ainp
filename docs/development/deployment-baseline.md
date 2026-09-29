# 下游部署基线同步

部署基线把公共发布/部署工具、依赖、测试、工作流和规则绑定到一个已发布的 AINP 稳定版本。仅升级 Go/Admin 或通过文档快照校验，不代表实际运行文件已同步。此协议不操作云资源、凭据、数据库或 `.deploy-state`。

## 来源与文件契约

`deploy/baseline/policy.json` 定义依赖闭包；发布 Job 从固定 Tag 检出文件，以原始字节 SHA256 和 Git 可执行位（0644/0755）生成 `deployment-baseline.json` Release 资产。commit 在发布时注入，不写回源提交；稳定根 Tag 与 `platform/` Tag 都必须解引用到同一完整 commit。下载器经已认证 `gh` 验证 Published、非 Draft、非 Prerelease、资产摘要、两个 Tag、文件哈希/模式和源树依赖闭包。来源查询失败、缺失资产或任意不一致均中止；没有 main/latest 回退。

`v0.4.0` 是审查基准，并不包含本工具和新的公开 PDF/CJK 测试包。配置示例中的版本须替换为首个包含本能力的后续正式版本；不能改写 `v0.4.0` Tag，也不能对它生成补挂资产后声称具备该能力。

| 归属 | 同步行为 |
| --- | --- |
| upstream | scripts/deployment、发布与发布说明入口、必要 Python 测试、MuPDF 脚本、示例、规则、文档受哈希管理 |
| project | deploy/linux/project.json、build.json 由下游拥有；工具不创建、不覆盖、不删除 |
| integration | 独立命名的原生/发布/预检/部署基线工作流逐文件接入；既有文件需明确确认归属，接入后受三方比较保护 |

不会覆盖整个 `.github/workflows`、`.gitignore`、`.agent/rules/main.md`、adapters 或业务目录。gitignore 与 Agent 登记按 `deploy/baseline/templates` 手工合并；verify 检查必需集成。现有 integration 文件如有业务步骤，先将业务步骤抽到独立工作流，再审查是否采用上游文件；不能用强制覆盖绕过差异。

路径必须是仓库内规范相对路径，拒绝 `..`、绝对路径、重复分隔符、符号链接和私有/账本目标。哈希不做 LF 转换；Windows 应确保这些公共文件检出为 LF，否则漂移检查会失败。模式比较按可执行位判断，不依赖宿主 umask。

## 项目配置

AINP 使用 `deploy/linux/project.json`，保持 `main`、原 required checks 和双 Tag。普通下游从 `deploy/baseline/examples/project.json`、`build.json` 建立自己的配置：

- repository 必须是自身仓库；发布预检、工作流与按版本部署都核对实际 remote/Actions repository。
- `tag_strategy=application` 只创建应用根 Tag，绝不推送 platform 配对 Tag。Go 依赖仍固定消费 AINP 的 platform 版本。
- `required_checks` 指向自己 PR 的门禁名称；发布追溯自身默认分支合并 PR 的精确 head，不能借用 AINP CI。
- `go_dir=backend`，单 Web 使用 `frontend_dir=admin`；多 Web 从 `deploy/baseline/examples/build-web-apps.json` 建立配置，`web_apps` 存在时可省略 `frontend_dir`，每项 source/mount/command/env/health 校验见 [多 Web 应用发布](binary-deployment.md#多-web-应用发布issue-374--376)。三个 commands 指向业务 composition；Host 默认不装配 Demo，禁止用官方演示入口代替业务入口。
- `build.json` 同时声明受约束的 `node_version`、`pnpm_version`、`admin_application`、`admin_modules` 和 `go_private`。`admin_application`/`admin_modules` 由原生打包脚本强制注入 Vite，不能依赖 runner 环境中的未声明变量；`go_private` 只声明模块匹配模式，不包含凭据。
- `cjk_test_package=github.com/yzxingtu2026/ainp/platform/contracts/pdfcjk` 在项目固定的 Go 依赖中执行三个公开 PDF 测试。使用完整 MuPDF 1.28.2 与 `extlib,static`，三项必须实际 PASS，不接受 skip 或 `[no tests to run]`。该包仅使用公开解析 API，不复制 internal 实现。
- `platform_version` 记录项目期望的统一基线。verify 对照 Go 依赖、部署 lock；通过 `admin_baseline.path` 与 `commit_field`（支持 `source.commit` 等嵌套路径）读取项目既有 Admin 门禁元数据，要求 source commit 一致。已有其它 Admin 门禁继续执行，保留其文件格式并配置同一来源 commit 的位置，不能以规则快照代替文件校验。

下游调用该 reusable workflow 时，只有确实声明了 `go_private` 才传递仓库 Secret，例如 `secrets: { private-module-token: ${{ secrets.PRIVATE_MODULE_TOKEN }}}`；不使用 `secrets: inherit`，避免把无关凭据扩散到构建 Job。

native-build 工作流从配置解析 Go version/sum、Node/pnpm 版本、Admin application/modules 和私有 Go 模块模式；可选 workflow inputs 必须与配置一致。构建继续注入同一 version/commit/build_time，并强制 Web 同源 API。`deploy/linux/build.json` 的 `node_version`、`pnpm_version`、`admin_application`、`admin_modules` 和 `go_private` 均为非敏感项目契约；私有模块凭据只能通过 reusable workflow 的可选 `private-module-token` secret 传入，禁止写入 JSON、日志、Artifact 或镜像。构建前仅在 `go_private` 非空时配置临时 Git URL 凭据和 `GOPRIVATE`，构建后清理凭据。 仓库身份预检统一读取 Git 配置中原始的 `remote.origin.url`，避免 `insteadOf` 展开出含凭据的 URL；仍须与 `project.json` 和 `GITHUB_REPOSITORY` 严格一致，任何不匹配都拒绝。

项目可在 `build.json` 增加可选 `pre_build_hook`（固定 `runtime-assets` 类型与项目拥有的 `deploy/linux/runtime-assets.json` 路径）。该配置只声明单输出资产指针（文件或目录中的明确文件）、Web public 目标、大小和 SHA-256；`fetch_runtime_assets.py` 与测试属于 upstream，三个原生构建/发布工作流属于 integration，均受 `policy.json` 哈希管理。`runtime-assets.json` 始终由下游拥有，apply 不创建或覆盖它；下游无需 fork integration 文件。固定可选凭据 `asset-store-key-id`、`asset-store-key-secret`、`asset-store-endpoint` 由调用方显式传递；不使用 `secrets: inherit`。本地/CI/云端验收边界与最小 OSS 只读权限见 `binary-deployment.md` 的构建前资产钩子章节。

`cloud.py update --target ... --version vX.Y.Z` 默认查询项目配置中的仓库；显式 `--repository` 必须一致。`--release` 仍支持离线/CI 精确引用，账本、apply 和 schema 兼容确认保持原契约。项目配置不放任何云或业务密钥。

## 首次从旧基线接入

1. 在下游短分支备份并确认现有部署、CI 和规则归属。保留 Windows/Compose 运行环境与部署账本。
2. 选择已发布、包含此能力的 AINP Tag，并从 Tag 解引用取得完整 commit。通过只读凭据获取该版本工具，在源检出目录运行工具并指定下游 `--root`；不能先覆盖下游公共脚本再做预检。
3. 在 `retired_workflows` 登记已停用的旧镜像工作流路径（没有旧发布路径时使用空数组）；同步预检和 verify 均拒绝已登记路径仍存在或发现 Docker 镜像推送步骤。
4. 从普通下游配置示例创建项目拥有的 project/build 配置，填写实际业务 composition、仓库与固定 platform 版本。同步相同版本的 Go/Admin 基线。
5. 合并 gitignore 与 Agent 模板。审查旧镜像发布工作流，在本分支显式停用/移出 `.github/workflows` 并记录迁移；原生发包启用前必须确保旧镜像发布路径不再由 Tag 触发。不得改动现有生产部署或数据库。
6. 先 plan，逐个处理 collision。已有同名公共文件只有经审查且内容/模式与选定来源完全一致，才可用重复的 `--adopt PATH` 登记。不同内容须先保留补丁并人工合并/迁出；工具没有 force。

```sh
python3 /path/to/pinned-source/scripts/deployment/baseline.py plan \
  --root /path/to/downstream --version vX.Y.Z --commit FULL_COMMIT
python3 /path/to/pinned-source/scripts/deployment/baseline.py apply \
  --root /path/to/downstream --version vX.Y.Z --commit FULL_COMMIT \
  --adopt scripts/deployment/cloud.py
cd /path/to/downstream
python3 scripts/deployment/baseline.py verify
```

plan 默认模式，只读取来源与目标；不修改下游文件、锁定记录或账本。apply 显式写入，所有文件预检完成后才执行；`deploy/baseline.lock.json` 最后写入，记录完整来源 manifest 与摘要，应提交到 Git。相同版本再次 apply 无内容或模式差异。

## 后续升级、冲突与恢复

升级同时更新期望 platform 版本、Go/Admin 与部署文件，重新对固定新 Tag/commit plan/apply/verify。工具以旧 lock 为祖先进行三方比较：新增文件不接管未托管路径；修改文件要求本地匹配旧版；删除仅限旧版托管且本地未改动文件；项目拥有文件始终保留。旧 lock 中同版本来源发生变化时拒绝执行。依赖闭包（`REQUIRED`）绑定生成它的工具版本，只在解析出的来源 manifest 与 verify 收敛的目标 manifest 上强制，不用当前版本的闭包回卡历史 lock；因此后续版本新增的必需文件不会阻断从旧 lock 升级，升级会把它们作为新增文件写入。

中途异常保留 `.deployment-baseline-transaction.json`，其中含旧文件与模式、旧 lock 和期望写入状态。工具拒绝继续同步；先检查错误，再运行：

```sh
python3 /path/to/pinned-source/scripts/deployment/baseline.py recover --root /path/to/downstream
```

recover 先检查全部文件仍处于旧/期望新状态，再恢复旧文件和 lock；若失败后有人编辑文件，拒绝覆盖并指出冲突。恢复成功后修复问题并重试 apply。同步/恢复由专用锁串行化；操作期间不要由编辑器或其它工具改动受管文件。

已成功升级的文件层回退采用 Git revert 同一升级提交，包含 lock、公共文件、Go/Admin 与项目版本配置，然后重新 verify。文件回退不等于应用或数据库回滚；生产回滚仍遵守维护窗口、备份、forward-only migration 与显式 schema 兼容确认。

## CI 与验证边界

部署基线工作流执行 `python3 scripts/deployment/baseline.py verify`：检查实际运行文件的原始字节与可执行位、部署依赖、项目仓库/配置、期望 platform 与 Go/Admin 来源、工作流和 Agent/gitignore 集成。将 `deployment-baseline` 加入下游 required checks；保留自身 Go/Admin/业务测试与 Ubuntu 22.04 native-build 门禁。

上游 Python 测试覆盖两个版本新增/修改/删除、幂等、未托管冲突、字节/模式漂移、来源失败、恶意路径、私有配置保护和失败恢复。独立 Linux fixture 使用 backend/admin、自己的仓库标识、应用单 Tag 和三个业务 composition 入口，执行真实 PDF/CJK 与 API/Worker/migrate/Web 构建。PR fixture 可以显式连接当前源码以验证尚未发布的新能力；这不是正式来源接入，正式 plan/apply 仍只接受已发布版本。

fixture 通过仅证明同步与构建契约，不证明任意现有下游的生产部署、Secrets/OIDC/OSS、数据库、Worker 实际任务、多实例 HA 或灾备已验收。
