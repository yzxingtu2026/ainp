ALTER TABLE auth_websocket_handshake_grants
  DROP CONSTRAINT IF EXISTS auth_websocket_handshake_grants_secret_hash_key;

ALTER TABLE auth_websocket_handshake_grants
  DROP CONSTRAINT IF EXISTS uni_auth_websocket_handshake_grants_secret_hash;

CREATE UNIQUE INDEX IF NOT EXISTS uni_auth_websocket_handshake_grants_secret_hash
  ON auth_websocket_handshake_grants (secret_hash);
