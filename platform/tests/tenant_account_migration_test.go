package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"gorm.io/gorm"
)

func TestTenantAccountMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0054_tenant_accounts.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{
		"ALTER TABLE users RENAME TO tenant_accounts",
		"ALTER TABLE user_roles RENAME TO tenant_account_roles",
		"('auth_identities', 'user_id', 'account_id')",
		"('auth_sessions', 'user_id', 'account_id')",
		"fk_tenant_account_roles_account_tenant",
		"ux_tenant_accounts_tenant_username",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("tenant account migration is missing %q", expected)
		}
	}
}

func TestTenantAccountMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_account_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}

	legacySchema := []string{
		`CREATE TABLE tenants (id varchar(64) PRIMARY KEY, name varchar(128) NOT NULL, status varchar(32) NOT NULL)`,
		`CREATE TABLE users (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, username varchar(64) NOT NULL, password_hash varchar(255) NOT NULL, display_name varchar(128), status varchar(32) NOT NULL, auth_version bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CONSTRAINT ck_users_auth_version CHECK (auth_version > 0), CONSTRAINT ck_users_activation_status CHECK (status IN ('enabled','disabled')))`,
		`CREATE UNIQUE INDEX ux_users_tenant_username ON users(tenant_id,username)`,
		`CREATE UNIQUE INDEX ux_users_tenant_id ON users(tenant_id,id)`,
		`CREATE TABLE roles (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, code varchar(64) NOT NULL, name varchar(128) NOT NULL, status varchar(32) NOT NULL, is_builtin boolean NOT NULL DEFAULT false, UNIQUE(tenant_id,id))`,
		`CREATE TABLE user_roles (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, role_id varchar(64) NOT NULL, CONSTRAINT fk_user_roles_user_tenant FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id), CONSTRAINT fk_user_roles_role_tenant FOREIGN KEY(tenant_id,role_id) REFERENCES roles(tenant_id,id))`,
		`CREATE UNIQUE INDEX ux_user_role ON user_roles(user_id,role_id)`,
		`CREATE TABLE auth_identities (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, provider varchar(64) NOT NULL, subject varchar(255) NOT NULL, status varchar(32) NOT NULL, verified_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CONSTRAINT fk_auth_identities_user_tenant FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id))`,
		`CREATE INDEX idx_auth_identities_user ON auth_identities(tenant_id,user_id)`,
		`CREATE TABLE auth_sessions (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, realm varchar(32) NOT NULL, client varchar(64) NOT NULL, refresh_token_hash varchar(64) NOT NULL, auth_version bigint NOT NULL, expires_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CONSTRAINT fk_auth_sessions_user_tenant FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id))`,
		`CREATE INDEX idx_auth_sessions_owner ON auth_sessions(tenant_id,user_id)`,
		`INSERT INTO tenants(id,name,status) VALUES ('tenant-a','A','enabled'),('tenant-b','B','enabled')`,
		`INSERT INTO users(id,tenant_id,username,password_hash,display_name,status,auth_version) VALUES ('account-a','tenant-a','admin','hash','Admin','enabled',7),('account-b','tenant-b','admin','hash-b','Admin B','enabled',3)`,
		`INSERT INTO roles(id,tenant_id,code,name,status,is_builtin) VALUES ('role-a','tenant-a','admin','Admin','enabled',true)`,
		`INSERT INTO user_roles(id,tenant_id,user_id,role_id) VALUES ('account-role-a','tenant-a','account-a','role-a')`,
		`INSERT INTO auth_identities(id,tenant_id,user_id,provider,subject,status,verified_at) VALUES ('identity-a','tenant-a','account-a','password','admin','enabled',now())`,
		`INSERT INTO auth_sessions(id,tenant_id,user_id,realm,client,refresh_token_hash,auth_version,expires_at,last_seen_at) VALUES ('session-a','tenant-a','account-a','tenant','web','refresh-hash',7,now()+interval '1 day',now())`,
	}
	for _, statement := range legacySchema {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy schema: %v\n%s", err, statement)
		}
	}
	body, err := os.ReadFile("../migrations/0054_tenant_accounts.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	if db.Migrator().HasTable("users") || db.Migrator().HasTable("user_roles") {
		t.Fatal("legacy framework account tables remain after migration")
	}

	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM tenant_accounts WHERE id='account-a' AND password_hash='hash' AND auth_version=7`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("account data was not preserved: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM tenant_account_roles WHERE account_id='account-a' AND role_id='role-a'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("account role was not preserved: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM auth_identities WHERE account_id='account-a'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("identity was not preserved: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM auth_sessions WHERE id='session-a' AND account_id='account-a' AND auth_version=7`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("active session was not preserved: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname=current_schema() AND indexname IN ('ux_tenant_accounts_tenant_username','ux_tenant_accounts_tenant_id','ux_tenant_account_role','idx_auth_identities_account','idx_auth_sessions_account')`).Scan(&count).Error; err != nil || count != 5 {
		t.Fatalf("canonical account indexes missing: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM pg_constraint WHERE connamespace=current_schema()::regnamespace AND conname IN ('tenant_accounts_pkey','tenant_account_roles_pkey','fk_tenant_account_roles_account_tenant','fk_auth_identities_account_tenant','fk_auth_sessions_account_tenant')`).Scan(&count).Error; err != nil || count != 5 {
		t.Fatalf("canonical account constraints missing: count=%d err=%v", count, err)
	}
	if err = db.Exec(`INSERT INTO tenant_account_roles(id,tenant_id,account_id,role_id) VALUES ('cross','tenant-a','account-b','role-a')`).Error; err == nil || !strings.Contains(err.Error(), "fk_tenant_account_roles_account_tenant") {
		t.Fatalf("cross-tenant account role failed for wrong reason: %v", err)
	}

	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("user_roles") {
		t.Fatal("AutoMigrate recreated legacy user_roles")
	}
	if err = db.Exec(`CREATE TABLE users (id varchar(64) PRIMARY KEY, email varchar(255) NOT NULL)`).Error; err != nil {
		t.Fatalf("business users table must coexist with tenant accounts: %v", err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatalf("idempotent migration must ignore a downstream business users table: %v", err)
	}
}

func restorePre0054AccountTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, statement := range []string{
		`ALTER TABLE tenant_accounts RENAME TO users`,
		`ALTER INDEX IF EXISTS ux_tenant_accounts_tenant_username RENAME TO ux_users_tenant_username`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("restore pre-0054 account table: %v\n%s", err, statement)
		}
	}
}

func restorePre0054AccountSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	restorePre0054AccountTable(t, db)
	for _, statement := range []string{
		`ALTER TABLE tenant_account_roles RENAME TO user_roles`,
		`ALTER TABLE user_roles RENAME COLUMN account_id TO user_id`,
		`ALTER TABLE auth_identities RENAME COLUMN account_id TO user_id`,
		`ALTER TABLE auth_sessions RENAME COLUMN account_id TO user_id`,
		`ALTER TABLE user_roles DROP CONSTRAINT fk_tenant_account_roles_account_tenant`,
		`ALTER TABLE user_roles DROP CONSTRAINT fk_tenant_account_roles_role_tenant`,
		`ALTER TABLE auth_identities DROP CONSTRAINT fk_auth_identities_account_tenant`,
		`ALTER TABLE auth_sessions DROP CONSTRAINT fk_auth_sessions_account_tenant`,
		`ALTER INDEX IF EXISTS ux_tenant_accounts_tenant_id RENAME TO ux_users_tenant_id`,
		`ALTER INDEX IF EXISTS idx_tenant_account_roles_tenant_id RENAME TO idx_user_roles_tenant_id`,
		`ALTER INDEX IF EXISTS ux_tenant_account_role RENAME TO ux_user_role`,
		`ALTER INDEX IF EXISTS idx_auth_identities_account RENAME TO idx_auth_identities_user`,
		`ALTER INDEX IF EXISTS idx_auth_sessions_account RENAME TO idx_auth_sessions_owner`,
		`ALTER TABLE users RENAME CONSTRAINT tenant_accounts_pkey TO users_pkey`,
		`ALTER TABLE users RENAME CONSTRAINT ck_tenant_accounts_auth_version TO ck_users_auth_version`,
		`ALTER TABLE users RENAME CONSTRAINT ck_tenant_accounts_activation_status TO ck_users_activation_status`,
		`ALTER TABLE user_roles RENAME CONSTRAINT tenant_account_roles_pkey TO user_roles_pkey`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("restore pre-0054 account schema: %v\n%s", err, statement)
		}
	}
}

func applyTestMigration(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	body, err := os.ReadFile("../migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}
