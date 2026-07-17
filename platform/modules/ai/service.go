package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"gorm.io/gorm"
)

const enabled = "enabled"

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
var aliasCodePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func credentialAAD(tenantID, providerID string) []byte {
	return []byte("ai-provider:" + tenantID + ":" + providerID)
}
func configureProviderView(provider *Provider) {
	provider.CredentialConfigured = provider.CredentialCiphertext != ""
}

type Service struct {
	db          *gorm.DB
	types       *aimodel.ProviderTypeRegistry
	credentials aimodel.CredentialCipher
	adapters    map[string]aimodel.ModelDiscoveryProvider
}

func NewService(db *gorm.DB, types *aimodel.ProviderTypeRegistry, credentials aimodel.CredentialCipher) *Service {
	return &Service{db: db, types: types, credentials: credentials, adapters: map[string]aimodel.ModelDiscoveryProvider{aimodel.ProtocolOpenAI: aimodel.NewHTTPDiscoveryAdapter(aimodel.ProtocolOpenAI), aimodel.ProtocolAnthropic: aimodel.NewHTTPDiscoveryAdapter(aimodel.ProtocolAnthropic), aimodel.ProtocolOpenAICompatible: aimodel.NewHTTPDiscoveryAdapter(aimodel.ProtocolOpenAICompatible)}}
}
func tenant(ctx context.Context) (string, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return "", errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac.TenantID, nil
}
func statusOK(v string) bool        { return v == "enabled" || v == "disabled" }
func marshal(v any) json.RawMessage { b, _ := json.Marshal(v); return json.RawMessage(b) }
func dbError(err error, what string) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "duplicate") || strings.Contains(s, "unique") {
		return errs.New(errs.CodeConflict, what+" already exists")
	}
	return errs.Wrap(errs.CodeInternal, what+" failed", err)
}

type ListRequest struct {
	pagination.Request
	Keyword string `json:"keyword"`
	Status  string `json:"status"`
}
type IDRequest struct {
	ID string `json:"id" validate:"required"`
}
type ProviderCreateRequest struct {
	Code       string         `json:"code" validate:"required"`
	Name       string         `json:"name" validate:"required"`
	Protocol   string         `json:"protocol" validate:"required"`
	BaseURL    string         `json:"base_url"`
	Credential string         `json:"credential" validate:"required"`
	Status     string         `json:"status"`
	Config     map[string]any `json:"config"`
}
type ProviderUpdateRequest struct {
	ID         string         `json:"id" validate:"required"`
	Name       string         `json:"name" validate:"required"`
	BaseURL    string         `json:"base_url"`
	Credential string         `json:"credential"`
	Config     map[string]any `json:"config"`
}
type StatusRequest struct {
	ID     string `json:"id" validate:"required"`
	Status string `json:"status" validate:"required"`
}
type DiscoverRequest struct {
	ProviderID string `json:"provider_id" validate:"required"`
	Cursor     string `json:"cursor"`
	Limit      int    `json:"limit"`
}
type DeploymentCreateRequest struct {
	ProviderID    string               `json:"provider_id" validate:"required"`
	Code          string               `json:"code" validate:"required"`
	RemoteModelID string               `json:"remote_model_id" validate:"required"`
	DisplayName   string               `json:"display_name" validate:"required"`
	ModelType     aimodel.ModelType    `json:"model_type" validate:"required"`
	Capabilities  aimodel.Capabilities `json:"capabilities"`
	Limits        aimodel.Limits       `json:"limits"`
	Status        string               `json:"status"`
	Config        map[string]any       `json:"config"`
}
type DeploymentUpdateRequest struct {
	ID           string               `json:"id" validate:"required"`
	DisplayName  string               `json:"display_name" validate:"required"`
	ModelType    aimodel.ModelType    `json:"model_type" validate:"required"`
	Capabilities aimodel.Capabilities `json:"capabilities"`
	Limits       aimodel.Limits       `json:"limits"`
	Config       map[string]any       `json:"config"`
}
type ImportRequest struct {
	Items []DeploymentCreateRequest `json:"items" validate:"required,min=1,max=100,dive"`
}
type AliasUpsertRequest struct {
	Code         string `json:"code" validate:"required"`
	DeploymentID string `json:"deployment_id" validate:"required"`
}

func (r *AliasUpsertRequest) Normalize() {
	r.Code = strings.TrimSpace(r.Code)
	r.DeploymentID = strings.TrimSpace(r.DeploymentID)
}

type AliasDeleteRequest struct {
	Code string `json:"code" validate:"required"`
}

func (s *Service) ProviderTypes(context.Context, struct{}) (any, error) { return s.types.List(), nil }
func (s *Service) ListProviders(ctx context.Context, r ListRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ?", tid)
	if r.Keyword != "" {
		q = q.Where("code ILIKE ? OR name ILIKE ?", "%"+r.Keyword+"%", "%"+r.Keyword+"%")
	}
	if r.Status != "" {
		q = q.Where("status = ?", r.Status)
	}
	var n int64
	q.Model(&Provider{}).Count(&n)
	var rows []Provider
	e = q.Order("created_at desc").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error
	for i := range rows {
		configureProviderView(&rows[i])
	}
	return pagination.NewResult(rows, n, r.Request), dbError(e, "list providers")
}
func (s *Service) GetProvider(ctx context.Context, r IDRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	var v Provider
	e = s.db.WithContext(ctx).Where("tenant_id=? AND id=?", tid, r.ID).First(&v).Error
	if e == gorm.ErrRecordNotFound {
		return nil, errs.New(errs.CodeNotFound, "provider not found")
	}
	configureProviderView(&v)
	return v, dbError(e, "get provider")
}
func (s *Service) CreateProvider(ctx context.Context, r ProviderCreateRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	if !codePattern.MatchString(r.Code) {
		return nil, errs.Validation("invalid code")
	}
	if _, ok := s.types.Get(r.Protocol); !ok {
		return nil, errs.Validation("unknown protocol")
	}
	if r.Status == "" {
		r.Status = enabled
	}
	if !statusOK(r.Status) {
		return nil, errs.Validation("invalid status")
	}
	if r.BaseURL != "" {
		if _, e = aimodel.SafeEndpoint(r.BaseURL); e != nil {
			return nil, errs.Validation(e.Error())
		}
	}
	providerID := uuid.NewString()
	ciphertext, e := s.credentials.Encrypt(ctx, r.Credential, credentialAAD(tid, providerID))
	if e != nil {
		return nil, errs.Wrap(errs.CodeInternal, "encrypt provider credential failed", e)
	}
	v := Provider{ID: providerID, TenantID: tid, Code: r.Code, Name: r.Name, Protocol: r.Protocol, BaseURL: r.BaseURL, CredentialCiphertext: ciphertext, Status: r.Status, ConfigJSON: marshal(r.Config)}
	e = s.db.WithContext(ctx).Create(&v).Error
	configureProviderView(&v)
	return v, dbError(e, "provider")
}
func (s *Service) UpdateProvider(ctx context.Context, r ProviderUpdateRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	if r.BaseURL != "" {
		if _, e = aimodel.SafeEndpoint(r.BaseURL); e != nil {
			return nil, errs.Validation(e.Error())
		}
	}
	updates := map[string]any{"name": r.Name, "base_url": r.BaseURL, "config_json": marshal(r.Config)}
	if r.Credential != "" {
		ciphertext, err := s.credentials.Encrypt(ctx, r.Credential, credentialAAD(tid, r.ID))
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "encrypt provider credential failed", err)
		}
		updates["credential_ciphertext"] = ciphertext
	}
	res := s.db.WithContext(ctx).Model(&Provider{}).Where("tenant_id=? AND id=?", tid, r.ID).Updates(updates)
	if res.Error != nil {
		return nil, dbError(res.Error, "update provider")
	}
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "provider not found")
	}
	return s.GetProvider(ctx, IDRequest{r.ID})
}
func (s *Service) UpdateProviderStatus(ctx context.Context, r StatusRequest) (any, error) {
	if !statusOK(r.Status) {
		return nil, errs.Validation("invalid status")
	}
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	res := s.db.WithContext(ctx).Model(&Provider{}).Where("tenant_id=? AND id=?", tid, r.ID).Update("status", r.Status)
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "provider not found")
	}
	return s.GetProvider(ctx, IDRequest{r.ID})
}
func (s *Service) Discover(ctx context.Context, r DiscoverRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	var p Provider
	e = s.db.WithContext(ctx).Where("tenant_id=? AND id=? AND status=?", tid, r.ProviderID, enabled).First(&p).Error
	if e != nil {
		return nil, errs.New(errs.CodeNotFound, "enabled provider not found")
	}
	pt, ok := s.types.Get(p.Protocol)
	if !ok || !pt.ModelDiscovery {
		return nil, errs.New(errs.CodeValidation, "model discovery is not supported")
	}
	key, e := s.credentials.Decrypt(ctx, p.CredentialCiphertext, credentialAAD(tid, p.ID))
	if e != nil {
		return nil, errs.New(errs.CodeValidation, "provider credential cannot be decrypted")
	}
	adapter := s.adapters[p.Protocol]
	if adapter == nil {
		return nil, errs.New(errs.CodeValidation, "model discovery adapter is unavailable")
	}
	page, err := adapter.Discover(ctx, aimodel.DiscoveryRequest{BaseURL: p.BaseURL, APIKey: key, Cursor: r.Cursor, Limit: r.Limit})
	if err != nil {
		return nil, err
	}
	var imported []string
	if err := s.db.WithContext(ctx).Model(&Deployment{}).Where("tenant_id=? AND provider_id=?", tid, p.ID).Pluck("remote_model_id", &imported).Error; err != nil {
		return nil, dbError(err, "load imported models")
	}
	seen := make(map[string]bool, len(imported))
	for _, id := range imported {
		seen[id] = true
	}
	for i := range page.Items {
		page.Items[i].Imported = seen[page.Items[i].ID]
	}
	return page, nil
}

func validateDeployment(r DeploymentCreateRequest) error {
	if !codePattern.MatchString(r.Code) || !r.ModelType.Valid() {
		return errs.Validation("invalid deployment code or model type")
	}
	if err := r.Capabilities.Validate(); err != nil {
		return errs.Validation(err.Error())
	}
	if err := r.Limits.Validate(); err != nil {
		return errs.Validation(err.Error())
	}
	if r.Status == "" {
		return nil
	}
	if !statusOK(r.Status) {
		return errs.Validation("invalid status")
	}
	return nil
}
func (s *Service) createDeployment(ctx context.Context, tid string, r DeploymentCreateRequest) (Deployment, error) {
	if e := validateDeployment(r); e != nil {
		return Deployment{}, e
	}
	var count int64
	if e := s.db.WithContext(ctx).Model(&Provider{}).Where("tenant_id=? AND id=?", tid, r.ProviderID).Count(&count).Error; e != nil || count == 0 {
		return Deployment{}, errs.New(errs.CodeNotFound, "provider not found")
	}
	if r.Status == "" {
		r.Status = enabled
	}
	v := Deployment{ID: uuid.NewString(), TenantID: tid, ProviderID: r.ProviderID, Code: r.Code, RemoteModelID: r.RemoteModelID, DisplayName: r.DisplayName, ModelType: string(r.ModelType), CapabilitiesJSON: marshal(r.Capabilities), LimitsJSON: marshal(r.Limits), Status: r.Status, ConfigJSON: marshal(r.Config)}
	e := s.db.WithContext(ctx).Create(&v).Error
	return v, dbError(e, "deployment")
}
func (s *Service) CreateDeployment(ctx context.Context, r DeploymentCreateRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	return s.createDeployment(ctx, tid, r)
}
func (s *Service) ImportDeployments(ctx context.Context, r ImportRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]Deployment, 0, len(r.Items))
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		copy := *s
		copy.db = tx
		for _, item := range r.Items {
			v, e := copy.createDeployment(ctx, tid, item)
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		return nil
	})
	return out, e
}
func (s *Service) ListDeployments(ctx context.Context, r ListRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	q := s.db.WithContext(ctx).Where("tenant_id=?", tid)
	if r.Keyword != "" {
		q = q.Where("code ILIKE ? OR display_name ILIKE ? OR remote_model_id ILIKE ?", "%"+r.Keyword+"%", "%"+r.Keyword+"%", "%"+r.Keyword+"%")
	}
	if r.Status != "" {
		q = q.Where("status=?", r.Status)
	}
	var n int64
	q.Model(&Deployment{}).Count(&n)
	var rows []Deployment
	e = q.Order("created_at desc").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error
	return pagination.NewResult(rows, n, r.Request), dbError(e, "list deployments")
}
func (s *Service) GetDeployment(ctx context.Context, r IDRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	var v Deployment
	e = s.db.WithContext(ctx).Where("tenant_id=? AND id=?", tid, r.ID).First(&v).Error
	if e == gorm.ErrRecordNotFound {
		return nil, errs.New(errs.CodeNotFound, "deployment not found")
	}
	return v, dbError(e, "get deployment")
}
func (s *Service) UpdateDeployment(ctx context.Context, r DeploymentUpdateRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	tmp := DeploymentCreateRequest{Code: "valid", ModelType: r.ModelType, Capabilities: r.Capabilities, Limits: r.Limits}
	if e = validateDeployment(tmp); e != nil {
		return nil, e
	}
	res := s.db.WithContext(ctx).Model(&Deployment{}).Where("tenant_id=? AND id=?", tid, r.ID).Updates(map[string]any{"display_name": r.DisplayName, "model_type": r.ModelType, "capabilities_json": marshal(r.Capabilities), "limits_json": marshal(r.Limits), "config_json": marshal(r.Config)})
	if res.Error != nil {
		return nil, dbError(res.Error, "update deployment")
	}
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "deployment not found")
	}
	return s.GetDeployment(ctx, IDRequest{r.ID})
}
func (s *Service) UpdateDeploymentStatus(ctx context.Context, r StatusRequest) (any, error) {
	if !statusOK(r.Status) {
		return nil, errs.Validation("invalid status")
	}
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	res := s.db.WithContext(ctx).Model(&Deployment{}).Where("tenant_id=? AND id=?", tid, r.ID).Update("status", r.Status)
	if res.Error != nil {
		return nil, dbError(res.Error, "update deployment status")
	}
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "deployment not found")
	}
	return s.GetDeployment(ctx, IDRequest{r.ID})
}
func (s *Service) DeleteDeployment(ctx context.Context, r IDRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	var aliases int64
	if e = s.db.WithContext(ctx).Model(&Alias{}).Where("tenant_id=? AND deployment_id=?", tid, r.ID).Count(&aliases).Error; e != nil {
		return nil, dbError(e, "check deployment aliases")
	}
	if aliases > 0 {
		return nil, errs.New(errs.CodeConflict, "deployment is referenced by aliases")
	}
	res := s.db.WithContext(ctx).Where("tenant_id=? AND id=?", tid, r.ID).Delete(&Deployment{})
	if res.Error != nil {
		return nil, dbError(res.Error, "delete deployment")
	}
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "deployment not found")
	}
	return map[string]any{"deleted": true}, nil
}
func (s *Service) ListAliases(ctx context.Context, _ struct{}) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	var rows []Alias
	e = s.db.WithContext(ctx).Where("tenant_id=?", tid).Order("code").Find(&rows).Error
	return rows, dbError(e, "list aliases")
}
func (s *Service) UpsertAlias(ctx context.Context, r AliasUpsertRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	if !aliasCodePattern.MatchString(r.Code) {
		return nil, errs.ValidationKey("common.validation_failed", "invalid alias code", errs.FieldError{Field: "code", Message: "must start with a lowercase letter and contain only lowercase letters, digits, dot, underscore or hyphen", MessageKey: "validation.invalid"})
	}
	var n int64
	if e = s.db.WithContext(ctx).Model(&Deployment{}).Where("tenant_id=? AND id=?", tid, r.DeploymentID).Count(&n).Error; e != nil {
		return nil, dbError(e, "check alias deployment")
	}
	if n == 0 {
		return nil, errs.New(errs.CodeNotFound, "deployment not found")
	}
	var v Alias
	e = s.db.WithContext(ctx).Where("tenant_id=? AND code=?", tid, r.Code).First(&v).Error
	if e == gorm.ErrRecordNotFound {
		v = Alias{ID: uuid.NewString(), TenantID: tid, Code: r.Code, DeploymentID: r.DeploymentID}
		e = s.db.WithContext(ctx).Create(&v).Error
	} else if e == nil {
		e = s.db.WithContext(ctx).Model(&v).Update("deployment_id", r.DeploymentID).Error
	}
	return v, dbError(e, "alias")
}
func (s *Service) DeleteAlias(ctx context.Context, r AliasDeleteRequest) (any, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	res := s.db.WithContext(ctx).Where("tenant_id=? AND code=?", tid, r.Code).Delete(&Alias{})
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeNotFound, "alias not found")
	}
	return map[string]any{"deleted": true}, dbError(res.Error, "delete alias")
}

func (s *Service) Resolve(ctx context.Context, code string, req aimodel.CapabilityRequirements) (*aimodel.ResolvedModel, error) {
	tid, e := tenant(ctx)
	if e != nil {
		return nil, e
	}
	if e = req.Validate(); e != nil {
		return nil, errs.Validation(e.Error())
	}
	var a Alias
	if e = s.db.WithContext(ctx).Where("tenant_id=? AND code=?", tid, code).First(&a).Error; e != nil {
		return nil, errs.New(errs.CodeNotFound, "alias not found")
	}
	var d Deployment
	if e = s.db.WithContext(ctx).Where("tenant_id=? AND id=? AND status=?", tid, a.DeploymentID, enabled).First(&d).Error; e != nil {
		return nil, errs.New(errs.CodeNotFound, "enabled deployment not found")
	}
	var p Provider
	if e = s.db.WithContext(ctx).Where("tenant_id=? AND id=? AND status=?", tid, d.ProviderID, enabled).First(&p).Error; e != nil {
		return nil, errs.New(errs.CodeNotFound, "enabled provider not found")
	}
	pt, ok := s.types.Get(p.Protocol)
	if !ok {
		return nil, errs.New(errs.CodeValidation, "provider protocol is unavailable")
	}
	if aimodel.ModelType(d.ModelType) != req.ModelType {
		return nil, errs.New(errs.CodeValidation, "model type requirement is not satisfied")
	}
	var declared aimodel.Capabilities
	var limits aimodel.Limits
	var config map[string]any
	if json.Unmarshal(d.CapabilitiesJSON, &declared) != nil || json.Unmarshal(d.LimitsJSON, &limits) != nil {
		return nil, errs.New(errs.CodeValidation, "deployment metadata is invalid")
	}
	effective := aimodel.Intersect(pt.Capabilities, declared)
	if !aimodel.Satisfies(effective, req) {
		return nil, modelCapabilityError(effective, req)
	}
	credential, err := s.credentials.Decrypt(ctx, p.CredentialCiphertext, credentialAAD(tid, p.ID))
	if err != nil {
		return nil, errs.New(errs.CodeValidation, "provider credential cannot be decrypted")
	}
	_ = json.Unmarshal(d.ConfigJSON, &config)
	return &aimodel.ResolvedModel{ProviderID: p.ID, Protocol: p.Protocol, BaseURL: p.BaseURL, Credential: credential, DeploymentID: d.ID, RemoteModelID: d.RemoteModelID, ModelType: aimodel.ModelType(d.ModelType), Capabilities: effective, Limits: limits, Config: config}, nil
}

func modelCapabilityError(have aimodel.Capabilities, req aimodel.CapabilityRequirements) error {
	if containsFeature(req.Features, aimodel.FeatureStreaming) && !containsFeature(have.Features, aimodel.FeatureStreaming) {
		return errs.ValidationKey("ai.model.streaming_not_supported", "model does not support streaming")
	}
	if containsFeature(req.Features, aimodel.FeatureVision) && !containsFeature(have.Features, aimodel.FeatureVision) {
		return errs.ValidationKey("ai.model.vision_not_supported", "model does not support vision")
	}
	return errs.ValidationKey("ai.model.capability_not_supported", "model capability requirement is not satisfied")
}

func containsFeature(items []aimodel.Feature, target aimodel.Feature) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

var _ aimodel.Resolver = (*Service)(nil)
var _ = fmt.Sprintf
