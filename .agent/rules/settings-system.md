# 统一设置系统

- 分类/设置组/字段 Schema 与默认值由代码注册；数据库只存租户覆盖值与 revision。
- 租户隔离 fail-closed；禁止信任 Body/Query `tenant_id`；禁止用可空 tenant 混装平台设置。
- **管理端读写**必须走 `system.SettingService`（Action：`system.setting_list|get|update|reset`）；禁止业务/Handler 直接写 `tenant_setting_values` 或绕过校验与审计。
- **业务只读**经 `setting.Provider.Get` 或模块类型化封装（如 `SettingService.Preferences(ctx)`）；禁止散落字符串 key + 手工 JSON；业务侧不自行 Save，写操作统一走管理 Action。
- 用户可见设置用统一动态表单；禁止后端下发组件路径/脚本。
- `multi_select` 自定义值必须由字段显式开启 `allow_custom_values`，并优先配置 `custom_value_pattern`；禁止全局放开任意选项。
- 字段关联显隐只用声明式 `visible_when`（同组兄弟字段 + `eq`/`in`）；禁止表达式/脚本；隐藏字段跳过必填校验且保留值。
- 启动配置（DSN/JWT/端口）不得纳入设置系统；Setting 不承载状态/业务字典。
- `ai.memory_policy` 只由 Settings Action 管理；Agent 模块经类型化 Provider 读取并在发布时冻结，禁止直接读写设置表或散落 JSON。
- 变更同步 `docs/development/settings-system.md` 并 `agent-rules sync`。

详规：`docs/development/settings-system.md`
