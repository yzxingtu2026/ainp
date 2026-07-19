package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RuntimePreferencesKey = "profile.runtime_preferences"

type RuntimePreferences struct {
	PreferredAddressOverride string `json:"preferred_address_override"`
	Timezone                 string `json:"timezone"`
	Locale                   string `json:"locale"`
	ResponseStyle            string `json:"response_style"`
	DateFormat               string `json:"date_format"`
	TimeFormat               string `json:"time_format"`
	AutoMemoryEnabled        bool   `json:"auto_memory_enabled"`
}

type userSettingStore struct{ db *gorm.DB }

var userSettingRegistration sync.Once

func newUserSettingStore(db *gorm.DB) *userSettingStore {
	userSettingRegistration.Do(func() {
		if err := tenant.DefaultRegistry.Register(model.UserSettingValue{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
			panic(err)
		}
	})
	return &userSettingStore{db: db}
}

func (s *userSettingStore) get(ctx context.Context, userID, key string) (*model.UserSettingValue, error) {
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	repo, err := tenant.NewTenantRepository[model.UserSettingValue](tdb)
	if err != nil {
		return nil, err
	}
	row, err := repo.First("user_id = ? AND setting_key = ?", userID, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (s *userSettingStore) put(ctx context.Context, userID, key string, expected int64, schema int, values map[string]any) (*model.UserSettingValue, error) {
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	var out *model.UserSettingValue
	err = tdb.Transaction(func(tx tenant.TenantDatabase) error {
		repo, _ := tenant.NewTenantRepository[model.UserSettingValue](tx)
		row, qerr := repo.Clauses(clause.Locking{Strength: "UPDATE"}).First("user_id = ? AND setting_key = ?", userID, key)
		if errors.Is(qerr, gorm.ErrRecordNotFound) {
			if expected != 0 {
				return setting.ErrRevisionConflict
			}
			created := &model.UserSettingValue{ID: uuid.NewString(), UserID: userID, SettingKey: key, SchemaVersion: schema, ValueJSON: payload, Revision: 1, UpdatedBy: userID}
			if err := repo.Create(created); err != nil {
				return err
			}
			out = created
			return nil
		}
		if qerr != nil {
			return qerr
		}
		if row.Revision != expected {
			return setting.ErrRevisionConflict
		}
		res := repo.Where("id = ? AND revision = ?", row.ID, expected).Updates(map[string]any{"schema_version": schema, "value_json": payload, "revision": expected + 1, "updated_by": userID, "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return setting.ErrRevisionConflict
		}
		row.SchemaVersion, row.ValueJSON, row.Revision, row.UpdatedBy = schema, payload, expected+1, userID
		out = &row
		return nil
	})
	return out, err
}

type UserSettingService struct {
	registry *setting.Registry
	store    *userSettingStore
}

func NewUserSettingService(db *gorm.DB, registry *setting.Registry) *UserSettingService {
	return &UserSettingService{registry: registry, store: newUserSettingStore(db)}
}

func (s *UserSettingService) effective(ctx context.Context) (*setting.Effective, error) {
	_, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	def, ok := s.registry.GetDefinition(RuntimePreferencesKey)
	if !ok || def.Scope != setting.ScopeUser {
		return nil, fmt.Errorf("user runtime preferences definition missing")
	}
	row, err := s.store.get(ctx, userID, def.Key)
	if err != nil {
		return nil, err
	}
	eff := &setting.Effective{Definition: def, Values: setting.DefaultValues(def), SchemaVersion: def.SchemaVersion, Source: "default"}
	if row != nil {
		if row.SchemaVersion != def.SchemaVersion {
			return nil, setting.ErrSchemaVersionMismatch
		}
		if err := json.Unmarshal(row.ValueJSON, &eff.Values); err != nil {
			return nil, err
		}
		eff.Revision, eff.Source = row.Revision, "user"
	}
	return eff, nil
}

func (s *UserSettingService) Get(ctx context.Context, _ json.RawMessage) (any, error) {
	eff, err := s.effective(ctx)
	if err != nil {
		return nil, mapSettingError(err)
	}
	return toDefinitionView(auth.MustFromContext(ctx).Language, eff), nil
}

func (s *UserSettingService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	_, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		Revision int64          `json:"revision"`
		Values   map[string]any `json:"values"`
	}
	if json.Unmarshal(input, &req) != nil || req.Values == nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid user setting update request")
	}
	def, ok := s.registry.GetDefinition(RuntimePreferencesKey)
	if !ok || def.Scope != setting.ScopeUser {
		return nil, errs.New(errs.CodeInternal, "user preferences unavailable")
	}
	if tz, _ := req.Values["timezone"].(string); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, errs.ValidationKey("setting.validation_failed", "timezone must be a valid IANA timezone")
		}
	}
	if err := setting.ValidateValues(def, req.Values); err != nil {
		return nil, errs.ValidationKey("setting.validation_failed", err.Error())
	}
	setSettingAudit(ctx, RuntimePreferencesKey, fmt.Sprintf("user setting revision %d requested", req.Revision))
	row, err := s.store.put(ctx, userID, def.Key, req.Revision, def.SchemaVersion, req.Values)
	if err != nil {
		return nil, mapSettingError(err)
	}
	eff := &setting.Effective{Definition: def, Values: req.Values, Revision: row.Revision, SchemaVersion: def.SchemaVersion, Source: "user"}
	return toDefinitionView(auth.MustFromContext(ctx).Language, eff), nil
}

func (s *UserSettingService) Preferences(ctx context.Context) (*RuntimePreferences, error) {
	eff, err := s.effective(ctx)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(eff.Values)
	var out RuntimePreferences
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func registerRuntimePreferencesSetting(a interface {
	RegisterSettingCategory(setting.Category) error
	RegisterSettingDefinition(setting.Definition) error
}) error {
	if err := a.RegisterSettingCategory(setting.Category{Key: "profile", Title: "个人偏好", Icon: "ph:user-circle", Order: 5, Surface: auth.RealmTenant}); err != nil {
		return err
	}
	maxAddress := 64
	return a.RegisterSettingDefinition(setting.Definition{Key: RuntimePreferencesKey, CategoryKey: "profile", Title: "对话偏好", Scope: setting.ScopeUser, Surface: auth.RealmTenant, Order: 10, SchemaVersion: 1, Fields: []setting.Field{
		{Key: "preferred_address_override", Type: setting.FieldText, Label: "AI 称呼我的方式", Default: "", MaxLength: &maxAddress},
		{Key: "timezone", Type: setting.FieldSelect, Label: "时区", Required: true, Default: "Asia/Shanghai", Options: []setting.FieldOption{{Value: "Asia/Shanghai", Label: "Asia/Shanghai"}, {Value: "UTC", Label: "UTC"}, {Value: "America/New_York", Label: "America/New_York"}}},
		{Key: "locale", Type: setting.FieldSelect, Label: "语言", Required: true, Default: "zh-CN", Options: []setting.FieldOption{{Value: "zh-CN", Label: "简体中文"}, {Value: "en-US", Label: "English"}}},
		{Key: "response_style", Type: setting.FieldSelect, Label: "回答风格", Required: true, Default: "standard", Options: []setting.FieldOption{{Value: "concise", Label: "简洁"}, {Value: "standard", Label: "标准"}, {Value: "detailed", Label: "详细"}}},
		{Key: "date_format", Type: setting.FieldSelect, Label: "日期格式", Required: true, Default: "YYYY-MM-DD", Options: []setting.FieldOption{{Value: "YYYY-MM-DD", Label: "YYYY-MM-DD"}, {Value: "YYYY/MM/DD", Label: "YYYY/MM/DD"}, {Value: "DD/MM/YYYY", Label: "DD/MM/YYYY"}}},
		{Key: "time_format", Type: setting.FieldSelect, Label: "时间格式", Required: true, Default: "24h", Options: []setting.FieldOption{{Value: "24h", Label: "24 小时"}, {Value: "12h", Label: "12 小时"}}},
		{Key: "auto_memory_enabled", Type: setting.FieldBoolean, Label: "自动记忆", Default: true},
	}})
}

func trimAddress(v string) string { return strings.TrimSpace(v) }
