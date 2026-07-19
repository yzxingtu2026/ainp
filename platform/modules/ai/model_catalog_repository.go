package ai

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var modelCatalogRegistration struct {
	sync.Once
	err error
}

func registerModelCatalogTenantModels() error {
	modelCatalogRegistration.Do(func() {
		for _, model := range []any{Provider{}, Deployment{}, Alias{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				modelCatalogRegistration.err = err
				return
			}
		}
	})
	return modelCatalogRegistration.err
}
