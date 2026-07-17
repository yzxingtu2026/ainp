package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	systemmodel "github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type AgentService struct {
	db        *gorm.DB
	contracts *aiagent.Registry
	models    aimodel.Resolver
	tools     *aitool.Registry
}

func NewAgentService(db *gorm.DB, contracts *aiagent.Registry, models aimodel.Resolver, tools *aitool.Registry) *AgentService {
	return &AgentService{db: db, contracts: contracts, models: models, tools: tools}
}

func (s *AgentService) ReconcileBuiltinForTenant(ctx context.Context, db *gorm.DB, tenantID, userID string) error {
	c, ok := s.contracts.Get("system.operations", "1.0.0")
	if !ok {
		return errs.New(errs.CodeInternal, "builtin contract missing")
	}
	var count int64
	if err := db.WithContext(ctx).Model(&Agent{}).Where("tenant_id = ? AND contract_key = ?", tenantID, c.Key).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	owner := userID
	return db.WithContext(ctx).Create(&Agent{ID: uuid.NewString(), TenantID: tenantID, OwnerUserID: &owner, ContractKey: c.Key, ContractVersion: c.Version, Code: "system-operations", Name: c.Name, Description: c.Description, Source: "builtin", Visibility: "tenant", Status: "draft", Revision: 1}).Error
}

type AgentListRequest struct {
	pagination.Request
	Keyword     string `json:"keyword"`
	Source      string `json:"source"`
	Visibility  string `json:"visibility"`
	Status      string `json:"status"`
	ContractKey string `json:"contract_key"`
	OwnerUserID string `json:"owner_user_id"`
}
type AgentCreateRequest struct {
	Code               string         `json:"code" validate:"required"`
	Name               string         `json:"name" validate:"required"`
	Description        string         `json:"description"`
	Source             string         `json:"source"`
	Visibility         string         `json:"visibility"`
	ModelAlias         string         `json:"model_alias" validate:"required"`
	TenantInstructions string         `json:"tenant_instructions"`
	AvatarAttachmentID *string        `json:"avatar_attachment_id"`
	GenerationConfig   map[string]any `json:"generation_config"`
	ToolPolicy         map[string]any `json:"tool_policy"`
}
type AgentUpdateRequest struct {
	ID                 string         `json:"id" validate:"required"`
	Name               string         `json:"name" validate:"required"`
	Description        string         `json:"description"`
	AvatarAttachmentID *string        `json:"avatar_attachment_id"`
	// 以下为 Builder 草稿运行配置；写入 Agent 草稿列，不创建 Version、不晋升 current。
	ModelAlias         string         `json:"model_alias"`
	TenantInstructions string         `json:"tenant_instructions"`
	GenerationConfig   map[string]any `json:"generation_config"`
	ToolPolicy         map[string]any `json:"tool_policy"`
	SaveDraftConfig    bool           `json:"save_draft_config"`
	Revision           uint64         `json:"revision" validate:"required"`
}
type AgentVersionCreateRequest struct {
	AgentID            string         `json:"agent_id" validate:"required"`
	ModelAlias         string         `json:"model_alias" validate:"required"`
	TenantInstructions string         `json:"tenant_instructions"`
	GenerationConfig   map[string]any `json:"generation_config"`
	ToolPolicy         map[string]any `json:"tool_policy"`
	Revision           uint64         `json:"revision" validate:"required"`
}

// AgentPreviewMaterializeRequest 将 Builder 当前编辑状态物化为 preview Version，不晋升 current_version。
type AgentPreviewMaterializeRequest struct {
	AgentID            string         `json:"agent_id" validate:"required"`
	ModelAlias         string         `json:"model_alias" validate:"required"`
	TenantInstructions string         `json:"tenant_instructions"`
	GenerationConfig   map[string]any `json:"generation_config"`
	ToolPolicy         map[string]any `json:"tool_policy"`
	Revision           uint64         `json:"revision" validate:"required"`
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
type AgentToolSearchRequest struct {
	Query      string        `json:"query" validate:"max=512"`
	Capability string        `json:"capability"`
	Source     aitool.Source `json:"source"`
	Limit      int           `json:"limit" validate:"omitempty,min=1,max=100"`
}
type AgentEmptyRequest struct{}

func agentActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}
func canManageTenant(ac *auth.Context) bool {
	for _, permission := range []string{
		ActionAgentCreate, ActionAgentUpdate, ActionAgentDuplicate, ActionAgentStatus,
		ActionAgentVersionCreate, ActionAgentVersionPublish, ActionAgentPreviewMaterialize,
		ActionAgentToolBind, ActionAgentToolUnbind, ActionAgentBuiltinReconcile,
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
	var attachment systemmodel.Attachment
	if err := s.db.WithContext(ctx).Where("tenant_id=? AND id=? AND status=?", ac.TenantID, *id, "ready").First(&attachment).Error; err != nil {
		return errs.New(errs.CodeNotFound, "avatar attachment not found")
	}
	if !canManageTenant(ac) && attachment.UploaderID != ac.UserID {
		return errs.New(errs.CodeNotFound, "avatar attachment not found")
	}
	if !strings.HasPrefix(attachment.ContentType, "image/") {
		return errs.New(errs.CodeValidation, "avatar attachment must be an image")
	}
	return nil
}

func (s *AgentService) visibleQuery(ctx context.Context) (*gorm.DB, *auth.Context, error) {
	ac, err := agentActor(ctx)
	if err != nil {
		return nil, nil, err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ?", ac.TenantID)
	if !canManageTenant(ac) {
		q = q.Where("visibility = ? OR owner_user_id = ?", "tenant", ac.UserID)
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
	var total int64
	if err := q.Model(&Agent{}).Count(&total).Error; err != nil {
		return nil, dbError(err, "list agents")
	}
	var rows []Agent
	if err := q.Order("updated_at desc").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error; err != nil {
		return nil, dbError(err, "list agents")
	}
	return map[string]any{"items": rows, "total": total, "page": r.Page, "page_size": r.PageSize}, nil
}

func (s *AgentService) get(ctx context.Context, id string, edit bool) (*Agent, *auth.Context, error) {
	q, ac, err := s.visibleQuery(ctx)
	if err != nil {
		return nil, nil, err
	}
	var row Agent
	if err = q.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, nil, errs.New(errs.CodeNotFound, "agent not found")
	}
	if edit && !canManageTenant(ac) && (row.OwnerUserID == nil || *row.OwnerUserID != ac.UserID || row.Source != "personal") {
		return nil, nil, errs.New(errs.CodeNotFound, "agent not found")
	}
	return &row, ac, nil
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
	if _, err := s.models.Resolve(ctx, r.ModelAlias, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat}); err != nil {
		return nil, err
	}
	owner := ac.UserID
	row := Agent{ID: uuid.NewString(), TenantID: ac.TenantID, OwnerUserID: &owner, Code: r.Code, Name: r.Name, Description: r.Description, AvatarAttachmentID: r.AvatarAttachmentID, Source: r.Source, Visibility: r.Visibility, Status: "draft", Revision: 1}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&row).Error; e != nil {
			return dbError(e, "create agent")
		}
		genCfg, toolPolicy := marshal(r.GenerationConfig), marshal(r.ToolPolicy)
		hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy)
		v := AgentVersion{
			ID: uuid.NewString(), TenantID: ac.TenantID, AgentID: row.ID, Version: 1,
			Kind: AgentVersionKindRelease, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, CreatedBy: ac.UserID,
		}
		if e := tx.Create(&v).Error; e != nil {
			return e
		}
		row.CurrentVersion = 1
		return tx.Model(&row).Update("current_version", 1).Error
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
	var version AgentVersion
	if err = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=? AND version=?", row.TenantID, row.ID, row.CurrentVersion).First(&version).Error; err != nil {
		return nil, errs.New(errs.CodeConflict, "source agent has no current version")
	}
	request := AgentCreateRequest{Code: r.Code, Name: r.Name, Description: row.Description, Source: "personal", Visibility: "private", ModelAlias: version.ModelAlias, TenantInstructions: version.TenantInstructions}
	if canManageTenant(ac) && row.Source != "personal" {
		request.Source = "tenant"
		request.Visibility = "tenant"
	}
	_ = json.Unmarshal(version.GenerationConfig, &request.GenerationConfig)
	_ = json.Unmarshal(version.ToolPolicy, &request.ToolPolicy)
	created, err := s.Create(ctx, request)
	if err != nil {
		return nil, err
	}
	target := created.(Agent)
	var bindings []AgentToolBinding
	_ = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=?", row.TenantID, row.ID).Find(&bindings).Error
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
			if _, err := s.models.Resolve(ctx, r.ModelAlias, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat}); err != nil {
				return nil, err
			}
		}
		updates["has_draft"] = true
		updates["draft_tenant_instructions"] = r.TenantInstructions
		updates["draft_model_alias"] = strings.TrimSpace(r.ModelAlias)
		updates["draft_generation_config"] = marshal(r.GenerationConfig)
		updates["draft_tool_policy"] = marshal(r.ToolPolicy)
	}
	result := s.db.WithContext(ctx).Model(&Agent{}).Where("id=? AND tenant_id=? AND revision=?", row.ID, row.TenantID, r.Revision).Updates(updates)
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
		d, _ := s.Diagnostics(ctx, IDRequest{ID: r.ID})
		if !d.(map[string]any)["healthy"].(bool) {
			return nil, errs.New(errs.CodeConflict, "agent diagnostics failed")
		}
	}
	result := s.db.WithContext(ctx).Model(&Agent{}).Where("id=? AND tenant_id=? AND revision=?", row.ID, row.TenantID, r.Revision).Updates(map[string]any{"status": r.Status, "revision": gorm.Expr("revision + 1")})
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
	hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy)
	var version AgentVersion
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		next, err := nextAgentVersionNumber(ctx, tx, row.TenantID, row.ID)
		if err != nil {
			return err
		}
		version = AgentVersion{
			ID: uuid.NewString(), TenantID: row.TenantID, AgentID: row.ID, Version: next,
			Kind: AgentVersionKindRelease, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, CreatedBy: ac.UserID,
		}
		if e := tx.Create(&version).Error; e != nil {
			return e
		}
		res := tx.Model(&Agent{}).Where("id=? AND tenant_id=? AND revision=?", row.ID, row.TenantID, r.Revision).Updates(map[string]any{
			"current_version":            next,
			"revision":                   gorm.Expr("revision + 1"),
			"has_draft":                  false,
			"draft_tenant_instructions":  "",
			"draft_model_alias":          "",
			"draft_generation_config":    marshal(map[string]any{}),
			"draft_tool_policy":          marshal(map[string]any{}),
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
	hash := versionContentHash(r.ModelAlias, r.TenantInstructions, genCfg, toolPolicy)
	var existing AgentVersion
	if e = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=? AND kind=? AND content_hash=?", row.TenantID, row.ID, AgentVersionKindPreview, hash).Order("version desc").First(&existing).Error; e == nil {
		return existing, nil
	}
	var version AgentVersion
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		next, err := nextAgentVersionNumber(ctx, tx, row.TenantID, row.ID)
		if err != nil {
			return err
		}
		version = AgentVersion{
			ID: uuid.NewString(), TenantID: row.TenantID, AgentID: row.ID, Version: next,
			Kind: AgentVersionKindPreview, ContentHash: hash, ModelAlias: r.ModelAlias,
			TenantInstructions: r.TenantInstructions, GenerationConfig: genCfg, ToolPolicy: toolPolicy, CreatedBy: ac.UserID,
		}
		return tx.Create(&version).Error
	})
	return version, e
}

func (s *AgentService) validateVersionInputs(ctx context.Context, row *Agent, modelAlias, tenantInstructions string) error {
	req := aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat}
	if row.ContractKey != "" {
		if c, ok := s.contracts.Get(row.ContractKey, row.ContractVersion); ok {
			for _, v := range c.RequiredModelCapabilities {
				if v == "streaming" {
					req.Features = append(req.Features, aimodel.FeatureStreaming)
				}
				if v == "tools" {
					req.Features = append(req.Features, aimodel.FeatureToolCalling)
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

func nextAgentVersionNumber(ctx context.Context, tx *gorm.DB, tenantID, agentID string) (int, error) {
	var max int
	if err := tx.WithContext(ctx).Model(&AgentVersion{}).Where("tenant_id=? AND agent_id=?", tenantID, agentID).Select("COALESCE(MAX(version),0)").Scan(&max).Error; err != nil {
		return 0, err
	}
	return max + 1, nil
}

func versionContentHash(modelAlias, tenantInstructions string, generationConfig, toolPolicy json.RawMessage) string {
	sum := sha256.Sum256([]byte(modelAlias + "\n" + tenantInstructions + "\n" + string(generationConfig) + "\n" + string(toolPolicy)))
	return hex.EncodeToString(sum[:])
}

func (s *AgentService) ListVersions(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	var rows []AgentVersion
	e = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=? AND kind=?", row.TenantID, row.ID, AgentVersionKindRelease).Order("version desc").Find(&rows).Error
	return rows, e
}
func (s *AgentService) GetVersion(ctx context.Context, r IDRequest) (any, error) {
	ac, e := agentActor(ctx)
	if e != nil {
		return nil, e
	}
	var v AgentVersion
	e = s.db.WithContext(ctx).Where("tenant_id=? AND id=?", ac.TenantID, r.ID).First(&v).Error
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
	if !ok || tool.Status != "enabled" {
		return nil, errs.New(errs.CodeNotFound, "tool not found")
	}
	binding := AgentToolBinding{ID: uuid.NewString(), TenantID: row.TenantID, AgentID: row.ID, ToolID: tool.ID, Source: string(tool.Source), Status: "enabled", CreatedBy: ac.UserID}
	e = s.db.WithContext(ctx).Create(&binding).Error
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
	for _, required := range s.requiredTools(row) {
		if required == r.ToolID {
			return nil, errs.New(errs.CodeForbidden, "required contract tool cannot be unbound")
		}
	}
	res := s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=? AND tool_id=?", row.TenantID, row.ID, r.ToolID).Delete(&AgentToolBinding{})
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
	var rows []AgentToolBinding
	e = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=?", row.TenantID, row.ID).Order("tool_id").Find(&rows).Error
	return rows, e
}
func (s *AgentService) SearchTools(ctx context.Context, r AgentToolSearchRequest) (any, error) {
	if _, e := agentActor(ctx); e != nil {
		return nil, e
	}
	// 管理端绑定弹窗走 Browse；模型侧 meta:tool.search 仍使用硬顶 10 的 Search。
	return aitool.Browse(ctx, s.tools.List(), r.Query, r.Capability, r.Source, r.Limit, func(d aitool.Definition) bool { return d.Status == "enabled" })
}
func (s *AgentService) requiredTools(row *Agent) []string {
	if row.ContractKey == "" {
		return nil
	}
	if c, ok := s.contracts.Get(row.ContractKey, row.ContractVersion); ok {
		return c.RequiredToolSelectors
	}
	return nil
}
func (s *AgentService) Diagnostics(ctx context.Context, r IDRequest) (any, error) {
	row, _, e := s.get(ctx, r.ID, false)
	if e != nil {
		return nil, e
	}
	issues := []map[string]string{}
	var v AgentVersion
	if e = s.db.WithContext(ctx).Where("tenant_id=? AND agent_id=? AND version=?", row.TenantID, row.ID, row.CurrentVersion).First(&v).Error; e != nil {
		issues = append(issues, map[string]string{"code": "version.missing", "message": "current version is missing"})
	} else if _, x := s.models.Resolve(ctx, v.ModelAlias, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat}); x != nil {
		issues = append(issues, map[string]string{"code": "model.unavailable", "message": x.Error()})
	}
	for _, id := range s.requiredTools(row) {
		if t, ok := s.tools.Get(id); !ok || t.Status != "enabled" {
			issues = append(issues, map[string]string{"code": "tool.unavailable", "resource": id, "message": "required tool is unavailable"})
		}
	}
	return map[string]any{"healthy": len(issues) == 0, "issues": issues}, nil
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
	var row Agent
	e = s.db.WithContext(ctx).Where("tenant_id=? AND contract_key=?", ac.TenantID, c.Key).First(&row).Error
	if e == nil {
		return map[string]any{"created": false, "agent": row}, nil
	}
	if e != gorm.ErrRecordNotFound {
		return nil, e
	}
	owner := ac.UserID
	row = Agent{ID: uuid.NewString(), TenantID: ac.TenantID, OwnerUserID: &owner, ContractKey: c.Key, ContractVersion: c.Version, Code: "system-operations", Name: c.Name, Description: c.Description, Source: "builtin", Visibility: "tenant", Status: "draft", Revision: 1}
	e = s.db.WithContext(ctx).Create(&row).Error
	return map[string]any{"created": e == nil, "agent": row}, e
}
