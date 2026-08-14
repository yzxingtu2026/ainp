package settings

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type SettingService struct {
	db         *gorm.DB
	registry   *setting.Registry
	provider   *setting.Provider
	store      setting.ValueStore
	changes    *setting.ChangeRegistry
	validators *setting.ValidatorRegistry
}

func localizedDefinition(lang, key, fallback string) string {
	if key == "" {
		return fallback
	}
	translated := i18n.T(lang, key)
	if translated == key {
		return fallback
	}
	return translated
}

func NewSettingService(db *gorm.DB, registry *setting.Registry, ciphers ...setting.SecretCipher) *SettingService {
	store := NewSettingValueStore(db)
	return &SettingService{
		db:       db,
		registry: registry,
		store:    store,
		provider: setting.NewProvider(registry, store, ciphers...),
	}
}

func (s *SettingService) Provider() *setting.Provider                       { return s.provider }
func (s *SettingService) SetChangeRegistry(changes *setting.ChangeRegistry) { s.changes = changes }
func (s *SettingService) SetValidatorRegistry(validators *setting.ValidatorRegistry) {
	s.validators = validators
}

func (s *SettingService) notifyChange(ctx context.Context, change setting.Change) {
	if s.changes == nil {
		return
	}
	if err := s.changes.Notify(ctx, change); err != nil {
		logging.Error(ctx, "setting post-commit change handler failed", err, "setting_key", change.Key, "operation", change.Operation)
	}
}

type settingKeyRequest struct {
	Key string `json:"key" validate:"required"`
}

type settingUpdateRequest struct {
	Key      string         `json:"key" validate:"required"`
	Revision *int64         `json:"revision" validate:"required"`
	Values   map[string]any `json:"values" validate:"required"`
}

type settingResetRequest struct {
	Key      string `json:"key" validate:"required"`
	Revision *int64 `json:"revision" validate:"required"`
}

type settingSecretUpdateRequest struct {
	Key               string            `json:"key" validate:"required"`
	Revision          *int64            `json:"revision" validate:"required"`
	Values            map[string]any    `json:"values"`
	Secrets           map[string]string `json:"secrets"`
	ClearSecretFields []string          `json:"clear_secret_fields"`
}

type SettingFieldView struct {
	Key                string                      `json:"key"`
	Type               setting.FieldType           `json:"type"`
	Label              string                      `json:"label"`
	LabelKey           string                      `json:"label_key,omitempty"`
	Description        string                      `json:"description,omitempty"`
	DescriptionKey     string                      `json:"description_key,omitempty"`
	Required           bool                        `json:"required"`
	Default            any                         `json:"default,omitempty"`
	MinLength          *int                        `json:"min_length,omitempty"`
	MaxLength          *int                        `json:"max_length,omitempty"`
	Min                *float64                    `json:"min,omitempty"`
	Max                *float64                    `json:"max,omitempty"`
	Step               *float64                    `json:"step,omitempty"`
	Options            []setting.FieldOption       `json:"options,omitempty"`
	RemoteSource       *setting.RemoteOptionSource `json:"remote_source,omitempty"`
	AllowCustomValues  bool                        `json:"allow_custom_values,omitempty"`
	CustomValuePattern string                      `json:"custom_value_pattern,omitempty"`
	VisibleWhen        *setting.VisibleWhen        `json:"visible_when,omitempty"`
	SecretConfigured   *bool                       `json:"secret_configured,omitempty"`
}

type SettingDefinitionView struct {
	Key            string             `json:"key"`
	CategoryKey    string             `json:"category_key"`
	Title          string             `json:"title"`
	TitleKey       string             `json:"title_key,omitempty"`
	Description    string             `json:"description,omitempty"`
	DescriptionKey string             `json:"description_key,omitempty"`
	Permission     string             `json:"permission,omitempty"`
	Order          int                `json:"order"`
	SchemaVersion  int                `json:"schema_version"`
	Revision       int64              `json:"revision"`
	Source         string             `json:"source"`
	Values         map[string]any     `json:"values"`
	Fields         []SettingFieldView `json:"fields"`
}

type SettingCategoryView struct {
	Key            string                  `json:"key"`
	Title          string                  `json:"title"`
	TitleKey       string                  `json:"title_key,omitempty"`
	Description    string                  `json:"description,omitempty"`
	DescriptionKey string                  `json:"description_key,omitempty"`
	Icon           string                  `json:"icon,omitempty"`
	LocalIcon      string                  `json:"local_icon,omitempty"`
	Order          int                     `json:"order"`
	Definitions    []SettingDefinitionView `json:"definitions"`
}

func (s *SettingService) List(ctx context.Context, _ json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	ac := auth.MustFromContext(ctx)
	lang := ac.Language

	categories := s.registry.ListCategories(auth.RealmTenant)
	definitions := s.registry.ListDefinitionsByScope(auth.RealmTenant, setting.ScopeTenant)
	byCategory := map[string][]SettingDefinitionView{}
	for _, def := range definitions {
		if def.Permission != "" && !ac.HasPermission(def.Permission) {
			continue
		}
		view, err := s.definitionView(ctx, tenantID, lang, def)
		if err != nil {
			return nil, mapSettingError(err)
		}
		byCategory[def.CategoryKey] = append(byCategory[def.CategoryKey], view)
	}

	result := make([]SettingCategoryView, 0, len(categories))
	for _, cat := range categories {
		defs := byCategory[cat.Key]
		if len(defs) == 0 {
			continue
		}
		sort.Slice(defs, func(i, j int) bool {
			if defs[i].Order == defs[j].Order {
				return defs[i].Key < defs[j].Key
			}
			return defs[i].Order < defs[j].Order
		})
		result = append(result, SettingCategoryView{
			Key: cat.Key, Title: localizedDefinition(lang, cat.TitleKey, cat.Title), TitleKey: cat.TitleKey,
			Description: localizedDefinition(lang, cat.DescriptionKey, cat.Description), DescriptionKey: cat.DescriptionKey,
			Icon: cat.Icon, LocalIcon: cat.LocalIcon, Order: cat.Order, Definitions: defs,
		})
	}
	return map[string]any{"categories": result}, nil
}

func (s *SettingService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req settingKeyRequest
	if err := json.Unmarshal(input, &req); err != nil || req.Key == "" {
		return nil, errs.ValidationKey("common.validation_failed", "setting key required")
	}
	def, ok := s.registry.GetDefinition(req.Key)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	if def.Scope != setting.ScopeTenant {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	ac := auth.MustFromContext(ctx)
	if def.Permission != "" && !ac.HasPermission(def.Permission) {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	view, err := s.definitionView(ctx, tenantID, ac.Language, def)
	if err != nil {
		return nil, mapSettingError(err)
	}
	return view, nil
}

func (s *SettingService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req settingUpdateRequest
	if err := json.Unmarshal(input, &req); err != nil || req.Key == "" || req.Revision == nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid setting update request")
	}
	def, ok := s.registry.GetDefinition(req.Key)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	if def.Scope != setting.ScopeTenant {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	if setting.HasSecretFields(def) {
		return nil, errs.ValidationKey("setting.secret_action_required", "setting contains secret fields and requires the secret update endpoint")
	}
	ac := auth.MustFromContext(ctx)
	if def.Permission != "" && !ac.HasPermission(def.Permission) {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	if req.Values == nil {
		req.Values = map[string]any{}
	}
	if err := setting.ValidateValues(def, req.Values); err != nil {
		return nil, errs.ValidationKey("setting.validation_failed", err.Error())
	}
	if s.validators != nil {
		if err := s.validators.Validate(ctx, setting.Validation{TenantID: tenantID, ActorID: userID, Key: req.Key, Values: req.Values}); err != nil {
			return nil, errs.ValidationKey("setting.validation_failed", err.Error())
		}
	}
	setSettingAudit(ctx, req.Key, "setting updated")
	stored, err := s.store.Put(ctx, tenantID, req.Key, *req.Revision, def.SchemaVersion, req.Values, userID)
	if err != nil {
		return nil, mapSettingError(err)
	}
	s.provider.Invalidate(tenantID, req.Key)
	s.notifyChange(ctx, setting.Change{TenantID: tenantID, ActorID: userID, Key: req.Key, Operation: setting.ChangeUpdated})
	eff := &setting.Effective{
		Definition: def, Values: stored.Values, Revision: stored.Revision,
		SchemaVersion: stored.SchemaVersion, Source: "tenant",
	}
	return toDefinitionView(ac.Language, eff), nil
}

func (s *SettingService) UpdateSecret(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req settingSecretUpdateRequest
	if err := json.Unmarshal(input, &req); err != nil || req.Key == "" || req.Revision == nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid setting secret update request")
	}
	def, ok := s.registry.GetDefinition(req.Key)
	if !ok || def.Scope != setting.ScopeTenant {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	if !setting.HasSecretFields(def) {
		return nil, errs.ValidationKey("setting.validation_failed", "setting has no secret fields")
	}
	ac := auth.MustFromContext(ctx)
	if def.Permission != "" && !ac.HasPermission(def.Permission) {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	plain, unavailable, err := s.loadSecretValues(ctx, tenantID, def)
	if err != nil {
		return nil, mapSettingError(err)
	}
	secretFields := map[string]setting.Field{}
	for _, field := range def.Fields {
		if field.Type == setting.FieldSecret {
			secretFields[field.Key] = field
		}
	}
	for key, value := range req.Values {
		if _, secret := secretFields[key]; secret {
			return nil, errs.ValidationKey("setting.validation_failed", "secret fields must use the secrets object")
		}
		plain[key] = value
	}
	changed := make([]string, 0, len(req.Secrets)+len(req.ClearSecretFields))
	for key, value := range req.Secrets {
		if _, ok := secretFields[key]; !ok || strings.TrimSpace(value) == "" {
			return nil, errs.ValidationKey("setting.validation_failed", "invalid secret field update")
		}
		plain[key] = value
		changed = append(changed, key)
	}
	cleared := map[string]struct{}{}
	for _, key := range req.ClearSecretFields {
		field, ok := secretFields[key]
		if !ok || field.Required {
			return nil, errs.ValidationKey("setting.validation_failed", "secret field cannot be cleared")
		}
		if _, duplicate := cleared[key]; duplicate {
			return nil, errs.ValidationKey("setting.validation_failed", "duplicate secret field clear")
		}
		cleared[key] = struct{}{}
		delete(plain, key)
		changed = append(changed, key)
	}
	for key := range unavailable {
		_, replaced := req.Secrets[key]
		_, cleared := cleared[key]
		if !replaced && !cleared {
			return nil, errs.NewKey(errs.CodeConflict, "setting.secret_unavailable", "setting secret unavailable and must be reconfigured")
		}
	}
	if err := setting.ValidateValues(def, plain); err != nil {
		return nil, errs.ValidationKey("setting.validation_failed", err.Error())
	}
	if s.validators != nil {
		if err := s.validators.Validate(ctx, setting.Validation{TenantID: tenantID, ActorID: userID, Key: req.Key, Values: plain}); err != nil {
			return nil, errs.ValidationKey("setting.validation_failed", err.Error())
		}
	}
	storedValues := cloneSettingValues(plain)
	for key := range secretFields {
		value, exists := plain[key].(string)
		if !exists || value == "" {
			delete(storedValues, key)
			continue
		}
		if s.provider == nil || s.provider.SecretCipher() == nil {
			return nil, errs.NewKey(errs.CodeInternal, "setting.secret_unavailable", "setting secret unavailable")
		}
		ciphertext, encryptErr := s.provider.SecretCipher().Encrypt(ctx, value, setting.SecretAAD(tenantID, req.Key, key, def.SchemaVersion))
		if encryptErr != nil {
			return nil, errs.NewKey(errs.CodeInternal, "setting.secret_unavailable", "setting secret unavailable")
		}
		storedValues[key] = ciphertext
	}
	sort.Strings(changed)
	setSettingAudit(ctx, req.Key, "setting secret fields updated: "+strings.Join(changed, ","))
	if _, err := s.store.Put(ctx, tenantID, req.Key, *req.Revision, def.SchemaVersion, storedValues, userID); err != nil {
		return nil, mapSettingError(err)
	}
	s.provider.Invalidate(tenantID, req.Key)
	s.notifyChange(ctx, setting.Change{TenantID: tenantID, ActorID: userID, Key: req.Key, Operation: setting.ChangeUpdated})
	return s.definitionView(ctx, tenantID, ac.Language, def)
}

func (s *SettingService) loadSecretValues(ctx context.Context, tenantID string, def setting.Definition) (map[string]any, map[string]struct{}, error) {
	plain := setting.DefaultValues(def)
	unavailable := map[string]struct{}{}
	stored, err := s.store.Get(ctx, tenantID, def.Key)
	if err != nil || stored == nil {
		return plain, unavailable, err
	}
	if stored.SchemaVersion != def.SchemaVersion {
		return nil, nil, setting.ErrSchemaVersionMismatch
	}
	fieldTypes := make(map[string]setting.FieldType, len(def.Fields))
	for _, field := range def.Fields {
		fieldTypes[field.Key] = field.Type
	}
	for key, value := range stored.Values {
		if fieldTypes[key] != setting.FieldSecret {
			plain[key] = value
			continue
		}
		ciphertext, ok := value.(string)
		if !ok || ciphertext == "" || s.provider.SecretCipher() == nil {
			unavailable[key] = struct{}{}
			continue
		}
		plaintext, decryptErr := s.provider.SecretCipher().Decrypt(ctx, ciphertext, setting.SecretAAD(tenantID, def.Key, key, stored.SchemaVersion))
		if decryptErr != nil || plaintext == "" {
			unavailable[key] = struct{}{}
			continue
		}
		plain[key] = plaintext
	}
	return plain, unavailable, nil
}

func (s *SettingService) Reset(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req settingResetRequest
	if err := json.Unmarshal(input, &req); err != nil || req.Key == "" || req.Revision == nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid setting reset request")
	}
	def, ok := s.registry.GetDefinition(req.Key)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	if def.Scope != setting.ScopeTenant {
		return nil, errs.NewKey(errs.CodeNotFound, "setting.not_found", "setting not found")
	}
	ac := auth.MustFromContext(ctx)
	if def.Permission != "" && !ac.HasPermission(def.Permission) {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	setSettingAudit(ctx, req.Key, "setting reset")
	if err := s.store.Delete(ctx, tenantID, req.Key, *req.Revision); err != nil {
		return nil, mapSettingError(err)
	}
	s.provider.Invalidate(tenantID, req.Key)
	s.notifyChange(ctx, setting.Change{TenantID: tenantID, ActorID: userID, Key: req.Key, Operation: setting.ChangeReset})
	return s.definitionView(ctx, tenantID, ac.Language, def)
}

// Preferences is the typed view of general.preferences.
type Preferences struct {
	DisplayName string   `json:"display_name"`
	PageSize    float64  `json:"page_size"`
	DarkMode    bool     `json:"dark_mode"`
	Language    string   `json:"language"`
	Tags        []string `json:"tags"`
	BrandColor  string   `json:"brand_color"`
}

func (s *SettingService) Preferences(ctx context.Context) (*Preferences, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	eff, err := s.provider.Get(ctx, tenantID, "general.preferences")
	if err != nil {
		return nil, mapSettingError(err)
	}
	raw, err := json.Marshal(eff.Values)
	if err != nil {
		return nil, err
	}
	var prefs Preferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

func toDefinitionView(lang string, eff *setting.Effective, configured ...map[string]bool) SettingDefinitionView {
	def := eff.Definition
	fields := make([]SettingFieldView, 0, len(def.Fields))
	for _, field := range def.Fields {
		localizedOpts := make([]setting.FieldOption, 0, len(field.Options))
		for _, opt := range field.Options {
			localizedOpts = append(localizedOpts, setting.FieldOption{
				Value: opt.Value, Label: localizedDefinition(lang, opt.LabelKey, opt.Label), LabelKey: opt.LabelKey,
			})
		}
		view := SettingFieldView{
			Key: field.Key, Type: field.Type,
			Label: localizedDefinition(lang, field.LabelKey, field.Label), LabelKey: field.LabelKey,
			Description: localizedDefinition(lang, field.DescriptionKey, field.Description), DescriptionKey: field.DescriptionKey,
			Required: field.Required, Default: field.Default,
			MinLength: field.MinLength, MaxLength: field.MaxLength, Min: field.Min, Max: field.Max, Step: field.Step,
			Options: localizedOpts, RemoteSource: field.RemoteSource, AllowCustomValues: field.AllowCustomValues,
			CustomValuePattern: field.CustomValuePattern, VisibleWhen: field.VisibleWhen,
		}
		if field.Type == setting.FieldSecret {
			value := false
			if len(configured) > 0 {
				value = configured[0][field.Key]
			}
			view.Default = nil
			view.SecretConfigured = &value
		}
		fields = append(fields, view)
	}
	values := cloneSettingValues(eff.Values)
	for _, field := range def.Fields {
		if field.Type == setting.FieldSecret {
			delete(values, field.Key)
		}
	}
	return SettingDefinitionView{
		Key: def.Key, CategoryKey: def.CategoryKey,
		Title: localizedDefinition(lang, def.TitleKey, def.Title), TitleKey: def.TitleKey,
		Description: localizedDefinition(lang, def.DescriptionKey, def.Description), DescriptionKey: def.DescriptionKey,
		Permission: def.Permission, Order: def.Order, SchemaVersion: eff.SchemaVersion,
		Revision: eff.Revision, Source: eff.Source, Values: values, Fields: fields,
	}
}

func (s *SettingService) definitionView(ctx context.Context, tenantID, lang string, def setting.Definition) (SettingDefinitionView, error) {
	if !setting.HasSecretFields(def) {
		eff, err := s.provider.Get(ctx, tenantID, def.Key)
		if err != nil {
			return SettingDefinitionView{}, err
		}
		return toDefinitionView(lang, eff), nil
	}
	stored, err := s.store.Get(ctx, tenantID, def.Key)
	if err != nil {
		return SettingDefinitionView{}, err
	}
	eff := &setting.Effective{Definition: def, Values: setting.DefaultValues(def), SchemaVersion: def.SchemaVersion, Source: "default"}
	configured := map[string]bool{}
	if stored != nil {
		if stored.SchemaVersion != def.SchemaVersion {
			return SettingDefinitionView{}, setting.ErrSchemaVersionMismatch
		}
		eff.Revision, eff.SchemaVersion, eff.Source = stored.Revision, stored.SchemaVersion, "tenant"
		for key, value := range stored.Values {
			eff.Values[key] = value
		}
		for _, field := range def.Fields {
			if field.Type != setting.FieldSecret {
				continue
			}
			ciphertext, ok := stored.Values[field.Key].(string)
			if !ok || ciphertext == "" || s.provider.SecretCipher() == nil {
				continue
			}
			plaintext, decryptErr := s.provider.SecretCipher().Decrypt(ctx, ciphertext, setting.SecretAAD(tenantID, def.Key, field.Key, stored.SchemaVersion))
			configured[field.Key] = decryptErr == nil && plaintext != ""
		}
	}
	return toDefinitionView(lang, eff, configured), nil
}

func cloneSettingValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func mapSettingError(err error) error {
	if errors.Is(err, setting.ErrRevisionConflict) {
		return errs.NewKey(errs.CodeConflict, "setting.revision_conflict", "setting revision conflict")
	}
	if errors.Is(err, setting.ErrSchemaVersionMismatch) {
		return errs.NewKey(errs.CodeConflict, "setting.schema_mismatch", "setting schema version mismatch")
	}
	if errors.Is(err, setting.ErrSecretUnavailable) {
		return errs.NewKey(errs.CodeConflict, "setting.secret_unavailable", "setting secret unavailable")
	}
	if errors.Is(err, tenant.ErrTenantContextRequired) {
		return errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	return errs.Wrap(errs.CodeInternal, "setting operation failed", err)
}

func setSettingAudit(ctx context.Context, settingKey, summary string) {
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.AuditResourceType = "setting"
		ac.AuditResourceID = settingKey
		ac.AuditSummary = summary
	}
}
