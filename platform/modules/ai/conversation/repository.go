package conversation

import (
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

var conversationModelRegistration struct {
	sync.Once
	err error
}

func registerConversationTenantModels() error {
	conversationModelRegistration.Do(func() {
		for _, model := range []any{Conversation{}, ConversationMessage{}, AssistantStream{}, AssistantStreamDelta{}, ContextSnapshot{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				conversationModelRegistration.err = err
				return
			}
		}
	})
	return conversationModelRegistration.err
}

func RegisterTenantModels() error {
	if err := registerConversationTenantModels(); err != nil {
		return err
	}
	return registerAgentRuntimeTenantModels()
}
