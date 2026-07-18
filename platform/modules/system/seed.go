package system

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/idempotency"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	SeedTenantID = "tenant_demo"
	SeedUserID   = "user_admin"
	SeedRoleID   = "role_admin"
	SeedUsername = "admin"
	SeedPassword = "Admin@123"
)

// AutoMigrate 系统域表（开发期 AutoMigrate；正式环境建议版本化迁移）。
func AutoMigrate(db *gorm.DB) error {
	// GORM does not remove the former global users.username unique index.
	if db.Migrator().HasIndex(&model.User{}, "idx_users_username") {
		if err := db.Migrator().DropIndex(&model.User{}, "idx_users_username"); err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(
		&tenant.Tenant{},
		&model.User{},
		&model.AuthIdentity{},
		&model.AuthSession{},
		&model.AuthChallenge{},
		&model.Role{},
		&model.RolePermission{},
		&model.UserRole{},
	); err != nil {
		return err
	}
	// Registered tenant-owned models require an explicit schema-installation exit.
	migrationDB, err := tenant.MigrationDB(db)
	if err != nil {
		return err
	}
	if err := migrationDB.AutoMigrate(&model.TenantSettingValue{}, &model.Attachment{}, &model.AttachmentLink{}); err != nil {
		return err
	}
	if err := outbox.AutoMigrate(db); err != nil {
		return err
	}
	if err := job.AutoMigrate(db); err != nil {
		return err
	}
	if err := idempotency.AutoMigrate(db); err != nil {
		return err
	}
	if err := ensureActivationStatusConstraints(db); err != nil {
		return err
	}
	if err := ensureAuthVersionConstraint(db); err != nil {
		return err
	}
	if err := ensureAttachmentStatusConstraint(migrationDB); err != nil {
		return err
	}
	if err := ensureAttachmentThumbStatusConstraint(migrationDB); err != nil {
		return err
	}
	return nil
}

func ensureAttachmentStatusConstraint(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&model.Attachment{}).Where("status NOT IN ?", []status.AttachmentStatus{
		status.AttachmentPending, status.AttachmentReady, status.AttachmentDeleting, status.AttachmentDeleted, status.AttachmentFailed,
	}).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("attachments contains %d invalid status values", invalid)
	}
	return db.Exec(`ALTER TABLE attachments DROP CONSTRAINT IF EXISTS ck_attachments_status;
ALTER TABLE attachments ADD CONSTRAINT ck_attachments_status CHECK (status IN ('pending','ready','deleting','deleted','failed'))`).Error
}

func ensureAttachmentThumbStatusConstraint(db *gorm.DB) error {
	// Normalize legacy empty string to NULL (CHECK forbids '').
	if err := db.Exec(`UPDATE attachments SET thumb_status = NULL WHERE thumb_status = ''`).Error; err != nil {
		return err
	}
	var invalid int64
	if err := db.Model(&model.Attachment{}).Where("thumb_status IS NOT NULL AND thumb_status NOT IN ?", []status.AttachmentThumbStatus{
		status.AttachmentThumbPending, status.AttachmentThumbReady, status.AttachmentThumbFailed, status.AttachmentThumbSkipped,
	}).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("attachments contains %d invalid thumb_status values", invalid)
	}
	return db.Exec(`ALTER TABLE attachments DROP CONSTRAINT IF EXISTS ck_attachments_thumb_status;
ALTER TABLE attachments ADD CONSTRAINT ck_attachments_thumb_status CHECK (thumb_status IS NULL OR thumb_status IN ('pending','ready','failed','skipped'))`).Error
}

func ensureAuthVersionConstraint(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&model.User{}).Where("auth_version < 1").Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("users contains %d invalid auth_version values", invalid)
	}
	return db.Exec("ALTER TABLE users DROP CONSTRAINT IF EXISTS ck_users_auth_version; ALTER TABLE users ADD CONSTRAINT ck_users_auth_version CHECK (auth_version > 0)").Error
}

func ensureActivationStatusConstraints(db *gorm.DB) error {
	for _, spec := range []struct{ table, name string }{
		{"tenants", "ck_tenants_activation_status"},
		{"users", "ck_users_activation_status"},
		{"roles", "ck_roles_activation_status"},
	} {
		var invalid int64
		if err := db.Table(spec.table).Where("status NOT IN ?", []status.ActivationStatus{status.ActivationEnabled, status.ActivationDisabled}).Count(&invalid).Error; err != nil {
			return err
		}
		if invalid > 0 {
			return fmt.Errorf("%s contains %d invalid activation status values", spec.table, invalid)
		}
		statement := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s; ALTER TABLE %s ADD CONSTRAINT %s CHECK (status IN ('enabled', 'disabled'))", spec.table, spec.name, spec.table, spec.name)
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// SeedDemo 幂等写入演示租户 / 管理员 / 角色权限。
// 如果数据库中已存在任何租户（如安装向导创建），则跳过种子数据。
func SeedDemo(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&tenant.Tenant{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		logging.Info(ctx, "system seed already present, skip")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(SeedPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&tenant.Tenant{
			ID:     SeedTenantID,
			Name:   "演示租户",
			Status: status.ActivationEnabled,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.User{
			ID:           SeedUserID,
			TenantID:     SeedTenantID,
			Username:     SeedUsername,
			PasswordHash: string(hash),
			DisplayName:  "系统管理员",
			Status:       status.ActivationEnabled,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.Role{
			ID:        SeedRoleID,
			TenantID:  SeedTenantID,
			Code:      "admin",
			Name:      "管理员",
			Status:    status.ActivationEnabled,
			IsBuiltin: true,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.UserRole{
			ID:       uuid.NewString(),
			TenantID: SeedTenantID,
			UserID:   SeedUserID,
			RoleID:   SeedRoleID,
		}).Error; err != nil {
			return err
		}

		perms := append([]string{
			"demo.complete",
			"demo.hello",
			"demo.echo",
			"demo.doc_list",
			"demo.doc_get",
			"demo.doc_create",
			"demo.doc_update",
		}, adminMinimumPermissions...)
		for _, p := range perms {
			if err := tx.Create(&model.RolePermission{
				ID:         uuid.NewString(),
				RoleID:     SeedRoleID,
				Permission: p,
			}).Error; err != nil {
				return err
			}
		}

		logging.Info(ctx, "system seed created",
			"tenant_id", SeedTenantID,
			"username", SeedUsername,
		)
		return nil
	})
}
