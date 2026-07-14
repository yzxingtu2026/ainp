# 单租户与 SaaS 演进架构

## 当前运行模型

AINP 当前只交付 tenant console。安装向导自动创建一个默认租户和管理员，登录由服务端解析默认租户，用户不选择、切换或传入 `tenant_id`。单租户部署与未来 SaaS 共用租户域代码，不存在第二套无租户业务路径。

## Realm 与 Surface

稳定值只有 `tenant` 和 `platform`：

| 位置 | 约定 |
|------|------|
| JWT | 包含 `realm`，tenant Token audience 为 `ainp:tenant-console` |
| AuthContext | 验签后注入 `Realm`，普通登录固定为 `tenant` |
| Action / Menu | `Surface` 省略时兼容为 `tenant`，注册时拒绝未知值 |
| Action 执行 | realm 必须匹配 surface，之后才执行 permission 校验 |
| 菜单 | 先按 realm 过滤，再按 permission 过滤 |
| 日志 / 审计 | 显式记录 realm，不从 `IsSystemOperator` 推断 |
| 前端 | 用户信息保留 realm；当前只消费 tenant 菜单和登录态 |

本次 JWT 协议与旧 Token 不兼容。升级后旧 Token 需重新登录。未来 platform console 使用独立入口、独立 audience 和独立登录态；共享组件、请求层、主题和路由基础设施，不复制前端工程。

## 数据面与控制面

- tenant data plane：用户、角色、业务数据、租户菜单和普通 Action。所有访问必须有 tenant realm 与 `tenant_id`。
- platform control plane：未来的租户生命周期、运营和跨租户管理。只能通过明确命名的平台 Action 与 System Scope/Repository 访问。
- `Module` 只表达业务模块，`module == system` 不代表平台域。
- `IsSystemOperator` 只是平台入口的附加约束，不能让普通仓储取消租户条件。

普通租户 Scope 缺少 `tenant_id` 或 realm 不正确时直接返回错误。显式系统 Scope 必须同时满足：已认证、`platform` realm、`IsSystemOperator=true`；所有平台写操作必须审计。

## 三类边界

- 租户隔离决定“一条数据属于谁”，必须先于权限判断且 fail-closed。
- RBAC 决定“当前主体能执行什么 Action”，permission 默认等于 `action.id`。
- Entitlement 决定“租户购买或获配了什么能力”。它不能替代 RBAC，也不能放宽数据隔离。

## 数据与迁移

当前采用 pooled-first：共享数据库、共享表、`tenant_id` 作为强制隔离键。用户名只在租户内唯一，数据库约束为 `(tenant_id, username)`；启动迁移显式删除旧全局唯一索引后再创建联合索引。

未来可在仓储边界加入租户路由，将特定租户迁移到独库或 Cell；PostgreSQL RLS 可作为纵深防御，但不能替代 AuthContext 和仓储 Scope。迁移需保持租户 ID 稳定、逐租户校验行数，并为跨 Cell 操作提供平台审计。

## SaaS 实施顺序

1. 新增独立 platform audience、平台账号和登录入口。
2. 建立租户生命周期 API 与 platform console。
3. 增加 Entitlement、套餐、配额和用量，不改变租户隔离语义。
4. 按容量引入独库、Cell 路由或 RLS，并提供可回滚的数据迁移。

兼容约束：既有 tenant Action/Menu 默认 surface 不变；普通业务不得依赖平台账号；请求 Body/Query 不得成为租户信任来源；协议字段变化需给出 Token 重新登录或双读窗口。

## 当前不交付

不包含 platform console、平台账号、租户管理或切换、代登录、套餐计费、Entitlement、配额、域名证书、RLS、独库/分片/Cell、ABAC/ReBAC、微服务拆分或第二套前端源码。
