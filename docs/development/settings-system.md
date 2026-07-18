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
- `multi_select` 默认只接受注册选项；确有业务需要时可显式启用 `allow_custom_values`，并用 `custom_value_pattern` 限制自定义值格式
- 图标：Iconify `collection:name`，默认 `mdi`
- 启动期拒绝：重复 key、未知分类、非法类型、重复字段、无效默认值、非法 surface、非法 `visible_when`

## 字段关联显隐（VisibleWhen）

声明式兄弟字段显隐，**禁止**表达式 / 脚本 / 远程代码。

```go
VisibleWhen: &setting.VisibleWhen{Field: "dark_mode", Eq: true}
// 或
VisibleWhen: &setting.VisibleWhen{Field: "mode", In: []any{"a", "b"}}
```

约束：

- 仅引用**同一 Definition** 内其他字段；禁止自引用；依赖图无环
- `eq` 与 `in` **恰好其一**
- 若依赖字段自身不可见，则当前字段也不可见（传递）
- 隐藏字段：前端保留表单值；保存校验跳过必填与类型；提交值可原样写入（不强制清默认）
- 前端按 `visible_when` 动态渲染；后端 `ValidateValues` 以服务端求值为准

示例：

- `general.preferences.brand_color`：`dark_mode == true` 时显示（`eq`）
- `general.visibility_demo`：专门联调 `eq` / `in` / 传递隐藏（见 `modules/system/setting_definitions.go`）

## 租户值

- 表：`tenant_setting_values`，`UNIQUE (tenant_id, setting_key)`，`tenant_id NOT NULL`
- 读写经 AuthContext tenant fail-closed；禁止信任 Body/Query `tenant_id`
- `revision` 乐观锁；`schema_version` 不匹配不得静默误读
- 未覆盖返回代码默认值；重置删除覆盖行

## Action / 读写入口 / 前端

### 管理端（统一读写）

- 入口：`platform/modules/system.SettingService`
- Actions：`system.setting_list` / `system.setting_get` / `system.setting_update` / `system.setting_reset`
- 职责：权限、Schema 校验（含 `visible_when`）、revision 乐观锁、审计、缓存失效
- **禁止**：Handler/业务直接操作 `tenant_setting_values`，或绕过 Service 调用 `ValueStore`

### 业务侧（统一只读）

- 底层：`platform/framework/setting.Provider.Get(ctx, tenantID, settingKey)`（默认值合并 + 可选缓存）
- 推荐：模块内类型化封装，例如 `SettingService.Preferences(ctx)`
- **禁止**：业务散落 `Get("xxx")` + 手工抠 JSON；**禁止**业务自行实现 Save（写操作一律走管理 Action）

### 前端

- 页面：`/system/settings` 按 Category 导航 + Definition 动态表单
- 禁止后端下发组件路径/脚本；特殊控件仅通过前端白名单 Renderer 扩展
- 动态表单仅在字段声明 `allow_custom_values` 时开放多选自定义项；其他多选仍保持注册选项白名单

## 检查清单

- [ ] 新设置组由代码注册并补齐全部语言 i18n key
- [ ] 管理页使用统一动态表单，未手写第二套设置表
- [ ] `visible_when` 仅声明式 `eq`/`in`，无表达式 DSL
- [ ] 写操作审计含 setting key / revision，无敏感明文
- [ ] 变更同步本文档与 `.agent/rules/settings-system.md`，并 `agent-rules sync`
