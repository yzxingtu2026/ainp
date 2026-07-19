# 时间展示与本地化

本文档约定 AINP 用户可见时间的展示边界。Agent 要点：`.agent/rules/datetime-display.md`。

## 原则

- **API 只返回机器时间**：`created_at` / `updated_at` 等字段使用带时区的 RFC3339（ISO 8601），例如 `2026-07-15T11:30:57.077998+08:00`。
- **用户可见格式化在前端完成**：跟随当前 locale（`zh-CN` / `en-US`）；账号级 IANA 时区由 Runtime Preferences 保存，前端接入前以浏览器时区安全回退。
- **禁止**在 API 中返回 `*_display`、多语言时间文案，或按 `Accept-Language` 格式化时间字符串。

## 前端用法

- 工具：`frontend/src/utils/datetime.ts` → `formatDateTime(value, format?)`
- 组件：`DateTimeText`（`frontend/src/components/common/datetime-text.vue`），适合模板绑定
- `format`：
  - `datetime`（默认）：`YYYY-MM-DD HH:mm`，列表/详情默认
  - `date`：`YYYY-MM-DD`
  - `time`：`HH:mm:ss`
  - `relative`：相对时间（跟随 dayjs locale）
- 空值或非法值统一展示 `-`，不得抛未捕获异常
- 表格列使用 `render: row => formatDateTime(row.updated_at)`，或模板中 `<DateTimeText :value="row.updated_at" />`
- **禁止**在模板/列中直接渲染后端原始 ISO 字符串

## 后端约定

- 默认保持现状；字段语义为机器时间，不做展示层本地化。
- 若发现无偏移、格式混用等契约不一致，仅做最小规范化为带时区 RFC3339，并补测试。
- Agent/记忆的相对日期不是展示逻辑：必须使用 `runtimecontext.TimeSnapshot` 与确定性范围解析器，禁止模型猜测或业务临时 `time.Now()`。

## 检查清单

- [ ] 新增用户可见时间列复用 `formatDateTime` / `DateTimeText`
- [ ] 未直接展示 RFC3339 原串
- [ ] API 未新增展示用时间字段
- [ ] 变更已同步本文档与 `.agent/rules/datetime-display.md`，并 `agent-rules sync`
