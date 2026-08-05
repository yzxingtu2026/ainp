package system

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/state"
	setupcap "github.com/yzxingtu2026/ainp/platform/modules/system/setup"
	"gorm.io/gorm"
)

type SetupService = setupcap.SetupService

const (
	SeedTenantID  = setupcap.SeedTenantID
	SeedAccountID = setupcap.SeedAccountID
	// Deprecated: use SeedAccountID.
	SeedUserID   = SeedAccountID
	SeedRoleID   = setupcap.SeedRoleID
	SeedUsername = setupcap.SeedUsername
	SeedPassword = setupcap.SeedPassword
)

func NewSetupService(cfg *config.Config, statePath string, st *state.State, onInstalled func()) *SetupService {
	return setupcap.NewSetupService(cfg, statePath, st, onInstalled)
}
func RegisterSetupActions(a *app.App, svc *SetupService) error {
	return setupcap.RegisterSetupActions(a, svc)
}
func AutoMigrate(db *gorm.DB) error                   { return setupcap.AutoMigrate(db) }
func SeedDemo(ctx context.Context, db *gorm.DB) error { return setupcap.SeedDemo(ctx, db) }
