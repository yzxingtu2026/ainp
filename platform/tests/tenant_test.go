package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

func TestSystemScopeRequiresPlatformOperator(t *testing.T) {
	db := &gorm.DB{Config: &gorm.Config{}}
	ctx := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmTenant, Authenticated: true, IsSystemOperator: true})
	tenant.SystemScope(ctx)(db)
	if !errors.Is(db.Error, tenant.ErrPlatformRealmRequired) {
		t.Fatalf("expected platform realm error, got %v", db.Error)
	}
	allowed := &gorm.DB{Config: &gorm.Config{}}
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmPlatform, Authenticated: true, IsSystemOperator: true})
	tenant.SystemScope(platformCtx)(allowed)
	if allowed.Error != nil {
		t.Fatalf("expected explicit platform scope to pass, got %v", allowed.Error)
	}
}
