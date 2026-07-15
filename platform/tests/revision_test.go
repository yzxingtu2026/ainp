package tests

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/revision"
	"gorm.io/gorm"
)

type revisionTestDoc struct {
	ID       string `gorm:"primaryKey;size:64"`
	TenantID string `gorm:"size:64;not null"`
	Title    string `gorm:"size:256"`
	Revision int64  `gorm:"not null;default:1"`
}

func (revisionTestDoc) TableName() string { return "revision_test_docs" }

func openRevisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_rev_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.AutoMigrate(&revisionTestDoc{}); err != nil {
		t.Fatal(err)
	}
	return testDB
}

func TestRevisionConditionalUpdate(t *testing.T) {
	db := openRevisionTestDB(t)

	doc := revisionTestDoc{ID: uuid.NewString(), TenantID: "t1", Title: "v1", Revision: 1}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}

	// Successful update with correct revision
	err := revision.ConditionalUpdate(db, "revision_test_docs", doc.ID, 1, map[string]any{
		"title": "v2",
	})
	if err != nil {
		t.Fatal("expected successful update:", err)
	}

	// Conflict: revision already incremented to 2
	err = revision.ConditionalUpdate(db, "revision_test_docs", doc.ID, 1, map[string]any{
		"title": "v3",
	})
	if err == nil {
		t.Fatal("expected revision conflict")
	}
	if err != revision.ErrRevisionConflict {
		t.Fatalf("expected ErrRevisionConflict, got: %v", err)
	}
}

func TestRevisionConcurrentUpdates(t *testing.T) {
	db := openRevisionTestDB(t)

	doc := revisionTestDoc{ID: uuid.NewString(), TenantID: "t1", Title: "initial", Revision: 1}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var conflictCount atomic.Int64

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := revision.ConditionalUpdate(db, "revision_test_docs", doc.ID, 1, map[string]any{
				"title": fmt.Sprintf("v%d", idx),
			})
			if err == nil {
				successCount.Add(1)
			} else if err == revision.ErrRevisionConflict {
				conflictCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("exactly one concurrent update should succeed, got %d", successCount.Load())
	}
	if conflictCount.Load() != 4 {
		t.Fatalf("other 4 updates should conflict, got %d", conflictCount.Load())
	}
}

func TestRevisionWithExpectedRevisionScope(t *testing.T) {
	db := openRevisionTestDB(t)

	doc := revisionTestDoc{ID: uuid.NewString(), TenantID: "t1", Title: "v1", Revision: 1}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}

	// Using scope with correct revision
	result := db.Model(&revisionTestDoc{}).
		Where("id = ?", doc.ID).
		Scopes(revision.WithExpectedRevision(1)).
		Updates(map[string]any{"title": "v2", "revision": gorm.Expr("revision + 1")})

	if err := revision.CheckUpdateResult(result); err != nil {
		t.Fatal("expected success:", err)
	}

	// Using scope with stale revision
	result = db.Model(&revisionTestDoc{}).
		Where("id = ?", doc.ID).
		Scopes(revision.WithExpectedRevision(1)).
		Updates(map[string]any{"title": "v3", "revision": gorm.Expr("revision + 1")})

	if err := revision.CheckUpdateResult(result); err != revision.ErrRevisionConflict {
		t.Fatalf("expected conflict, got: %v", err)
	}
}
