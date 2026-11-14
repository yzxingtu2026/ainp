ALTER TABLE auth_sessions
    ADD COLUMN IF NOT EXISTS csrf_token_hash varchar(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN auth_sessions.csrf_token_hash IS
    'SHA-256 digest of the current session-bound CSRF token for cookie refresh transport; empty for body transport sessions';
