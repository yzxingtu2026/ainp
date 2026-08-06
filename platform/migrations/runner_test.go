package migrations

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadFSMigrationsAreOrderedAndChecksummed(t *testing.T) {
	items, err := loadFS(fstest.MapFS{
		"0003_three.sql": {Data: []byte("SELECT 3;")},
		"0002_two.sql":   {Data: []byte("SELECT 2;")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Version != 2 || items[1].Version != 3 || len(items[0].Checksum) != 64 {
		t.Fatalf("unexpected migrations: %#v", items)
	}
}

func TestLoadFSMigrationsRejectsGap(t *testing.T) {
	_, err := loadFS(fstest.MapFS{
		"0002_two.sql":  {Data: []byte("SELECT 2;")},
		"0004_four.sql": {Data: []byte("SELECT 4;")},
	})
	if err == nil || !strings.Contains(err.Error(), "sequence gap") {
		t.Fatalf("expected sequence gap, got %v", err)
	}
}

func TestValidateAppliedRejectsMutationAndOutOfOrder(t *testing.T) {
	known := []Migration{{Version: 2, Name: "0002_two.sql", Checksum: "a"}, {Version: 3, Name: "0003_three.sql", Checksum: "b"}}
	if err := validateApplied(known, []Applied{{Version: 2, Name: "0002_two.sql", Checksum: "changed"}}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	if err := validateApplied(known, []Applied{{Version: 3, Name: "0003_three.sql", Checksum: "b"}}); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("expected ordering failure, got %v", err)
	}
}

func TestValidateNamespacedAppliedRejectsNonPrefixHistory(t *testing.T) {
	known := []NamespacedMigration{
		{Owner: "orders", Version: 1, Name: "0001_init.sql", Checksum: "a"},
		{Owner: "orders", Version: 2, Name: "0002_next.sql", Checksum: "b"},
	}
	_, err := validateNamespacedApplied(known, []NamespacedApplied{{Owner: "orders", Version: 2, Name: "0002_next.sql", Checksum: "b"}})
	if err == nil || !strings.Contains(err.Error(), "out of order") || !strings.Contains(err.Error(), "orders:0002") {
		t.Fatalf("expected owner-qualified ordering failure, got %v", err)
	}
}

func TestBaselineThroughRejectsUnknownVersion(t *testing.T) {
	if _, err := BaselineThrough(nil, 1); err == nil || !strings.Contains(err.Error(), "is not known") {
		t.Fatalf("expected unknown baseline version failure, got %v", err)
	}
}
