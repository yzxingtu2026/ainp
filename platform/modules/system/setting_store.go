package system

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type settingValueStore struct {
	db *gorm.DB
}

func NewSettingValueStore(db *gorm.DB) setting.ValueStore {
	return &settingValueStore{db: db}
}

func (s *settingValueStore) Get(ctx context.Context, tenantID, settingKey string) (*setting.StoredValue, error) {
	var row model.TenantSettingValue
	err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("tenant_id = ? AND setting_key = ?", tenantID, settingKey).
		First(&row).Error
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.TenantSettingValue
		qerr := tx.Scopes(tenant.ScopeByTenant(ctx)).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND setting_key = ?", tenantID, settingKey).
			First(&row).Error
		if errors.Is(qerr, gorm.ErrRecordNotFound) {
			if expectedRevision != 0 {
				return setting.ErrRevisionConflict
			}
			created := model.TenantSettingValue{
				ID: uuid.NewString(), TenantID: tenantID, SettingKey: settingKey,
				SchemaVersion: schemaVersion, ValueJSON: payload, Revision: 1, UpdatedBy: updatedBy,
			}
			if err := tx.Create(&created).Error; err != nil {
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
		if err := tx.Model(&model.TenantSettingValue{}).
			Scopes(tenant.ScopeByTenant(ctx)).
			Where("id = ? AND tenant_id = ? AND revision = ?", row.ID, tenantID, expectedRevision).
			Updates(updates).Error; err != nil {
			return err
		}
		result = &setting.StoredValue{SchemaVersion: schemaVersion, Revision: expectedRevision + 1, Values: values, UpdatedBy: updatedBy}
		return nil
	})
	return result, err
}

func (s *settingValueStore) Delete(ctx context.Context, tenantID, settingKey string, expectedRevision int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.TenantSettingValue
		err := tx.Scopes(tenant.ScopeByTenant(ctx)).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND setting_key = ?", tenantID, settingKey).
			First(&row).Error
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
		return tx.Where("id = ? AND tenant_id = ?", row.ID, tenantID).Delete(&model.TenantSettingValue{}).Error
	})
}
