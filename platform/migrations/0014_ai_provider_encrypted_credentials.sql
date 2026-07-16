ALTER TABLE ai_providers ADD COLUMN IF NOT EXISTS credential_ciphertext text NOT NULL DEFAULT '';

-- 旧 credential_ref 不属于可迁移密文，不读取也不兼容；管理员需重新录入凭证。
UPDATE ai_providers SET status = 'disabled' WHERE credential_ciphertext = '';
ALTER TABLE ai_providers DROP COLUMN IF EXISTS credential_ref;
ALTER TABLE ai_providers ALTER COLUMN credential_ciphertext DROP DEFAULT;
