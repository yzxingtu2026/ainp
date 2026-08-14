# 统一设置系统

本文档约定代码定义、租户隔离及当前用户自助维护的运行时业务设置。Agent 要点：`.agent/rules/settings-system.md`。

## 边界

- **启动配置**（YAML / 环境变量）：DSN、JWT Secret、监听端口等，**不进入**设置系统。
- **运行时业务设置**：模块用 Go 代码注册 Category / Definition / Field；数据库只存 tenant 或 user scope 覆盖值。
- Setting **不承载**固定状态字典或租户业务字典（见 `code-sets-and-status.md`）。

## 注册协议

- 包：`platform/framework/setting`
- `RegisterCategory` / `RegisterDefinition`（经 `app.App`）
- Category / Definition key 全局唯一；Definition 必须引用已注册 Category
- tenant surface 启用 `tenant` 与 `user` scope；`platform` scope 仍保留未启用
- 字段类型：`text` / `textarea` / `number` / `boolean` / `select` / `multi_select` / `remote_select` / `color` / `secret`
- `multi_select` 默认只接受注册选项；确有业务需要时可显式启用 `allow_custom_values`，并用 `custom_value_pattern` 限制自定义值格式
- `remote_select` 必须声明同源 `/api/v1/...` 选项 endpoint、固定参数和 label/value 字段映射；endpoint 自身负责 Action 权限与租户隔离，禁止前端按 Setting key 硬编码业务请求
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
- Secret 字段可依赖普通兄弟字段；其他字段禁止依赖 Secret，因为管理端永远不接收 Secret 明文

示例：

- `general.preferences.brand_color`：`dark_mode == true` 时显示（`eq`）
- `general.visibility_demo`：专门联调 `eq` / `in` / 传递隐藏（见 `modules/system/setting_definitions.go`）

## 租户值

- 表：`tenant_setting_values`，`UNIQUE (tenant_id, setting_key)`，`tenant_id NOT NULL`
- 读写经 AuthContext tenant fail-closed；禁止信任 Body/Query `tenant_id`
- `revision` 乐观锁；`schema_version` 不匹配不得静默误读
- 未覆盖返回代码默认值；重置删除覆盖行

### Secret 字段

- `secret` 一期仅允许 tenant scope；`password` 只是管理端 Renderer，不是第二种协议类型。
- Secret 默认值只能为空，值必须是非空字符串并遵循 `min_length/max_length`；required 表示当前密文可解密或本次明确提交新值。
- Secret 由 `framework/credential` 使用 AES-256-GCM 和随机 nonce 加密，版本化密文保存在 `value_json`；AAD 固定绑定 scope、tenant、setting key、field key 与 schema version，跨租户/组/字段替换必须失败。
- 平台根密钥使用 `credential_encryption_key` 或 `CREDENTIAL_ENCRYPTION_KEY` 注入。旧 `ai.credential_encryption_key` / `AI_CREDENTIAL_ENCRYPTION_KEY` 保留一个弃用周期并输出不含值的告警；根密钥不得存入 Setting、不得与 JWT Secret 共用。
- 根密钥必须由部署 Secret Manager 备份并保持稳定；直接替换会使已有 Setting 与 AI Provider 密文不可解密。本阶段不支持在线轮换或多 Key Ring，遗失后只能重新配置凭据。
- `setting.Provider` 只向受信后端返回解密值；解密失败返回稳定 unavailable 错误。重新配置必须明确替换或清除每个不可用字段，禁止静默保留损坏密文。

## 当前用户值

- 表：`user_setting_values`，`UNIQUE (tenant_id, user_id, setting_key)`，并以复合外键关联用户。
- `user_id` 与 `tenant_id` 只能来自通过认证的 AuthContext；请求体和 Query 不接受 owner 字段。
- 读写入口固定为 self-only authenticated Action；管理员权限不赋予代读、代改私人偏好的能力。
- 使用与 tenant scope 相同的代码 Schema、强类型验证、`schema_version` 和 `revision` 乐观锁。
- 审计只记录 setting key 与 revision，不记录称呼、时区等设置值。
- `profile.runtime_preferences` 提供称呼覆盖、IANA 时区、locale、回答风格、日期/时间格式和自动记忆开关；业务通过 `UserSettingService.Preferences` 类型化读取。
- `general.runtime_defaults` 提供用户未主动设置时的租户 timezone/locale 回退；RuntimeContextProvider 按 user → tenant → system 解析并记录来源。

## Action / 读写入口 / 前端

### 管理端（统一读写）

- 入口：`platform/modules/system.SettingService`
- Actions：`system.setting_list` / `system.setting_get` / `system.setting_update` / `system.setting_secret_update` / `system.setting_reset`
- 职责：权限、Schema 校验（含 `visible_when`）、revision 乐观锁、审计、缓存失效
- **禁止**：Handler/业务直接操作 `tenant_setting_values`，或绕过 Service 调用 `ValueStore`

含 Secret 的 Definition 禁止通过会投影为 AI Tool 的 `system.setting_update` 修改，必须走复用 `system.setting_update` 权限但 `AIExcludeSensitiveCredential` 的专用 Action。请求中的普通 `values` 与 `secrets`、`clear_secret_fields` 在同一 revision/事务提交：Secret 未提供表示保留，非空值表示替换，可选字段只能显式二次确认后清除，required 字段不得清除。List/Get/Update/Reset 响应均删除 Secret value/default，只在 Field View 返回 `secret_configured`；审计与 Change Hook 仅记录 key、revision/字段名，不记录值、长度或密文。

设置保存或重置后的模块派生更新统一通过 `setting.ChangeRegistry` 订阅稳定 setting key。`SettingService` 在提交、缓存失效后同步发布只含 tenant/user/key/operation 的变更通知；订阅方只能刷新自身派生状态，不得读取设置明文日志、反向写设置或扩展请求权限。Hook 失败记录结构化错误但不把已经提交的设置伪装为回滚成功，模块必须通过状态接口暴露后续恢复或重建入口。

长期记忆租户默认值注册为 `ai.memory_policy`。Agent 模块只能通过注入的 `setting.Provider` 读取并类型化解析；管理员修改默认值不会改写已经发布的 Agent Version。

Agent Runtime 预算注册为 `ai.agent_runtime_budget`。Agent 模块通过类型化 `RuntimeBudgetProvider` 读取，严格校验整数与平台范围并在 Run 创建时固化；业务禁止散落解析设置值，客户端不得提交预算，后续修改仅影响新 Run。累计 `run_token_budget` 与 Deployment 决定的单次上下文容量是两个独立概念。

Tool Runtime 语义检索 Alias 注册为 `ai.tool_search.embedding_model_alias`。空值明确表示词法降级；修改 Alias 后调用 `ai.tool_catalog_rebuild` 生成新 revision 并重建向量，禁止直接修改 Search Document provenance。

Navigation 语义检索 Alias 注册为 `ai.navigation_search.embedding_model_alias`。保存或重置后由 Change Hook 立即 reconcile，并经 Outbox → Job 异步更新；设置页面展示 disabled/not_indexed/pending/ready/failed/degraded，允许具有 `system.setting_update` 权限的用户立即重建。

### 业务侧（统一只读）

- 底层：`platform/framework/setting.Provider.Get(ctx, tenantID, settingKey)`（默认值合并 + 可选缓存）
- tenant Provider 会拒绝 user-scope Definition；用户偏好只能通过 self-only `UserSettingService` 读取。
- 推荐：模块内类型化封装，例如 `SettingService.Preferences(ctx)`
- **禁止**：业务散落 `Get("xxx")` + 手工抠 JSON；**禁止**业务自行实现 Save（写操作一律走管理 Action）

### 前端

- 页面：`/system/settings` 按 Category 导航 + Definition 动态表单
- 禁止后端下发组件路径/脚本；特殊控件仅通过前端白名单 Renderer 扩展
- 远程下拉由 `remote_source` Schema 驱动并在首次聚焦时加载；只接受后端注册的同源 API，选项接口必须返回受当前用户权限和租户约束的数据
- 动态表单仅在字段声明 `allow_custom_values` 时开放多选自定义项；其他多选仍保持注册选项白名单
- Secret 使用密码输入控件且 `autocomplete=new-password`；不回填明文、密文或掩码，不进入普通 dirty baseline。已配置时留空表示保留；替换成功立即清空输入；可选值清除必须二次确认。禁止写入 LocalStorage、持久化 Pinia、URL 或客户端日志。
- 当前用户从头像菜单进入 `/system/profile`；基本资料、对话偏好、记忆与隐私、账号安全用 `?tab=` 深链，不进入管理员设置页面。

## 检查清单

- [ ] 新设置组由代码注册并补齐全部语言 i18n key
- [ ] 管理页使用统一动态表单，未手写第二套设置表
- [ ] `visible_when` 仅声明式 `eq`/`in`，无表达式 DSL
- [ ] 写操作审计含 setting key / revision，无敏感明文
- [ ] Secret Definition 使用专用非 AI Action，数据库/API/日志/审计/Hook/前端存储均无明文或密文泄漏
- [ ] 变更同步本文档与 `.agent/rules/settings-system.md`，并 `agent-rules sync`
