package tooling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var toolCapabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

const (
	ToolCatalogChangedEvent     = "ai.tool_catalog.changed.v1"
	ToolInvocationFinishedEvent = "ai.tool_invocation.finished.v1"
	toolCatalogChangedEvent     = ToolCatalogChangedEvent
	toolInvocationFinishedEvent = ToolInvocationFinishedEvent
)
const toolScopeTTL = 30 * time.Minute
const toolHandleTTL = 10 * time.Minute
const conversationToolWorkingSetLimit = 32

type ToolActivationResult struct {
	Definition     aitool.Definition
	NewlyActivated bool
}

type referenceEvidence struct {
	ResolverID      string    `json:"resolver_id"`
	DiscoveryToolID string    `json:"discovery_tool_id"`
	Version         string    `json:"version"`
	InvocationID    string    `json:"invocation_id"`
	ReadAt          time.Time `json:"read_at"`
}

type ToolService struct {
	db                    tenantdata.GuardedDatabase
	registry              *aitool.Registry
	signer                *aitool.HandleSigner
	writer                outbox.Writer
	embeddings            aimodelruntime.EmbeddingRuntime
	settings              *setting.Provider
	catalogProjectionSync bool
	onConfirmationPending func(context.Context, *gorm.DB, *ToolConfirmation) error
	onConfirmationApprove func(context.Context, *ToolConfirmation) error
	onConfirmationDecided func(context.Context, *ToolConfirmation) error
}

func NewToolService(db *gorm.DB, r *aitool.Registry, signer *aitool.HandleSigner, w outbox.Writer) *ToolService {
	if err := registerToolTenantModels(); err != nil {
		panic(err)
	}
	return &ToolService{db: tenantdata.GuardedDB(db), registry: r, signer: signer, writer: w}
}

func (s *ToolService) Registry() *aitool.Registry { return s.registry }

func ModelToolName(id string) string                                         { return modelToolName(id) }
func (s *ToolService) SetEmbeddingRuntime(v aimodelruntime.EmbeddingRuntime) { s.embeddings = v }
func (s *ToolService) SetSettingProvider(v *setting.Provider)                { s.settings = v }
func (s *ToolService) SetCatalogProjectionSynchronous(enabled bool) {
	s.catalogProjectionSync = enabled
}
func (s *ToolService) SetConfirmationHooks(pending func(context.Context, *gorm.DB, *ToolConfirmation) error, decided func(context.Context, *ToolConfirmation) error) {
	s.onConfirmationPending, s.onConfirmationDecided = pending, decided
}
func (s *ToolService) SetConfirmationApproveHook(approve func(context.Context, *ToolConfirmation) error) {
	s.onConfirmationApprove = approve
}

func (s *ToolService) ProjectModelTools(ctx context.Context, scopeID string) (string, []aimodelruntime.ToolDefinition, error) {
	if err := s.registry.Refresh(ctx); err != nil {
		return "", nil, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return "", nil, err
	}
	scope, err := s.scope(ctx, ac, scopeID)
	if err != nil {
		return "", nil, err
	}
	ids := append(decodeStrings(scope.AllowedToolsJSON), decodeStrings(scope.LoadedToolsJSON)...)
	out := make([]aimodelruntime.ToolDefinition, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		d, ok := s.registry.Get(id)
		if !ok || d.Status != "enabled" || !scopeAllows(scope, d) || !toolVisible(ac, d) {
			continue
		}
		d, ok, err = s.project(ctx, scope, d)
		if err != nil {
			return "", nil, err
		}
		if !ok {
			continue
		}
		out = append(out, aimodelruntime.ToolDefinition{Name: modelToolName(d.ID), Description: d.DescriptionForLLM, Parameters: d.InputSchema})
	}
	return scope.ID, out, nil
}

type ToolSearchRequest struct {
	ScopeID    string        `json:"scope_id,omitempty"`
	Query      string        `json:"query" validate:"required,max=512"`
	Capability string        `json:"capability,omitempty"`
	Source     aitool.Source `json:"source,omitempty"`
	Limit      int           `json:"limit,omitempty"`
}
type ToolSearchResponse struct {
	ScopeID       string             `json:"scope_id"`
	Items         []aitool.Candidate `json:"items"`
	Degraded      bool               `json:"degraded"`
	DegradeReason string             `json:"degrade_reason,omitempty"`
}
type ToolLoadRequest struct {
	ScopeID string   `json:"scope_id" validate:"required"`
	ToolIDs []string `json:"tool_ids" validate:"required,min=1,max=3"`
}
type ToolLoadResponse struct {
	ScopeID              string              `json:"scope_id"`
	Tools                []aitool.LoadedTool `json:"tools"`
	NewlyLoadedToolIDs   []string            `json:"newly_loaded_tool_ids"`
	AlreadyLoadedToolIDs []string            `json:"already_loaded_tool_ids"`
}
type ToolExecuteRequest struct {
	Handle         string          `json:"handle" validate:"required"`
	ToolCallID     string          `json:"tool_call_id" validate:"required,max=128"`
	Arguments      json.RawMessage `json:"arguments" validate:"required"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}
type ToolConfirmationRequest struct {
	ID string `json:"id" validate:"required"`
}
type ToolConfirmationListRequest struct {
	ScopeID string `json:"scope_id,omitempty"`
}

func (r ToolSearchRequest) Validate(context.Context) error {
	if r.Source != "" {
		switch r.Source {
		case aitool.SourceMeta, aitool.SourceAction, aitool.SourceBuiltin, aitool.SourcePlugin, aitool.SourceHTTP, aitool.SourceMCP:
		default:
			return fmt.Errorf("unknown tool source")
		}
	}
	if r.Capability != "" && !toolCapabilityPattern.MatchString(r.Capability) {
		return fmt.Errorf("capability must be a stable capability code")
	}
	if r.Limit < 0 || r.Limit > 10 {
		return fmt.Errorf("limit must be between 1 and 10")
	}
	return nil
}
func (r ToolLoadRequest) Validate(context.Context) error {
	seen := map[string]bool{}
	for _, id := range r.ToolIDs {
		if seen[id] {
			return fmt.Errorf("duplicate tool id")
		}
		seen[id] = true
	}
	return nil
}

func (s *ToolService) Search(ctx context.Context, r ToolSearchRequest) (any, error) {
	if err := r.Validate(ctx); err != nil {
		return nil, err
	}
	if err := s.registry.Refresh(ctx); err != nil {
		return nil, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := s.scope(ctx, ac, r.ScopeID)
	if err != nil {
		return nil, err
	}
	definitions := s.registry.List()
	projected := make([]aitool.Definition, 0, len(definitions))
	visibleIDs := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		var visible bool
		definition, visible, err = s.project(ctx, scope, definition)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		projected = append(projected, definition)
		visibleIDs[definition.ID] = true
	}
	requestedLimit := r.Limit
	if requestedLimit <= 0 {
		requestedLimit = 5
	}
	searchRequest := r
	if _, ok := pagecontext.FromContext(ctx); ok {
		searchRequest.Limit = 10
	}
	items, err := aitool.Search(ctx, projected, searchRequest.Query, searchRequest.Capability, searchRequest.Source, searchRequest.Limit, func(d aitool.Definition) bool { return scopeAllows(scope, d) && toolVisible(ac, d) })
	if err != nil {
		return nil, err
	}
	items, degraded, degradeReason := s.mergeHybrid(ctx, ac, scope, searchRequest, items)
	items = slices.DeleteFunc(items, func(item aitool.Candidate) bool { return !visibleIDs[item.ID] })
	items = s.boostPageContextCandidates(ctx, items)
	if len(items) > requestedLimit {
		items = items[:requestedLimit]
	}
	return ToolSearchResponse{ScopeID: scope.ID, Items: items, Degraded: degraded, DegradeReason: degradeReason}, nil
}

func (s *ToolService) boostPageContextCandidates(ctx context.Context, items []aitool.Candidate) []aitool.Candidate {
	snapshot, ok := pagecontext.FromContext(ctx)
	if !ok || len(items) == 0 {
		return items
	}
	capabilities := make(map[string]bool, len(snapshot.ToolHints.Capabilities))
	tags := make(map[string]bool, len(snapshot.ToolHints.Tags))
	for _, value := range snapshot.ToolHints.Capabilities {
		capabilities[value] = true
	}
	for _, value := range snapshot.ToolHints.Tags {
		tags[value] = true
	}
	for i := range items {
		definition, exists := s.registry.Get(items[i].ID)
		if !exists {
			continue
		}
		if capabilities[definition.Capability] {
			items[i].Score += 0.25
		}
		for _, tag := range definition.Tags {
			if tags[strings.ToLower(tag)] || tags[tag] {
				items[i].Score += 0.1
				break
			}
		}
		text := strings.ToLower(strings.Join(append([]string{definition.Name, definition.DescriptionForLLM}, definition.Tags...), " "))
		for _, query := range snapshot.ToolHints.Queries {
			for _, term := range aitool.LexicalTerms(query) {
				if strings.Contains(text, term) {
					items[i].Score += 0.02
					break
				}
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].ID < items[j].ID
		}
		return items[i].Score > items[j].Score
	})
	return items
}
func (s *ToolService) Load(ctx context.Context, r ToolLoadRequest) (any, error) {
	if err := s.registry.Refresh(ctx); err != nil {
		return nil, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := s.scope(ctx, ac, r.ScopeID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := []aitool.LoadedTool{}
	newlyLoaded := []string{}
	alreadyLoaded := []string{}
	for _, id := range r.ToolIDs {
		activation, activateErr := s.activateIntoScope(ctx, ac, scope, id)
		if activateErr != nil {
			return nil, activateErr
		}
		d := activation.Definition
		if activation.NewlyActivated {
			newlyLoaded = append(newlyLoaded, id)
		} else {
			alreadyLoaded = append(alreadyLoaded, id)
		}
		if err := s.recordWorkingSetActivation(ctx, scope, d, now); err != nil {
			return nil, err
		}
		exp := now.Add(toolHandleTTL)
		if exp.After(scope.ExpiresAt) {
			exp = scope.ExpiresAt
		}
		token, err := s.signer.Sign(aitool.HandleClaims{TenantID: ac.TenantID, PrincipalID: ac.PrincipalID, PrincipalKind: string(toolPrincipal(ac).Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience, Realm: string(auth.NormalizeRealm(ac.Realm)), SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, ScopeID: scope.ID, ToolID: d.ID, RuntimeRef: d.RuntimeRef, SchemaHash: d.SchemaHash, ExpiresAt: exp.Unix()})
		if err != nil {
			return nil, err
		}
		out = append(out, aitool.LoadedTool{Definition: d, Handle: token, ExpiresAt: exp.Unix()})
	}
	return ToolLoadResponse{ScopeID: scope.ID, Tools: out, NewlyLoadedToolIDs: newlyLoaded, AlreadyLoadedToolIDs: alreadyLoaded}, nil
}

func (s *ToolService) ActivateIntoScope(ctx context.Context, scopeID, toolID string) (ToolActivationResult, error) {
	if err := s.registry.Refresh(ctx); err != nil {
		return ToolActivationResult{}, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return ToolActivationResult{}, err
	}
	scope, err := s.scope(ctx, ac, scopeID)
	if err != nil {
		return ToolActivationResult{}, err
	}
	return s.activateIntoScope(ctx, ac, scope, toolID)
}

func (s *ToolService) activateIntoScope(ctx context.Context, ac *auth.Context, scope *ToolScope, toolID string) (ToolActivationResult, error) {
	d, ok := s.registry.Get(toolID)
	if !ok || d.Status != "enabled" || !scopeAllows(scope, d) || !toolVisible(ac, d) {
		return ToolActivationResult{}, errs.New(errs.CodeForbidden, "tool is not visible")
	}
	d, ok, err := s.project(ctx, scope, d)
	if err != nil {
		return ToolActivationResult{}, err
	}
	if !ok {
		return ToolActivationResult{}, errs.New(errs.CodeForbidden, "tool is not visible")
	}
	loaded := decodeStrings(scope.LoadedToolsJSON)
	if contains(loaded, toolID) {
		return ToolActivationResult{Definition: d}, nil
	}
	if len(loaded) >= 8 {
		return ToolActivationResult{}, errs.Validation("tool scope limit exceeded")
	}
	bytes := scope.SchemaBytes + len(d.InputSchema) + len(d.OutputSchema)
	if bytes > 32<<10 {
		return ToolActivationResult{}, errs.Validation("tool schema budget exceeded")
	}
	loaded = append(loaded, toolID)
	if err := s.db.WithContext(ctx).Model(&ToolScope{}).Where("id=? AND principal_id=?", scope.ID, ac.PrincipalID).Updates(map[string]any{"loaded_tools_json": jsonBytes(loaded), "schema_bytes": bytes}).Error; err != nil {
		return ToolActivationResult{}, err
	}
	scope.LoadedToolsJSON = jsonBytes(loaded)
	scope.SchemaBytes = bytes
	return ToolActivationResult{Definition: d, NewlyActivated: true}, nil
}

func (s *ToolService) recordWorkingSetActivation(ctx context.Context, scope *ToolScope, d aitool.Definition, now time.Time) error {
	if scope.ConversationID == nil || *scope.ConversationID == "" {
		return nil
	}
	row := ConversationToolWorkingSet{ID: uuid.NewString(), TenantID: scope.TenantID, ConversationID: *scope.ConversationID, ToolID: d.ID, SchemaHash: d.SchemaHash, ActivatedAt: now}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "conversation_id"}, {Name: "tool_id"}},
			DoUpdates: clause.Assignments(map[string]any{"schema_hash": d.SchemaHash, "activated_at": now, "updated_at": now}),
		}).Create(&row).Error; err != nil {
			return err
		}
		var stale []ConversationToolWorkingSet
		if err := tx.Where("conversation_id=?", *scope.ConversationID).
			Order("COALESCE(last_used_at, activated_at) ASC, use_count ASC, tool_id ASC").
			Offset(conversationToolWorkingSetLimit).Find(&stale).Error; err != nil {
			return err
		}
		if len(stale) == 0 {
			return nil
		}
		ids := make([]string, 0, len(stale))
		for _, item := range stale {
			ids = append(ids, item.ID)
		}
		return tx.Where("conversation_id=? AND id IN ?", *scope.ConversationID, ids).Delete(&ConversationToolWorkingSet{}).Error
	})
}

func (s *ToolService) WorkingSetCandidates(ctx context.Context, conversationID string, limit int) ([]ConversationToolWorkingSet, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []ConversationToolWorkingSet
	err := s.db.WithContext(ctx).Where("conversation_id=?", conversationID).
		Order("last_used_at DESC NULLS LAST, use_count DESC, activated_at DESC, tool_id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *ToolService) WorkingSetEntry(ctx context.Context, conversationID, toolID string) (*ConversationToolWorkingSet, error) {
	var row ConversationToolWorkingSet
	if err := s.db.WithContext(ctx).Where("conversation_id=? AND tool_id=?", conversationID, toolID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *ToolService) IsLoaded(ctx context.Context, scopeID, toolID string) (bool, error) {
	ac, err := toolActor(ctx)
	if err != nil {
		return false, err
	}
	scope, err := s.scope(ctx, ac, scopeID)
	if err != nil {
		return false, err
	}
	return contains(decodeStrings(scope.LoadedToolsJSON), toolID), nil
}

func (s *ToolService) IsProjected(ctx context.Context, scopeID, toolID string) (bool, error) {
	ac, err := toolActor(ctx)
	if err != nil {
		return false, err
	}
	scope, err := s.scope(ctx, ac, scopeID)
	if err != nil {
		return false, err
	}
	return contains(decodeStrings(scope.AllowedToolsJSON), toolID) || contains(decodeStrings(scope.LoadedToolsJSON), toolID), nil
}

func (s *ToolService) Execute(ctx context.Context, r ToolExecuteRequest) (any, error) {
	if err := s.registry.Refresh(ctx); err != nil {
		return nil, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	if len(r.Arguments) == 0 || !json.Valid(r.Arguments) || r.Arguments[0] != '{' {
		return nil, errs.Validation("tool arguments must be a JSON object")
	}
	tool, scope, err := s.resolveLoaded(ctx, ac, r.Handle)
	if err != nil {
		return nil, err
	}
	if err := validateToolArguments(tool.Definition.InputSchema, r.Arguments); err != nil {
		return nil, errs.Validation("invalid tool arguments")
	}
	if r.IdempotencyKey != "" {
		clone := *ac
		clone.IdempotencyKey = r.IdempotencyKey
		ctx = auth.WithContext(ctx, &clone)
		ac = &clone
	}
	return s.execute(ctx, ac, scope, tool, r.ToolCallID, r.Arguments, nil)
}

func (s *ToolService) Approve(ctx context.Context, r ToolConfirmationRequest) (any, error) {
	if err := s.registry.Refresh(ctx); err != nil {
		return nil, err
	}
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	var c ToolConfirmation
	if err = s.db.WithContext(ctx).Where("id=? AND principal_id=?", r.ID, ac.PrincipalID).First(&c).Error; err != nil {
		return nil, err
	}
	if c.Status != ToolConfirmationPending || time.Now().UTC().After(c.ExpiresAt) {
		return nil, confirmationDecisionConflict(c.Status)
	}
	scope, err := s.scope(ctx, ac, c.ScopeID)
	if err != nil {
		return nil, err
	}
	d, ok := s.registry.Get(c.ToolID)
	if !ok || d.SchemaHash != c.SchemaHash || !toolVisible(ac, d) {
		return nil, errs.New(errs.CodeConflict, "tool contract changed")
	}
	d, ok, err = s.project(ctx, scope, d)
	if err != nil || !ok {
		return nil, errs.New(errs.CodeConflict, "tool authorization changed")
	}
	if s.onConfirmationApprove != nil {
		if err := s.onConfirmationApprove(ctx, &c); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claimConfirmation := tx.Model(&ToolConfirmation{}).Where("id=? AND principal_id=? AND status=?", c.ID, ac.PrincipalID, ToolConfirmationPending).Updates(map[string]any{"status": ToolConfirmationApproved, "decided_at": now})
		if claimConfirmation.Error != nil {
			return claimConfirmation.Error
		}
		if claimConfirmation.RowsAffected != 1 {
			return confirmationDecisionConflict("")
		}
		claimInvocation := tx.Model(&ToolInvocation{}).Where("id=? AND principal_id=? AND status=?", c.InvocationID, ac.PrincipalID, ToolInvocationWaitingConfirmation).Update("status", ToolInvocationRunning)
		if claimInvocation.Error != nil {
			return claimInvocation.Error
		}
		if claimInvocation.RowsAffected != 1 {
			return confirmationDecisionConflict("")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	c.Status = ToolConfirmationApproved
	c.DecidedAt = &now
	loaded := aitool.LoadedTool{Definition: d}
	result, err := s.execute(ctx, ac, scope, loaded, c.ToolCallID, c.ArgumentsJSON, &c)
	if err != nil {
		return nil, err
	}
	if s.onConfirmationDecided != nil {
		var latest ToolConfirmation
		if s.db.WithContext(ctx).Where("id=?", c.ID).First(&latest).Error == nil {
			if err := s.onConfirmationDecided(ctx, &latest); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
func (s *ToolService) Reject(ctx context.Context, r ToolConfirmationRequest) (any, error) {
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	var c ToolConfirmation
	if err := s.db.WithContext(ctx).Where("id=? AND principal_id=?", r.ID, ac.PrincipalID).First(&c).Error; err != nil {
		return nil, err
	}
	if c.Status != ToolConfirmationPending {
		return nil, confirmationDecisionConflict(c.Status)
	}
	now := time.Now().UTC()
	var inv ToolInvocation
	if err := s.db.WithContext(ctx).Where("id=?", c.InvocationID).First(&inv).Error; err != nil {
		return nil, err
	}
	rejectedResult := &aitool.Result{CallID: c.ToolCallID, Status: "failed", Summary: "tool call was not executed because confirmation was rejected", ErrorCategory: "confirmation_rejected"}
	if err := s.finishConfirmedInvocation(ctx, &inv, &c, rejectedResult, ToolInvocationFailed, "confirmation_rejected", ToolConfirmationRejected, now); err != nil {
		return nil, err
	}
	c.Status = ToolConfirmationRejected
	c.DecidedAt = &now
	if s.onConfirmationDecided != nil {
		if err := s.onConfirmationDecided(ctx, &c); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": r.ID, "status": "rejected"}, nil
}

func confirmationDecisionConflict(_ string) error {
	return errs.New(errs.CodeConflict, "confirmation has already been handled")
}
func (s *ToolService) ListConfirmations(ctx context.Context, r ToolConfirmationListRequest) (any, error) {
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Where("principal_id=? AND status=? AND expires_at>?", ac.PrincipalID, ToolConfirmationPending, time.Now().UTC())
	if r.ScopeID != "" {
		q = q.Where("scope_id=?", r.ScopeID)
	}
	var rows []ToolConfirmation
	if err = q.Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ToolService) execute(ctx context.Context, ac *auth.Context, scope *ToolScope, loaded aitool.LoadedTool, callID string, args json.RawMessage, approved *ToolConfirmation) (any, error) {
	d := loaded.Definition
	if failure, err := s.validateDynamicReferences(ctx, scope, d, args); err != nil {
		return nil, err
	} else if failure != nil {
		failure.CallID = callID
		if approved != nil {
			var inv ToolInvocation
			if err := s.db.WithContext(ctx).Where("id=? AND principal_id=? AND status=?", approved.InvocationID, ac.PrincipalID, ToolInvocationRunning).First(&inv).Error; err != nil {
				return nil, err
			}
			if err := s.finishApprovedExecution(ctx, &inv, approved, failure, "failed", failure.ErrorCategory); err != nil {
				return nil, err
			}
		}
		return failure, nil
	}
	needs := d.RequiresConfirmation || d.Risk == aitool.RiskHigh || d.SideEffect == aitool.SideEffectWrite || d.SideEffect == aitool.SideEffectExternal || d.SideEffect == aitool.SideEffectPaid || d.SideEffect == aitool.SideEffectDestructive
	if d.Risk == aitool.RiskCritical {
		return nil, errs.New(errs.CodeForbidden, "critical tool execution is disabled")
	}
	now := time.Now().UTC()
	var inv ToolInvocation
	if approved == nil {
		inv = ToolInvocation{ID: uuid.NewString(), TenantID: ac.TenantID, PrincipalID: ac.PrincipalID, ScopeID: scope.ID, AgentRunID: scope.AgentRunID, ToolCallID: callID, ToolID: d.ID, Source: string(d.Source), TargetAction: d.RuntimeRef, Risk: string(d.Risk), SideEffect: string(d.SideEffect), Status: ToolInvocationRunning, StartedAt: now, RequestID: ac.RequestID, TraceID: ac.TraceID}
	}
	if approved == nil && needs {
		hash := hashJSON(args)
		summary := redactArguments(args)
		c := ToolConfirmation{ID: uuid.NewString(), TenantID: ac.TenantID, PrincipalID: ac.PrincipalID, ScopeID: scope.ID, InvocationID: inv.ID, ToolCallID: callID, ToolID: d.ID, SchemaHash: d.SchemaHash, ArgumentsHash: hash, ArgumentsJSON: append(json.RawMessage(nil), args...), ArgumentsSummaryJSON: summary, Risk: string(d.Risk), SideEffect: string(d.SideEffect), Status: ToolConfirmationPending, ExpiresAt: now.Add(10 * time.Minute)}
		inv.Status = ToolInvocationWaitingConfirmation
		inv.ConfirmationID = &c.ID
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			presentation := s.registry.PresentConfirmation(ctx, d, args)
			c.PresentationJSON = jsonBytes(presentation)
			c.PresentationVersion = aitool.ConfirmationPresentationVersion
			c.Presentation = &presentation
			if err := tx.Create(&inv).Error; err != nil {
				return err
			}
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
			if s.onConfirmationPending != nil {
				return s.onConfirmationPending(ctx, tx, &c)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		return &aitool.Result{CallID: callID, Status: "confirmation_required", Summary: "user confirmation is required", Data: c}, nil
	}
	if approved != nil {
		if err := s.db.WithContext(ctx).Where("id=? AND principal_id=? AND status=?", approved.InvocationID, ac.PrincipalID, ToolInvocationRunning).First(&inv).Error; err != nil {
			return nil, err
		}
	} else if err := s.db.WithContext(ctx).Create(&inv).Error; err != nil {
		return nil, err
	}
	if metadata, isSlot := s.registry.SlotMetadata(d.ID); isSlot && metadata.MaxCallsPerRun > 0 && scope.AgentRunID != nil {
		var calls int64
		if err := s.db.WithContext(ctx).Model(&ToolInvocation{}).Where("agent_run_id=? AND tool_id=?", *scope.AgentRunID, d.ID).Count(&calls).Error; err != nil {
			if approved != nil {
				result := &aitool.Result{CallID: callID, Status: "failed", Summary: "tool execution failed", ErrorCategory: "execution_failed"}
				if finishErr := s.finishApprovedExecution(ctx, &inv, approved, result, "failed", result.ErrorCategory); finishErr != nil {
					return nil, errors.Join(err, finishErr)
				}
				return result, nil
			} else {
				_ = s.finishInvocation(ctx, &inv, nil, "failed", "budget_check_failed", err)
			}
			return nil, err
		}
		if calls > int64(metadata.MaxCallsPerRun) {
			result := &aitool.Result{CallID: callID, Status: "failed", Summary: metadata.BudgetCategory + " budget exhausted", ErrorCategory: metadata.BudgetCategory + "_budget_exhausted"}
			if err := s.finishExecution(ctx, &inv, approved, result, "failed", result.ErrorCategory); err != nil {
				return nil, err
			}
			return result, nil
		}
	}
	executor, ok := s.registry.ExecutorFor(d)
	if !ok {
		result := &aitool.Result{CallID: callID, Status: "failed", Summary: "tool execution failed", ErrorCategory: "execution_failed"}
		if err := s.finishExecution(ctx, &inv, approved, result, "failed", result.ErrorCategory); err != nil {
			return nil, err
		}
		return result, nil
	}
	loaded.Handle = ""
	executionCtx := ctx
	if current, ok := auth.FromContext(ctx); ok && current != nil {
		invocationContext := *current
		invocationContext.ToolInvocationID = inv.ID
		if approved != nil {
			invocationContext.IdempotencyKey = "tool-invocation:" + inv.ID
		}
		executionCtx = auth.WithContext(ctx, &invocationContext)
	}
	result, execErr := executor.Execute(aitool.WithScopeID(executionCtx, scope.ID), loaded, args)
	if execErr != nil {
		var recoverable bool
		result, recoverable = executorFailure(callID, execErr)
		if approved == nil && !recoverable {
			if finishErr := s.finishInvocation(ctx, &inv, result, "failed", result.ErrorCategory, execErr); finishErr != nil {
				return nil, errors.Join(execErr, finishErr)
			}
			return nil, execErr
		}
	}
	result.CallID = callID
	if execErr == nil && result.Status != aitool.ResultStatusFailed {
		if err := s.recordReferenceEvidence(executionCtx, scope, d, inv.ID); err != nil {
			return nil, err
		}
	}
	aitool.SanitizeResult(result, 64<<10)
	invocationStatus, category := "completed", ""
	if result.Status == aitool.ResultStatusFailed {
		invocationStatus, category = "failed", result.ErrorCategory
		if metadata, isSlot := s.registry.SlotMetadata(d.ID); isSlot && !slices.Contains(metadata.FailureCategories, category) {
			category = "execution_failed"
			result.ErrorCategory = category
		}
	}
	if err := s.finishExecution(ctx, &inv, approved, result, invocationStatus, category); err != nil {
		return nil, err
	}
	return result, nil
}

func executorFailure(callID string, err error) (*aitool.Result, bool) {
	category := "execution_failed"
	summary := "tool execution failed"
	recoverable := false
	var details *errs.SafeDetails
	var appErr *errs.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case errs.CodeValidation, errs.CodeIdempotencyKeyRequired:
			category, recoverable = "invalid_arguments", true
		case errs.CodeNotFound:
			category, recoverable = "resource_not_found", true
		case errs.CodeConflict:
			category, recoverable = "conflict", true
		case errs.CodeRevisionConflict:
			category, recoverable = "revision_conflict", true
		case errs.CodeIdempotencyConflict:
			category, recoverable = "idempotency_conflict", true
		case errs.CodeOperationInProgress:
			category = "operation_in_progress"
		case errs.CodeResourceLocked:
			category = "resource_locked"
		case errs.CodeUnauthorized, errs.CodeForbidden:
			category = "authorization_failed"
		case errs.CodeActionNotFound:
			category = "tool_unavailable"
		case errs.CodeTooManyRequests:
			category = "rate_limited"
		}
		if recoverable {
			if strings.TrimSpace(appErr.Message) != "" {
				summary = appErr.Message
			} else if category == "invalid_arguments" {
				summary = "tool arguments are invalid; correct them to match the input schema and retry"
			}
			details = appErr.Details
		}
	}
	if stable := errs.StableCategory(err); stable != "" {
		category = stable
	}
	return &aitool.Result{CallID: callID, Status: "failed", Summary: summary, ErrorCategory: category, ErrorDetails: details}, recoverable
}

func (s *ToolService) validateDynamicReferences(ctx context.Context, scope *ToolScope, d aitool.Definition, args json.RawMessage) (*aitool.Result, error) {
	if len(d.DynamicReferences) == 0 {
		return nil, nil
	}
	executor, ok := s.registry.ExecutorFor(d)
	validator, ok := executor.(aitool.ReferenceValidator)
	if !ok {
		return nil, fmt.Errorf("tool %s reference validator is unavailable", d.ID)
	}
	evidence := decodeReferenceEvidence(scope.ReferenceEvidenceJSON)
	for _, ref := range d.DynamicReferences {
		values, err := action.ExtractReferenceValues(args, ref.Field)
		if err != nil {
			return nil, err
		}
		var proof *referenceEvidence
		for i := range evidence {
			if evidence[i].ResolverID == ref.ResolverID && evidence[i].DiscoveryToolID == ref.DiscoveryToolID {
				proof = &evidence[i]
				break
			}
		}
		if ref.RequireEvidence && proof == nil {
			failure := referenceFailure(ref, action.ReferenceRequired, nil)
			logReferenceFailure(ctx, ref, failure)
			return failure, nil
		}
		snapshot, err := validator.ResolveReference(ctx, ref.ResolverID)
		if err != nil {
			return nil, err
		}
		if proof != nil && proof.Version != snapshot.Version {
			failure := referenceFailure(ref, action.ReferenceStale, nil)
			logReferenceFailure(ctx, ref, failure)
			return failure, nil
		}
		invalid := action.InvalidReferenceValues(values, snapshot.Values)
		if len(invalid) > 0 {
			failure := referenceFailure(ref, action.InvalidReference, invalid)
			logReferenceFailure(ctx, ref, failure)
			return failure, nil
		}
	}
	return nil, nil
}

func logReferenceFailure(ctx context.Context, ref aitool.DynamicReference, failure *aitool.Result) {
	invalidCount, truncated := 0, false
	if failure.ErrorDetails != nil {
		invalidCount, truncated = failure.ErrorDetails.InvalidCount, failure.ErrorDetails.ValuesTruncated
	}
	logging.Warn(ctx, "tool dynamic reference rejected", "error_category", failure.ErrorCategory, "resolver_id", ref.ResolverID, "field", ref.Field, "invalid_count", invalidCount, "values_truncated", truncated)
}

func referenceFailure(ref aitool.DynamicReference, reason string, invalid []string) *aitool.Result {
	maxValues, maxBytes := ref.MaxInvalidValues, ref.MaxInvalidBytes
	if maxValues <= 0 {
		maxValues = 8
	}
	if maxBytes <= 0 {
		maxBytes = 512
	}
	count, size, truncated := len(invalid), 0, false
	limited := make([]string, 0, len(invalid))
	for _, value := range invalid {
		if len(limited) >= maxValues || size+len(value) > maxBytes {
			truncated = true
			break
		}
		limited = append(limited, value)
		size += len(value)
	}
	details := &errs.SafeDetails{Field: ref.Field, Reason: reason, InvalidValues: limited, InvalidCount: count, ValuesTruncated: truncated, RefreshAction: strings.TrimPrefix(ref.DiscoveryToolID, "action:"), RefreshRecommended: true}
	return &aitool.Result{Status: "failed", Summary: "dynamic reference validation failed", ErrorCategory: reason, ErrorDetails: details}
}

func decodeReferenceEvidence(raw json.RawMessage) []referenceEvidence {
	var out []referenceEvidence
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func (s *ToolService) recordReferenceEvidence(ctx context.Context, scope *ToolScope, d aitool.Definition, invocationID string) error {
	definitions := s.registry.List()
	needed := map[string]bool{}
	for _, candidate := range definitions {
		for _, ref := range candidate.DynamicReferences {
			if ref.DiscoveryToolID == d.ID {
				needed[ref.ResolverID] = true
			}
		}
	}
	if len(needed) == 0 {
		return nil
	}
	executor, ok := s.registry.ExecutorFor(d)
	validator, ok := executor.(aitool.ReferenceValidator)
	if !ok {
		return fmt.Errorf("discovery tool reference validator is unavailable")
	}
	evidence := decodeReferenceEvidence(scope.ReferenceEvidenceJSON)
	now := time.Now().UTC()
	for resolverID := range needed {
		snapshot, err := validator.ResolveReference(ctx, resolverID)
		if err != nil {
			return err
		}
		next := evidence[:0]
		for _, item := range evidence {
			if item.ResolverID != resolverID {
				next = append(next, item)
			}
		}
		evidence = next
		evidence = append(evidence, referenceEvidence{ResolverID: resolverID, DiscoveryToolID: d.ID, Version: snapshot.Version, InvocationID: invocationID, ReadAt: now})
	}
	if len(evidence) > 16 {
		evidence = evidence[len(evidence)-16:]
	}
	scope.ReferenceEvidenceJSON = jsonBytes(evidence)
	return s.db.WithContext(ctx).Model(&ToolScope{}).Where("id=? AND principal_id=?", scope.ID, scope.PrincipalID).Update("reference_evidence_json", scope.ReferenceEvidenceJSON).Error
}

func (s *ToolService) finishExecution(ctx context.Context, inv *ToolInvocation, approved *ToolConfirmation, result *aitool.Result, status, category string) error {
	if approved != nil {
		return s.finishApprovedExecution(ctx, inv, approved, result, status, category)
	}
	return s.finishInvocation(ctx, inv, result, status, category, nil)
}

func (s *ToolService) finishApprovedExecution(ctx context.Context, inv *ToolInvocation, approved *ToolConfirmation, result *aitool.Result, status, category string) error {
	return s.finishConfirmedInvocation(ctx, inv, approved, result, status, category, ToolConfirmationConsumed, time.Now().UTC())
}

func (s *ToolService) finishInvocation(ctx context.Context, inv *ToolInvocation, result *aitool.Result, status, category string, cause error) error {
	now := time.Now().UTC()
	duration := now.Sub(inv.StartedAt).Milliseconds()
	updates := map[string]any{"status": status, "finished_at": now, "duration_ms": duration, "error_category": category}
	if result != nil {
		updates["result_json"] = jsonBytes(aitool.Result{CallID: result.CallID, Status: result.Status, Summary: result.Summary, ResourceRefs: append([]string(nil), result.ResourceRefs...), Truncated: result.Truncated, ErrorCategory: result.ErrorCategory, ErrorDetails: result.ErrorDetails})
	}
	payload := map[string]any{"invocation_id": inv.ID, "tool_id": inv.ToolID, "source": inv.Source, "status": status, "duration_ms": duration, "error_category": category}
	if metadata, ok := s.registry.SlotMetadata(inv.ToolID); ok {
		payload["invocation_class"] = metadata.InvocationClass
		payload["budget_category"] = metadata.BudgetCategory
	}
	finish := func(tx *gorm.DB, w outbox.Writer) error {
		res := tx.Model(&ToolInvocation{}).Where("id=? AND status IN ?", inv.ID, []string{"running", "waiting_confirmation"}).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if status == ToolInvocationCompleted {
			if err := recordWorkingSetUse(tx, inv, now); err != nil {
				return err
			}
		}
		if w != nil {
			_, err := w.Append(ctx, tx, outbox.AppendInput{EventType: toolInvocationFinishedEvent, AggregateType: "ai_tool_invocation", AggregateID: inv.ID, AggregateVersion: 1, Payload: payload})
			return err
		}
		return nil
	}
	if s.writer == nil {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return finish(tx, nil) })
	}
	return outbox.WithinTx(ctx, s.db.WithContext(ctx), s.writer, finish)
}

func (s *ToolService) finishConfirmedInvocation(ctx context.Context, inv *ToolInvocation, confirmation *ToolConfirmation, result *aitool.Result, invocationStatus, category, confirmationStatus string, decidedAt time.Time) error {
	now := time.Now().UTC()
	duration := now.Sub(inv.StartedAt).Milliseconds()
	invocationUpdates := map[string]any{
		"status": invocationStatus, "finished_at": now, "duration_ms": duration, "error_category": category,
		"result_json": jsonBytes(aitool.Result{CallID: result.CallID, Status: result.Status, Summary: result.Summary, ResourceRefs: append([]string(nil), result.ResourceRefs...), Truncated: result.Truncated, ErrorCategory: result.ErrorCategory, ErrorDetails: result.ErrorDetails}),
	}
	confirmationUpdates := map[string]any{"status": confirmationStatus}
	if confirmationStatus == ToolConfirmationConsumed {
		confirmationUpdates["consumed_at"] = decidedAt
	} else {
		confirmationUpdates["decided_at"] = decidedAt
	}
	payload := map[string]any{"invocation_id": inv.ID, "tool_id": inv.ToolID, "source": inv.Source, "status": invocationStatus, "duration_ms": duration, "error_category": category}
	if metadata, ok := s.registry.SlotMetadata(inv.ToolID); ok {
		payload["invocation_class"] = metadata.InvocationClass
		payload["budget_category"] = metadata.BudgetCategory
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		expectedInvocationStatus := ToolInvocationRunning
		if confirmationStatus == ToolConfirmationRejected {
			expectedInvocationStatus = ToolInvocationWaitingConfirmation
		}
		finished := tx.Model(&ToolInvocation{}).Where("id=? AND status=?", inv.ID, expectedInvocationStatus).Updates(invocationUpdates)
		if finished.Error != nil {
			return finished.Error
		}
		if finished.RowsAffected != 1 {
			return confirmationDecisionConflict("")
		}
		if invocationStatus == ToolInvocationCompleted {
			if err := recordWorkingSetUse(tx, inv, now); err != nil {
				return err
			}
		}
		resolved := tx.Model(&ToolConfirmation{}).Where("id=? AND status=?", confirmation.ID, confirmation.Status).Updates(confirmationUpdates)
		if resolved.Error != nil {
			return resolved.Error
		}
		if resolved.RowsAffected != 1 {
			return confirmationDecisionConflict("")
		}
		if s.writer != nil {
			_, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: toolInvocationFinishedEvent, AggregateType: "ai_tool_invocation", AggregateID: inv.ID, AggregateVersion: 1, Payload: payload})
			return err
		}
		return nil
	})
}

func recordWorkingSetUse(tx *gorm.DB, inv *ToolInvocation, now time.Time) error {
	var scope ToolScope
	if err := tx.Select("tenant_id", "conversation_id").Where("id=?", inv.ScopeID).First(&scope).Error; err != nil {
		return err
	}
	if scope.ConversationID == nil || *scope.ConversationID == "" {
		return nil
	}
	return tx.Model(&ConversationToolWorkingSet{}).
		Where("conversation_id=? AND tool_id=?", *scope.ConversationID, inv.ToolID).
		Updates(map[string]any{"last_used_at": now, "use_count": gorm.Expr("use_count + 1"), "updated_at": now}).Error
}

func (s *ToolService) resolveLoaded(ctx context.Context, ac *auth.Context, token string) (aitool.LoadedTool, *ToolScope, error) {
	var zero aitool.LoadedTool
	claims, err := s.signer.Verify(token, time.Now().UTC())
	if err != nil {
		return zero, nil, errs.New(errs.CodeForbidden, err.Error())
	}
	if claims.TenantID != ac.TenantID || claims.PrincipalID != ac.PrincipalID || claims.PrincipalKind != string(toolPrincipal(ac).Kind) || claims.ClientSurface != ac.ClientSurface || claims.Audience != ac.Audience || claims.SessionID != ac.SessionID || claims.AuthVersion != ac.AuthVersion || claims.Realm != string(auth.NormalizeRealm(ac.Realm)) {
		return zero, nil, errs.New(errs.CodeForbidden, "tool handle actor mismatch")
	}
	scope, err := s.scope(ctx, ac, claims.ScopeID)
	if err != nil {
		return zero, nil, err
	}
	if !contains(decodeStrings(scope.LoadedToolsJSON), claims.ToolID) {
		return zero, nil, errs.New(errs.CodeForbidden, "tool is not loaded")
	}
	d, ok := s.registry.Get(claims.ToolID)
	if !ok || d.SchemaHash != claims.SchemaHash || d.RuntimeRef != claims.RuntimeRef || !toolVisible(ac, d) {
		return zero, nil, errs.New(errs.CodeConflict, "tool contract changed")
	}
	d, ok, err = s.project(ctx, scope, d)
	if err != nil {
		return zero, nil, err
	}
	if !ok {
		return zero, nil, errs.New(errs.CodeForbidden, "tool is no longer available")
	}
	return aitool.LoadedTool{Definition: d, Handle: token, ExpiresAt: claims.ExpiresAt}, scope, nil
}
func (s *ToolService) project(ctx context.Context, scope *ToolScope, d aitool.Definition) (aitool.Definition, bool, error) {
	runtimeScope := aitool.RuntimeScope{ID: scope.ID, TenantID: scope.TenantID, PrincipalID: scope.PrincipalID, PrincipalKind: scope.PrincipalKind, ClientSurface: scope.ClientSurface, Audience: scope.Audience, Realm: scope.Realm, SessionID: scope.SessionID, AuthVersion: scope.AuthVersion}
	if scope.AgentRunID != nil {
		runtimeScope.AgentRunID = *scope.AgentRunID
	}
	return s.registry.Project(ctx, runtimeScope, d)
}
func (s *ToolService) scope(ctx context.Context, ac *auth.Context, id string) (*ToolScope, error) {
	now := time.Now().UTC()
	if id == "" {
		v := &ToolScope{ID: uuid.NewString(), TenantID: ac.TenantID, PrincipalID: ac.PrincipalID, PrincipalKind: string(toolPrincipal(ac).Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience, Realm: string(auth.NormalizeRealm(ac.Realm)), SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, AllowedSourcesJSON: jsonBytes([]string{}), AllowedCapabilitiesJSON: jsonBytes([]string{}), AllowedToolsJSON: jsonBytes([]string{}), LoadedToolsJSON: jsonBytes([]string{}), ReferenceEvidenceJSON: jsonBytes([]referenceEvidence{}), ExpiresAt: now.Add(toolScopeTTL)}
		if err := s.db.WithContext(ctx).Create(v).Error; err != nil {
			return nil, err
		}
		return v, nil
	}
	var v ToolScope
	if err := s.db.WithContext(ctx).Where("id=? AND principal_id=?", id, ac.PrincipalID).First(&v).Error; err != nil {
		return nil, err
	}
	if v.PrincipalKind != string(toolPrincipal(ac).Kind) || v.ClientSurface != ac.ClientSurface || v.Audience != ac.Audience || v.SessionID != ac.SessionID || v.AuthVersion != ac.AuthVersion || v.Realm != string(auth.NormalizeRealm(ac.Realm)) || !v.ExpiresAt.After(now) {
		return nil, errs.New(errs.CodeForbidden, "tool scope expired or actor changed")
	}
	return &v, nil
}
func (s *ToolService) mergeFTS(ctx context.Context, ac *auth.Context, scope *ToolScope, r ToolSearchRequest, current []aitool.Candidate) []aitool.Candidate {
	if strings.TrimSpace(r.Query) == "" {
		return current
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return current
	}
	lexicalQuery := strings.Join(aitool.LexicalTerms(r.Query), " ")
	if lexicalQuery == "" {
		return current
	}
	rows, err := tenantdata.TenantRaw[struct {
		ToolID string `gorm:"column:tool_id"`
	}](tdb, "SELECT tool_id FROM ai_tool_search_documents WHERE tenant_id = ? AND index_status = 'ready' AND to_tsvector('simple', search_text) @@ plainto_tsquery('simple', ?) ORDER BY ts_rank(to_tsvector('simple', search_text), plainto_tsquery('simple', ?)) DESC LIMIT 10", lexicalQuery, lexicalQuery)
	if err != nil {
		return current
	}
	seen := map[string]bool{}
	for _, item := range current {
		seen[item.ID] = true
	}
	for _, row := range rows {
		id := row.ToolID
		if seen[id] {
			continue
		}
		d, ok := s.registry.Get(id)
		if !ok || !scopeAllows(scope, d) || !toolVisible(ac, d) {
			continue
		}
		extra, _ := aitool.Search(ctx, []aitool.Definition{d}, "", r.Capability, r.Source, 1, func(aitool.Definition) bool { return true })
		if len(extra) > 0 {
			extra[0].Score = .5
			current = append(current, extra[0])
			seen[id] = true
		}
	}
	limit := r.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	if len(current) > limit {
		current = current[:limit]
	}
	return current
}

func validateToolArguments(schema, arguments json.RawMessage) error {
	if err := aitool.ValidateArguments(schema, arguments); err != nil {
		return errs.Validation("arguments do not match tool schema")
	}
	return nil
}

func toolActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" || ac.SessionID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "authenticated tenant actor is required")
	}
	return ac, nil
}
func toolPrincipal(ac *auth.Context) auth.PrincipalRef {
	if ac.Principal.Valid() {
		return ac.Principal
	}
	return auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: ac.PrincipalID}
}
func toolVisible(ac *auth.Context, d aitool.Definition) bool {
	for _, p := range d.PermissionRefs {
		if !ac.HasPermission(p) {
			return false
		}
	}
	return true
}
func scopeAllows(s *ToolScope, d aitool.Definition) bool {
	sources := decodeStrings(s.AllowedSourcesJSON)
	capabilities := decodeStrings(s.AllowedCapabilitiesJSON)
	tools := decodeStrings(s.AllowedToolsJSON)
	if len(sources) == 0 && len(capabilities) == 0 && len(tools) == 0 {
		return true
	}
	// Actions are progressively authorized only by the Contract's source and
	// capability scope. A stale AgentToolBinding must never make one exact.
	if contains(tools, d.ID) && d.Source != aitool.SourceAction {
		return true
	}
	return len(sources) > 0 && contains(sources, string(d.Source)) && (len(capabilities) == 0 || contains(capabilities, d.Capability))
}
func contains(v []string, x string) bool {
	for _, item := range v {
		if item == x {
			return true
		}
	}
	return false
}
func decodeStrings(v json.RawMessage) []string {
	var out []string
	_ = json.Unmarshal(v, &out)
	return out
}
func jsonBytes(v any) json.RawMessage   { b, _ := json.Marshal(v); return b }
func hashJSON(v json.RawMessage) string { sum := sha256.Sum256(v); return hex.EncodeToString(sum[:]) }
func redactArguments(v json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(v, &obj) != nil {
		return jsonBytes(map[string]any{})
	}
	out := map[string]any{}
	for k, val := range obj {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "key") {
			out[k] = "[REDACTED]"
		} else {
			switch x := val.(type) {
			case string:
				if len(x) > 80 {
					out[k] = x[:80] + "..."
				} else {
					out[k] = x
				}
			case float64, bool, nil:
				out[k] = x
			default:
				out[k] = "[complex value]"
			}
		}
	}
	return jsonBytes(out)
}
func modelToolName(id string) string {
	r := strings.NewReplacer(":", "__", ".", "_", "-", "_")
	return r.Replace(id)
}
