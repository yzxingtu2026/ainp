package modelruntime

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var modelInvocationRegistration struct {
	sync.Once
	err error
}

func registerModelInvocationTenantModel() error {
	modelInvocationRegistration.Do(func() {
		modelInvocationRegistration.err = tenantdata.DefaultRegistry.Register(ModelInvocation{}, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true})
	})
	return modelInvocationRegistration.err
}
