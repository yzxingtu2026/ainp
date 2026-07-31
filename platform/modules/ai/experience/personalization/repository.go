package personalization

import (
	"context"
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

var personalizationModelRegistration struct {
	sync.Once
	err error
}

func registerPersonalizationTenantModel() error {
	personalizationModelRegistration.Do(func() {
		personalizationModelRegistration.err = tenantdata.DefaultRegistry.Register(AssistantPersonalizationSnapshot{}, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true})
	})
	return personalizationModelRegistration.err
}

func RegisterTenantModels() error { return registerPersonalizationTenantModel() }

func personalizationRepository(ctx context.Context, db *gorm.DB) (*tenantdata.Repository[AssistantPersonalizationSnapshot], error) {
	tdb, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		return nil, err
	}
	return tenantdata.NewTenantRepository[AssistantPersonalizationSnapshot](tdb)
}
