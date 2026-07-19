package ai

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var agentRuntimeModelRegistration struct {
	sync.Once
	err error
}

func registerAgentRuntimeTenantModels() error {
	agentRuntimeModelRegistration.Do(func() {
		for _, model := range []any{AgentRun{}, AgentStep{}, AgentRunEvent{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				agentRuntimeModelRegistration.err = err
				return
			}
		}
	})
	return agentRuntimeModelRegistration.err
}
