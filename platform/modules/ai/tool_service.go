package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

var toolCapabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

const toolCatalogChangedEvent = "ai.tool_catalog.changed.v1"
const toolInvocationFinishedEvent = "ai.tool_invocation.finished.v1"
const toolScopeTTL = 30 * time.Minute
const toolHandleTTL = 10 * time.Minute

type ToolService struct {
	db                    tenantdata.GuardedDatabase
	registry              *aitool.Registry
	signer                *aitool.HandleSigner
	writer                outbox.Writer
	embeddings            aimodelruntime.EmbeddingRuntime
	settings              *setting.Provider
	catalogProjectionSync bool
	onConfirmationPending func(context.Context, *gorm.DB, *ToolConfirmation) error
	onConfirmationDecided func(context.Context, *ToolConfirmation) error
}

func NewToolService(db *gorm.DB, r *aitool.Registry, signer *aitool.HandleSigner, w outbox.Writer) *ToolService {
	if err := registerToolTenantModels(); err != nil {
		panic(err)
	}
	return &ToolService{db: tenantdata.GuardedDB(db), registry: r, signer: signer, writer: w}
}
func (s *ToolService) SetEmbeddingRuntime(v aimodelruntime.EmbeddingRuntime) { s.embeddings = v }
func (s *ToolService) SetSettingProvider(v *setting.Provider)                { s.settings = v }
func (s *ToolService) SetCatalogProjectionSynchronous(enabled bool) {
	s.catalogProjectionSync = enabled
}
func (s *ToolService) SetConfirmationHooks(pending func(context.Context, *gorm.DB, *ToolConfirmation) error, decided func(context.Context, *ToolConfirmation) error) {
	s.onConfirmationPending, s.onConfirmationDecided = pending, decided
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
	ScopeID string              `json:"scope_id"`
	Tools   []aitool.LoadedTool `json:"tools"`
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
	loaded := decodeStrings(scope.LoadedToolsJSON)
	set := map[string]bool{}
	for _, id := range loaded {
		set[id] = true
	}
	now := time.Now().UTC()
	out := []aitool.LoadedTool{}
	bytes := scope.SchemaBytes
	for _, id := range r.ToolIDs {
		d, ok := s.registry.Get(id)
		if !ok || d.Status != "enabled" || !scopeAllows(scope, d) || !toolVisible(ac, d) {
			return nil, errs.New(errs.CodeForbidden, "tool is not visible")
		}
		d, ok, err = s.project(ctx, scope, d)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errs.New(errs.CodeForbidden, "tool is not visible")
		}
		if !set[id] {
			if len(loaded) >= 8 {
				return nil, errs.Validation("tool scope limit exceeded")
			}
			bytes += len(d.InputSchema) + len(d.OutputSchema)
			if bytes > 32<<10 {
				return nil, errs.Validation("tool schema budget exceeded")
			}
			loaded = append(loaded, id)
			set[id] = true
		}
		exp := now.Add(toolHandleTTL)
		if exp.After(scope.ExpiresAt) {
			exp = scope.ExpiresAt
		}
		token, err := s.signer.Sign(aitool.HandleClaims{TenantID: ac.TenantID, UserID: ac.UserID, Realm: string(auth.NormalizeRealm(ac.Realm)), SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, ScopeID: scope.ID, ToolID: d.ID, RuntimeRef: d.RuntimeRef, SchemaHash: d.SchemaHash, ExpiresAt: exp.Unix()})
		if err != nil {
			return nil, err
		}
		out = append(out, aitool.LoadedTool{Definition: d, Handle: token, ExpiresAt: exp.Unix()})
	}
	if err = s.db.WithContext(ctx).Model(&ToolScope{}).Where("id=? AND user_id=?", scope.ID, ac.UserID).Updates(map[string]any{"loaded_tools_json": jsonBytes(loaded), "schema_bytes": bytes}).Error; err != nil {
		return nil, err
	}
	return ToolLoadResponse{ScopeID: scope.ID, Tools: out}, nil
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
	if err = s.db.WithContext(ctx).Where("id=? AND user_id=?", r.ID, ac.UserID).First(&c).Error; err != nil {
		return nil, err
	}
	if (c.Status != "pending" && c.Status != "approved") || time.Now().UTC().After(c.ExpiresAt) {
		return nil, errs.New(errs.CodeConflict, "confirmation is not pending")
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
	if c.Status == "pending" {
		now := time.Now().UTC()
		claim := s.db.WithContext(ctx).Model(&ToolConfirmation{}).Where("id=? AND user_id=? AND status='pending'", c.ID, ac.UserID).Updates(map[string]any{"status": "approved", "decided_at": now})
		if claim.Error != nil {
			return nil, claim.Error
		}
		if claim.RowsAffected == 0 {
			return nil, errs.New(errs.CodeConflict, "confirmation is not pending")
		}
		c.Status = "approved"
		c.DecidedAt = &now
	}
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
	if err := s.db.WithContext(ctx).Where("id=? AND user_id=? AND status='pending'", r.ID, ac.UserID).First(&c).Error; err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Model(&ToolConfirmation{}).Where("id=? AND user_id=? AND status='pending'", r.ID, ac.UserID).Updates(map[string]any{"status": "rejected", "decided_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errs.New(errs.CodeConflict, "confirmation is not pending")
	}
	var inv ToolInvocation
	if err := s.db.WithContext(ctx).Where("id=?", c.InvocationID).First(&inv).Error; err == nil {
		_ = s.finishInvocation(ctx, &inv, nil, "failed", "confirmation_rejected", nil)
	}
	c.Status = "rejected"
	c.DecidedAt = &now
	if s.onConfirmationDecided != nil {
		if err := s.onConfirmationDecided(ctx, &c); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": r.ID, "status": "rejected"}, nil
}
func (s *ToolService) ListConfirmations(ctx context.Context, r ToolConfirmationListRequest) (any, error) {
	ac, err := toolActor(ctx)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Where("user_id=? AND status='pending' AND expires_at>?", ac.UserID, time.Now().UTC())
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
	needs := d.RequiresConfirmation || d.Risk == aitool.RiskHigh || d.SideEffect == aitool.SideEffectWrite || d.SideEffect == aitool.SideEffectExternal || d.SideEffect == aitool.SideEffectPaid || d.SideEffect == aitool.SideEffectDestructive
	if d.Risk == aitool.RiskCritical {
		return nil, errs.New(errs.CodeForbidden, "critical tool execution is disabled")
	}
	now := time.Now().UTC()
	var inv ToolInvocation
	if approved == nil {
		inv = ToolInvocation{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, ScopeID: scope.ID, AgentRunID: scope.AgentRunID, ToolCallID: callID, ToolID: d.ID, Source: string(d.Source), TargetAction: d.RuntimeRef, Risk: string(d.Risk), SideEffect: string(d.SideEffect), Status: "running", StartedAt: now, RequestID: ac.RequestID, TraceID: ac.TraceID}
	}
	if approved == nil && needs {
		hash := hashJSON(args)
		summary := redactArguments(args)
		c := ToolConfirmation{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, ScopeID: scope.ID, InvocationID: inv.ID, ToolCallID: callID, ToolID: d.ID, SchemaHash: d.SchemaHash, ArgumentsHash: hash, ArgumentsJSON: append(json.RawMessage(nil), args...), ArgumentsSummaryJSON: summary, Risk: string(d.Risk), SideEffect: string(d.SideEffect), Status: "pending", ExpiresAt: now.Add(10 * time.Minute)}
		inv.Status = "waiting_confirmation"
		inv.ConfirmationID = &c.ID
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		claim := s.db.WithContext(ctx).Model(&ToolInvocation{}).Where("id=? AND user_id=? AND status='waiting_confirmation'", approved.InvocationID, ac.UserID).Update("status", "running")
		if claim.Error != nil {
			return nil, claim.Error
		}
		if claim.RowsAffected != 1 {
			return nil, errs.New(errs.CodeConflict, "confirmation invocation is not waiting")
		}
		if err := s.db.WithContext(ctx).Where("id=? AND user_id=? AND status='running'", approved.InvocationID, ac.UserID).First(&inv).Error; err != nil {
			return nil, err
		}
	} else if err := s.db.WithContext(ctx).Create(&inv).Error; err != nil {
		return nil, err
	}
	if metadata, isSlot := s.registry.SlotMetadata(d.ID); isSlot && metadata.MaxCallsPerRun > 0 && scope.AgentRunID != nil {
		var calls int64
		if err := s.db.WithContext(ctx).Model(&ToolInvocation{}).Where("agent_run_id=? AND tool_id=?", *scope.AgentRunID, d.ID).Count(&calls).Error; err != nil {
			_ = s.finishInvocation(ctx, &inv, nil, "failed", "budget_check_failed", err)
			return nil, err
		}
		if calls > int64(metadata.MaxCallsPerRun) {
			result := &aitool.Result{Status: "failed", Summary: metadata.BudgetCategory + " budget exhausted", ErrorCategory: metadata.BudgetCategory + "_budget_exhausted"}
			if err := s.finishInvocation(ctx, &inv, result, "failed", result.ErrorCategory, nil); err != nil {
				return nil, err
			}
			return result, nil
		}
	}
	executor, ok := s.registry.ExecutorFor(d)
	if !ok {
		return nil, s.finishInvocation(ctx, &inv, nil, "failed", "execution_failed", fmt.Errorf("tool executor unavailable"))
	}
	loaded.Handle = ""
	executionCtx := ctx
	if current, ok := auth.FromContext(ctx); ok && current != nil {
		invocationContext := *current
		invocationContext.ToolInvocationID = inv.ID
		executionCtx = auth.WithContext(ctx, &invocationContext)
	}
	result, execErr := executor.Execute(aitool.WithScopeID(executionCtx, scope.ID), loaded, args)
	if execErr != nil {
		_ = s.finishInvocation(ctx, &inv, nil, "failed", "execution_failed", execErr)
		return nil, execErr
	}
	result.CallID = callID
	result.Data, result.Truncated = sanitizeToolData(result.Data, 0, 64<<10)
	invocationStatus, category := "completed", ""
	if result.Status == "failed" {
		invocationStatus, category = "failed", result.ErrorCategory
		if metadata, isSlot := s.registry.SlotMetadata(d.ID); isSlot && !slices.Contains(metadata.FailureCategories, category) {
			category = "execution_failed"
			result.ErrorCategory = category
		}
	}
	if err := s.finishInvocation(ctx, &inv, result, invocationStatus, category, nil); err != nil {
		return nil, err
	}
	if approved != nil {
		done := time.Now().UTC()
		_ = s.db.WithContext(ctx).Model(&ToolConfirmation{}).Where("id=? AND status='approved'", approved.ID).Updates(map[string]any{"status": "consumed", "consumed_at": done}).Error
	}
	return result, nil
}

func (s *ToolService) finishInvocation(ctx context.Context, inv *ToolInvocation, result *aitool.Result, status, category string, cause error) error {
	now := time.Now().UTC()
	duration := now.Sub(inv.StartedAt).Milliseconds()
	updates := map[string]any{"status": status, "finished_at": now, "duration_ms": duration, "error_category": category}
	if result != nil {
		updates["result_json"] = jsonBytes(aitool.Result{CallID: result.CallID, Status: result.Status, Summary: result.Summary, ResourceRefs: append([]string(nil), result.ResourceRefs...), Truncated: result.Truncated, ErrorCategory: result.ErrorCategory})
	}
	payload := map[string]any{"invocation_id": inv.ID, "tool_id": inv.ToolID, "source": inv.Source, "status": status, "duration_ms": duration, "error_category": category}
	if metadata, ok := s.registry.SlotMetadata(inv.ToolID); ok {
		payload["invocation_class"] = metadata.InvocationClass
		payload["budget_category"] = metadata.BudgetCategory
	}
	if s.writer == nil {
		return s.db.WithContext(ctx).Model(&ToolInvocation{}).Where("id=? AND status IN ?", inv.ID, []string{"running", "waiting_confirmation"}).Updates(updates).Error
	}
	return outbox.WithinTx(ctx, s.db.WithContext(ctx), s.writer, func(tx *gorm.DB, w outbox.Writer) error {
		res := tx.Model(&ToolInvocation{}).Where("id=? AND status IN ?", inv.ID, []string{"running", "waiting_confirmation"}).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		_, err := w.Append(ctx, tx, outbox.AppendInput{EventType: toolInvocationFinishedEvent, AggregateType: "ai_tool_invocation", AggregateID: inv.ID, AggregateVersion: 1, Payload: payload})
		return err
	})
}

func (s *ToolService) resolveLoaded(ctx context.Context, ac *auth.Context, token string) (aitool.LoadedTool, *ToolScope, error) {
	var zero aitool.LoadedTool
	claims, err := s.signer.Verify(token, time.Now().UTC())
	if err != nil {
		return zero, nil, errs.New(errs.CodeForbidden, err.Error())
	}
	if claims.TenantID != ac.TenantID || claims.UserID != ac.UserID || claims.SessionID != ac.SessionID || claims.AuthVersion != ac.AuthVersion || claims.Realm != string(auth.NormalizeRealm(ac.Realm)) {
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
	runtimeScope := aitool.RuntimeScope{ID: scope.ID, TenantID: scope.TenantID, UserID: scope.UserID, Realm: scope.Realm, SessionID: scope.SessionID, AuthVersion: scope.AuthVersion}
	if scope.AgentRunID != nil {
		runtimeScope.AgentRunID = *scope.AgentRunID
	}
	return s.registry.Project(ctx, runtimeScope, d)
}
func (s *ToolService) scope(ctx context.Context, ac *auth.Context, id string) (*ToolScope, error) {
	now := time.Now().UTC()
	if id == "" {
		v := &ToolScope{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, Realm: string(auth.NormalizeRealm(ac.Realm)), SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, AllowedSourcesJSON: jsonBytes([]string{}), AllowedCapabilitiesJSON: jsonBytes([]string{}), AllowedToolsJSON: jsonBytes([]string{}), LoadedToolsJSON: jsonBytes([]string{}), ExpiresAt: now.Add(toolScopeTTL)}
		if err := s.db.WithContext(ctx).Create(v).Error; err != nil {
			return nil, err
		}
		return v, nil
	}
	var v ToolScope
	if err := s.db.WithContext(ctx).Where("id=? AND user_id=?", id, ac.UserID).First(&v).Error; err != nil {
		return nil, err
	}
	if v.SessionID != ac.SessionID || v.AuthVersion != ac.AuthVersion || v.Realm != string(auth.NormalizeRealm(ac.Realm)) || !v.ExpiresAt.After(now) {
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

func sanitizeToolData(value any, depth, budget int) (any, bool) {
	if depth > 8 {
		return "[truncated]", true
	}
	if encoded, err := json.Marshal(value); err == nil && len(encoded) > budget && depth > 0 {
		return "[truncated]", true
	}
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		cut := false
		for k, item := range v {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "authorization") || strings.Contains(lower, "credential") {
				out[k] = "[REDACTED]"
				continue
			}
			clean, itemCut := sanitizeToolData(item, depth+1, budget/2)
			out[k] = clean
			cut = cut || itemCut
		}
		return out, cut
	case []any:
		limit := len(v)
		cut := false
		if limit > 100 {
			limit = 100
			cut = true
		}
		out := make([]any, 0, limit)
		for _, item := range v[:limit] {
			clean, itemCut := sanitizeToolData(item, depth+1, budget/2)
			out = append(out, clean)
			cut = cut || itemCut
		}
		return out, cut
	case []byte:
		return "[binary omitted]", true
	default:
		return value, false
	}
}

func validateToolArguments(schema, arguments json.RawMessage) error {
	var spec struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Additional bool `json:"additionalProperties"`
	}
	if json.Unmarshal(schema, &spec) != nil {
		return errs.Validation("invalid tool schema")
	}
	var values map[string]any
	if json.Unmarshal(arguments, &values) != nil {
		return errs.Validation("arguments must be object")
	}
	for _, key := range spec.Required {
		if _, ok := values[key]; !ok {
			return errs.Validation("required argument missing")
		}
	}
	for key, value := range values {
		field, ok := spec.Properties[key]
		if !ok && !spec.Additional {
			return errs.Validation("unknown argument")
		}
		if !ok {
			continue
		}
		valid := field.Type == "" || (field.Type == "string" && isString(value)) || (field.Type == "number" && isNumber(value)) || (field.Type == "integer" && isInteger(value)) || (field.Type == "boolean" && isBool(value)) || (field.Type == "object" && isObject(value)) || (field.Type == "array" && isArray(value))
		if !valid {
			return errs.Validation("argument type mismatch")
		}
	}
	return nil
}
func isString(v any) bool  { _, ok := v.(string); return ok }
func isNumber(v any) bool  { _, ok := v.(float64); return ok }
func isInteger(v any) bool { n, ok := v.(float64); return ok && n == float64(int64(n)) }
func isBool(v any) bool    { _, ok := v.(bool); return ok }
func isObject(v any) bool  { _, ok := v.(map[string]any); return ok }
func isArray(v any) bool   { _, ok := v.([]any); return ok }

func toolActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" || ac.SessionID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "authenticated tenant actor is required")
	}
	return ac, nil
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
