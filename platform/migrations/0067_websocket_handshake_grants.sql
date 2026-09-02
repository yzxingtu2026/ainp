CREATE TABLE auth_websocket_handshake_grants (
  id varchar(64) PRIMARY KEY,
  secret_hash varchar(64) NOT NULL UNIQUE,
  tenant_id varchar(64) NOT NULL,
  principal_kind varchar(128) NOT NULL,
  principal_id varchar(255) NOT NULL,
  session_id varchar(64) NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
  realm varchar(32) NOT NULL,
  client_surface varchar(128) NOT NULL,
  audience varchar(255) NOT NULL,
  target_id varchar(128) NOT NULL,
  target_path varchar(512) NOT NULL,
  auth_version bigint NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ck_ws_handshake_grants_realm CHECK (realm = 'tenant'),
  CONSTRAINT ck_ws_handshake_grants_auth_version CHECK (auth_version > 0)
);

CREATE INDEX idx_ws_handshake_grants_principal
  ON auth_websocket_handshake_grants (tenant_id, principal_kind, principal_id);
CREATE INDEX idx_ws_handshake_grants_session ON auth_websocket_handshake_grants (session_id);
CREATE INDEX idx_ws_handshake_grants_expires ON auth_websocket_handshake_grants (expires_at);
CREATE INDEX idx_ws_handshake_grants_consumed ON auth_websocket_handshake_grants (consumed_at);
