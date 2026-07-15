package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/dblock"
	"gorm.io/gorm"
)

type dblockTestRow struct {
	ID       string `gorm:"primaryKey;size:64"`
	TenantID string `gorm:"size:64;not null"`
	Title    string `gorm:"size:256"`
}

func (dblockTestRow) TableName() string { return "dblock_test_rows" }

func openDblockTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_dblk_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&dblockTestRow{}); err != nil {
		t.Fatal(err)
	}
	return testDB
}

func TestDblockLockForTenant(t *testing.T) {
	db := openDblockTestDB(t)

	row := dblockTestRow{ID: uuid.NewString(), TenantID: "t1", Title: "locked"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	err := dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		return dblock.LockForTenant(tx, "dblock_test_rows", "t1", row.ID)
	})
	if err != nil {
		t.Fatal("LockForTenant should succeed:", err)
	}

	// Wrong tenant should fail
	err = dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		return dblock.LockForTenant(tx, "dblock_test_rows", "wrong-tenant", row.ID)
	})
	if err == nil {
		t.Fatal("LockForTenant with wrong tenant should fail")
	}
}

func TestDblockLockForTenantEmptyTenant(t *testing.T) {
	db := openDblockTestDB(t)

	err := dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		return dblock.LockForTenant(tx, "dblock_test_rows", "", "any-id")
	})
	if err == nil {
		t.Fatal("LockForTenant with empty tenant should fail")
	}
}

func TestDblockLockRowsForUpdate(t *testing.T) {
	db := openDblockTestDB(t)

	for i := 0; i < 3; i++ {
		row := dblockTestRow{ID: uuid.NewString(), TenantID: "t1", Title: fmt.Sprintf("row-%d", i)}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	var ids []string
	db.Model(&dblockTestRow{}).Where("tenant_id = ?", "t1").Pluck("id", &ids)

	err := dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		return dblock.LockRowsForUpdate(tx, "dblock_test_rows", "t1", ids)
	})
	if err != nil {
		t.Fatal("LockRowsForUpdate should succeed:", err)
	}

	// Empty ids should be no-op
	err = dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		return dblock.LockRowsForUpdate(tx, "dblock_test_rows", "t1", nil)
	})
	if err != nil {
		t.Fatal("LockRowsForUpdate with nil ids should succeed:", err)
	}
}

func TestDblockWithTimeout(t *testing.T) {
	err := dblock.WithTimeout(context.Background(), 50*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err == nil {
		t.Fatal("expected context deadline exceeded")
	}
}

func TestDblockSortLockKeys(t *testing.T) {
	keys := []string{"c", "a", "b"}
	sorted := dblock.SortLockKeys(keys)
	if sorted[0] != "a" || sorted[1] != "b" || sorted[2] != "c" {
		t.Fatalf("expected [a b c], got %v", sorted)
	}
	// Original should be unchanged
	if keys[0] != "c" {
		t.Fatal("SortLockKeys should not modify original slice")
	}
}

func TestDblockWithinTxRollback(t *testing.T) {
	db := openDblockTestDB(t)

	row := dblockTestRow{ID: uuid.NewString(), TenantID: "t1", Title: "original"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	// Transaction that returns an error should rollback
	rollbackErr := fmt.Errorf("intentional rollback")
	err := dblock.WithinTx(context.Background(), db, func(tx *gorm.DB) error {
		tx.Model(&dblockTestRow{}).Where("id = ?", row.ID).Update("title", "modified")
		return rollbackErr
	})
	if err != rollbackErr {
		t.Fatalf("expected rollback error, got: %v", err)
	}

	// Title should remain unchanged
	var found dblockTestRow
	db.First(&found, "id = ?", row.ID)
	if found.Title != "original" {
		t.Fatalf("expected 'original' after rollback, got %q", found.Title)
	}
}
