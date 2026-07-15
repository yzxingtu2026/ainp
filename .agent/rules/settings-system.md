# 统一设置系统

- 分类/设置组/字段 Schema 与默认值由代码注册；数据库只存租户覆盖值与 revision。
- 租户隔离 fail-closed；禁止信任 Body/Query `tenant_id`；禁止用可空 tenant 混装平台设置。
- 用户可见设置走统一 Action 与动态表单；禁止后端下发组件路径/脚本；业务读取用类型化 Provider。
- 字段关联显隐只用声明式 `visible_when`（同组兄弟字段 + `eq`/`in`）；禁止表达式/脚本；隐藏字段跳过必填校验且保留值。
- 启动配置（DSN/JWT/端口）不得纳入设置系统；Setting 不承载状态/业务字典。
- 变更同步 `docs/development/settings-system.md` 并 `agent-rules sync`。

详规：`docs/development/settings-system.md`
