# 时间展示与本地化

- 用户可见时间必须走统一前端格式化（`formatDateTime` / `DateTimeText`），**禁止**直接展示后端原始 RFC3339 字符串。
- API 只返回带时区的机器时间；**禁止**返回 `*_display` 等展示用时间文案，或由后端按语言格式化时间。
- 新增列表/详情时间列必须复用公共 formatter / 组件，禁止业务内自行拼接。
- Phase 1 时区使用浏览器本地时区；账号级时区不在此范围。
- 变更同步 `docs/development/datetime-display.md` 并 `agent-rules sync`。

详规：`docs/development/datetime-display.md`
