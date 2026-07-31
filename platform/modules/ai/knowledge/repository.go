package knowledge

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var knowledgeModelRegistration struct {
	sync.Once
	err error
}

func registerKnowledgeTenantModels() error {
	knowledgeModelRegistration.Do(func() {
		for _, model := range []any{KnowledgeBase{}, KnowledgeDocument{}, KnowledgeIndexVersion{}, KnowledgeChunk{}, KnowledgeChunkPayload{}, AgentKnowledgeBinding{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				knowledgeModelRegistration.err = err
				return
			}
		}
	})
	return knowledgeModelRegistration.err
}
