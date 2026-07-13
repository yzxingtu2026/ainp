package system

import (
	"context"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
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
	return db.AutoMigrate(
		&tenant.Tenant{},
		&model.User{},
		&model.Role{},
		&model.RolePermission{},
		&model.UserRole{},
	)
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
			Status: tenant.StatusEnabled,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.User{
			ID:           SeedUserID,
			TenantID:     SeedTenantID,
			Username:     SeedUsername,
			PasswordHash: string(hash),
			DisplayName:  "系统管理员",
			Status:       model.StatusEnabled,
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.Role{
			ID:       SeedRoleID,
			TenantID: SeedTenantID,
			Code:     "admin",
			Name:     "管理员",
			Status:   model.StatusEnabled,
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

		perms := []string{
			"demo.hello",
			"demo.echo",
			"system.auth_me",
			"system.auth_permissions",
		}
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
