# 多租户与鉴权上下文

- 租户/用户只从 Token/会话解析写入 `AuthContext`；**禁止**信任 Body/Query 的 `tenant_id`。
- **一用户一租户**；权限为 Role ↔ permission 字符串（默认 `action.id`）；按钮藏UI不够，Action 必须鉴权。
- Phase 1：Org/Dept 仅预留；禁止普通业务跨租户；DB 须带租户条件。
- 预留租户绑定自定义域名（Phase 1 可不实现）。

详规：`docs/development/auth-context.md`
