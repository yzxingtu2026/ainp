# 原生二进制部署

- 正式 Linux 应用产物是同版本 API/Worker/migrate 二进制 + 静态 Web，不新增 ACR/GHCR 构建或自建运维面板；Docker/Compose 仅保留存量兼容。
- 部署公共实现统一在 scripts/deployment，下游差异用 build/target/private 配置；不得各业务另抄发布脚本。
- 云操作默认 plan，显式 apply；仅固定数字版本启动模板、按量实例、RAM/STS/IMDSv2；创建/下发前落盘幂等账本，超时未知不得盲重试。
- 首次云发布前运行手动 OSS 预检（专用 `_checks/` 对象）；上传成功不等于覆盖保护成立，版本控制不能替代禁止覆盖。
- 正式部署使用 `--version`，经已认证 gh 解析 Published 稳定 Release 引用、校验 Tag commit 并留存防漂移；默认读取项目配置的仓库；显式 `--repository` 必须一致。该入口 plan 只读 GitHub 且不写文件、不调阿里云；`--release` 保留离线引用和 CI 精确选包能力。团队仍须受控共享账本与串行执行。
- 测试发包仅从 `main` 手动运行固定 `v0.0.0-ci`，必须本地包校验与 OSS 四对象读回；`-ci` 包禁止进入 ECS 生产部署。
- 发布包不可变，验证项目/版本/commit/平台/SHA256，安全解包；私有配置/证书/附件/state 不入包。
- 多 Web 配置使用 `web_apps`，存在时可省略 `frontend_dir`；预检与打包共用 `project.build_web_apps` 校验，新增字段或默认行为必须同步 fixture、规范、测试与发布基线。
- 发布 Web 必须使用同源 API（`VITE_SERVICE_BASE_URL=/`，其他服务同源）；原生打包显式覆盖构建机环境，禁止把本地 loopback 地址编译进发布包。
- 专用非 root systemd 进程，API loopback；精确 Nginx OAuth/MCP/SSE 路由，应用更新不能覆盖 TLS 资产。
- 首装必须使用 Host setup 初始化与 baseline，重启并确认 installed；仅 healthz 成功不足以判断安装完成。
- 私有配置顶层 `redis_db` 为可选非负整数，默认 0；预检、API/两个 Worker 配置和首次 setup 必须使用同一 DB，不支持时失败而非回退。独立应用共用 Redis 实例需分配不同逻辑 DB，Cluster 仅 DB 0；更新/回滚不自动切 DB 或迁移队列。
- 更新/回滚有维护窗口：停应用、持久化 DB 备份、forward-only 迁移、切包、版本及进程校验。失败关闭流量，不自动数据库降级。
- 回滚只接受该主机已成功部署的版本及显式 schema 兼容确认；真实 Worker 任务/SSL 自动续期/灾备需独立验收，不以离线测试冒充。
- 外部 PG 显式选择专用网络 `sslmode=disable` 或 `verify-full` + CA，不做静默 TLS 降级；示例默认前者。
- 部署实时输出仅使用固定标签的 `AINP_PROGRESS` 阶段/心跳，禁止输出私有配置、argv 或原始异常；底层输出保留 root-only 主机日志，本地增量读取并去重。
- 部署工具升级必须同步发布版本的 manifest/lock，先 plan、显式 apply、再 verify 实际文件；禁止强制覆盖本地漂移、项目配置或账本。首次接入必须逐文件确认归属并显式停用旧镜像发布工作流；步骤见 docs/development/deployment-baseline.md。
- 详规见 docs/development/binary-deployment.md。
