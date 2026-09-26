# 原生二进制部署

- 正式 Linux 应用产物是同版本 API/Worker/migrate 二进制 + 静态 Web，不新增 ACR/GHCR 构建或自建运维面板；Docker/Compose 仅保留存量兼容。
- 部署公共实现统一在 scripts/deployment，下游差异用 build/target/private 配置；不得各业务另抄发布脚本。
- 云操作默认 plan，显式 apply；仅固定数字版本启动模板、按量实例、RAM/STS/IMDSv2；创建/下发前落盘幂等账本，超时未知不得盲重试。
- 首次云发布前运行手动 OSS 预检（专用 `_checks/` 对象）；上传成功不等于覆盖保护成立，版本控制不能替代禁止覆盖。
- 发布包不可变，验证项目/版本/commit/平台/SHA256，安全解包；私有配置/证书/附件/state 不入包。
- 专用非 root systemd 进程，API loopback；精确 Nginx OAuth/MCP/SSE 路由，应用更新不能覆盖 TLS 资产。
- 首装必须使用 Host setup 初始化与 baseline，重启并确认 installed；仅 healthz 成功不足以判断安装完成。
- 更新/回滚有维护窗口：停应用、持久化 DB 备份、forward-only 迁移、切包、版本及进程校验。失败关闭流量，不自动数据库降级。
- 回滚只接受该主机已成功部署的版本及显式 schema 兼容确认；真实 Worker 任务/SSL 自动续期/灾备需独立验收，不以离线测试冒充。
- 详规见 docs/development/binary-deployment.md。
