-- 移除 Agent Version 用户指令列。
-- 个性化与用户偏好改由后续 Workspace 对话 / 记忆系统承担，不再写入不可变版本快照。
ALTER TABLE ai_agent_versions DROP COLUMN IF EXISTS user_instructions;
