package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestUserTenantUsernameMigration(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_test_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}

	// Reproduce the pre-migration global unique index.
	if err := testDB.Exec(`CREATE TABLE users (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, username varchar(64) NOT NULL, password_hash varchar(255) NOT NULL, status varchar(32) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`CREATE UNIQUE INDEX idx_users_username ON users (username)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	if testDB.Migrator().HasIndex(&model.User{}, "idx_users_username") {
		t.Fatal("legacy global username index still exists")
	}

	users := []model.User{
		{ID: "u1", TenantID: "t1", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "u2", TenantID: "t2", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled},
	}
	for i := range users {
		if err := testDB.Create(&users[i]).Error; err != nil {
			t.Fatalf("cross-tenant duplicate username must be allowed: %v", err)
		}
	}
	duplicate := model.User{ID: "u3", TenantID: "t1", Username: "same", PasswordHash: "x", Status: status.ActivationEnabled}
	if err := testDB.Create(&duplicate).Error; err == nil {
		t.Fatal("same-tenant duplicate username must be rejected")
	}
	invalid := model.User{ID: "u4", TenantID: "t3", Username: "invalid", PasswordHash: "x", Status: status.ActivationStatus("pending")}
	if err := testDB.Create(&invalid).Error; err == nil {
		t.Fatal("database CHECK must reject invalid activation status")
	}
}
