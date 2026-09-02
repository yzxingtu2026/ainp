package tests

import (
	"os"
	"strings"
	"testing"
)

func TestWebSocketHandshakeMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0067_websocket_handshake_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"auth_websocket_handshake_grants", "secret_hash", "principal_kind", "principal_id", "session_id",
		"client_surface", "audience", "target_id", "target_path", "auth_version", "expires_at", "consumed_at",
		"REFERENCES auth_sessions(id)", "CHECK (auth_version > 0)",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
