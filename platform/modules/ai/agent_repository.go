package ai

import (
	"context"
	"sync"

	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

var agentModelRegistration struct {
	sync.Once
	err error
}

func registerAgentTenantModels() error {
	agentModelRegistration.Do(func() {
		for _, model := range []any{Agent{}, AgentVersion{}, AgentToolBinding{}} {
			if err := tenantdata.DefaultRegistry.Register(model, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				agentModelRegistration.err = err
				return
			}
		}
	})
	return agentModelRegistration.err
}

type agentRepositories struct {
	db        tenantdata.TenantDatabase
	agents    *tenantdata.Repository[Agent]
	versions  *tenantdata.Repository[AgentVersion]
	tools     *tenantdata.Repository[AgentToolBinding]
	knowledge *tenantdata.Repository[AgentKnowledgeBinding]
}

func newAgentRepositories(ctx context.Context, db *gorm.DB) (*agentRepositories, error) {
	tdb, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		return nil, err
	}
	agents, _ := tenantdata.NewTenantRepository[Agent](tdb)
	versions, _ := tenantdata.NewTenantRepository[AgentVersion](tdb)
	tools, _ := tenantdata.NewTenantRepository[AgentToolBinding](tdb)
	knowledge, _ := tenantdata.NewTenantRepository[AgentKnowledgeBinding](tdb)
	return &agentRepositories{db: tdb, agents: agents, versions: versions, tools: tools, knowledge: knowledge}, nil
}

func (r *agentRepositories) transaction(fn func(*agentRepositories) error) error {
	return r.db.Transaction(func(tx tenantdata.TenantDatabase) error {
		agents, _ := tenantdata.NewTenantRepository[Agent](tx)
		versions, _ := tenantdata.NewTenantRepository[AgentVersion](tx)
		tools, _ := tenantdata.NewTenantRepository[AgentToolBinding](tx)
		knowledge, _ := tenantdata.NewTenantRepository[AgentKnowledgeBinding](tx)
		return fn(&agentRepositories{db: tx, agents: agents, versions: versions, tools: tools, knowledge: knowledge})
	})
}
