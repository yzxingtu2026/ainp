package tooling

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var toolModelRegistration struct {
	sync.Once
	err error
}

func registerToolTenantModels() error {
	toolModelRegistration.Do(func() {
		for _, model := range []any{ToolCatalog{}, ToolSearchDocument{}, ToolScope{}, ConversationToolWorkingSet{}, ToolConfirmation{}, ToolInvocation{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				toolModelRegistration.err = err
				return
			}
		}
	})
	return toolModelRegistration.err
}

func RegisterTenantModels() error { return registerToolTenantModels() }
