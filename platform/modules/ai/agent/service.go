package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const (
	AgentAvatarResourceType = "ai.agent"
	AgentAvatarField        = "avatar"
)

type AgentService struct {
	db        *gorm.DB
	contracts *aiagent.Registry
	models    aimodel.Resolver
	tools     *aitool.Registry
	settings  *setting.Provider
	avatars   AvatarValidator
}

type AvatarValidator interface {
	ValidateAvatar(context.Context, string, string, bool) error
}

func (s *AgentService) SetAvatarValidator(validator AvatarValidator) { s.avatars = validator }

func (s *AgentService) SetMemoryPolicyProvider(p *setting.Provider) { s.settings = p }

func NewAgentService(db *gorm.DB, contracts *aiagent.Registry, models aimodel.Resolver, tools *aitool.Registry) *AgentService {
	if err := registerAgentTenantModels(); err != nil {
		panic(err)
	}
	return &AgentService{db: db, contracts: contracts, models: models, tools: tools}
}

func (s *AgentService) ReconcileBuiltinForTenant(ctx context.Context, db *gorm.DB, tenantID, userID string) error {
	c, ok := s.contracts.Get("system.operations", "1.0.0")
	if !ok {
		return errs.New(errs.CodeInternal, "builtin contract missing")
	}
	ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: tenantID, PrincipalID: userID})
	repos, err := newAgentRepositories(ctx, db)
	if err != nil {
		return err
	}
	count, err := repos.agents.Where("contract_key = ?", c.Key).Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	owner := userID
	return repos.agents.Create(&Agent{ID: uuid.NewString(), OwnerUserID: &owner, ContractKey: c.Key, ContractVersion: c.Version, Code: "system-operations", Name: c.Name, Description: c.Description, Source: "builtin", Visibility: "tenant", Status: "draft", Revision: 1})
}

type AgentListRequest struct {
	pagination.Request
	Keyword     string `json:"keyword" validate:"omitempty"`
	Source      string `json:"source" validate:"omitempty"`
	Visibility  string `json:"visibility" validate:"omitempty"`
	Status      string `json:"status" validate:"omitempty"`
	ContractKey string `json:"contract_key" validate:"omitempty"`
	OwnerUserID string `json:"owner_user_id" validate:"omitempty"`
}
type AgentCreateRequest struct {
	Code               string           `json:"code" validate:"required"`
	Name               string           `json:"name" validate:"required"`
	Description        string           `json:"description" validate:"omitempty"`
	Source             string           `json:"source" validate:"omitempty"`
	Visibility         string           `json:"visibility" validate:"omitempty"`
	ModelAlias         string           `json:"model_alias" validate:"required"`
	TenantInstructions string           `json:"tenant_instructions" validate:"omitempty"`
	AvatarAttachmentID *string          `json:"avatar_attachment_id" validate:"omitempty"`
	GenerationConfig   map[string]any   `json:"generation_config" validate:"omitempty"`
	ToolPolicy         map[string]any   `json:"tool_policy" validate:"omitempty"`
	MemoryPolicy       *aimemory.Policy `json:"memory_policy,omitempty" validate:"omitempty"`
}
type AgentUpdateRequest struct {
	ID                 string  `json:"id" validate:"required"`
	Name               string  `json:"name" validate:"required"`
	Description        string  `json:"description" validate:"omitempty"`
	AvatarAttachmentID *string `json:"avatar_attachment_id" validate:"omitempty"`
	// 以下为 Builder 草稿运行配置；写入 Agent 草稿列，不创建 Version、不晋升 current。
	ModelAlias         string           `json:"model_alias" validate:"omitempty"`
	TenantInstructions string           `json:"tenant_instructions" validate:"omitempty"`
	GenerationConfig   map[string]any   `json:"generation_config" validate:"omitempty"`
	ToolPolicy         map[string]any   `json:"tool_policy" validate:"omitempty"`
	MemoryPolicy       *aimemory.Policy `json:"memory_policy,omitempty" validate:"omitempty"`
	SaveDraftConfig    bool             `json:"save_draft_config" validate:"omitempty"`
	Revision           uint64           `json:"revision" validate:"required"`
}
type AgentVersionCreateRequest struct {
	AgentID            string           `json:"agent_id" validate:"required"`
	ModelAlias         string           `json:"model_alias" validate:"required"`
	TenantInstructions string           `json:"tenant_instructions" validate:"omitempty"`
	GenerationConfig   map[string]any   `json:"generation_config" validate:"omitempty"`
	ToolPolicy         map[string]any   `json:"tool_policy" validate:"omitempty"`
	MemoryPolicy       *aimemory.Policy `json:"memory_policy,omitempty" validate:"omitempty"`
	Revision           uint64           `json:"revision" validate:"required"`
}

// AgentPreviewMaterializeRequest 将 Builder 当前编辑状态物化为 preview Version，不晋升 current_version。
type AgentPreviewMaterializeRequest struct {
	AgentID            string           `json:"agent_id" validate:"required"`
	ModelAlias         string           `json:"model_alias" validate:"required"`
	TenantInstructions string           `json:"tenant_instructions" validate:"omitempty"`
	GenerationConfig   map[string]any   `json:"generation_config" validate:"omitempty"`
	ToolPolicy         map[string]any   `json:"tool_policy" validate:"omitempty"`
	MemoryPolicy       *aimemory.Policy `json:"memory_policy,omitempty" validate:"omitempty"`
	Revision           uint64           `json:"revision" validate:"required"`
}
type AgentBindRequest struct {
	AgentID string `json:"agent_id" validate:"required"`
	ToolID  string `json:"tool_id" validate:"required"`
}
type AgentUnbindRequest struct {
	AgentID string `json:"agent_id" validate:"required"`
	ToolID  string `json:"tool_id" validate:"required"`
}
type AgentStatusRequest struct {
	ID       string `json:"id" validate:"required"`
	Status   string `json:"status" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}
type AgentDiagnosticIssue struct {
	Code     string `json:"code"`
	Resource string `json:"resource,omitempty"`
	Message  string `json:"message"`
}
type AgentDiagnostics struct {
	Healthy bool                   `json:"healthy"`
	Issues  []AgentDiagnosticIssue `json:"issues"`
}
type AgentToolSearchRequest struct {
	Query      string        `json:"query" validate:"omitempty,max=512"`
	Capability string        `json:"capability" validate:"omitempty"`
	Source     aitool.Source `json:"source" validate:"omitempty"`
	Limit      int           `json:"limit" validate:"omitempty,min=1,max=100"`
}
type AgentEmptyRequest struct{}

func agentActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}
func canManageTenant(ac *auth.Context) bool {
	for _, permission := range []string{
		ActionCreate, ActionUpdate, ActionDuplicate, ActionStatus,
		ActionVersionCreate, ActionVersionPublish, ActionPreviewMaterialize,
		ActionToolBind, ActionToolUnbind, ActionBuiltinReconcile,
	} {
		if ac.HasPermission(permission) {
			return true
		}
	}
	return false
}

func (s *AgentService) validateAvatar(ctx context.Context, ac *auth.Context, id *string) error {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	if s.avatars == nil {
		return errs.New(errs.CodeInternal, "avatar validator unavailable")
	}
	return s.avatars.ValidateAvatar(ctx, *id, ac.PrincipalID, canManageTenant(ac))
}

type agentAvatarAttachmentPolicy struct {
	service *AgentService
}

func NewAvatarAttachmentPolicy(service *AgentService) attachment.AccessPolicy {
	return agentAvatarAttachmentPolicy{service: service}
}

func (p agentAvatarAttachmentPolicy) ResourceType() string { return AgentAvatarResourceType }

func (p agentAvatarAttachmentPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	ac, err := agentActor(ctx)
	if err != nil || ac.TenantID != tenantID || ac.PrincipalID != userID || field != AgentAvatarField {
		return errs.New(errs.CodeNotFound, "agent avatar not found")
	}
	edit := attachment.ResourceLinkOperationFromContext(ctx) != ""
	_, _, err = p.service.get(ctx, resourceID, edit)
	return err
}

func (s *AgentService) visibleQuery(ctx context.Context) (*tenantdata.Repository[Agent], *auth.Context, error) {
	ac, err := agentActor(ctx)
	if err != nil {
		return nil, nil, err
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, nil, e
	}
	q := repos.agents
	if !canManageTenant(ac) {
		q = q.Where("visibility = ? OR owner_user_id = ?", "tenant", ac.PrincipalID)
	}
	return q, ac, nil
}

func (s *AgentService) List(ctx context.Context, r AgentListRequest) (any, error) {
	q, _, err := s.visibleQuery(ctx)
	if err != nil {
		return nil, err
	}
	if r.Keyword != "" {
		like := "%" + strings.TrimSpace(r.Keyword) + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	if r.Source != "" {
		q = q.Where("source = ?", r.Source)
	}
	if r.Visibility != "" {
		q = q.Where("visibility = ?", r.Visibility)
	}
	if r.Status != "" {
		q = q.Where("status = ?", r.Status)
	}
	if r.ContractKey != "" {
		q = q.Where("contract_key = ?", r.ContractKey)
	}
	if r.OwnerUserID != "" {
		q = q.Where("owner_user_id = ?", r.OwnerUserID)
	}
	total, err := q.Count()
	if err != nil {
		return nil, dbError(err, "list agents")
	}
	rows, err := q.Order("updated_at desc").List(r.Offset(), r.PageSize)
	if err != nil {
		return nil, dbError(err, "list agents")
	}
	return map[string]any{"items": rows, "total": total, "page": r.Page, "page_size": r.PageSize}, nil
}

func (s *AgentService) get(ctx context.Context, id string, edit bool) (*Agent, *auth.Context, error) {
	q, ac, err := s.visibleQuery(ctx)
	if err != nil {
		return nil, nil, err
	}
	row, err := q.First("id = ?", id)
	if err != nil {
		return nil, nil, errs.New(errs.CodeNotFound, "agent not found")
	}
	if edit && !canManageTenant(ac) && (row.OwnerUserID == nil || *row.OwnerUserID != ac.PrincipalID || row.Source != "personal") {
		return nil, nil, errs.New(errs.CodeNotFound, "agent not found")
	}
	return &row, ac, nil
}

func (s *AgentService) GetVisible(ctx context.Context, id string, edit bool) (*Agent, *auth.Context, error) {
	return s.get(ctx, id, edit)
}

// AgentDetail is the builder-facing agent payload. Contract fields are read-only overlays.
type AgentDetail struct {
	Agent
	BaseInstructions      string   `json:"base_instructions,omitempty"`
	ManagedFields         []string `json:"managed_fields,omitempty"`
	RequiredToolSelectors []string `json:"required_tool_selectors,omitempty"`
}

func (s *AgentService) Get(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	return s.toDetail(row), nil
}

func (s *AgentService) toDetail(row *Agent) AgentDetail {
	detail := AgentDetail{Agent: *row}
	if row.ContractKey == "" || row.ContractVersion == "" || s.contracts == nil {
		return detail
	}
	contract, ok := s.contracts.Get(row.ContractKey, row.ContractVersion)
	if !ok {
		return detail
	}
	detail.BaseInstructions = contract.BaseInstructions
	detail.ManagedFields = append([]string{}, contract.ManagedFields...)
	detail.RequiredToolSelectors = append([]string{}, contract.RequiredToolSelectors...)
	return detail
}

func (s *AgentService) Create(ctx context.Context, r AgentCreateRequest) (any, error) {
	ac, e := agentActor(ctx)
	if e != nil {
		return nil, e
	}
	if !codePattern.MatchString(r.Code) || strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.ModelAlias) == "" {
		return nil, errs.New(errs.CodeValidation, "invalid agent")
	}
	if len(r.TenantInstructions) > 12000 {
		return nil, errs.New(errs.CodeValidation, "instructions exceed limit")
	}
	if err := s.validateAvatar(ctx, ac, r.AvatarAttachmentID); err != nil {
		return nil, err
	}
	if r.Source == "" {
		r.Source = "personal"
	}
	if r.Visibility == "" {
		r.Visibility = "private"
	}
	if r.Source != "personal" && r.Source != "tenant" {
		return nil, errs.New(errs.CodeValidation, "invalid agent source")
	}
	if (r.Source == "tenant" || r.Visibility == "tenant") && !canManageTenant(ac) {
		return nil, errs.New(errs.CodeForbidden, "tenant agent management permission required")
	}
	if r.Source == "personal" && r.Visibility != "private" {
		return nil, errs.New(errs.CodeValidation, "personal agent must be private")
	}
	if _, err := s.models.Resolve(ctx, r.ModelAlias, agentRuntimeModelRequirements()); err != nil {
		return nil, err
	}
	memoryPolicy, memorySource, err := s.resolveMemoryPolicy(ctx, r.MemoryPolicy)
	if err != nil {
		return nil, err
	}
	memoryJSON := marshal(memoryPolicy)
	owner := ac.PrincipalID
	row := Agent{ID: uuid.NewString(), OwnerUserID: &owner, Code: r.Code, Name: r.Name, Description: r.Description, AvatarAttachmentID: r.AvatarAttachmentID, Source: r.Source, Visibility: r.Visibility, Status: "draft", Revision: 1}
	repos, err := newAgentRepositories(ctx, s.db)
	if err != nil {
		return nil, err
	}
	err = repos.transaction(func(tx *agentRepositories) error {
		if e := tx.agents.Create(&row); e != nil {
			return dbError(e, "create agent")
		}
		genCfg, toolPolicy := marshal(r.GenerationConfig), marshal(r.ToolPolicy)
		hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy, memoryJSON)
		v := AgentVersion{
			ID: uuid.NewString(), AgentID: row.ID, Version: 1,
			Kind: AgentVersionKindRelease, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, MemoryPolicy: memoryJSON, MemoryPolicySource: memorySource, CreatedBy: ac.PrincipalID,
		}
		if e := tx.versions.Create(&v); e != nil {
			return e
		}
		row.CurrentVersion = 1
		return tx.agents.Where("id = ?", row.ID).Updates(map[string]any{"current_version": 1}).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

type AgentDuplicateRequest struct {
	ID   string `json:"id" validate:"required"`
	Code string `json:"code" validate:"required"`
	Name string `json:"name" validate:"required"`
}

func (s *AgentService) Duplicate(ctx context.Context, r AgentDuplicateRequest) (any, error) {
	row, ac, err := s.get(ctx, r.ID, false)
	if err != nil {
		return nil, err
	}
	if !codePattern.MatchString(r.Code) || strings.TrimSpace(r.Name) == "" {
		return nil, errs.New(errs.CodeValidation, "invalid duplicate identity")
	}
	repos, err := newAgentRepositories(ctx, s.db)
	if err != nil {
		return nil, err
	}
	version, err := repos.versions.First("agent_id=? AND version=?", row.ID, row.CurrentVersion)
	if err != nil {
		return nil, errs.New(errs.CodeConflict, "source agent has no current version")
	}
	request := AgentCreateRequest{Code: r.Code, Name: r.Name, Description: row.Description, Source: "personal", Visibility: "private", ModelAlias: version.ModelAlias, TenantInstructions: version.TenantInstructions}
	if canManageTenant(ac) && row.Source != "personal" {
		request.Source = "tenant"
		request.Visibility = "tenant"
	}
	_ = json.Unmarshal(version.GenerationConfig, &request.GenerationConfig)
	_ = json.Unmarshal(version.ToolPolicy, &request.ToolPolicy)
	_ = json.Unmarshal(version.MemoryPolicy, &request.MemoryPolicy)
	created, err := s.Create(ctx, request)
	if err != nil {
		return nil, err
	}
	target := created.(Agent)
	bindings, _ := repos.tools.Find("agent_id=?", row.ID)
	for _, binding := range bindings {
		if tool, ok := s.tools.Get(binding.ToolID); ok && tool.Status == "enabled" {
			_, _ = s.BindTool(ctx, AgentBindRequest{AgentID: target.ID, ToolID: binding.ToolID})
		}
	}
	return target, nil
}

func (s *AgentService) PublishVersion(ctx context.Context, r AgentVersionCreateRequest) (any, error) {
	return s.CreateVersion(ctx, r)
}

func (s *AgentService) Update(ctx context.Context, r AgentUpdateRequest) (any, error) {
	row, ac, e := s.get(ctx, r.ID, true)
	if e != nil {
		return nil, e
	}
	if row.Source == "builtin" && r.Name == "" {
		return nil, errs.New(errs.CodeValidation, "builtin name cannot be empty")
	}
	if e = s.validateAvatar(ctx, ac, r.AvatarAttachmentID); e != nil {
		return nil, e
	}
	updates := map[string]any{
		"name":                 r.Name,
		"description":          r.Description,
		"avatar_attachment_id": r.AvatarAttachmentID,
		"revision":             gorm.Expr("revision + 1"),
	}
	if r.SaveDraftConfig {
		if len(r.TenantInstructions) > 12000 {
			return nil, errs.New(errs.CodeValidation, "instructions exceed limit")
		}
		if strings.TrimSpace(r.ModelAlias) != "" {
			if _, err := s.models.Resolve(ctx, r.ModelAlias, agentRuntimeModelRequirements()); err != nil {
				return nil, err
			}
		}
		updates["has_draft"] = true
		updates["draft_tenant_instructions"] = r.TenantInstructions
		updates["draft_model_alias"] = strings.TrimSpace(r.ModelAlias)
		updates["draft_generation_config"] = marshal(r.GenerationConfig)
		updates["draft_tool_policy"] = marshal(r.ToolPolicy)
		updates["draft_memory_policy"] = marshal(r.MemoryPolicy)
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	result := repos.agents.Where("id=? AND revision=?", row.ID, r.Revision).Updates(updates)
	if result.Error != nil {
		return nil, dbError(result.Error, "update agent")
	}
	if result.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent revision conflict")
	}
	return s.Get(ctx, IDRequest{ID: r.ID})
}
func (s *AgentService) Status(ctx context.Context, r AgentStatusRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, true)
	if e != nil {
		return nil, e
	}
	if !statusOK(r.Status) {
		return nil, errs.New(errs.CodeValidation, "invalid status")
	}
	if r.Status == "enabled" {
		if row.CurrentVersion <= 0 {
			return nil, errs.NewKey(errs.CodeConflict, "ai.agent.publish_required", "publish the first agent version before enabling")
		}
		d := s.agentDiagnostics(ctx, row)
		if !d.Healthy {
			return nil, errs.NewKey(errs.CodeConflict, "ai.agent.diagnostics_failed", "agent diagnostics failed")
		}
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	result := repos.agents.Where("id=? AND revision=?", row.ID, r.Revision).Updates(map[string]any{"status": r.Status, "revision": gorm.Expr("revision + 1")})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, errs.New(errs.CodeConflict, "agent revision conflict")
	}
	return s.Get(ctx, IDRequest{ID: r.ID})
}

func (s *AgentService) CreateVersion(ctx context.Context, r AgentVersionCreateRequest) (any, error) {
	row, ac, e := s.get(ctx, r.AgentID, true)
	if e != nil {
		return nil, e
	}
	if e = s.validateVersionInputs(ctx, row, r.ModelAlias, r.TenantInstructions); e != nil {
		return nil, e
	}
	genCfg, toolPolicy := marshal(r.GenerationConfig), marshal(r.ToolPolicy)
	memoryPolicy, memorySource, e := s.resolveMemoryPolicy(ctx, r.MemoryPolicy)
	if e != nil {
		return nil, e
	}
	memoryJSON := marshal(memoryPolicy)
	hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy, memoryJSON)
	var version AgentVersion
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	e = repos.transaction(func(tx *agentRepositories) error {
		next, err := nextAgentVersionNumber(tx, row.ID)
		if err != nil {
			return err
		}
		version = AgentVersion{
			ID: uuid.NewString(), AgentID: row.ID, Version: next,
			Kind: AgentVersionKindRelease, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, MemoryPolicy: memoryJSON, MemoryPolicySource: memorySource, CreatedBy: ac.PrincipalID,
		}
		if e := tx.versions.Create(&version); e != nil {
			return e
		}
		res := tx.agents.Where("id=? AND revision=?", row.ID, r.Revision).Updates(map[string]any{
			"current_version":           next,
			"revision":                  gorm.Expr("revision + 1"),
			"has_draft":                 false,
			"draft_tenant_instructions": "",
			"draft_model_alias":         "",
			"draft_generation_config":   marshal(map[string]any{}),
			"draft_tool_policy":         marshal(map[string]any{}),
			"draft_memory_policy":       nil,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent revision conflict")
		}
		return nil
	})
	return version, e
}

func (s *AgentService) MaterializePreview(ctx context.Context, r AgentPreviewMaterializeRequest) (any, error) {
	row, ac, e := s.get(ctx, r.AgentID, true)
	if e != nil {
		return nil, e
	}
	if row.Revision != r.Revision {
		return nil, errs.New(errs.CodeConflict, "agent revision conflict")
	}
	if e = s.validateVersionInputs(ctx, row, r.ModelAlias, r.TenantInstructions); e != nil {
		return nil, e
	}
	genCfg, toolPolicy := marshal(r.GenerationConfig), marshal(r.ToolPolicy)
	memoryPolicy, memorySource, e := s.resolveMemoryPolicy(ctx, r.MemoryPolicy)
	if e != nil {
		return nil, e
	}
	memoryJSON := marshal(memoryPolicy)
	hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy, memoryJSON)
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	existing, e := repos.versions.Where("agent_id=? AND kind=? AND content_hash=?", row.ID, AgentVersionKindPreview, hash).Order("version desc").First("1=1")
	if e == nil {
		return existing, nil
	}
	var version AgentVersion
	e = repos.transaction(func(tx *agentRepositories) error {
		next, err := nextAgentVersionNumber(tx, row.ID)
		if err != nil {
			return err
		}
		version = AgentVersion{
			ID: uuid.NewString(), AgentID: row.ID, Version: next,
			Kind: AgentVersionKindPreview, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, MemoryPolicy: memoryJSON, MemoryPolicySource: memorySource, CreatedBy: ac.PrincipalID,
		}
		return tx.versions.Create(&version)
	})
	return version, e
}

func (s *AgentService) validateVersionInputs(ctx context.Context, row *Agent, modelAlias, tenantInstructions string) error {
	req := agentRuntimeModelRequirements()
	if row.ContractKey != "" {
		if c, ok := s.contracts.Get(row.ContractKey, row.ContractVersion); ok {
			for _, v := range c.RequiredModelCapabilities {
				if v == "vision" {
					req.Features = append(req.Features, aimodel.FeatureVision)
				}
			}
		}
	}
	if _, err := s.models.Resolve(ctx, modelAlias, req); err != nil {
		return err
	}
	if len(tenantInstructions) > 12000 {
		return errs.New(errs.CodeValidation, "instructions exceed limit")
	}
	return nil
}

func agentRuntimeModelRequirements() aimodel.CapabilityRequirements {
	return aimodel.CapabilityRequirements{
		ModelType: aimodel.ModelTypeChat,
		Features:  []aimodel.Feature{aimodel.FeatureStreaming, aimodel.FeatureToolCalling},
	}
}

func memoryModelRequirements() aimodel.CapabilityRequirements {
	return aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat, InputModalities: []aimodel.Modality{aimodel.ModalityText}, OutputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: []aimodel.Feature{aimodel.FeatureStructuredOutput}}
}

func (s *AgentService) resolveMemoryPolicy(ctx context.Context, override *aimemory.Policy) (aimemory.Policy, string, error) {
	policy, source := aimemory.SystemDefaultPolicy(), "system_default"
	ac := auth.MustFromContext(ctx)
	if override == nil && s.settings != nil {
		effective, err := s.settings.Get(ctx, ac.TenantID, aimemory.SettingKeyPolicyDefaults)
		if err != nil {
			return policy, source, err
		}
		policy = memoryPolicyFromValues(effective.Values)
		if effective.Source == "tenant" {
			source = "tenant_default"
		}
	}
	if override != nil {
		policy, source = *override, "agent_version"
	}
	policy.ExtractionModelAlias = strings.TrimSpace(policy.ExtractionModelAlias)
	policy.ConsolidationModelAlias = strings.TrimSpace(policy.ConsolidationModelAlias)
	if policy.ConsolidationModelAlias == "" {
		policy.ConsolidationModelAlias = policy.ExtractionModelAlias
	}
	if err := policy.Validate(); err != nil {
		return policy, source, errs.New(errs.CodeValidation, err.Error())
	}
	if !policy.Enabled {
		return policy, source, nil
	}
	if s.models == nil {
		return policy, source, errs.New(errs.CodeConflict, "memory model resolver unavailable")
	}
	seenAliases := map[string]bool{}
	for _, alias := range []string{policy.ExtractionModelAlias, policy.ConsolidationModelAlias} {
		if seenAliases[alias] {
			continue
		}
		seenAliases[alias] = true
		resolved, err := s.models.Resolve(ctx, alias, memoryModelRequirements())
		if err != nil {
			return policy, source, err
		}
		if resolved.Limits.ContextWindow > 0 && resolved.Limits.ContextWindow < 4096 {
			return policy, source, errs.New(errs.CodeValidation, "memory model context window is too small")
		}
	}
	return policy, source, nil
}
func (s *AgentService) ResolveMemoryPolicy(ctx context.Context, override *aimemory.Policy) (aimemory.Policy, string, error) {
	return s.resolveMemoryPolicy(ctx, override)
}

func memoryPolicyFromValues(v map[string]any) aimemory.Policy {
	d := aimemory.SystemDefaultPolicy()
	if x, ok := v["enabled"].(bool); ok {
		d.Enabled = x
	}
	if x, ok := v["extraction_model_alias"].(string); ok {
		d.ExtractionModelAlias = x
	}
	if x, ok := v["consolidation_model_alias"].(string); ok {
		d.ConsolidationModelAlias = x
	}
	if x, ok := v["auto_save_low_sensitive"].(bool); ok {
		d.AutoSaveLowSensitive = x
	}
	if x, ok := v["write_approval_mode"].(string); ok {
		d.WriteApprovalMode = aimemory.WriteApprovalMode(x)
	}
	if x, ok := v["agent_specific_scope"].(bool); ok {
		d.AgentSpecificScope = x
	}
	d.MaxCandidatesPerTurn = intValue(v["max_candidates_per_turn"], d.MaxCandidatesPerTurn)
	d.MaxConsolidationItems = intValue(v["max_consolidation_items"], d.MaxConsolidationItems)
	d.MaxRecallItems = intValue(v["max_recall_items"], d.MaxRecallItems)
	d.MaxRecallTokens = intValue(v["max_recall_tokens"], d.MaxRecallTokens)
	if x, ok := v["policy_version"].(string); ok {
		d.PolicyVersion = x
	}
	if x, ok := v["prompt_version"].(string); ok {
		d.PromptVersion = x
	}
	if x, ok := v["schema_version"].(string); ok {
		d.SchemaVersion = x
	}
	return d
}
func intValue(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return fallback
}

func nextAgentVersionNumber(repos *agentRepositories, agentID string) (int, error) {
	latest, err := repos.versions.Where("agent_id=?", agentID).Order("version desc").First("1=1")
	if tenantdata.IsNotFound(err) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return latest.Version + 1, nil
}

func versionContentHash(modelAlias, tenantInstructions string, generationConfig, toolPolicy, memoryPolicy json.RawMessage) string {
	sum := sha256.Sum256([]byte(modelAlias + "\n" + tenantInstructions + "\n" + string(generationConfig) + "\n" + string(toolPolicy) + "\n" + string(memoryPolicy)))
	return hex.EncodeToString(sum[:])
}

func (s *AgentService) ListVersions(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	rows, e := repos.versions.Where("agent_id=? AND kind=?", row.ID, AgentVersionKindRelease).Order("version desc").Find("1=1")
	return rows, e
}
func (s *AgentService) GetVersion(ctx context.Context, r IDRequest) (any, error) {
	_, e := agentActor(ctx)
	if e != nil {
		return nil, e
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	v, e := repos.versions.GetByID(r.ID)
	if e != nil {
		return nil, errs.New(errs.CodeNotFound, "agent version not found")
	}
	if _, _, e = s.get(ctx, v.AgentID, false); e != nil {
		return nil, e
	}
	return v, nil
}

func (s *AgentService) BindTool(ctx context.Context, r AgentBindRequest) (any, error) {
	row, ac, e := s.get(ctx, r.AgentID, true)
	if e != nil {
		return nil, e
	}
	tool, ok := s.tools.Get(r.ToolID)
	if !ok || tool.Status != "enabled" || !s.tools.IsBindable(r.ToolID) {
		return nil, errs.New(errs.CodeNotFound, "tool not found")
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	group := s.tools.BindingGroup(tool.ID)
	var binding AgentToolBinding
	e = repos.transaction(func(tx *agentRepositories) error {
		existing, err := tx.tools.Find("agent_id=?", row.ID)
		if err != nil {
			return err
		}
		bound := make(map[string]AgentToolBinding, len(existing))
		for _, item := range existing {
			bound[item.ToolID] = item
		}
		for _, member := range group {
			if !s.tools.IsBindable(member.ID) {
				return errs.New(errs.CodeNotFound, "tool not found")
			}
			if current, ok := bound[member.ID]; ok {
				if member.ID == tool.ID {
					binding = current
				}
				continue
			}
			created := AgentToolBinding{ID: uuid.NewString(), AgentID: row.ID, ToolID: member.ID, Source: string(member.Source), Status: "enabled", CreatedBy: ac.PrincipalID}
			if err = tx.tools.Create(&created); err != nil {
				return err
			}
			if member.ID == tool.ID {
				binding = created
			}
		}
		return nil
	})
	if e != nil {
		return nil, dbError(e, "bind tool")
	}
	return binding, nil
}
func (s *AgentService) UnbindTool(ctx context.Context, r AgentUnbindRequest) (any, error) {
	row, _, e := s.get(ctx, r.AgentID, true)
	if e != nil {
		return nil, e
	}
	group := s.tools.BindingGroup(r.ToolID)
	if len(group) == 0 {
		return nil, errs.New(errs.CodeNotFound, "tool not found")
	}
	requiredSet := stringSet(s.requiredTools(row))
	ids := make([]string, 0, len(group))
	for _, member := range group {
		if requiredSet[member.ID] {
			return nil, errs.New(errs.CodeForbidden, "required contract tool cannot be unbound")
		}
		ids = append(ids, member.ID)
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	res := repos.tools.DeleteWhere("agent_id=? AND tool_id IN ?", row.ID, ids)
	if res.Error != nil {
		return nil, res.Error
	}
	return map[string]any{"deleted": res.RowsAffected == 1}, nil
}
func (s *AgentService) ListTools(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	rows, e := repos.tools.Where("agent_id=?", row.ID).Order("tool_id").Find("1=1")
	if e != nil {
		return nil, e
	}
	visible := make([]AgentToolBinding, 0, len(rows)+len(s.requiredTools(row)))
	seen := map[string]bool{}
	for _, binding := range rows {
		if s.tools.IsBindable(binding.ToolID) {
			visible = append(visible, s.bindingView(binding, false))
			seen[binding.ToolID] = true
		}
	}
	for _, id := range s.requiredTools(row) {
		if seen[id] {
			for i := range visible {
				if visible[i].ToolID == id {
					visible[i].Required = true
				}
			}
			continue
		}
		tool, ok := s.tools.Get(id)
		if !ok || tool.Status != "enabled" {
			continue
		}
		visible = append(visible, s.bindingView(AgentToolBinding{ID: "contract:" + row.ID + ":" + id, AgentID: row.ID, ToolID: id, Source: string(tool.Source), Status: "enabled", CreatedBy: "contract"}, true))
	}
	slices.SortFunc(visible, func(a, b AgentToolBinding) int { return strings.Compare(a.ToolID, b.ToolID) })
	return visible, nil
}

func (s *AgentService) bindingView(binding AgentToolBinding, required bool) AgentToolBinding {
	binding.Required = required
	if tool, ok := s.tools.Get(binding.ToolID); ok {
		binding.Name = tool.TitleForHuman
		binding.BindingMode, binding.LoadingMode, binding.BindingGroup = tool.BindingMode, tool.LoadingMode, tool.BindingGroup
	}
	return binding
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
func (s *AgentService) SearchTools(ctx context.Context, r AgentToolSearchRequest) (any, error) {
	if _, e := agentActor(ctx); e != nil {
		return nil, e
	}
	// 管理端绑定弹窗走 Browse；模型侧 meta:tool.search 仍使用硬顶 10 的 Search。
	definitions := s.tools.List()
	for i := range definitions {
		definitions[i].Name, definitions[i].DescriptionForLLM = definitions[i].TitleForHuman, definitions[i].DescriptionForHuman
	}
	return aitool.Browse(ctx, definitions, r.Query, r.Capability, r.Source, r.Limit, func(d aitool.Definition) bool { return s.tools.IsBindable(d.ID) })
}
func (s *AgentService) requiredTools(row *Agent) []string {
	if row.ContractKey == "" {
		return nil
	}
	if c, ok := s.contracts.Get(row.ContractKey, row.ContractVersion); ok {
		seen := map[string]bool{}
		out := make([]string, 0, len(c.RequiredToolSelectors))
		for _, id := range c.RequiredToolSelectors {
			group := s.tools.BindingGroup(id)
			if len(group) == 0 {
				if !seen[id] {
					seen[id], out = true, append(out, id)
				}
				continue
			}
			for _, member := range group {
				if !seen[member.ID] {
					seen[member.ID], out = true, append(out, member.ID)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	return nil
}
func (s *AgentService) Diagnostics(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	return s.agentDiagnostics(ctx, row), nil
}

func (s *AgentService) agentDiagnostics(ctx context.Context, row *Agent) AgentDiagnostics {
	issues := []AgentDiagnosticIssue{}
	if row.ContractKey != "" {
		contract, ok := s.contracts.Get(row.ContractKey, row.ContractVersion)
		if !ok || strings.TrimSpace(contract.BaseInstructions) == "" {
			issues = append(issues, AgentDiagnosticIssue{Code: "agent.contract_unavailable", Resource: row.ContractKey + "@" + row.ContractVersion, Message: "agent contract is unavailable"})
		}
	}
	if row.CurrentVersion <= 0 {
		issues = append(issues, AgentDiagnosticIssue{Code: "version.missing", Message: "publish the first version before enabling"})
	} else {
		repos, repoErr := newAgentRepositories(ctx, s.db)
		v, err := AgentVersion{}, repoErr
		if err == nil {
			v, err = repos.versions.First("agent_id=? AND version=?", row.ID, row.CurrentVersion)
		}
		if err != nil {
			issues = append(issues, AgentDiagnosticIssue{Code: "version.missing", Message: "current version is missing"})
		} else if s.models == nil {
			issues = append(issues, AgentDiagnosticIssue{Code: "model.unavailable", Message: "model resolver unavailable"})
		} else if resolved, resolveErr := s.models.Resolve(ctx, v.ModelAlias, agentRuntimeModelRequirements()); resolveErr != nil {
			issues = append(issues, AgentDiagnosticIssue{Code: "model.unavailable", Message: resolveErr.Error()})
		} else if issue := runtimeModelLimitsIssue(resolved); issue != nil {
			issues = append(issues, *issue)
		} else {
			var policy aimemory.Policy
			if len(v.MemoryPolicy) > 0 && string(v.MemoryPolicy) != "{}" {
				if err = json.Unmarshal(v.MemoryPolicy, &policy); err != nil || policy.Validate() != nil {
					issues = append(issues, AgentDiagnosticIssue{Code: "memory.policy_invalid", Message: "memory policy is invalid"})
				} else if policy.Enabled {
					seenAliases := map[string]bool{}
					for _, alias := range []string{policy.ExtractionModelAlias, policy.ConsolidationModelAlias} {
						if seenAliases[alias] {
							continue
						}
						seenAliases[alias] = true
						if _, resolveErr = s.models.Resolve(ctx, alias, memoryModelRequirements()); resolveErr != nil {
							issues = append(issues, AgentDiagnosticIssue{Code: "memory.model_unavailable", Resource: alias, Message: resolveErr.Error()})
						}
					}
				}
			}
		}
	}
	for _, id := range s.requiredTools(row) {
		if t, ok := s.tools.Get(id); !ok || t.Status != "enabled" {
			issues = append(issues, AgentDiagnosticIssue{Code: "tool.unavailable", Resource: id, Message: "required tool is unavailable"})
		}
	}
	return AgentDiagnostics{Healthy: len(issues) == 0, Issues: issues}
}

func runtimeModelLimitsIssue(resolved *aimodel.ResolvedModel) *AgentDiagnosticIssue {
	if resolved == nil || resolved.Limits.ContextWindow <= 0 {
		return &AgentDiagnosticIssue{
			Code:     "model.context_window_missing",
			Resource: resolvedDeploymentID(resolved),
			Message:  "model context window is not configured",
		}
	}
	return nil
}

func resolvedDeploymentID(resolved *aimodel.ResolvedModel) string {
	if resolved == nil {
		return ""
	}
	return resolved.DeploymentID
}

func (s *AgentService) ReconcileBuiltin(ctx context.Context, _ AgentEmptyRequest) (any, error) {
	ac, e := agentActor(ctx)
	if e != nil {
		return nil, e
	}
	if !canManageTenant(ac) {
		return nil, errs.New(errs.CodeForbidden, "agent management permission required")
	}
	c, ok := s.contracts.Get("system.operations", "1.0.0")
	if !ok {
		return nil, errs.New(errs.CodeInternal, "builtin contract missing")
	}
	repos, e := newAgentRepositories(ctx, s.db)
	if e != nil {
		return nil, e
	}
	row, e := repos.agents.First("contract_key=?", c.Key)
	if e == nil {
		if row.ContractVersion != c.Version || row.Code != "system-operations" || row.Source != "builtin" || row.Visibility != "tenant" {
			result := repos.agents.Where("id=? AND revision=?", row.ID, row.Revision).Updates(map[string]any{
				"contract_version": c.Version, "code": "system-operations", "source": "builtin", "visibility": "tenant", "revision": gorm.Expr("revision + 1"),
			})
			if result.Error != nil {
				return nil, result.Error
			}
			if result.RowsAffected != 1 {
				return nil, errs.New(errs.CodeConflict, "agent revision conflict")
			}
			row.ContractVersion, row.Code, row.Source, row.Visibility, row.Revision = c.Version, "system-operations", "builtin", "tenant", row.Revision+1
		}
		return map[string]any{"created": false, "agent": row}, nil
	}
	if e != gorm.ErrRecordNotFound {
		return nil, e
	}
	owner := ac.PrincipalID
	row = Agent{ID: uuid.NewString(), OwnerUserID: &owner, ContractKey: c.Key, ContractVersion: c.Version, Code: "system-operations", Name: c.Name, Description: c.Description, Source: "builtin", Visibility: "tenant", Status: "draft", Revision: 1}
	e = repos.agents.Create(&row)
	return map[string]any{"created": e == nil, "agent": row}, e
}
