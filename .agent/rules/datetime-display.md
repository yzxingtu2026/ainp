# 时间展示与本地化

- 用户可见时间必须走统一前端格式化（`formatDateTime` / `DateTimeText`），**禁止**直接展示后端原始 RFC3339 字符串。
- API 只返回带时区的机器时间；**禁止**返回 `*_display` 等展示用时间文案，或由后端按语言格式化时间。
- 新增列表/详情时间列必须复用公共 formatter / 组件，禁止业务内自行拼接。
- 账号级 IANA 时区由 `profile.runtime_preferences` 保存；展示层接入该偏好前仍以浏览器本地时区为安全回退，禁止业务自行拼时区。
- 变更同步 `docs/development/datetime-display.md` 并 `agent-rules sync`。

详规：`docs/development/datetime-display.md`
