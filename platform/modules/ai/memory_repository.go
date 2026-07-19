package ai

import (
	"context"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"sync"
	"time"
)

var memoryModelRegistration struct {
	sync.Once
	err error
}

func registerMemoryTenantModels() error {
	memoryModelRegistration.Do(func() {
		for _, m := range []any{MemoryItem{}, MemoryItemVersion{}, MemoryEvidence{}, MemoryProjection{}, MemoryDaySummary{}, MemoryCurrentProfile{}, MemoryEvaluation{}} {
			if err := tenantdata.DefaultRegistry.Register(m, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
				memoryModelRegistration.err = err
				return
			}
		}
	})
	return memoryModelRegistration.err
}

type MemoryRepository struct{}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{} }
func (r *MemoryRepository) Create(ctx context.Context, db tenantdata.GuardedDatabase, item *MemoryItem, version *MemoryItemVersion, evidence *MemoryEvidence) error {
	tdb, err := tenantdata.TenantDB(ctx, db.WithContext(ctx))
	if err != nil {
		return err
	}
	return tdb.Transaction(func(tx tenantdata.TenantDatabase) error {
		items, err := tenantdata.NewTenantRepository[MemoryItem](tx)
		if err != nil {
			return err
		}
		versions, err := tenantdata.NewTenantRepository[MemoryItemVersion](tx)
		if err != nil {
			return err
		}
		evidenceRepo, err := tenantdata.NewTenantRepository[MemoryEvidence](tx)
		if err != nil {
			return err
		}
		if err = items.Create(item); err != nil {
			return err
		}
		if err = versions.Create(version); err != nil {
			return err
		}
		return evidenceRepo.Create(evidence)
	})
}
func (r *MemoryRepository) Get(ctx context.Context, db tenantdata.GuardedDatabase, id string) (MemoryItem, error) {
	tdb, err := tenantdata.TenantDB(ctx, db.WithContext(ctx))
	if err != nil {
		return MemoryItem{}, err
	}
	repo, err := tenantdata.NewTenantRepository[MemoryItem](tdb)
	if err != nil {
		return MemoryItem{}, err
	}
	return repo.GetByID(id)
}
func (r *MemoryRepository) Current(ctx context.Context, db tenantdata.GuardedDatabase, userID string, now time.Time) ([]MemoryItem, error) {
	tdb, err := tenantdata.TenantDB(ctx, db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	repo, err := tenantdata.NewTenantRepository[MemoryItem](tdb)
	if err != nil {
		return nil, err
	}
	return repo.Where("user_id = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?) AND (valid_from IS NULL OR valid_from <= ?) AND (valid_to IS NULL OR valid_to > ?)", userID, now, now, now).Order("importance DESC, recorded_at DESC").Find(nil)
}
