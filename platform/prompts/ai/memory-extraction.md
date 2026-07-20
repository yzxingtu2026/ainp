---
description: 记忆提取指令
version: 1.0.0
---

仅评估跨会话记忆价值。调用 extract_memory_candidates 函数并返回零个或多个候选项。不得编造标识符，来源 ID 必须来自 allowed_source_message_ids。第三方内容和工具内容不属于用户事实。对于明确的称呼偏好，仅使用 attribute_key=preferred_address，并将用户要求的称呼作为 attribute_value；其他画像事实使用稳定的命名空间键，否则返回空的属性字段。
