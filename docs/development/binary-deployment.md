# 原生二进制发布与阿里云 ECS 部署 SOP（v1）

关联：Issue #352。此协议适用于 AINP 及遵守公开 Host/配置/迁移契约的下游项目。

> **验收状态：实现基线，Linux CI 已通过，待 OIDC/OSS 和真实测试 ECS 验收。** 离线测试不等于生产验收；不能直接把已有 Windows/Compose 生产环境交给 `init`。
> v1 是**单项目、单 ECS、有维护窗口**部署，不承诺零停机或任务无中断。不创建 ACR，不构建应用容器镜像，不部署运维面板。

## 1. 组成与边界

```text
PR CI → Linux/CJK 门禁（不在合入 main 后重复运行）
正式 SemVer Tag → 原生构建 → 私有 OSS 不可变发布包 → GitHub Release 元数据
管理员部署命令 → 阿里云 CLI → 固定版本 ECS 启动模板（仅首次）→ 云助手
  → 校验发布包 → systemd API + Worker + AI Worker / Nginx 静态 Web
```

- 三个可执行文件：`api`、`worker`、`migrate`。通用 Worker 与 AI Worker 共用同一个 `worker`，队列/并发不同。Web 是静态文件，没有 Node 常驻进程。
- 初始支持 Ubuntu **22.04 / amd64**、PostgreSQL **17 + pgvector**、兼容 Asynq 的 Redis（需认证与 Lua）。不是任意 Linux 通用包。
- MuPDF 1.28.2 必须保留完整 CJK 字库，复用已校验源码构建脚本；使用 `extlib,static`，不等于所有 glibc 依赖都静态链接。原生包仍可能较大，不能用格式转换承诺体积降到某个数值。
- 发布只传包，不自动更新生产。管理员显式执行 `--apply` 才会创建付费资源或改生产；默认仅打印计划，无云 API 调用。
- 基础设施容器可以保留：**不构建应用镜像不代表必须去掉 PostgreSQL/Redis 容器**。
- 本版不创建数据库、Redis、VPC、安全组、RAM/OIDC、OSS、域名记录或签发证书。这些是可复用的公司基础设施/启动模板前置条件，不在每次应用发布中重新创建。
- 不自动清理 ECS、磁盘、旧版本或备份；避免意外销毁。费用预算、保留周期与磁盘告警需平台管理员设置。

## 2. 公司级一次性准备

### 2.1 三种权限分离

1. **CI 发布角色**：GitHub OIDC 换取短期 STS；只允许该项目 release 前缀的 `oss:PutObject` / `oss:GetObject`（重试核验）。不要授予 ECS 或配置/数据库备份读取权限。
2. **管理员部署角色**：本地 Alibaba Cloud CLI profile 使用短期凭证/角色；按地域、项目标签约束 `ecs:RunInstances`、`ecs:DescribeInstances`、`ecs:RunCommand`、`ecs:DescribeInvocationResults`、`ecs:DescribeCloudAssistantStatus` 及指定实例角色的 `ram:PassRole`。RunCommand 是 root 执行权限，绝不能交给公开 PR。
3. **ECS 实例 RAM 角色**：仅读当前项目 release/config 对象，写/读（幂等核验用）自己的 backup 前缀；不允许上传/覆盖 release、删除备份或管理 ECS。通过 **IMDSv2** 获取短期凭证，不写长期 AK。

OIDC 信任策略必须约束公司 GitHub 组织、仓库及 `release` environment 的 subject/audience；GitHub `release` environment 设置允许正式 Tag、保护规则/审批。仅配置 action 输入不等于已经建立信任关系。

OSS：私有桶、禁止公共 ACL、启用服务端加密；release 与配置/备份最好分桶。按项目分前缀。对象默认禁止覆盖；发布先上传包，最后上传 `reference.json` 完成标记。当前发布协议依赖 `x-oss-forbid-overwrite`；OSS 开启或暂停版本控制时该请求头不生效，不能用版本控制替代禁止覆盖。优先使用未开启过版本控制的发布桶；已有版本控制桶需另行验证服务端禁止覆盖策略，连接测试通过后再发布。备份桶的版本控制/保留策略单独配置。实例通过 target 的 `release_oss_endpoint` 指定同地域 HTTPS 内网 endpoint（只改访问端点，不改变桶、对象或校验和）；GitHub Runner 使用发布用公网 endpoint，Release reference 保留发布端点。

> 实例上的进程可接触实例身份，IMDSv2 不是 UID 隔离。业务进程/任意插件不应被视为不可信租户；需要更严格隔离时增加主机 IMDS 访问控制/专用身份代理，不把多个互不信任项目放在同一 ECS。

### 2.2 首次配置后的 OSS 连接测试

先运行 `Check OSS Release Access`，无需构建应用或创建正式 Tag。工作流复用 `release` environment 的五个变量和 OIDC 角色，手动执行，PR CI 不访问云资源。

- 工作流文件必须先通过 PR 进入默认分支，GitHub 才开放 `workflow_dispatch`。合入前只能做离线测试，不能把它记录成真实云端通过。
- 在 `release` environment 设置允许 `main`（手动预检）及正式版本 Tag 的部署规则，并按团队权限配置审批；不要为方便测试向所有分支开放发布角色。
- Actions → **Check OSS Release Access** → **Run workflow** → 选择 `main`，project 填 `ainp`。下游使用与其 RAM 目录授权一致的项目名。
- 检查 OIDC 换证、Put/Get/Head、同内容重试与不同内容禁止覆盖。测试对象为 `releases/ainp/_checks/<随机ID>/probe.txt`（前缀/项目可配置），无发布 manifest/reference，无法作为部署包使用。
- 测试只留一份几十字节的对象，不申请删除权限。可由管理员定期清理 `_checks/`，不会触碰正式版本。
- STS 步骤失败时检查 issuer/audience/subject/角色信任；OSS 403 时检查角色策略与 Bucket 策略；覆盖保护失败时检查 Bucket 版本控制及服务端保护配置。失败不得继续正式发布。

```bash
gh workflow run check-oss.yml --ref main -f project=ainp
```

连接通过仅证明发布身份和 OSS 路径可用。应用构建、真实测试 ECS、数据库和 Worker、更新回滚仍需独立验收；本入口不会创建 ECS、发布正式 Tag 或放宽生产对 `-ci` 包的限制。

### 2.3 测试原生发包

合入默认分支后，使用 `Test Native Release` 手动验证完整的“Linux 构建 → 私有 OSS”链路。工作流只允许从 `main` 运行，版本固定为 `v0.0.0-ci`，先验证本地包的 manifest、归档、SHA256 和安全解包，再通过同一 OIDC 角色上传并逐对象读回验证四个文件。它不会创建 GitHub Release、ECS 或生产变更。

测试对象路径为 `releases/ainp/v0.0.0-ci/<commit>/`。重复运行同一提交必须命中不可变对象的同内容重试；不同内容或覆盖保护失败必须失败。`cloud.py` 仅允许 `-ci` 包进入明确标记为 `environment=test` 的测试环境；生产初始化、更新和回滚仍必须使用稳定版本。

Actions → **Test Native Release** → **Run workflow**，分支选择 `main`，版本保持默认 `v0.0.0-ci`。测试发包通过后才进入测试 ECS；通过不等于真实 ECS、数据库、Worker、Nginx/SSL 验收。

### Web API 地址与构建约束

生产 Web 使用当前页面同源地址：`VITE_SERVICE_BASE_URL=/`、`VITE_OTHER_SERVICE_BASE_URL={"demo":"/"}`，由 Nginx 转发 API、附件、OAuth 和 SSE 请求。`frontend/.env.prod` 默认同源；原生 `package.py` 打包时还会显式覆盖构建机同名环境变量，与存量 Docker 构建保持一致。本地 `pnpm dev` 继续使用 `.env.test` 的 loopback 后端和开发代理。

这些 Vite 变量会编译进静态 JS；修改 ECS 环境变量、target/private 配置或重启服务不会改变已发布 Web 的请求地址。若旧包误编入 `127.0.0.1:8080`，必须从修复后的提交重新构建完整包，以新的 commit/reference 执行现有 ECS 的 `update`，不要重新 `init`，也不要覆盖原有不可变 OSS 包或单独替换 Web 破坏版本与校验和一致性。测试环境可继续使用 `v0.0.0-ci`（新 commit 路径），无需创建正式版本 Tag。

### 2.4 ECS 启动模板

使用**数字版本**，禁止 `Latest`/`Default`。模板应固定并评审：

- Ubuntu 22.04 amd64 系统镜像、按量规格、磁盘类型/容量/删除策略、VPC/交换机/安全组。
- 仅开放公网 80/443，SSH 按管理员来源限制或不开放；API 仅监听 127.0.0.1；数据库/Redis 不能暴露公网。
- 公网 IP/EIP、出口/NAT/带宽计费策略由模板/平台预先规划。控制器不自动申请 EIP 或修改 DNS。
- 云助手 Agent 已装且可连通；实例 RAM 角色正确；控制器创建时强制 PostPaid、IMDSv2 required。已有 ECS 也必须按模板设置这两项及正确角色。
- 可访问 Ubuntu/PGDG apt、PyPI、OSS 与元数据服务。首次装 Nginx、Python venv、PG17 client 等；依赖准备成功后复用，不在每次更新重复 apt 安装。bootstrap 在首次 apt 操作前，仅将 Ubuntu 官方 `archive.ubuntu.com`/`security.ubuntu.com` 源切换到阿里云 ECS VPC 内网镜像 `http://mirrors.cloud.aliyuncs.com/ubuntu`；不会改写 PGDG 或其他第三方源。
- 配置需要的数据库 CA 证书已存在且服务用户可读。ECS 启动模板不应该硬编码数据库密码、JWT 或管理员密码。
- 如果使用本机 PG/Redis 容器：模板/独立基础设施脚本启动固定版本、持久化目录与认证，端口发布到 **127.0.0.1**；Docker 引擎/数据库生命周期不由应用脚本维护。数据库容器需随开机启动且应用服务启动前可用。

一个目标文件绑定一个项目/环境/实例。已有实例须有 `ainp-project=<项目>`、`ainp-environment=<环境>` 标签且为按量实例。指定 `instance_id` 可跳过购买；这不是把任意现有业务环境自动迁移进来。

## 3. 仓库 CI / 发布配置

GitHub environment **release** 配置变量：

| 变量 | 含义 |
|---|---|
| `OSS_BUCKET` | 私有发布桶 |
| `OSS_ENDPOINT` | CI 可达的 HTTPS OSS endpoint |
| `OSS_PREFIX` | 例如 `releases`，不以 `/` 开头 |
| `ALIBABA_CLOUD_OIDC_PROVIDER_ARN` | 公司 RAM OIDC provider |
| `ALIBABA_CLOUD_RELEASE_ROLE_ARN` | 受限 CI 发布角色 |

既有 `RELEASE_NOTES_*` 可选变量/Secret 保留，模型失败降级确定性发布说明。

- `.github/workflows/native-build.yml`：可复用构建（不持有云权限）。Go/Node/pnpm、固定 MuPDF，CJK 测试、CLI 启动 smoke、动态库检查、前端构建、SHA256 清单。
- `.github/workflows/ci.yml`：只监听 Pull Request，保留 backend/frontend/windows/repository gates，替换 `containers` 为 **`linux-release`** 聚合门禁。合入 `main` 不再重复运行完整构建；分支保护仍须要求这些 PR checks，不能仅删除旧 job 而不更新规则。
- `.github/workflows/release-binaries.yml`：根 Tag 与 `platform/` Tag 同提交，并通过 GitHub API 找到产生该 `main` 提交的已合并 PR，再校验该 PR head 的完整 CI 成功；私有 OSS 发布成功后发布 GitHub Release。已发布 Release 重跑跳过，不重新生成或覆盖包。
- `.github/workflows/test-native-release.yml`：仅允许 `main` 手动运行固定 `v0.0.0-ci`，验证完整包上传和 OSS 读回；不创建 Release/ECS。该包只能用于 `environment=test` 的真实 ECS 验收，不能用于生产部署。
- 不再有 GHCR 推送/SSH 自动演示部署。旧 Docker/Compose 文件仅为存量兼容，不属于新发布主路径。
- 发布入口仍为仓库根目录 `python3 scripts/release.py vX.Y.Z`；需干净、同步 main。

发布包只有 `bin/*`、`web/*` 和 `release.json`；源码、私有配置、数据库密码、证书私钥、state、附件不得混入。`reference.json` 记录项目/版本/commit/平台/OSS key/包 SHA256，从**可信 CI Release**取得，不从用户提交或未经审核消息复制；SHA256 是完整性校验，不替代来源信任或签名。

## 4. 环境配置与数据库

复制 `deploy/linux/target.example.json` 为已忽略的 `target.local.json`；复制 `private.example.json` 到安全位置，仅填真实配置后上传**私有配置桶**。用 SHA256 固定配置对象。目标文件仅有资源标识与哈希，没有 AK/数据库密码；也不建议公开。

- `JWT_SECRET`：至少 32 字符随机秘密。
- `CREDENTIAL_ENCRYPTION_KEY`：**32 随机字节的标准 Base64 编码**（例如 `openssl rand -base64 32`）。不能随应用升级更换，否则既有加密凭证无法解密。
- 外部 PG：`database_mode=external`。示例默认用于专用网络 RDS，显式设置 `sslmode=disable`，无需 CA；部署者负责确认使用内网地址及 VPC/安全组隔离。公网或需要 TLS 的连接使用 `sslmode=verify-full sslrootcert=<已安装的 CA 文件>`。预检只接受这两种明确选择，不自动降级 TLS；应用与 pg_dump 复用同一 DSN。
- 本机容器 PG：`database_mode=local-container`，DSN 用 `host=127.0.0.1 port=<映射端口> ... sslmode=disable`。**不能沿用 Compose 内的 `host=postgres`**。同网络另一 ECS 上的容器 PG 使用 external 模式，并按上述网络边界显式选择 SSL 模式。
- 两种模式都不自动删除/创建数据库，必须先准备空数据库（首次）或已安装的同项目数据库（更新），并提供 migrations/pgvector 必要权限。预检 PG17、vector 可用、Redis 认证/Lua。
- 本版 Redis DB 固定为 0，独立 Redis 或经隔离评估的实例；不要把未隔离的多个项目任务混进同一 DB。
- `backup_oss` / `backup_prefix` 是迁移前 PG custom-format 逻辑备份目的地。备份上传并校验后才迁移。
- 私有配置首次写入 `/etc/<project>/private.json`，root-only；服务仅能读取受限 runtime/app 配置。更新复用主机配置，不自动从 OSS 刷新，也不旋转凭证。

生产配置是 JSON **数据**，不 `source`、不拼 shell。生成的 `app.json` 是 YAML 可接受的 JSON 子集；launcher 用 `execve` 注入精确环境，避免密码中 `$`、`%`、换行被 shell/systemd 解释。

## 5. 一键操作

前置：安装 Python 3.10+、支持 RunCommand ClientToken/TerminationMode 参数的阿里云 CLI 并配置相应 profile；从已审核的当前框架基线工作目录运行。以下 `reference.json` 是发布成功后取得的文件，不是自己填写的版本号。

```bash
# 在仓库根目录：默认只看计划，不创建/更新资源
python3 scripts/deployment/cloud.py init \
  --target deploy/linux/target.local.json --release /secure/releases/vX.Y.Z/reference.json

# 首次购买按量 ECS 并安装（加 --apply 才执行）
python3 scripts/deployment/cloud.py init \
  --target deploy/linux/target.local.json --release /secure/releases/vX.Y.Z/reference.json --apply

# 更新同一目标，先不加 --apply 看计划，然后在维护窗口执行
python3 scripts/deployment/cloud.py update \
  --target deploy/linux/target.local.json --release /secure/releases/vX.Y.Z/reference.json --apply

# 回滚到该主机实际成功运行过的旧包：先评审数据库向后兼容，再显式确认
python3 scripts/deployment/cloud.py rollback \
  --target deploy/linux/target.local.json --release /secure/releases/OLD/reference.json \
  --confirm-schema-compatible --apply

# 查询已下发的命令；不重新下发，也不要求 --apply
python3 scripts/deployment/cloud.py status --target deploy/linux/target.local.json
```

`.deploy-state/<project>-<environment>.json` 是控制端操作账本，含 ClientToken、InstanceId、InvokeId、目标绑定哈希，必须持久保存/限制权限/备份。团队使用**一个受控部署工作站/执行器及共享串行审批**，不要各自删账本执行 init。不同工作站的本地锁不构成跨工作站互斥，主机锁负责远程部署互斥，但不能阻止从丢失账本的工作站另买实例。

创建请求发出前落盘 ClientToken；云助手请求前记 `dispatching`，传入独立 ClientToken 并使用 ProcessTree 超时终止模式，拿到 InvokeId 后记 `running`。不保存可再次执行的云助手命令。不确定请求结果时拒绝自动重试，避免重复购买或同时更新。资源已创建但尚未下发命令时，重跑可继续同一实例；已开始 init 失败不能盲目重初始化。

部署日志：`bootstrap.sh`、`apt.sh`、`reporting.py` 与主机 Python 文件一起压缩进命令，在 ECS 临时目录解包执行，仍受原始 16 KiB/API 编码上限约束。安装依赖、OSS 下载、PG/Redis 预检、迁移、初始化和健康检查输出 `AINP_PROGRESS` 时间戳记录；长步骤每 15 秒输出心跳，完成/失败立即刷新。Python 使用无缓冲模式，`cloud.py` 每 5 秒读取云助手 `Output`，只增量打印完整、安全的进度记录并去重，控制台显示仍受云助手上报/刷新周期影响。apt 锁重试保持最多 600 秒，bootstrap 子命令原始 stdout/stderr 写入 `/var/log/ainp-bootstrap.log`（0600），主机迁移等原始失败输出仍在 `/var/log/ainp-deployment-errors.log`，不为实时日志暴露 DSN、请求体或密码。

修改本地 `private.*.json` 不会更新 OSS 或现有 ECS：需另行上传新的不可变私有配置对象并更新目标中的 key/SHA256。已落盘 `/etc/<project>/private.json` 的失败 init 必须先核对主机状态后恢复；不要删除本地账本或以修改哈希绕过恢复保护。此流程不能与 apt 阶段（尚未写入私有配置）的安全重试混淆。

阿里云 CLI 调用失败时，控制器只从 JSON 响应中输出经过长度限制和单行化处理的 `error_code`、HTTP 状态、`message`、`RequestId`。命令参数、完整 stdout/stderr、恢复命令和未知字段不会回显；非 JSON、空响应、超大响应、CLI 缺失或超时仍使用通用错误。云助手命令失败时输出状态、退出码、错误码及最多 4000 字符的日志尾部；日志会移除控制序列并脱敏常见 password/secret/token/authorization 和 URL 密码，但仍应仅在受控部署终端查看。任何失败都必须结合已持久化状态与云审计核对后再重试。

## 6. 主机执行顺序与失败语义

主机目录：

```text
/opt/<project>/releases/<version>-<sha前16位>/    root-owned，可执行文件与静态 Web
/opt/<project>/current                         原子替换软链接
/opt/<project>/deployment.json                 当前/前一个/历史版本与备份事实
/etc/<project>/{private,runtime,app}.json        root/服务组最小权限
/etc/<project>/tls/{fullchain,privkey}.pem       与应用发布独立
/var/lib/<project>/{state,objects,logs}/         服务用户可写持久数据
```

首次：
1. 校验目标配置、OSS SHA256、全部成员 SHA256、平台/版本/commit；安全解包禁止链接、设备、路径穿越、重复成员、超限与清单外文件。
2. 预检空 PG/pgvector、Redis；创建专用非 root 用户及三个 systemd units，API 仅监听 loopback。
3. Nginx 进入 HTTPS 503 维护页，启动 API，仅从本机调用 setup initialize，创建租户/管理员/当前 schema/baseline。**不会直接伪造 `installed: true`**。
4. 重启 API、确认 state installed、healthz 版本/commit、Worker 进程稳定且来自目标 release；开放 Nginx。
5. 从本机经域名 SNI/系统 CA 验证 HTTPS API 与 Web `build.json`，成功后开启服务开机启动。真实公网 DNS/安全组可达性另行验收。

更新：校验/预检 → 维护页 → 关闭开机启动并停止所有应用进程（给 Worker 90 秒退出）→ `pg_dump17` + 私有 OSS 备份 → 非 root migration runner forward-only → 切软链 → 启动/验收 → 开放流量/恢复开机启动。

更新/回滚停止服务后，以精确 glob（例如 `[a]inp-api.service`）对本项目三个服务执行 `systemctl reset-failed`：已从 systemd 内存卸载的服务安全跳过，仍加载的服务重置失败状态与重启计数。不得不带服务参数全局重置，也不得忽略命令错误；实际重置失败仍保留维护状态并中止备份/迁移。该修复不绕过已有失败部署的恢复检查，`failed-maintenance` 现场仍需审计后恢复。

回滚：同样停止应用并备份，**不执行 down/up migration**，切换到已知成功的旧包；必须人工确认旧代码兼容现有 schema。存储附件、state、JWT/key、证书不回退。框架无法自动证明任意业务迁移可回滚，`--confirm-schema-compatible` 不是自动校验证明。

- 迁移/健康检查失败：保留维护页、停止应用、不自动恢复旧代码或数据库，保留备份 key 与状态。否则旧 Worker 可能运行在不兼容 schema 上造成二次损害。
- 断电/进程被杀时可能只留下 `deploying`，需检查真实主机状态；脚本不能将 SIGKILL 的现场解释为已回滚。
- Worker 的自动门禁是 Redis 能力、systemd 稳定性与 `/proc/PID/exe` 路径，不是“已完成真实任务”的证明。验收要实际执行通用任务、AI 任务、SSE、MCP/OAuth 和附件上传。
- 临时维护是计划内行为；已有执行中的长任务按框架重试/幂等处理，不承诺不重试或不丢失业务外部副作用。

HTTPS 验收在 Nginx reload 后对瞬时错误进行有限重试（最多 5 次重试，1 秒间隔，重试窗口 30 秒），避免旧维护页进程短暂返回 503 导致误判；证书校验、API/Web 版本与 commit 校验仍必须通过。

## 7. 故障与恢复

1. **控制台请求超时**：看本地账本，拿 InvokeId 到云助手查看；`status` 只轮询。`creating`/`dispatching` 无返回 ID 时必须根据 ECS/ActionTrail 审计与项目标签人工核对；确认准确 InstanceId/InvokeId 后才允许修复账本。禁止直接删除账本重试。
2. **首次 init 的 apt/dpkg 锁失败**：bootstrap 对 apt/dpkg 锁失败最多等待 600 秒，只重试锁错误；超时返回 `AINP_APT_LOCK_TIMEOUT`，不删除 lock 文件、不终止 `unattended-upgrades`。如果本地账本为 `status=failed` 且已经保存 `instance_id`/`invoke_id`，确认系统更新已结束后，用**同一 target、同一 release reference（包括 release SHA256）**再次执行原 `init --apply`。控制器会先校验本地 target SHA256、发布包身份、配置 SHA256，然后重新 Describe ECS，强制校验项目/环境标签和 `PostPaid`，只在原实例上重新派发 Cloud Assistant，不会再次 `RunInstances`；历史尝试保存在 `attempts` 中。若账本为 `creating`/`dispatching` 或缺少实例/调用 ID，先查 ECS/ActionTrail，禁止删除 `.deploy-state` 或盲目重试。
3. **迁移失败**：查 `/opt/<project>/deployment.json`、root-only `/var/log/ainp-deployment-errors.log`、对应服务 journal，评估迁移事务及备份。可兼容则显式回滚；不可兼容则制作前向修复包或按专项恢复方案恢复数据库。不要自动 drop 数据库。
4. **首次 setup 失败**：可能已有部分表/管理员/state，先审计，不能自动再建租户/覆盖 state。确定是一次性测试库才人工重建并重新 provision；已有数据必须走专项恢复。
5. **备份验证**：定期在隔离 PG17 上运行 `pg_restore --list` 和完整恢复演练。脚本只验证 dump 命令/非空/OSS 上传，不能替代恢复测试。
6. **整机故障**：应用包可重建，不等于完整灾备。`state`、附件目录及本机 PG/Redis 数据盘另用 ECS 快照/应用备份；远端数据库使用其备份/PITR。不要只备份二进制或 PG 而忽略附件。
7. **磁盘/日志**：发布包保留不自动删；定期审计容量，保留当前及回滚候选，维护备份与日志生命周期。应用有日志轮转，部署失败诊断日志含潜在敏感信息，root-only，管理员另设 logrotate/保留策略。

## 8. HTTPS 与证书生命周期

初次私有配置带已签发 fullchain/private key；只在 init 写入固定 TLS 路径。更新/回滚不覆盖证书文件，拒绝回退 HTTP。签发前需要域名授权等步骤，不能由发布脚本凭空完成。

**自动续期/部署不是本脚本自行实现的能力。** 公司按所购证书产品，在阿里云数字证书管理服务中配置 ECS/Nginx 自动部署任务/续期任务及权限（产品资格/费用/地域以账号控制台实际能力为准）。固定覆盖上面的两个文件后执行 `nginx -t && systemctl reload nginx`；要求失败不替换有效证书，先在测试 ECS 验证该任务的原子性/回退方式。配置证书临期与任务失败告警。

证书平台如果不能满足原子部署，要单独实现先下载到临时目录、验证 key/cert/有效期/域名、原子切换、nginx 校验与失败恢复的 adapter；**未接好续期任务前不能宣称 SSL 已实现自动管理**。这不阻塞应用二进制发布基线，但阻塞“无人值守 HTTPS”验收。

## 9. 下游接入与基线更新

同步 `.github/workflows/{native-build,release-binaries}.yml`、`scripts/deployment/`、相关测试和本文约定。不要复制后改出四套互不兼容的脚本。

允许的项目差异放入 `deploy/linux/build.json`：project、Go module 目录、API/Worker/migrate 入口、buildinfo package、frontend 目录、CJK 测试 package。`native-build` 还支持 `go-version-file` / `go-sum-file` inputs。不同目录结构同时修改调用端对应 input；不能只改 JSON 而让 setup-go 读错 go.mod。

下游必须满足：
- Go 程序支持 `-config`；migration runner 支持 `up`，首次初始化走公开 Host setup hooks，业务 schema 必须由 hooks 纳入。
- API / Worker 注入同一 version/commit/build_time，`/healthz` 信封与内嵌资源契约保持一致。
- Worker 支持 `WORKER_QUEUES` / `WORKER_CONCURRENCY`，应用与迁移入口来自同一业务 composition。
- 前端构建为 dist 静态资源；保留精确 OAuth/MCP 路由，不使用宽泛 `/oauth/` 代理覆盖 SPA 授权页面。
- 正式 Tag 协议如果下游不发布 `platform/` 子模块 Tag，应替换 release validate 的配对规则及 `scripts/release.py` 的仓库身份/remote/Tag 逻辑；不得误向 ainp 发 Tag。原生构建/部署契约不要求下游伪造 platform Tag。
- 各环境私有配置不随基线传播。数据库密码/备份/域名/端口/租户管理员各项目独立。

基线更新后每个下游先跑 CI + 临时环境验收；**不是拉最新框架就立刻自动更新生产**。

## 10. 验收清单（交付前逐项记录结果）

- [x] Ubuntu 原生 MuPDF/CJK 三项测试、三个可执行程序加载、Web 构建与产物清单（实现基线 CI run `35998721922` 通过；后续提交继续以各自 CI 为准）。
- [x] OIDC 最小权限上传私有 OSS、重复发布相同内容成功/不同内容拒绝、无生产自动部署（OSS preflight run 36222217392）。
- [ ] Test Native Release 完整 Linux 包上传、逐对象读回和重复运行验收。
- [ ] 固定模板新建一台 ECS，云助手在线、安全组正确、初始化后无公开 setup。
- [ ] external PG 与 loopback local-container PG 分别安装并验证；失败预检无业务变更。
- [ ] 通用 Worker 与 AI Worker 各完成真实任务；OAuth、MCP、SSE、附件正常。
- [ ] 更新前停进程/备份、迁移顺序正确，API/Web/Worker 同一版本。
- [ ] 包篡改、备份失败、迁移失败、健康失败时 fail-closed；不中断其他项目、不降级证书。
- [ ] 回滚到已知兼容旧包；数据库不自动回退；断电/超时/重试不重复买 ECS/重复初始化。
- [ ] 自动证书续期部署 + 临期告警成功；公网 DNS/HTTPS 正常。
- [ ] DB restore、state/附件/数据盘恢复演练；容量与按量资源预算告警已配置。
