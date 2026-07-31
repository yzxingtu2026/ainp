package navigation

import (
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"sync"
)

var navigationModelRegistration struct {
	sync.Once
	err error
}

func registerNavigationTenantModels() error {
	navigationModelRegistration.Do(func() {
		navigationModelRegistration.err = tenantdata.DefaultRegistry.Register(NavigationSearchDocument{}, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true})
	})
	return navigationModelRegistration.err
}

func RegisterTenantModels() error { return registerNavigationTenantModels() }
