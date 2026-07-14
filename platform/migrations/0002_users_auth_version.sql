-- Existing JWTs intentionally become invalid after this migration because they
-- do not contain auth_version.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS auth_version bigint NOT NULL DEFAULT 1;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS ck_users_auth_version;

ALTER TABLE users
    ADD CONSTRAINT ck_users_auth_version CHECK (auth_version > 0);
