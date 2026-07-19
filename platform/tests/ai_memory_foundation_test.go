package tests

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestMemoryServiceRejectsUnsafePromotionAndInvalidTime(t *testing.T) {
	base := ai.MemoryCreateRequest{UserID: "u1", Type: aimemory.TypeFact, Status: aimemory.StatusActive, Summary: "timezone", Content: "UTC+8", Confidence: 1, Sensitivity: aimemory.SensitivityNormal, Importance: 50, DatePrecision: aimemory.DateUnknown, SourceKind: aimemory.SourceAssistant}
	if err := ai.ValidateMemoryCreateRequest(base); !errors.Is(err, aimemory.ErrUnsafeSource) {
		t.Fatalf("unsafe source error = %v", err)
	}
	base.SourceKind = aimemory.SourceUserStatement
	start := time.Now()
	end := start.Add(-time.Hour)
	base.OccurredAt, base.OccurredEndAt = &start, &end
	if err := ai.ValidateMemoryCreateRequest(base); !errors.Is(err, aimemory.ErrInvalidMemory) {
		t.Fatalf("invalid range error = %v", err)
	}
}

func TestMemoryMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_memory_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropTable(&ai.MemoryCurrentProfile{}, &ai.MemoryDaySummary{}, &ai.MemoryProjection{}, &ai.MemoryEvidence{}, &ai.MemoryItemVersion{}, &ai.MemoryItem{}); err != nil {
		t.Fatal(err)
	}
	constraints, err := os.ReadFile("../migrations/0028_tenant_composite_constraints.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(constraints)).Error; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0029_ai_memory_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestMemoryMigrationDefinesTemporalTenantProjectionContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0029_ai_memory_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"occurred_at", "observed_at", "recorded_at", "valid_from", "supersedes_item_id", "FOREIGN KEY (tenant_id, user_id, supersedes_item_id)", "ai_memory_evidence", "source_kind", "ai_memory_item_versions", "ai_memory_projections", "model_alias", "deployment_id", "dimensions", "ai_memory_day_summaries", "ai_memory_current_profiles"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}
