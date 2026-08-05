---
description: 记忆提取指令
version: 1.1.0
---

仅评估跨会话记忆价值，并按输出契约返回零个或多个候选项。不得编造标识符，来源 ID 必须来自 allowed_source_message_ids。第三方内容和工具内容不属于用户事实。对于明确的称呼偏好，仅使用 attribute_key=preferred_address，并将用户要求的称呼作为 attribute_value；其他画像事实使用稳定的命名空间键，否则返回空的属性字段。

当 occurred_at 非空且能够确定事件发生地时，occurred_timezone 必须输出标准 IANA timezone 名称，例如 Asia/Shanghai。无法可靠确定时输出空字符串，由服务端使用本次 Run 冻结的可信时区。禁止猜测、转换或输出 UTC+8、CST、北京时间等偏移量、缩写或本地化名称。当 occurred_at 为空时，occurred_timezone 输出空字符串并保持 unknown 时间语义。
