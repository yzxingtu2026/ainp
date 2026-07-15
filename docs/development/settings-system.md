# 统一设置系统

本文档约定代码定义、租户隔离的运行时业务设置。Agent 要点：`.agent/rules/settings-system.md`。

## 边界

- **启动配置**（YAML / 环境变量）：DSN、JWT Secret、监听端口等，**不进入**设置系统。
- **运行时业务设置**：模块用 Go 代码注册 Category / Definition / Field；数据库只存租户覆盖值。
- Setting **不承载**固定状态字典或租户业务字典（见 `code-sets-and-status.md`）。

## 注册协议

- 包：`platform/framework/setting`
- `RegisterCategory` / `RegisterDefinition`（经 `app.App`）
- Category / Definition key 全局唯一；Definition 必须引用已注册 Category
- Phase 1 仅启用 `tenant` scope / surface
- 字段类型：`text` / `textarea` / `number` / `boolean` / `select` / `multi_select` / `color`
- 图标：Iconify `collection:name`，默认 `mdi`
- 启动期拒绝：重复 key、未知分类、非法类型、重复字段、无效默认值、非法 surface

## 租户值

- 表：`tenant_setting_values`，`UNIQUE (tenant_id, setting_key)`，`tenant_id NOT NULL`
- 读写经 AuthContext tenant fail-closed；禁止信任 Body/Query `tenant_id`
- `revision` 乐观锁；`schema_version` 不匹配不得静默误读
- 未覆盖返回代码默认值；重置删除覆盖行

## Action / 前端

- Actions：`system.setting_list|get|update|reset`
- 前端：`/system/settings` 按 Category 导航 + Definition 动态表单；禁止后端下发组件路径/脚本
- 业务读取用类型化 Provider（如 `Preferences(ctx)`），禁止散落字符串 key + 手工 JSON

## 检查清单

- [ ] 新设置组由代码注册并补齐全部语言 i18n key
- [ ] 管理页使用统一动态表单，未手写第二套设置表
- [ ] 写操作审计含 setting key / revision，无敏感明文
- [ ] 变更同步本文档与 `.agent/rules/settings-system.md`，并 `agent-rules sync`
