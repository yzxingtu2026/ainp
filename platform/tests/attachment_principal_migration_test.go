package tests

import (
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func TestAttachmentPrincipalContextMigration(t *testing.T) {
	migration, err := migrations.Find(62)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"uploader_kind", "client_surface", "audience"} {
		if !strings.Contains(migration.SQL, want) {
			t.Fatalf("migration 0062 missing %q", want)
		}
	}
}
