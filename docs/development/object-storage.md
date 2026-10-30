# 统一对象存储（ObjectStore）

本文档约定与业务无关的二进制对象存储基础设施。Agent 要点：`.agents/rules/object-storage.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| ObjectStore（本 Issue） | Put/Open/Range/Stat/Delete/Exists、本地 Provider、key 安全 | 租户权限、附件元数据、HTTP 上传 API |
| 附件系统（#18） | 元数据、上传令牌、业务绑定、审计（见 `attachments.md`） | 直接拼磁盘路径 / 厂商 SDK |
| 租户设置（#13） | 大小/类型/保留策略等业务策略 | 创建 ObjectStore 或在线切换 Provider |

启动配置（`storage.provider`、本地根目录、未来 Endpoint/Credential）属 YAML/环境变量/Secret，**不进入**普通租户设置。

## 接口

包：`platform/framework/storage`

- `ObjectStore`：`Put` / `Open` / `OpenRange` / `Stat` / `Delete` / `Exists`
- `PutRequest`：`io.Reader` + 明确 `Size`（`>=0` 时长度必须匹配）；禁止要求整文件读入内存
- `ObjectInfo`：key、size、content_type、etag、modified_at、provider
- 稳定错误：`ErrNotFound` / `ErrInvalidKey` / `ErrSizeMismatch` / `ErrRangeInvalid` / `ErrProviderUnavailable` / `ErrIO` / `ErrUnknownProvider`
- 可选：`ReadyChecker.CheckReady`（深度就绪，勿塞进轻量 healthz）、`Presigner`（未来预签名）、`MetricsRecorder`

## Key

- key 是逻辑对象标识，不是本地绝对路径或 URL
- 生成：`BuildKey(KeyParts{TenantID, Namespace, ObjectID, At})` → `tenant/{tenant_id}/{namespace}/{yyyy}/{mm}/{object_id}`
- 原始文件名只作业务元数据，不进路径；禁止客户端提交完整 key
- `ValidateKey` 拒绝绝对路径、`..`、空段、反斜杠等

## Local Provider

- 配置：`storage.provider=local`，`storage.local.root_dir`（可用环境变量 `STORAGE_LOCAL_ROOT` / `STORAGE_PROVIDER`）
- Put：同目录临时文件 → sync/close → 原子 rename；失败清理；size mismatch 不留正式对象
- Delete 幂等；拒绝删除目录；Open/Range 流式读取并关闭句柄
- 路径解析拒绝符号链接段与根目录逃逸；文件权限保守（目录 `0750`，文件 `0640`）

## Factory 与注入

- `storage.NewFromConfig` 构造唯一实例；`app.App.Storage` 由 `cmd/ainp` 在正常模式注入
- 未知 provider、空根目录、不可写根在启动失败并给出明确错误
- 业务代码禁止 `os.Open` 管理持久对象，禁止 `if provider == oss` 分支

## 未来 Adapter 契约

S3/MinIO/OSS Adapter 须满足同一 `ObjectStore` 行为与错误映射，并复用 `platform/tests` 中的 contract suite。Provider 特有字段不得泄漏到业务模块。预签名通过可选 `Presigner`，不强制所有 Provider 实现。

## 检查清单

- [ ] 新业务只依赖 `ObjectStore` / `BuildKey`，不拼磁盘路径
- [ ] 本地契约测试与路径安全测试通过
- [ ] 配置示例不含真实密钥；变更同步本文档与 Agent 规则并 `agent-rules sync`
