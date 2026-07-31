package agent

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

func RegisterTenantModels() error {
	if err := registerAgentTenantModels(); err != nil {
		return err
	}
	return registerAgentSlotModels()
}

type agentRepositories struct {
	db       tenantdata.TenantDatabase
	agents   *tenantdata.Repository[Agent]
	versions *tenantdata.Repository[AgentVersion]
	tools    *tenantdata.Repository[AgentToolBinding]
}

func newAgentRepositories(ctx context.Context, db *gorm.DB) (*agentRepositories, error) {
	tdb, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		return nil, err
	}
	agents, _ := tenantdata.NewTenantRepository[Agent](tdb)
	versions, _ := tenantdata.NewTenantRepository[AgentVersion](tdb)
	tools, _ := tenantdata.NewTenantRepository[AgentToolBinding](tdb)
	return &agentRepositories{db: tdb, agents: agents, versions: versions, tools: tools}, nil
}

func (r *agentRepositories) transaction(fn func(*agentRepositories) error) error {
	return r.db.Transaction(func(tx tenantdata.TenantDatabase) error {
		agents, _ := tenantdata.NewTenantRepository[Agent](tx)
		versions, _ := tenantdata.NewTenantRepository[AgentVersion](tx)
		tools, _ := tenantdata.NewTenantRepository[AgentToolBinding](tx)
		return fn(&agentRepositories{db: tx, agents: agents, versions: versions, tools: tools})
	})
}

func FindVisible(ctx context.Context, db *gorm.DB, agentID, userID string) (Agent, error) {
	repos, err := newAgentRepositories(ctx, db.WithContext(ctx))
	if err != nil {
		return Agent{}, err
	}
	return repos.agents.First("id=? AND (owner_user_id=? OR visibility=?)", agentID, userID, "tenant")
}

func GetVersion(ctx context.Context, db *gorm.DB, versionID string) (AgentVersion, error) {
	repos, err := newAgentRepositories(ctx, db.WithContext(ctx))
	if err != nil {
		return AgentVersion{}, err
	}
	return repos.versions.GetByID(versionID)
}

func GetVersionForAgent(ctx context.Context, db *gorm.DB, versionID, agentID string) (AgentVersion, error) {
	repos, err := newAgentRepositories(ctx, db.WithContext(ctx))
	if err != nil {
		return AgentVersion{}, err
	}
	return repos.versions.First("id=? AND agent_id=?", versionID, agentID)
}

func GetForRuntime(ctx context.Context, db *gorm.DB, agentID string, allowDraft bool) (Agent, error) {
	repos, err := newAgentRepositories(ctx, db.WithContext(ctx))
	if err != nil {
		return Agent{}, err
	}
	query := repos.agents.Where("id=?", agentID)
	if allowDraft {
		query = query.Where("status IN ?", []string{"enabled", "draft"})
	} else {
		query = query.Where("status = ?", "enabled")
	}
	return query.First("1=1")
}

func ListEnabledToolBindings(ctx context.Context, db *gorm.DB, agentID string) ([]AgentToolBinding, error) {
	repos, err := newAgentRepositories(ctx, db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return repos.tools.Find("agent_id=? AND status=?", agentID, "enabled")
}
