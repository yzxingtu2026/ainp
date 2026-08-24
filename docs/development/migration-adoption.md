# 历史内嵌分叉接管

`ainp-migrate adopt` 是一次性、版本化、fail-closed 的历史内嵌分叉接管入口。它不是编辑迁移账本的通用命令，也不放宽正式迁移的不可变规则。

计划文件必须作为受审阅代码事实提交，包含来源项目/Tag/Commit、目标正式 SemVer Tag/Commit、旧 `schema_migrations` 全量版本/名称/checksum、共同不可变前缀、每条旧记录到当前内置或下游 owner 记录的显式映射、桥接标识和 Schema 前/后置条件。计划不允许携带任意 SQL；桥接实现只能来自 AINP 或下游注册的代码。

生产流程：先完成数据库快照和维护窗口确认，在副本上运行 `plan` 或 `dry-run`，归档确定性计划摘要；由变更审批人核对摘要后，用 `apply -confirm-digest <摘要>` 执行。apply 持有 PostgreSQL advisory lock，在单一事务中执行桥接、目标内置迁移、账本重建和 owner 迁移登记；失败自动回滚。完成后再次执行标准 `Inspect`，重复同一计划幂等，不同计划拒绝。

```bash
ainp-migrate -plan adoption.json adopt plan
ainp-migrate -plan adoption.json adopt dry-run
ainp-migrate -plan adoption.json -confirm-digest <plan_digest> adopt apply
ainp-migrate -plan adoption.json adopt verify
```

任何来源/目标身份、旧 ledger、MigrationSource checksum、顺序、桥接注册或 Schema 前置条件不匹配都会停止且不修改数据库。禁止手工更新 `schema_migrations` 或 `schema_module_migrations`；没有完整旧历史、无法证明等价后置条件或缺少备份确认时，必须拒绝接管。
