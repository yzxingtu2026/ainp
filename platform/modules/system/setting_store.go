package system

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type settingValueStore struct {
	db *gorm.DB
}

var settingValueRegistration sync.Once

func NewSettingValueStore(db *gorm.DB) setting.ValueStore {
	settingValueRegistration.Do(func() {
		if err := tenant.DefaultRegistry.Register(model.TenantSettingValue{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
			panic(err)
		}
	})
	return &settingValueStore{db: db}
}

func (s *settingValueStore) repository(ctx context.Context) (tenant.TenantDatabase, *tenant.Repository[model.TenantSettingValue], error) {
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return tdb, nil, err
	}
	repo, err := tenant.NewTenantRepository[model.TenantSettingValue](tdb)
	return tdb, repo, err
}

func validateStoreTenant(ctx context.Context, tenantID string) error {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || ac.TenantID != tenantID {
		return tenant.ErrTenantConflict
	}
	return nil
}

func (s *settingValueStore) Get(ctx context.Context, tenantID, settingKey string) (*setting.StoredValue, error) {
	if err := validateStoreTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	_, repo, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	row, err := repo.First("setting_key = ?", settingKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if len(row.ValueJSON) > 0 {
		if err := json.Unmarshal(row.ValueJSON, &values); err != nil {
			return nil, err
		}
	}
	return &setting.StoredValue{
		SchemaVersion: row.SchemaVersion,
		Revision:      row.Revision,
		Values:        values,
		UpdatedBy:     row.UpdatedBy,
	}, nil
}

func (s *settingValueStore) Put(ctx context.Context, tenantID, settingKey string, expectedRevision int64, schemaVersion int, values map[string]any, updatedBy string) (*setting.StoredValue, error) {
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var result *setting.StoredValue
	if err := validateStoreTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	tdb, _, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	err = tdb.Transaction(func(tx tenant.TenantDatabase) error {
		repo, _ := tenant.NewTenantRepository[model.TenantSettingValue](tx)
		row, qerr := repo.Clauses(clause.Locking{Strength: "UPDATE"}).First("setting_key = ?", settingKey)
		if errors.Is(qerr, gorm.ErrRecordNotFound) {
			if expectedRevision != 0 {
				return setting.ErrRevisionConflict
			}
			created := model.TenantSettingValue{
				ID: uuid.NewString(), SettingKey: settingKey,
				SchemaVersion: schemaVersion, ValueJSON: payload, Revision: 1, UpdatedBy: updatedBy,
			}
			if err := repo.Create(&created); err != nil {
				return err
			}
			result = &setting.StoredValue{SchemaVersion: created.SchemaVersion, Revision: created.Revision, Values: values, UpdatedBy: updatedBy}
			return nil
		}
		if qerr != nil {
			return qerr
		}
		if row.Revision != expectedRevision {
			return setting.ErrRevisionConflict
		}
		updates := map[string]any{
			"schema_version": schemaVersion,
			"value_json":     payload,
			"revision":       row.Revision + 1,
			"updated_by":     updatedBy,
		}
		res := repo.Where("id = ? AND revision = ?", row.ID, expectedRevision).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return setting.ErrRevisionConflict
		}
		result = &setting.StoredValue{SchemaVersion: schemaVersion, Revision: expectedRevision + 1, Values: values, UpdatedBy: updatedBy}
		return nil
	})
	return result, err
}

func (s *settingValueStore) Delete(ctx context.Context, tenantID, settingKey string, expectedRevision int64) error {
	if err := validateStoreTenant(ctx, tenantID); err != nil {
		return err
	}
	tdb, _, err := s.repository(ctx)
	if err != nil {
		return err
	}
	return tdb.Transaction(func(tx tenant.TenantDatabase) error {
		repo, _ := tenant.NewTenantRepository[model.TenantSettingValue](tx)
		row, err := repo.Clauses(clause.Locking{Strength: "UPDATE"}).First("setting_key = ?", settingKey)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expectedRevision != 0 {
				return setting.ErrRevisionConflict
			}
			return nil
		}
		if err != nil {
			return err
		}
		if row.Revision != expectedRevision {
			return setting.ErrRevisionConflict
		}
		return repo.Delete(&row)
	})
}
