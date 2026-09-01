package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	agentpkg "github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/modelruntime"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	agentRunRequestedEvent           = "ai.agent_run.requested.v1"
	agentRunResumeEvent              = "ai.agent_run.resume_requested.v1"
	agentRunTerminalEvent            = AgentRunTerminalEvent
	agentRunConfirmationTimeoutEvent = "ai.agent_run.confirmation_timeout.v1"
	confirmationResumeStepAllowance  = 4
	conversationToolRestoreLimit     = 4
	toolRehydrateLimitPerRun         = 3
)

type AgentRuntimeService struct {
	db              tenantdata.GuardedDatabase
	conversations   *ConversationService
	invocations     *modelruntime.InvocationService
	tools           *tooling.ToolService
	compactor       aiconversation.ContextCompactor
	writer          outbox.Writer
	authorization   auth.AuthorizationProvider
	runtimeContext  runtimecontext.Provider
	pageContexts    *pagecontext.Registry
	memoryRetriever MemoryRecallFunc
	contracts       *aiagent.Registry
	budgetProvider  *modelruntime.RuntimeBudgetProvider
	contextBudget   interface {
		ContextTokenLimit(context.Context, string) (int, error)
		ModelTurnBudget(context.Context, string) (ModelContextBudget, error)
		EstimateRequestTokens(context.Context, aimodelruntime.GenerateRequest) (int, error)
	}
	leaseDuration time.Duration
	now           func() time.Time
}

func (s *AgentRuntimeService) SetMemoryRetriever(retriever MemoryRecallFunc) {
	s.memoryRetriever = retriever
}

func (s *AgentRuntimeService) SetAgentContracts(contracts *aiagent.Registry) {
	s.contracts = contracts
}

func NewAgentRuntimeService(db *gorm.DB, c *ConversationService, i *modelruntime.InvocationService, t *tooling.ToolService, compactor aiconversation.ContextCompactor, w outbox.Writer, authorization auth.AuthorizationProvider, pageContexts *pagecontext.Registry, runtimeContext ...runtimecontext.Provider) *AgentRuntimeService {
	if err := registerAgentRuntimeTenantModels(); err != nil {
		panic(err)
	}
	var provider runtimecontext.Provider
	if len(runtimeContext) > 0 {
		provider = runtimeContext[0]
	}
	service := &AgentRuntimeService{db: tenantdata.GuardedDB(db), conversations: c, invocations: i, tools: t, compactor: compactor, writer: w, authorization: authorization, runtimeContext: provider, pageContexts: pageContexts, leaseDuration: 30 * time.Second, now: func() time.Time { return time.Now().UTC() }}
	service.contextBudget, _ = compactor.(interface {
		ContextTokenLimit(context.Context, string) (int, error)
		ModelTurnBudget(context.Context, string) (ModelContextBudget, error)
		EstimateRequestTokens(context.Context, aimodelruntime.GenerateRequest) (int, error)
	})
	return service
}

func (s *AgentRuntimeService) SetBudgetProvider(provider *modelruntime.RuntimeBudgetProvider) {
	s.budgetProvider = provider
}

type AgentRunStartRequest struct {
	ConversationID   string                `json:"conversation_id" validate:"required"`
	TriggerMessageID string                `json:"trigger_message_id" validate:"required"`
	PageContext      *pagecontext.Envelope `json:"page_context,omitempty"`
}

func (r AgentRunStartRequest) Validate(context.Context) error {
	if r.PageContext != nil {
		return r.PageContext.Validate()
	}
	return nil
}

type AgentRunIDRequest struct {
	ID       string `json:"id" validate:"required"`
	Revision uint64 `json:"revision,omitempty"`
}
type AgentRunListRequest struct {
	pagination.Request
	Status         string `json:"status"`
	ConversationID string `json:"conversation_id"`
}
type AgentRunEventsRequest struct {
	ID            string `json:"id" validate:"required"`
	AfterSequence uint64 `json:"after_sequence"`
	Limit         int    `json:"limit" validate:"omitempty,min=1,max=500"`
}

func runActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}
func (s *AgentRuntimeService) get(ctx context.Context, id string) (*AgentRun, *auth.Context, error) {
	ac, err := runActor(ctx)
	if err != nil {
		return nil, nil, err
	}
	var row AgentRun
	if err = s.db.WithContext(ctx).Where("user_id=? AND id=?", ac.PrincipalID, id).First(&row).Error; err != nil {
		return nil, nil, errs.New(errs.CodeNotFound, "agent run not found")
	}
	return &row, ac, nil
}

func (s *AgentRuntimeService) Start(ctx context.Context, r AgentRunStartRequest) (any, error) {
	ac, err := runActor(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(ac.IdempotencyKey)
	if key == "" {
		return nil, errs.New(errs.CodeIdempotencyKeyRequired, "idempotency key required")
	}
	var existingRun AgentRun
	if existingErr := s.db.WithContext(ctx).Where("user_id=? AND idempotency_key=?", ac.PrincipalID, key).First(&existingRun).Error; existingErr == nil {
		return existingRun, nil
	} else if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return nil, existingErr
	}
	conversation, _, err := s.conversations.get(ctx, r.ConversationID, true)
	if err != nil {
		return nil, err
	}
	if conversation.Status != "active" {
		return nil, errs.New(errs.CodeConflict, "conversation is not active")
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return nil, err
	}
	triggers, err := tenantdata.TenantRaw[ConversationMessage](tdb, `SELECT * FROM ai_messages
WHERE tenant_id=? AND conversation_id=? AND id=? AND role=? AND status=? LIMIT 1`, conversation.ID, r.TriggerMessageID, "user", "committed")
	if err != nil || len(triggers) != 1 {
		return nil, errs.New(errs.CodeNotFound, "trigger message not found")
	}
	trigger := triggers[0]
	budget, budgetSource, err := s.budgetProvider.Resolve(ctx, ac.TenantID)
	if err != nil {
		return nil, errs.New(errs.CodeInternal, "agent runtime budget unavailable")
	}
	budgetJSON, _ := json.Marshal(budget)
	logging.Info(ctx, "agent runtime budget resolved", "source", budgetSource, "max_steps", budget.MaxSteps, "max_model_calls", budget.MaxModelCalls, "max_tool_calls", budget.MaxToolCalls, "max_compactions", budget.MaxCompactions, "max_duration_seconds", budget.MaxDurationSeconds, "run_token_budget", budget.RunTokenBudget)
	pageContextJSON := json.RawMessage(`{}`)
	pageContextType, pageContextStatus, pageContextBytes := "", "none", 0
	if r.PageContext != nil {
		if s.pageContexts == nil {
			return nil, errs.New(errs.CodeValidation, "page context is unavailable")
		}
		resolved, resolveErr := s.pageContexts.Resolve(ctx, *r.PageContext)
		if resolveErr != nil {
			return nil, errs.New(errs.CodeValidation, "page context is unavailable")
		}
		pageContextJSON, _ = json.Marshal(resolved)
		pageContextType, pageContextStatus, pageContextBytes = resolved.PageType, "resolved", len(pageContextJSON)
		logging.Info(ctx, "page context resolved", "page_type", resolved.PageType, "mode", resolved.Mode, "snapshot_bytes", pageContextBytes, "trusted_ref_count", len(resolved.TrustedRefs))
	}
	traceID := strings.TrimSpace(ac.TraceID)
	if traceID == "" {
		traceID = ac.RequestID
	}
	principal := ac.Principal
	if !principal.Valid() {
		principal = auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: ac.PrincipalID}
	}
	run := AgentRun{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.PrincipalID, PrincipalKind: string(principal.Kind), ClientSurface: ac.ClientSurface, Audience: ac.Audience, ConversationID: conversation.ID, ConversationRevision: conversation.Revision, TriggerMessageID: trigger.ID, AgentVersionID: conversation.AgentVersionID, SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, OriginRequestID: ac.RequestID, TraceID: traceID, PageContextJSON: pageContextJSON, PageContextType: pageContextType, PageContextStatus: pageContextStatus, PageContextBytes: pageContextBytes, Status: string(aiagentruntime.RunQueued), ExecutionMode: string(aiagentruntime.ExecutionModeNormal), BudgetJSON: budgetJSON, IdempotencyKey: key, Revision: 1}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("user_id=? AND idempotency_key=?", ac.PrincipalID, key).First(&run).Error; e == nil {
			return nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e := tx.Create(&run).Error; e != nil {
			return e
		}
		if e := s.appendEvent(ctx, tx, &run, "run.queued", nil, map[string]any{"conversation_revision": conversation.Revision, "page_context_status": pageContextStatus, "page_type": pageContextType, "page_context_bytes": pageContextBytes}); e != nil {
			return e
		}
		if s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunRequestedEvent, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: 1, Payload: map[string]any{"run_id": run.ID}})
			return e
		}
		return nil
	})
	return run, err
}

func (s *AgentRuntimeService) Get(ctx context.Context, r AgentRunIDRequest) (any, error) {
	run, _, err := s.get(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	var steps []AgentStep
	if err = s.db.WithContext(ctx).Where("run_id=?", run.ID).Order("step_index ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	return map[string]any{"run": run, "steps": steps, "page_context": safePageContextProjection(run.PageContextJSON)}, nil
}

func safePageContextProjection(raw json.RawMessage) any {
	snapshot, err := pageContextSnapshot(raw)
	if err != nil || snapshot == nil {
		return nil
	}
	return map[string]any{"schema_version": snapshot.SchemaVersion, "page_type": snapshot.PageType, "route_name": snapshot.RouteName, "mode": snapshot.Mode, "trusted_refs": snapshot.TrustedRefs, "warnings": snapshot.Warnings}
}
func (s *AgentRuntimeService) List(ctx context.Context, r AgentRunListRequest) (any, error) {
	ac, err := runActor(ctx)
	if err != nil {
		return nil, err
	}
	r.Normalize()
	q := s.db.WithContext(ctx).Where("user_id=?", ac.PrincipalID)
	if r.Status != "" {
		q = q.Where("status=?", r.Status)
	}
	if r.ConversationID != "" {
		q = q.Where("conversation_id=?", r.ConversationID)
	}
	var total int64
	if err = q.Model(&AgentRun{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []AgentRun
	err = q.Order("created_at DESC").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error
	return pagination.NewResult(rows, total, r.Request), err
}
func (s *AgentRuntimeService) Events(ctx context.Context, r AgentRunEventsRequest) (any, error) {
	run, _, err := s.get(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	if r.Limit == 0 {
		r.Limit = 200
	}
	var rows []AgentRunEvent
	err = s.db.WithContext(ctx).Where("run_id=? AND sequence>?", run.ID, r.AfterSequence).Order("sequence ASC").Limit(r.Limit + 1).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	more := len(rows) > r.Limit
	if more {
		rows = rows[:r.Limit]
	}
	next := r.AfterSequence
	if len(rows) > 0 {
		next = rows[len(rows)-1].Sequence
	}
	return map[string]any{"items": rows, "next_sequence": next, "has_more": more}, nil
}

func (s *AgentRuntimeService) Subscribe(ctx context.Context, r AgentRunEventsRequest) (any, error) {
	run, _, err := s.get(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	events := make(chan action.StreamEvent, 128)
	streamCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(events)
		cursor := r.AfterSequence
		poll := time.NewTicker(50 * time.Millisecond)
		defer poll.Stop()
		for {
			var rows []AgentRunEvent
			if e := s.db.WithContext(streamCtx).Where("run_id=? AND sequence>?", run.ID, cursor).Order("sequence ASC").Limit(200).Find(&rows).Error; e != nil {
				return
			}
			terminal := false
			for _, event := range rows {
				cursor = event.Sequence
				select {
				case events <- action.StreamEvent{Event: event.Type, Data: event}:
				case <-streamCtx.Done():
					return
				}
				if event.Type == "run.completed" || event.Type == "run.failed" || event.Type == "run.cancelled" || event.Type == "run.waiting_confirmation" || event.Type == "run.paused" {
					terminal = true
				}
			}
			if terminal {
				return
			}
			select {
			case <-streamCtx.Done():
				return
			case <-poll.C:
			}
		}
	}()
	return action.StreamResult{Events: events, Cancel: cancel}, nil
}

func (s *AgentRuntimeService) Cancel(ctx context.Context, r AgentRunIDRequest) (any, error) {
	run, _, err := s.get(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	to := aiagentruntime.RunCancelled
	if err = aiagentruntime.ValidateRunTransition(aiagentruntime.RunStatus(run.Status), to); err != nil {
		return nil, errs.New(errs.CodeConflict, err.Error())
	}
	now := s.now()
	category := string(aimodelruntime.ErrorCancelled)
	decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{Status: to, ErrorCategory: category})
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRun{}).Where("id=? AND revision=? AND status=?", run.ID, r.Revision, run.Status).Updates(map[string]any{
			"status": string(to), "error_category": category, "finished_at": now,
			"terminal_protocol_version": decision.ProtocolVersion, "terminal_reason": decision.TerminalReason,
			"finalization_mode": decision.Mode, "retryable": decision.Retryable,
			"user_message_key": decision.UserMessageKey, "safe_summary": decision.SafeSummary,
			"lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run revision conflict")
		}
		var activeSteps []AgentStep
		if e := tx.Where("run_id=? AND status IN ?", run.ID, []string{"running", "waiting"}).Order("step_index ASC").Find(&activeSteps).Error; e != nil {
			return e
		}
		for index := range activeSteps {
			step := &activeSteps[index]
			updated := tx.Model(&AgentStep{}).Where("id=? AND status IN ?", step.ID, []string{"running", "waiting"}).Updates(map[string]any{"status": "cancelled", "error_category": category, "finished_at": now})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 1 {
				if e := s.appendEvent(ctx, tx, run, "step.cancelled", &step.ID, map[string]any{"type": step.Type, "status": "cancelled", "error_category": category}); e != nil {
					return e
				}
			}
		}
		operations, e := loadTerminalOperationSummary(tx, run.ID)
		if e != nil {
			return e
		}
		safeArgs := terminalSafeArgs(decision, operations)
		if e = tx.Model(&AgentRun{}).Where("id=?", run.ID).Updates(map[string]any{
			"completed_operation_count":  operations.Completed,
			"failed_operation_count":     operations.Failed,
			"unfinished_operation_count": operations.Unfinished,
			"safe_args":                  safeArgs,
		}).Error; e != nil {
			return e
		}
		payload := map[string]any{
			"error_category": category, "terminal_protocol_version": decision.ProtocolVersion,
			"terminal_reason": decision.TerminalReason, "finalization_mode": decision.Mode,
			"finalization_attempted": false, "retryable": decision.Retryable,
			"user_message_key": decision.UserMessageKey, "safe_args": safeArgs, "safe_summary": decision.SafeSummary,
			"completed_operation_count": operations.Completed, "failed_operation_count": operations.Failed,
			"unfinished_operation_count": operations.Unfinished,
		}
		if e := s.appendEvent(ctx, tx, run, "run.cancelled", nil, payload); e != nil {
			return e
		}
		if s.writer != nil {
			payload["run_id"], payload["status"] = run.ID, "cancelled"
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunTerminalEvent, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: int64(run.Revision + 1), Payload: payload})
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, AgentRunIDRequest{ID: run.ID})
}

func (s *AgentRuntimeService) Resume(ctx context.Context, r AgentRunIDRequest) (any, error) {
	return s.requeue(ctx, r, false)
}
func (s *AgentRuntimeService) Retry(ctx context.Context, r AgentRunIDRequest) (any, error) {
	return s.requeue(ctx, r, true)
}
func (s *AgentRuntimeService) requeue(ctx context.Context, r AgentRunIDRequest, retry bool) (any, error) {
	run, ac, err := s.get(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	if ac.AuthVersion != run.AuthVersion || ac.SessionID != run.SessionID {
		return nil, errs.New(errs.CodeForbidden, "run authorization changed")
	}
	from := aiagentruntime.RunStatus(run.Status)
	if retry && from != aiagentruntime.RunFailed {
		return nil, errs.New(errs.CodeConflict, "only failed runs can retry")
	}
	if !retry && from != aiagentruntime.RunWaitingConfirmation && from != aiagentruntime.RunPaused {
		return nil, errs.New(errs.CodeConflict, "run is not resumable")
	}
	if err = aiagentruntime.ValidateRunTransition(from, aiagentruntime.RunQueued); err != nil {
		return nil, err
	}
	return s.setQueued(ctx, run, r.Revision, agentRunResumeEvent)
}

func (s *AgentRuntimeService) setQueued(ctx context.Context, run *AgentRun, revision uint64, eventType string) (any, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRun{}).Where("id=? AND revision=? AND status=?", run.ID, revision, run.Status).Updates(map[string]any{"status": "queued", "error_category": "", "lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run revision conflict")
		}
		if e := s.appendEvent(ctx, tx, run, "run.queued", nil, map[string]any{"resume": true}); e != nil {
			return e
		}
		if s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: eventType, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: int64(run.Revision + 1), Payload: map[string]any{"run_id": run.ID}})
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, AgentRunIDRequest{ID: run.ID})
}

func (s *AgentRuntimeService) OnConfirmationPending(ctx context.Context, tx *gorm.DB, c *tooling.ToolConfirmation) error {
	if s.writer == nil {
		return nil
	}
	var scope tooling.ToolScope
	if err := tx.WithContext(ctx).Where("id=?", c.ScopeID).First(&scope).Error; err != nil || scope.AgentRunID == nil {
		return err
	}
	_, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunConfirmationTimeoutEvent, AggregateType: "ai_agent_run", AggregateID: *scope.AgentRunID, AggregateVersion: 1, Payload: map[string]any{"run_id": *scope.AgentRunID, "confirmation_id": c.ID, "expires_at": c.ExpiresAt}})
	return err
}
func (s *AgentRuntimeService) OnConfirmationDecided(ctx context.Context, c *tooling.ToolConfirmation) error {
	var scope tooling.ToolScope
	if err := s.db.WithContext(ctx).Where("id=?", c.ScopeID).First(&scope).Error; err != nil || scope.AgentRunID == nil {
		return err
	}
	var run AgentRun
	if err := s.db.WithContext(ctx).Where("id=? AND status=?", *scope.AgentRunID, "waiting_confirmation").First(&run).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	_, err := s.setQueued(ctx, &run, run.Revision, agentRunResumeEvent)
	return err
}
func (s *AgentRuntimeService) OnConfirmationApprove(ctx context.Context, c *tooling.ToolConfirmation) error {
	var scope tooling.ToolScope
	if err := s.db.WithContext(ctx).Where("id=?", c.ScopeID).First(&scope).Error; err != nil || scope.AgentRunID == nil {
		return err
	}
	var run AgentRun
	if err := s.db.WithContext(ctx).Where("id=? AND status=?", *scope.AgentRunID, "waiting_confirmation").First(&run).Error; err != nil {
		return errs.New(errs.CodeConflict, "agent run is not waiting for confirmation")
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return err
	}
	rows, err := tenantdata.TenantRaw[struct {
		Revision uint64
		Status   string
	}](tdb, `SELECT revision, status FROM ai_conversations WHERE tenant_id=? AND id=? LIMIT 1`, run.ConversationID)
	if err != nil || len(rows) != 1 || rows[0].Status != "active" || rows[0].Revision != run.ConversationRevision {
		return errs.New(errs.CodeRevisionConflict, "conversation changed while waiting for confirmation")
	}
	return nil
}
func (s *AgentRuntimeService) WakeExpiredConfirmation(ctx context.Context, runID string) (bool, error) {
	var run AgentRun
	if err := s.db.WithContext(ctx).Where("id=? AND status=?", runID, "waiting_confirmation").First(&run).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var step AgentStep
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=? AND confirmation_id IS NOT NULL", run.ID, "waiting").Order("step_index DESC").First(&step).Error; err != nil {
		return false, err
	}
	var c tooling.ToolConfirmation
	if err := s.db.WithContext(ctx).Where("id=?", *step.ConfirmationID).First(&c).Error; err != nil {
		return false, err
	}
	if c.Status == tooling.ToolConfirmationPending && c.ExpiresAt.After(s.now()) {
		return false, nil
	}
	if auth.MustFromContext(ctx).TenantID == "" {
		ctx = auth.WithContext(ctx, &auth.Context{TenantID: run.TenantID, PrincipalID: run.UserID, Principal: auth.PrincipalRef{Kind: auth.PrincipalKind(run.PrincipalKind), ID: run.UserID}, ClientSurface: run.ClientSurface, Audience: run.Audience, Realm: auth.RealmTenant, Authenticated: true})
	}
	_, err := s.setQueued(ctx, &run, run.Revision, agentRunResumeEvent)
	return err == nil, err
}

func (s *AgentRuntimeService) appendEvent(ctx context.Context, tx *gorm.DB, run *AgentRun, eventType string, stepID *string, payload any) error {
	var locked AgentRun
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id=?", run.ID).First(&locked).Error; err != nil {
		return err
	}
	var last uint64
	if err := tx.Model(&AgentRunEvent{}).Where("run_id=?", run.ID).Select("COALESCE(MAX(sequence),0)").Scan(&last).Error; err != nil {
		return err
	}
	body, _ := json.Marshal(payload)
	event := AgentRunEvent{ID: uuid.NewString(), TenantID: run.TenantID, RunID: run.ID, Sequence: last + 1, Type: eventType, StepID: stepID, Payload: body}
	if err := tx.Create(&event).Error; err != nil {
		return err
	}
	return nil
}

func (s *AgentRuntimeService) claim(ctx context.Context, runID, worker string) (*AgentRun, error) {
	var run AgentRun
	trustedTenant := auth.MustFromContext(ctx).TenantID
	if trustedTenant == "" {
		return nil, errs.New(errs.CodeUnauthorized, "tenant context required")
	}
	now := s.now()
	expires := now.Add(s.leaseDuration)
	noop := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", runID).First(&run).Error; e != nil {
			return e
		}
		if run.Status != "queued" && !(run.Status == "running" && (run.LeaseExpiresAt == nil || run.LeaseExpiresAt.Before(now))) {
			noop = true
			return nil
		}
		res := tx.Model(&AgentRun{}).Where("id=? AND revision=?", run.ID, run.Revision).Updates(map[string]any{"status": "running", "lease_owner": worker, "lease_expires_at": expires, "started_at": gorm.Expr("COALESCE(started_at, ?)", now), "revision": gorm.Expr("revision+1")})
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run lease conflict")
		}
		run.Status = "running"
		run.LeaseOwner = worker
		run.LeaseExpiresAt = &expires
		if run.StartedAt == nil {
			startedAt := now
			run.StartedAt = &startedAt
		}
		run.Revision++
		return s.appendEvent(ctx, tx, &run, "run.started", nil, map[string]any{"worker": worker})
	})
	if noop {
		return nil, nil
	}
	return &run, err
}

func (s *AgentRuntimeService) newStep(ctx context.Context, run *AgentRun, t aiagentruntime.StepType, input any) (*AgentStep, error) {
	if !aiagentruntime.ValidStepType(t) {
		return nil, fmt.Errorf("invalid step type")
	}
	body, _ := json.Marshal(input)
	now := s.now()
	step := AgentStep{ID: uuid.NewString(), TenantID: run.TenantID, RunID: run.ID, StepIndex: run.CurrentStep + 1, Type: string(t), Status: "running", InputSummary: body, OutputSummary: json.RawMessage(`{}`), CreatedAt: now, StartedAt: &now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&step).Error; e != nil {
			return e
		}
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", run.ID, "running", run.LeaseOwner).Updates(map[string]any{"current_step": step.StepIndex, "revision": gorm.Expr("revision+1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run lease changed")
		}
		run.CurrentStep = step.StepIndex
		return s.appendEvent(ctx, tx, run, "step.started", &step.ID, map[string]any{"type": step.Type, "index": step.StepIndex})
	})
	return &step, err
}
func (s *AgentRuntimeService) finishStep(ctx context.Context, run *AgentRun, step *AgentStep, status string, output any, category string) error {
	if err := aiagentruntime.ValidateStepTransition(aiagentruntime.StepStatus(step.Status), aiagentruntime.StepStatus(status)); err != nil {
		return errs.New(errs.CodeConflict, err.Error())
	}
	body, _ := json.Marshal(output)
	now := s.now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentStep{}).Where("id=? AND status IN ?", step.ID, []string{"running", "waiting"}).Updates(map[string]any{"status": status, "output_summary": body, "error_category": category, "finished_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent step status changed")
		}
		step.Status = status
		event := "step.completed"
		if status == "failed" {
			event = "step.failed"
		}
		if status == "waiting" {
			event = "step.waiting"
		}
		return s.appendEvent(ctx, tx, run, event, &step.ID, map[string]any{"type": step.Type, "status": status, "error_category": category})
	})
}

func safeSummary(values map[string]any) map[string]any { return values }

func (s *AgentRuntimeService) Execute(ctx context.Context, runID, worker string) error {
	run, err := s.claim(ctx, runID, worker)
	if err != nil {
		return err
	}
	if run == nil {
		return nil
	}
	execCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	go s.heartbeatLease(execCtx, cancelExecution, run.ID, worker)
	ctx = execCtx
	if s.authorization == nil {
		return s.fail(ctx, run, "authorization_unavailable")
	}
	principal := auth.PrincipalRef{Kind: auth.PrincipalKind(run.PrincipalKind), ID: run.UserID}
	if !principal.Valid() {
		return s.fail(ctx, run, "principal_context_invalid")
	}
	snapshot, authErr := s.authorization.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: run.TenantID, Principal: principal, PrincipalID: run.UserID, SessionID: run.SessionID, Realm: auth.RealmTenant, AuthVersion: run.AuthVersion, Audience: run.Audience, ClientSurface: run.ClientSurface})
	if authErr != nil || snapshot.AuthVersion != run.AuthVersion {
		return s.fail(ctx, run, "authorization_changed")
	}
	ctx = auth.WithContext(ctx, &auth.Context{RequestID: run.OriginRequestID, TraceID: run.TraceID, TenantID: snapshot.TenantID, Principal: principal, PrincipalID: snapshot.PrincipalID, Audience: snapshot.Audience, ClientSurface: snapshot.ClientSurface, SessionID: snapshot.SessionID, OrgID: snapshot.OrgID, DeptID: snapshot.DeptID, Roles: snapshot.Roles, Permissions: snapshot.Permissions, AuthVersion: snapshot.AuthVersion, Realm: auth.RealmTenant, Client: "agent", AgentRunID: run.ID, Authenticated: true})
	ctx = withDocumentResolveCache(ctx)
	pageSnapshot, pageErr := pageContextSnapshot(run.PageContextJSON)
	if pageErr != nil {
		return s.fail(ctx, run, "page_context_invalid")
	}
	if pageSnapshot != nil {
		ctx = pagecontext.WithSnapshot(ctx, pageSnapshot)
	}
	var budget aiagentruntime.Budget
	if json.Unmarshal(run.BudgetJSON, &budget) != nil || budget.Validate() != nil {
		return s.fail(ctx, run, "invalid_budget")
	}
	if run.ExecutionMode == "" {
		run.ExecutionMode = string(aiagentruntime.ExecutionModeNormal)
	}
	if err := s.syncModelUsage(ctx, run); err != nil {
		return err
	}
	if recovered, err := s.recoverRefusalFinalization(ctx, run); err != nil {
		return err
	} else if recovered {
		return nil
	}
	if recovered, err := s.recoverTerminalFinalization(ctx, run); err != nil {
		return err
	} else if recovered {
		return nil
	}
	if recovered, err := s.recoverFinalAnswer(ctx, run); err != nil {
		return err
	} else if recovered {
		completionReason := ""
		if run.ExecutionMode == string(aiagentruntime.ExecutionModeCompletionOnly) {
			completionReason = "budget_limit"
		}
		return s.completeWithReason(ctx, run, completionReason)
	}
	startedAt := s.now()
	if run.StartedAt != nil {
		startedAt = *run.StartedAt
	}
	deadline := startedAt.Add(time.Duration(budget.MaxDurationSeconds) * time.Second)
	version, err := agentpkg.GetVersion(ctx, s.db.WithContext(ctx), run.AgentVersionID)
	if err != nil {
		return s.fail(ctx, run, "agent_version_unavailable")
	}
	ctx = aimodelruntime.WithModelAlias(ctx, version.ModelAlias)
	if s.contextBudget == nil {
		return s.fail(ctx, run, "model_context_budget_unavailable")
	}
	modelBudget, err := s.contextBudget.ModelTurnBudget(ctx, version.ModelAlias)
	if err != nil {
		if errs.StableCategory(err) == aimodel.ContextBudgetInvalid {
			return s.fail(ctx, run, aimodel.ContextBudgetInvalid)
		}
		return s.fail(ctx, run, "model_context_budget_unavailable")
	}
	tdb, scopeErr := s.db.Tenant(ctx)
	if scopeErr != nil {
		return s.fail(ctx, run, "tenant_context_unavailable")
	}
	conversations, queryErr := tenantdata.TenantRaw[Conversation](tdb, `SELECT * FROM ai_conversations WHERE tenant_id=? AND id=? LIMIT 1`, run.ConversationID)
	if queryErr != nil || len(conversations) != 1 {
		return s.fail(ctx, run, "conversation_unavailable")
	}
	conversation := conversations[0]
	if conversation.Purpose == ConversationPurposeWorkspace && version.Kind == agentpkg.AgentVersionKindPreview {
		return s.fail(ctx, run, "preview_version_forbidden")
	}
	agentRow, err := agentpkg.GetForRuntime(ctx, s.db.WithContext(ctx), version.AgentID, conversation.Purpose == ConversationPurposeBuilderPreview)
	if err != nil {
		return s.fail(ctx, run, "agent_unavailable")
	}
	if agentRow.Status == "disabled" {
		return s.fail(ctx, run, "agent_unavailable")
	}
	trustedContext, err := s.resolveRuntimeContext(ctx, run, &conversation, &agentRow)
	if err != nil {
		return s.finalizeSafeFailure(ctx, run, "runtime_context_failed", version.ModelAlias, budget, startedAt)
	}
	if trustedContext.Locale != "" {
		localizedActor := *auth.MustFromContext(ctx)
		localizedActor.Language = trustedContext.Locale
		ctx = auth.WithContext(ctx, &localizedActor)
	}
	ctx = runtimecontext.WithTrusted(ctx, trustedContext)
	scopeID, err := s.ensureToolScope(ctx, run, &agentRow)
	if err != nil {
		return s.fail(ctx, run, "tool_scope_failed")
	}
	resumedConfirmation, err := s.resumeConfirmation(ctx, run)
	if err != nil {
		return err
	}
	if run.Status != "running" {
		return nil
	}
	stepLimit := stepLimitAfterConfirmation(budget.MaxSteps, run.CurrentStep, resumedConfirmation)
	waiting, queuedToolResultsPending, queueErr := s.continueDeferredConfirmationQueue(ctx, run, scopeID, budget.MaxToolCalls, stepLimit)
	if queueErr != nil {
		return s.fail(ctx, run, "deferred_confirmation_queue_failed")
	}
	if waiting {
		return nil
	}
	toolResultsPending := resumedConfirmation || queuedToolResultsPending
	for {
		allowed, stepCompletionOnly := reactTurnMode(run.CurrentStep, stepLimit, toolResultsPending)
		if !allowed || !s.now().Before(deadline) {
			break
		}
		toolResultsPending = false
		if run.ModelCalls >= budget.MaxModelCalls {
			if run.ExecutionMode == string(aiagentruntime.ExecutionModeCompletionOnly) {
				return s.fail(ctx, run, "budget_completion_unavailable")
			}
			return s.fail(ctx, run, "model_budget_exhausted")
		}
		reserve := s.completionReserve(run, budget.RunTokenBudget, int64(modelBudget.ReservedOutputTokens), 1)
		preCompletionOnly := stepCompletionOnly || run.ExecutionMode == string(aiagentruntime.ExecutionModeCompletionOnly) || run.ModelCalls+1 >= budget.MaxModelCalls
		remaining := budget.RunTokenBudget - run.InputTokens - run.OutputTokens
		compactionTarget := completionInputTarget(remaining, int64(modelBudget.AvailableTokens), reserve, preCompletionOnly)
		if compactionTarget <= 0 {
			return s.fail(ctx, run, "budget_completion_unavailable")
		}
		messages, snapshotID, err := s.runtimeMessages(ctx, run, version.ModelAlias, budget.MaxCompactions, trustedContext, int(compactionTarget), reserve, preCompletionOnly)
		if err != nil {
			category := documentRuntimeCategory(err)
			if category != "document_parse_failed" {
				return s.finalizeSafeFailure(ctx, run, category, version.ModelAlias, budget, startedAt)
			}
			return s.fail(ctx, run, "budget_completion_unavailable")
		}
		run.ContextSnapshotID = snapshotID
		if snapshotID != nil {
			_ = s.attachRuntimeContextRefs(ctx, *snapshotID, trustedContext)
			_ = s.attachPageContextRefs(ctx, *snapshotID, pageSnapshot)
		}
		messages, recallRefs := s.injectMemoryRecall(ctx, run, version, messages, trustedContext)
		if snapshotID != nil && len(recallRefs) > 0 {
			_ = s.attachMemoryRecallRefs(ctx, *snapshotID, recallRefs)
		}
		effective, err := aiagent.ResolveInstructions(s.contracts, agentRow.ContractKey, agentRow.ContractVersion, version.TenantInstructions)
		if err != nil {
			return s.fail(ctx, run, "agent_contract_unavailable")
		}
		metadata := map[string]string{"agent_id": agentRow.ID, "agent_run_id": run.ID, "has_tenant_supplement": fmt.Sprint(effective.HasTenantSupplement)}
		if effective.ContractKey != "" {
			metadata["contract_key"], metadata["contract_version"] = effective.ContractKey, effective.ContractVersion
		}
		request := aimodelruntime.GenerateRequest{ModelAlias: version.ModelAlias, ToolScopeID: scopeID, EnableTools: true, Instructions: effective.Text, Messages: messages, Metadata: metadata}
		forceCompletion := preCompletionOnly
		if forceCompletion {
			request = completionOnlyRequest(request)
		}
		estimatedInput, estimateErr := s.estimateRuntimeRequest(ctx, request)
		if estimateErr != nil {
			return s.fail(ctx, run, "budget_estimate_failed")
		}
		if run.CompletionReservedInputTokens == 0 {
			reserve = aiagentruntime.DefaultCompletionReserve(budget.RunTokenBudget, int64(modelBudget.ReservedOutputTokens), int64(estimatedInput))
		}
		plannerMode := aiagentruntime.ExecutionMode(run.ExecutionMode)
		if forceCompletion {
			plannerMode = aiagentruntime.ExecutionModeCompletionOnly
		}
		plan, planErr := aiagentruntime.PlanTurn(aiagentruntime.TurnBudgetInput{
			HardLimit: budget.RunTokenBudget, ConsumedInput: run.InputTokens, ConsumedOutput: run.OutputTokens,
			EstimatedInput: int64(estimatedInput), DeploymentMaxOutput: int64(modelBudget.ReservedOutputTokens),
			Reserve: reserve, CurrentMode: plannerMode, CompletionAttempted: run.CompletionAttempted,
		})
		if planErr != nil {
			return s.fail(ctx, run, "budget_estimate_failed")
		}
		budgetCompletion := !stepCompletionOnly && (run.ExecutionMode == string(aiagentruntime.ExecutionModeCompletionOnly) || plan.Mode == aiagentruntime.ExecutionModeCompletionOnly)
		completionOnly := stepCompletionOnly || plan.Mode == aiagentruntime.ExecutionModeCompletionOnly
		if completionOnly && request.EnableTools {
			request = completionOnlyRequest(request)
			estimatedInput, estimateErr = s.estimateRuntimeRequest(ctx, request)
			if estimateErr != nil {
				return s.fail(ctx, run, "budget_estimate_failed")
			}
			reserve.InputTokens = int64(estimatedInput)
			plan, planErr = aiagentruntime.PlanTurn(aiagentruntime.TurnBudgetInput{
				HardLimit: budget.RunTokenBudget, ConsumedInput: run.InputTokens, ConsumedOutput: run.OutputTokens,
				EstimatedInput: int64(estimatedInput), DeploymentMaxOutput: int64(modelBudget.ReservedOutputTokens),
				Reserve: reserve, CurrentMode: aiagentruntime.ExecutionModeCompletionOnly, CompletionAttempted: run.CompletionAttempted,
			})
			if planErr != nil {
				return s.fail(ctx, run, "budget_estimate_failed")
			}
		}
		if !plan.Allowed {
			logging.Info(ctx, "agent turn budget rejected", "run_token_budget", budget.RunTokenBudget, "input_tokens", run.InputTokens, "output_tokens", run.OutputTokens, "estimated_input_tokens", estimatedInput, "remaining_tokens", plan.RemainingTokens, "reason", plan.Reason)
			return s.fail(ctx, run, "budget_completion_unavailable")
		}
		if budgetCompletion && run.ExecutionMode != string(aiagentruntime.ExecutionModeCompletionOnly) {
			if err := s.enterCompletionOnly(ctx, run, reserve, plan.Reason); err != nil {
				return err
			}
		}
		if completionOnly && budgetCompletion {
			if err := s.markCompletionAttempted(ctx, run); err != nil {
				return err
			}
		}
		maxOutput := plan.MaxOutputTokens
		request.MaxOutputTokens = &maxOutput
		logging.Info(ctx, "agent turn budget planned", "mode", plan.Mode, "reason", plan.Reason, "run_token_budget", budget.RunTokenBudget, "input_tokens", run.InputTokens, "output_tokens", run.OutputTokens, "estimated_input_tokens", estimatedInput, "max_output_tokens", plan.MaxOutputTokens, "reserved_input_tokens", reserve.InputTokens, "reserved_output_tokens", reserve.OutputTokens, "safety_margin_tokens", reserve.SafetyTokens)
		step, err := s.newStep(ctx, run, aiagentruntime.StepModel, map[string]any{"message_count": len(messages), "context_snapshot_id": snapshotID, "execution_mode": plan.Mode, "estimated_input_tokens": estimatedInput, "max_output_tokens": plan.MaxOutputTokens})
		if err != nil {
			return err
		}
		response, callErr := s.streamModel(ctx, run, request)
		run.ModelCalls++
		terminalCtx, cancelTerminal := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if e := s.attachModelInvocation(terminalCtx, run, step, response.InvocationID); e != nil {
			cancelTerminal()
			return e
		}
		if e := s.syncModelUsage(terminalCtx, run); e != nil {
			cancelTerminal()
			return e
		}
		if callErr != nil {
			category := modelFailureCategory(callErr)
			var re *aimodelruntime.RuntimeError
			if errors.As(callErr, &re) && re.Category == aimodelruntime.ErrorContextLength && run.Compactions < budget.MaxCompactions {
				if err := s.finishStep(terminalCtx, run, step, "failed", map[string]any{}, category); err != nil {
					cancelTerminal()
					return err
				}
				cancelTerminal()
				continue
			}
			if err := s.finishStep(terminalCtx, run, step, "failed", map[string]any{}, category); err != nil {
				cancelTerminal()
				return err
			}
			err := s.terminateModelError(terminalCtx, run, category)
			cancelTerminal()
			return err
		}
		cancelTerminal()
		toolCalls := []aimodelruntime.OutputItem{}
		text := ""
		refusalText := ""
		for _, item := range response.Output {
			if item.Type == aimodelruntime.PartOutputText {
				text += item.Text
			}
			if item.Type == aimodelruntime.PartRefusal {
				refusalText += item.Text
			}
			if item.Type == aimodelruntime.PartToolCall {
				toolCalls = append(toolCalls, item)
			}
		}
		if response.FinishReason == aimodelruntime.FinishContentFilter || strings.TrimSpace(refusalText) != "" {
			if strings.TrimSpace(refusalText) == "" {
				refusalText = text
			}
			if strings.TrimSpace(refusalText) == "" {
				_ = s.finishStep(ctx, run, step, "failed", map[string]any{"finish_reason": response.FinishReason, "refusal_text_available": false}, string(aimodelruntime.ErrorContentFiltered))
				return s.fail(ctx, run, string(aimodelruntime.ErrorContentFiltered))
			}
			decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{
				Status: aiagentruntime.RunFailed, ErrorCategory: string(aimodelruntime.ErrorContentFiltered), HasSafeRefusalText: true,
			})
			if persistErr := s.markRefusalDetected(ctx, run, decision); persistErr != nil {
				return persistErr
			}
			messageID, commitErr := s.commitTerminalMessage(ctx, run, refusalText, "run-refusal:"+run.ID, startedAt)
			if commitErr != nil {
				_ = s.finishStep(ctx, run, step, "failed", map[string]any{"finish_reason": response.FinishReason, "fallback": "notice_only"}, "finalize_failed")
				return s.fail(ctx, run, "finalize_failed")
			}
			run.TerminalMessageID = &messageID
			_ = s.finishStep(ctx, run, step, "completed", map[string]any{"finish_reason": response.FinishReason, "refusal_text_committed": true, "terminal_message_id": messageID}, "")
			return s.terminalWithDecision(ctx, run, string(decision.Status), "", decision.CompletionReason, true, decision)
		}
		hardExceeded := run.InputTokens+run.OutputTokens > budget.RunTokenBudget
		usableText := strings.TrimSpace(text) != ""
		resultPlan := aiagentruntime.PlanTurnResult(aiagentruntime.TurnResultInput{HasUsableText: usableText, ToolCallCount: len(toolCalls), CompletionOnly: completionOnly, BudgetCompletion: budgetCompletion, HardExceeded: hardExceeded})
		if resultPlan.ErrorCategory != "" {
			_ = s.finishStep(ctx, run, step, "failed", map[string]any{"finish_reason": response.FinishReason, "output_items": len(response.Output), "tool_calls_ignored": resultPlan.IgnoreToolCalls}, resultPlan.ErrorCategory)
			return s.fail(ctx, run, resultPlan.ErrorCategory)
		}
		if resultPlan.CommitFinal {
			_ = s.finishStep(ctx, run, step, "completed", map[string]any{"finish_reason": response.FinishReason, "output_items": len(response.Output), "hard_budget_exceeded": hardExceeded, "tool_calls_ignored": resultPlan.IgnoreToolCalls}, "")
			return s.commitFinalAnswer(ctx, run, text, startedAt, resultPlan.CompletionReason)
		}
		_ = s.finishStep(ctx, run, step, "completed", map[string]any{"finish_reason": response.FinishReason, "output_items": len(response.Output)}, "")
		// Provider 要求 tool_result 前必须有配对的 assistant tool_call；先落库再执行工具。
		if e := s.appendAssistantToolCalls(ctx, run, step, text, toolCalls); e != nil {
			return s.fail(ctx, run, "assistant_tool_call_persist_failed")
		}
		for callIndex, call := range toolCalls {
			if run.ToolCalls >= budget.MaxToolCalls {
				return s.fail(ctx, run, "tool_budget_exhausted")
			}
			waiting, err := s.executeToolCall(ctx, run, scopeID, call)
			if err != nil {
				return s.finalizeSafeFailure(ctx, run, "tool_failed", version.ModelAlias, budget, startedAt)
			}
			run.ToolCalls++
			if err := s.persistCounters(ctx, run); err != nil {
				return err
			}
			if waiting {
				for _, deferredCall := range toolCalls[callIndex+1:] {
					if err := s.appendSyntheticToolFailure(ctx, run, deferredCall, "deferred_for_confirmation", "tool call was not executed because another call requires confirmation; retry after confirmation if still needed"); err != nil {
						return s.fail(ctx, run, "tool_result_persist_failed")
					}
				}
				if err := s.finalizeConfirmationWait(ctx, run); err != nil {
					return s.fail(ctx, run, "confirmation_wait_persist_failed")
				}
				return nil
			}
		}
		toolResultsPending = true
	}
	if !s.now().Before(deadline) {
		return s.fail(ctx, run, "timeout")
	}
	return s.fail(ctx, run, "step_budget_exhausted")
}

func reactTurnMode(currentStep, stepLimit int, toolResultsPending bool) (allowed, completionOnly bool) {
	if currentStep < stepLimit {
		return true, false
	}
	return toolResultsPending, toolResultsPending
}

func completionInputTarget(remaining, contextLimit int64, reserve aiagentruntime.CompletionReserve, completionOnly bool) int64 {
	available := remaining - reserve.OutputTokens - reserve.SafetyTokens
	if !completionOnly {
		available /= 2
	}
	if available > contextLimit {
		available = contextLimit
	}
	return available
}

func completionOnlyRequest(request aimodelruntime.GenerateRequest) aimodelruntime.GenerateRequest {
	request.EnableTools = false
	request.ToolScopeID = ""
	request.Tools = nil
	request.ToolChoice = &aimodelruntime.ToolChoice{Mode: "none"}
	request.Instructions = strings.TrimSpace(request.Instructions) + "\n\n" + prompt.MustGet("ai/budget-completion")
	return request
}

func (s *AgentRuntimeService) completionReserve(run *AgentRun, hardLimit, deploymentMaxOutput, estimatedInput int64) aiagentruntime.CompletionReserve {
	if run.CompletionReservedInputTokens > 0 || run.CompletionReservedOutputTokens > 0 || run.CompletionSafetyMarginTokens > 0 {
		return aiagentruntime.CompletionReserve{InputTokens: run.CompletionReservedInputTokens, OutputTokens: run.CompletionReservedOutputTokens, SafetyTokens: run.CompletionSafetyMarginTokens}
	}
	return aiagentruntime.DefaultCompletionReserve(hardLimit, deploymentMaxOutput, estimatedInput)
}

func (s *AgentRuntimeService) estimateRuntimeRequest(ctx context.Context, request aimodelruntime.GenerateRequest) (int, error) {
	estimate := request
	if estimate.EnableTools && s.tools != nil {
		_, definitions, err := s.tools.ProjectModelTools(ctx, estimate.ToolScopeID)
		if err != nil {
			return 0, err
		}
		estimate.Tools = definitions
	}
	return s.contextBudget.EstimateRequestTokens(ctx, estimate)
}

func (s *AgentRuntimeService) enterCompletionOnly(ctx context.Context, run *AgentRun, reserve aiagentruntime.CompletionReserve, reason string) error {
	if run.ExecutionMode == string(aiagentruntime.ExecutionModeCompletionOnly) {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"execution_mode":                    string(aiagentruntime.ExecutionModeCompletionOnly),
			"completion_reserved_input_tokens":  reserve.InputTokens,
			"completion_reserved_output_tokens": reserve.OutputTokens,
			"completion_safety_margin_tokens":   reserve.SafetyTokens,
			"revision":                          gorm.Expr("revision+1"),
		}
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=? AND execution_mode=?", run.ID, "running", run.LeaseOwner, string(aiagentruntime.ExecutionModeNormal)).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run execution mode changed")
		}
		run.ExecutionMode = string(aiagentruntime.ExecutionModeCompletionOnly)
		run.CompletionReservedInputTokens = reserve.InputTokens
		run.CompletionReservedOutputTokens = reserve.OutputTokens
		run.CompletionSafetyMarginTokens = reserve.SafetyTokens
		return s.appendEvent(ctx, tx, run, "run.mode_changed", nil, map[string]any{"execution_mode": run.ExecutionMode, "reason": reason, "reserved_input_tokens": reserve.InputTokens, "reserved_output_tokens": reserve.OutputTokens, "safety_margin_tokens": reserve.SafetyTokens})
	})
}

func (s *AgentRuntimeService) markCompletionAttempted(ctx context.Context, run *AgentRun) error {
	if run.CompletionAttempted {
		return errs.New(errs.CodeConflict, "agent run completion already attempted")
	}
	res := s.db.WithContext(ctx).Model(&AgentRun{}).
		Where("id=? AND status=? AND lease_owner=? AND execution_mode=? AND completion_attempted=?", run.ID, "running", run.LeaseOwner, string(aiagentruntime.ExecutionModeCompletionOnly), false).
		Updates(map[string]any{"completion_attempted": true, "revision": gorm.Expr("revision+1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errs.New(errs.CodeConflict, "agent run completion attempt changed")
	}
	run.CompletionAttempted = true
	return nil
}

func (s *AgentRuntimeService) syncModelUsage(ctx context.Context, run *AgentRun) error {
	var usage struct {
		InputTokens  int64 `gorm:"column:input_tokens"`
		OutputTokens int64 `gorm:"column:output_tokens"`
	}
	if err := s.db.WithContext(ctx).Model(&modelruntime.ModelInvocation{}).
		Where("agent_run_id=?", run.ID).
		Select("COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens").
		Scan(&usage).Error; err != nil {
		return err
	}
	if usage.InputTokens > run.InputTokens {
		run.InputTokens = usage.InputTokens
	}
	if usage.OutputTokens > run.OutputTokens {
		run.OutputTokens = usage.OutputTokens
	}
	return s.persistCounters(ctx, run)
}

func (s *AgentRuntimeService) markFinalizationAttempted(ctx context.Context, run *AgentRun, decision aiagentruntime.TerminalDecision, category string) error {
	if run.FinalizationAttempted {
		return errs.New(errs.CodeConflict, "agent run finalization already attempted")
	}
	updates := map[string]any{
		"finalization_attempted":    true,
		"finalization_mode":         decision.Mode,
		"terminal_protocol_version": decision.ProtocolVersion,
		"terminal_reason":           decision.TerminalReason,
		"retryable":                 decision.Retryable,
		"user_message_key":          decision.UserMessageKey,
		"safe_summary":              decision.SafeSummary,
		"error_category":            category,
		"revision":                  gorm.Expr("revision+1"),
	}
	res := s.db.WithContext(ctx).Model(&AgentRun{}).
		Where("id=? AND status=? AND lease_owner=? AND finalization_attempted=?", run.ID, "running", run.LeaseOwner, false).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errs.New(errs.CodeConflict, "agent run finalization attempt changed")
	}
	run.FinalizationAttempted = true
	run.FinalizationMode = string(decision.Mode)
	run.TerminalProtocolVersion = decision.ProtocolVersion
	run.TerminalReason = decision.TerminalReason
	run.Retryable = decision.Retryable
	run.UserMessageKey = decision.UserMessageKey
	run.SafeSummary = decision.SafeSummary
	run.ErrorCategory = category
	return nil
}

func (s *AgentRuntimeService) markRefusalDetected(ctx context.Context, run *AgentRun, decision aiagentruntime.TerminalDecision) error {
	category := string(aimodelruntime.ErrorContentFiltered)
	updates := map[string]any{
		"error_category": category, "terminal_protocol_version": decision.ProtocolVersion,
		"terminal_reason": decision.TerminalReason, "completion_reason": decision.CompletionReason,
		"finalization_mode": decision.Mode, "retryable": decision.Retryable,
		"user_message_key": decision.UserMessageKey, "safe_summary": decision.SafeSummary,
		"revision": gorm.Expr("revision+1"),
	}
	res := s.db.WithContext(ctx).Model(&AgentRun{}).
		Where("id=? AND status=? AND lease_owner=?", run.ID, "running", run.LeaseOwner).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errs.New(errs.CodeConflict, "agent run refusal state changed")
	}
	run.ErrorCategory = category
	run.TerminalProtocolVersion = decision.ProtocolVersion
	run.TerminalReason = decision.TerminalReason
	run.CompletionReason = decision.CompletionReason
	run.FinalizationMode = string(decision.Mode)
	run.Retryable = decision.Retryable
	run.UserMessageKey = decision.UserMessageKey
	run.SafeSummary = decision.SafeSummary
	return nil
}

func (s *AgentRuntimeService) finalizeSafeFailure(ctx context.Context, run *AgentRun, category, modelAlias string, budget aiagentruntime.Budget, startedAt time.Time) error {
	operations, err := loadTerminalOperationSummary(s.db.WithContext(ctx), run.ID)
	if err != nil {
		return s.fail(ctx, run, category)
	}
	canUseModel := s.invocations != nil && s.conversations != nil && s.contextBudget != nil &&
		!run.FinalizationAttempted && run.ModelCalls < budget.MaxModelCalls
	decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{
		Status: aiagentruntime.RunFailed, ErrorCategory: category, CanUseModel: canUseModel,
		CompletedOperations: operations.Completed,
	})
	if decision.Mode != aiagentruntime.FinalizationModel {
		return s.fail(ctx, run, category)
	}
	locale := ""
	if actor, ok := auth.FromContext(ctx); ok {
		locale = actor.Language
	}
	envelope, err := json.Marshal(aiagentruntime.BuildFailureEnvelope(decision, aiagentruntime.TerminalOperationSummary{
		Completed: operations.Completed, Failed: operations.Failed, Unfinished: operations.Unfinished,
	}, locale))
	if err != nil {
		return s.fail(ctx, run, category)
	}
	request := safeTerminalRequest(modelAlias, run.ID, envelope)
	estimatedInput, err := s.estimateRuntimeRequest(ctx, request)
	if err != nil {
		return s.fail(ctx, run, category)
	}
	remaining := budget.RunTokenBudget - run.InputTokens - run.OutputTokens - int64(estimatedInput)
	maxOutput := int64(512)
	if remaining < maxOutput {
		maxOutput = remaining
	}
	if maxOutput < aiagentruntime.MinCompletionOutputTokens {
		return s.fail(ctx, run, category)
	}
	request.MaxOutputTokens = &maxOutput
	if err = s.markFinalizationAttempted(ctx, run, decision, category); err != nil {
		return err
	}
	step, err := s.newStep(ctx, run, aiagentruntime.StepFinalize, map[string]any{
		"terminal_protocol_version": decision.ProtocolVersion, "terminal_reason": decision.TerminalReason,
		"finalization_mode": decision.Mode, "retryable": decision.Retryable,
		"completed_operation_count": operations.Completed, "max_output_tokens": maxOutput,
	})
	if err != nil {
		return s.fail(ctx, run, category)
	}
	value, callErr := s.invocations.Generate(ctx, request)
	response, responseOK := asGenerateResponse(value)
	if callErr == nil && !responseOK {
		callErr = &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorResponseMismatch, Message: "invalid terminal finalization response"}
	}
	run.ModelCalls++
	terminalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = s.attachModelInvocationMode(terminalCtx, run, step, response.InvocationID, "generate")
	_ = s.syncModelUsage(terminalCtx, run)
	if callErr != nil {
		_ = s.finishStep(terminalCtx, run, step, "failed", map[string]any{"fallback": "template"}, modelFailureCategory(callErr))
		return s.fail(terminalCtx, run, category)
	}
	text, hasToolCall, hasRefusal := terminalResponseText(response)
	if strings.TrimSpace(text) == "" || hasToolCall || hasRefusal || response.FinishReason == aimodelruntime.FinishContentFilter {
		_ = s.finishStep(terminalCtx, run, step, "failed", map[string]any{"fallback": "template", "tool_calls_ignored": hasToolCall}, "finalization_invalid_output")
		return s.fail(terminalCtx, run, category)
	}
	messageID, err := s.commitTerminalMessage(terminalCtx, run, text, "run-terminal-final:"+run.ID, startedAt)
	if err != nil {
		_ = s.finishStep(terminalCtx, run, step, "failed", map[string]any{"fallback": "notice_only"}, "finalize_failed")
		return s.fail(terminalCtx, run, "finalize_failed")
	}
	run.TerminalMessageID = &messageID
	_ = s.finishStep(terminalCtx, run, step, "completed", map[string]any{"message_committed": true, "terminal_message_id": messageID}, "")
	return s.terminalWithDecision(terminalCtx, run, "failed", category, "", true, decision)
}

func safeTerminalRequest(modelAlias, runID string, envelope []byte) aimodelruntime.GenerateRequest {
	return aimodelruntime.GenerateRequest{
		ModelAlias:   modelAlias,
		EnableTools:  false,
		ToolChoice:   &aimodelruntime.ToolChoice{Mode: "none"},
		Instructions: prompt.MustGet("ai/safe-terminal-finalization"),
		Messages: []aimodelruntime.Message{{
			Role:    aimodelruntime.RoleUser,
			Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(envelope)}},
		}},
		Metadata: map[string]string{
			"agent_run_id": runID, "terminal_protocol_version": aiagentruntime.TerminalProtocolVersion, "finalization": "safe_failure",
		},
	}
}

func terminalResponseText(response aimodelruntime.GenerateResponse) (text string, hasToolCall, hasRefusal bool) {
	for _, item := range response.Output {
		switch item.Type {
		case aimodelruntime.PartOutputText:
			text += item.Text
		case aimodelruntime.PartToolCall:
			hasToolCall = true
		case aimodelruntime.PartRefusal:
			hasRefusal = true
		}
	}
	return text, hasToolCall, hasRefusal
}

func (s *AgentRuntimeService) commitTerminalMessage(ctx context.Context, run *AgentRun, text, idempotencyKey string, startedAt time.Time) (string, error) {
	streamID, err := s.conversations.Begin(ctx, run.ConversationID, idempotencyKey, run.ID)
	if err != nil {
		return "", err
	}
	if err = s.conversations.Append(ctx, streamID, aiconversation.StreamDelta{Cursor: 1, Type: "output_text.delta", Data: []byte(text)}); err != nil {
		return "", err
	}
	logging.Info(ctx, "agent terminal message persisted", "agent_first_delta_latency_ms", s.now().Sub(startedAt).Milliseconds(), "finalization_mode", run.FinalizationMode)
	return s.conversations.Commit(ctx, streamID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text}})
}

func (s *AgentRuntimeService) recoverTerminalFinalization(ctx context.Context, run *AgentRun) (bool, error) {
	if !run.FinalizationAttempted || run.Status != "running" {
		return false, nil
	}
	var stream AssistantStream
	err := s.db.WithContext(ctx).Where("conversation_id=? AND run_id=? AND idempotency_key=?", run.ConversationID, run.ID, "run-terminal-final:"+run.ID).First(&stream).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, s.fail(ctx, run, run.ErrorCategory)
	}
	if err != nil {
		return false, err
	}
	if stream.Status == "streaming" {
		var deltas []AssistantStreamDelta
		if err = s.db.WithContext(ctx).Where("stream_id=? AND type=?", stream.ID, "output_text.delta").Order("cursor ASC").Find(&deltas).Error; err != nil {
			return false, err
		}
		var text strings.Builder
		for _, delta := range deltas {
			text.Write(delta.Data)
		}
		if strings.TrimSpace(text.String()) == "" {
			return true, s.fail(ctx, run, run.ErrorCategory)
		}
		messageID, commitErr := s.conversations.Commit(ctx, stream.ID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text.String()}})
		if commitErr != nil {
			return true, s.fail(ctx, run, "finalize_failed")
		}
		stream.MessageID = &messageID
	}
	if stream.MessageID == nil {
		return true, s.fail(ctx, run, "finalize_failed")
	}
	run.TerminalMessageID = stream.MessageID
	decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: run.ErrorCategory, CanUseModel: true})
	return true, s.terminalWithDecision(ctx, run, "failed", run.ErrorCategory, "", true, decision)
}

func (s *AgentRuntimeService) recoverRefusalFinalization(ctx context.Context, run *AgentRun) (bool, error) {
	if run.Status != "running" || run.ErrorCategory != string(aimodelruntime.ErrorContentFiltered) || run.TerminalReason != "safety_refusal" {
		return false, nil
	}
	decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{
		Status: aiagentruntime.RunFailed, ErrorCategory: string(aimodelruntime.ErrorContentFiltered), HasSafeRefusalText: true,
	})
	var stream AssistantStream
	err := s.db.WithContext(ctx).Where("conversation_id=? AND run_id=? AND idempotency_key=?", run.ConversationID, run.ID, "run-refusal:"+run.ID).First(&stream).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, s.fail(ctx, run, string(aimodelruntime.ErrorContentFiltered))
	}
	if err != nil {
		return false, err
	}
	if stream.Status == "streaming" {
		var deltas []AssistantStreamDelta
		if err = s.db.WithContext(ctx).Where("stream_id=? AND type=?", stream.ID, "output_text.delta").Order("cursor ASC").Find(&deltas).Error; err != nil {
			return false, err
		}
		var text strings.Builder
		for _, delta := range deltas {
			text.Write(delta.Data)
		}
		if strings.TrimSpace(text.String()) == "" {
			return true, s.fail(ctx, run, string(aimodelruntime.ErrorContentFiltered))
		}
		messageID, commitErr := s.conversations.Commit(ctx, stream.ID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text.String()}})
		if commitErr != nil {
			return true, s.fail(ctx, run, "finalize_failed")
		}
		stream.MessageID = &messageID
	}
	if stream.MessageID == nil {
		return true, s.fail(ctx, run, "finalize_failed")
	}
	run.TerminalMessageID = stream.MessageID
	return true, s.terminalWithDecision(ctx, run, string(decision.Status), "", decision.CompletionReason, true, decision)
}

func (s *AgentRuntimeService) commitFinalAnswer(ctx context.Context, run *AgentRun, text string, startedAt time.Time, completionReason string) error {
	finalStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepFinalize, map[string]any{"text_bytes": len(text), "completion_reason": completionReason})
	if stepErr != nil {
		return stepErr
	}
	streamID, err := s.conversations.Begin(ctx, run.ConversationID, "run-final:"+run.ID, run.ID)
	if err != nil {
		return s.fail(ctx, run, "finalize_failed")
	}
	if err = s.conversations.Append(ctx, streamID, aiconversation.StreamDelta{Cursor: 1, Type: "output_text.delta", Data: []byte(text)}); err != nil {
		return s.fail(ctx, run, "finalize_failed")
	}
	logging.Info(ctx, "agent first delta persisted", "agent_first_delta_latency_ms", s.now().Sub(startedAt).Milliseconds())
	messageID, err := s.conversations.Commit(ctx, streamID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text}})
	if err != nil {
		return s.fail(ctx, run, "finalize_failed")
	}
	run.TerminalMessageID = &messageID
	_ = s.finishStep(ctx, run, finalStep, "completed", map[string]any{"message_committed": true, "completion_reason": completionReason, "terminal_message_id": messageID}, "")
	return s.completeWithReason(ctx, run, completionReason)
}

func (s *AgentRuntimeService) recoverFinalAnswer(ctx context.Context, run *AgentRun) (bool, error) {
	var stream AssistantStream
	err := s.db.WithContext(ctx).Where("conversation_id=? AND run_id=? AND idempotency_key=?", run.ConversationID, run.ID, "run-final:"+run.ID).First(&stream).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if stream.Status == "committed" {
		run.TerminalMessageID = stream.MessageID
		return true, nil
	}
	if stream.Status != "streaming" {
		return false, nil
	}
	var deltas []AssistantStreamDelta
	if err := s.db.WithContext(ctx).Where("stream_id=? AND type=?", stream.ID, "output_text.delta").Order("cursor ASC").Find(&deltas).Error; err != nil {
		return false, err
	}
	var text strings.Builder
	for _, delta := range deltas {
		text.Write(delta.Data)
	}
	if strings.TrimSpace(text.String()) == "" {
		return false, nil
	}
	messageID, err := s.conversations.Commit(ctx, stream.ID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text.String()}})
	if err != nil {
		return false, err
	}
	run.TerminalMessageID = &messageID
	var step AgentStep
	if err := s.db.WithContext(ctx).Where("run_id=? AND type=? AND status=?", run.ID, string(aiagentruntime.StepFinalize), string(aiagentruntime.StepRunning)).Order("step_index DESC").First(&step).Error; err == nil {
		_ = s.finishStep(ctx, run, &step, "completed", map[string]any{"message_committed": true, "recovered": true}, "")
	}
	return true, nil
}

func (s *AgentRuntimeService) persistCounters(ctx context.Context, run *AgentRun) error {
	res := s.db.WithContext(ctx).Model(&AgentRun{}).
		Where("id=? AND status=? AND lease_owner=?", run.ID, "running", run.LeaseOwner).
		Updates(map[string]any{"model_calls": run.ModelCalls, "tool_calls": run.ToolCalls, "compactions": run.Compactions, "input_tokens": run.InputTokens, "output_tokens": run.OutputTokens, "context_snapshot_id": run.ContextSnapshotID, "revision": gorm.Expr("revision+1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errs.New(errs.CodeConflict, "agent run lease changed")
	}
	return nil
}

func (s *AgentRuntimeService) heartbeatLease(ctx context.Context, cancel context.CancelFunc, runID, worker string) {
	interval := s.leaseDuration / 3
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expires := s.now().Add(s.leaseDuration)
			res := s.db.WithContext(ctx).Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", runID, "running", worker).Update("lease_expires_at", expires)
			if res.Error != nil || res.RowsAffected != 1 {
				cancel()
				return
			}
		}
	}
}

func (s *AgentRuntimeService) runtimeMessages(ctx context.Context, run *AgentRun, modelAlias string, maxCompactions int, trusted *runtimecontext.TrustedRuntimeContext, targetInputTokens int, reserve aiagentruntime.CompletionReserve, disableTools bool) ([]aimodelruntime.Message, *string, error) {
	pageSnapshot, err := pageContextSnapshot(run.PageContextJSON)
	if err != nil {
		return nil, nil, err
	}
	assembleStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepContextAssemble, map[string]any{"conversation_revision": s.currentConversationRevision(ctx, run)})
	if stepErr != nil {
		return nil, nil, stepErr
	}
	if s.contextBudget == nil {
		_ = s.finishStep(ctx, run, assembleStep, "failed", map[string]any{}, "model_context_budget_unavailable")
		return nil, nil, fmt.Errorf("model context budget unavailable")
	}
	contextTokenLimit, err := s.contextBudget.ContextTokenLimit(ctx, modelAlias)
	if err != nil {
		_ = s.finishStep(ctx, run, assembleStep, "failed", map[string]any{}, "model_context_budget_unavailable")
		return nil, nil, err
	}
	assembly, err := s.conversations.Assemble(ctx, aiconversation.AssemblyRequest{ConversationID: run.ConversationID, TriggerMessageID: run.TriggerMessageID, MaxTokens: contextTokenLimit, MaxBytes: contextTokenLimit * 8, MaxNativeImages: aimodelruntime.MaxNativeImagesPerRequest})
	if err != nil {
		_ = s.finishStep(ctx, run, assembleStep, "failed", map[string]any{}, "context_assemble_failed")
		return nil, nil, err
	}
	_ = s.finishStep(ctx, run, assembleStep, "completed", map[string]any{"status": assembly.Status, "estimated_tokens": assembly.Budget.EstimatedTokens}, "")
	if s.compactor != nil {
		compactStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepContextCompact, map[string]any{"model_alias": modelAlias, "parent_snapshot_id": run.ContextSnapshotID})
		if stepErr != nil {
			return nil, nil, stepErr
		}
		reason := ""
		if targetInputTokens > 0 && targetInputTokens < contextTokenLimit {
			reason = "run_budget_pressure"
		}
		result, err := s.compactor.Compact(ctx, aiconversation.CompactionRequest{
			ConversationID: run.ConversationID, TriggerMessageID: run.TriggerMessageID, ModelAlias: modelAlias,
			ReservedOutputTokens: int(reserve.OutputTokens), SafetyMarginTokens: int(reserve.SafetyTokens),
			AttachmentTokens:  assembly.AttachmentTokens,
			TargetInputTokens: targetInputTokens, Reason: reason, AgentRunID: run.ID, DisableTools: disableTools,
			IdempotencyKey: fmt.Sprintf("run:%s:compact:%d:%d", run.ID, run.Compactions, targetInputTokens), ParentSnapshotID: run.ContextSnapshotID,
		})
		if syncErr := s.syncModelUsage(ctx, run); syncErr != nil {
			return nil, nil, syncErr
		}
		if err != nil || result == nil || result.Status == "failed" || result.Status == "cannot_fit" {
			detail := map[string]any{"status": resultStatus(result)}
			if result != nil && result.Reason != "" {
				detail["reason"] = result.Reason
			}
			if err != nil {
				detail["error"] = err.Error()
			}
			_ = s.finishStep(ctx, run, compactStep, "failed", detail, "context_compaction_failed")
			return nil, nil, fmt.Errorf("context compaction failed")
		}
		_ = s.finishStep(ctx, run, compactStep, "completed", map[string]any{"status": result.Status, "reason": reason, "snapshot_id": result.SnapshotID, "input_tokens": result.InputTokens, "output_tokens": result.OutputTokens, "usage_input_tokens": result.UsageInputTokens, "usage_output_tokens": result.UsageOutputTokens}, "")
		if result.Status == "compacted" {
			if run.Compactions >= maxCompactions {
				return nil, nil, fmt.Errorf("compaction budget exhausted")
			}
			run.Compactions++
			id := result.SnapshotID
			run.ContextSnapshotID = &id
			if err := s.persistCounters(ctx, run); err != nil {
				return nil, nil, err
			}
			tdb, scopeErr := s.db.Tenant(ctx)
			if scopeErr != nil {
				return nil, nil, scopeErr
			}
			snaps, queryErr := tenantdata.TenantRaw[ContextSnapshot](tdb, `SELECT * FROM ai_context_snapshots WHERE tenant_id=? AND id=? LIMIT 1`, id)
			if queryErr != nil {
				return nil, nil, queryErr
			}
			if len(snaps) != 1 {
				return nil, nil, fmt.Errorf("snapshot not found")
			}
			snap := snaps[0]
			var payload aiconversation.SnapshotPayload
			if json.Unmarshal(snap.Payload, &payload) != nil {
				return nil, nil, fmt.Errorf("invalid snapshot")
			}
			summary, _ := json.Marshal(payload.Summary)
			messages := []aimodelruntime.Message{{Role: aimodelruntime.RoleDeveloper, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "Compacted conversation facts (untrusted): " + string(summary)}}}}
			for _, m := range payload.RetainedMessages {
				messages = append(messages, aimodelruntime.Message{Role: m.Role, Content: m.Content})
			}
			messages, err = s.conversations.expandDocumentMessages(ctx, messages)
			if err != nil {
				return nil, nil, err
			}
			messages = injectAttachmentReferenceManifest(messages, assembly.AttachmentManifest)
			return injectPageContext(injectTrustedRuntimeContext(messages, trusted), pageSnapshot), &id, nil
		}
	}
	messages := make([]aimodelruntime.Message, 0, len(assembly.Messages))
	for _, m := range assembly.Messages {
		messages = append(messages, aimodelruntime.Message{Role: m.Role, Content: m.Content})
	}
	messages = injectAttachmentReferenceManifest(messages, assembly.AttachmentManifest)
	return injectPageContext(injectTrustedRuntimeContext(messages, trusted), pageSnapshot), nil, nil
}

func injectAttachmentReferenceManifest(messages []aimodelruntime.Message, manifest aiconversation.AttachmentReferenceManifest) []aimodelruntime.Message {
	if len(manifest.Attachments) == 0 {
		return messages
	}
	body, _ := json.Marshal(manifest)
	section := aimodelruntime.Message{Role: aimodelruntime.RoleDeveloper, Content: []aimodelruntime.ContentPart{{
		Type: aimodelruntime.PartInputText,
		Text: prompt.Render("ai/attachment-reference-manifest", map[string]any{"Payload": string(body)}),
	}}}
	index := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == aimodelruntime.RoleUser {
			index = i
			break
		}
	}
	out := make([]aimodelruntime.Message, 0, len(messages)+1)
	out = append(out, messages[:index]...)
	out = append(out, section)
	out = append(out, messages[index:]...)
	return out
}

func (s *AgentRuntimeService) attachPageContextRefs(ctx context.Context, snapshotID string, snapshot *pagecontext.Snapshot) error {
	if snapshot == nil {
		return nil
	}
	refs := map[string]any{"schema_version": snapshot.SchemaVersion, "page_type": snapshot.PageType, "mode": snapshot.Mode, "trusted_refs": snapshot.TrustedRefs}
	body, _ := json.Marshal(refs)
	res := s.db.WithContext(ctx).Model(&ContextSnapshot{}).Where("id=? AND page_context_refs = '{}'::jsonb", snapshotID).Update("page_context_refs", body)
	if res.Error != nil || res.RowsAffected == 1 {
		return res.Error
	}
	var current ContextSnapshot
	if err := s.db.WithContext(ctx).Select("page_context_refs").Where("id=?", snapshotID).First(&current).Error; err != nil {
		return err
	}
	if string(current.PageContextRefs) != string(body) {
		return fmt.Errorf("snapshot page context refs conflict")
	}
	return nil
}

func pageContextSnapshot(raw json.RawMessage) (*pagecontext.Snapshot, error) {
	if len(raw) == 0 || string(raw) == "{}" {
		return nil, nil
	}
	var snapshot pagecontext.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.SchemaVersion != pagecontext.SchemaVersion || snapshot.PageType == "" {
		return nil, fmt.Errorf("invalid page context snapshot")
	}
	return &snapshot, nil
}

func injectPageContext(messages []aimodelruntime.Message, snapshot *pagecontext.Snapshot) []aimodelruntime.Message {
	if snapshot == nil {
		return messages
	}
	body, _ := json.Marshal(snapshot)
	section := aimodelruntime.Message{Role: aimodelruntime.RoleDeveloper, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: prompt.Render("ai/page-context", map[string]any{"Payload": string(body)})}}}
	index := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == aimodelruntime.RoleUser {
			index = i
			break
		}
	}
	out := make([]aimodelruntime.Message, 0, len(messages)+1)
	out = append(out, messages[:index]...)
	out = append(out, section)
	out = append(out, messages[index:]...)
	return out
}

func (s *AgentRuntimeService) resolveRuntimeContext(ctx context.Context, run *AgentRun, conversation *Conversation, agentRow *agentpkg.Agent) (*runtimecontext.TrustedRuntimeContext, error) {
	if s.runtimeContext == nil {
		return nil, fmt.Errorf("runtime context provider unavailable")
	}
	var existing *runtimecontext.TimeSnapshot
	if len(run.TimeSnapshotJSON) > 0 && string(run.TimeSnapshotJSON) != "{}" {
		var snap runtimecontext.TimeSnapshot
		if json.Unmarshal(run.TimeSnapshotJSON, &snap) != nil {
			return nil, fmt.Errorf("invalid stored time snapshot")
		}
		existing = &snap
	}
	trusted, err := s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: run.TenantID, PrincipalID: run.UserID, PrincipalKind: run.PrincipalKind, ClientSurface: run.ClientSurface, Audience: run.Audience, ConversationID: conversation.ID, ConversationCreatedAt: conversation.CreatedAt, AgentID: agentRow.ID, AgentVersionID: run.AgentVersionID, RunID: run.ID, TimeSnapshot: existing})
	if err != nil {
		return nil, err
	}
	if existing == nil {
		body, _ := json.Marshal(trusted.Time)
		res := s.db.WithContext(ctx).Model(&AgentRun{}).Where("id=? AND evaluated_at_utc IS NULL", run.ID).Updates(map[string]any{"evaluated_at_utc": trusted.Time.EvaluatedAtUTC, "timezone": trusted.Time.Timezone, "time_snapshot_json": body})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected != 1 {
			var current AgentRun
			if s.db.WithContext(ctx).Select("time_snapshot_json,evaluated_at_utc,timezone").Where("id=?", run.ID).First(&current).Error != nil {
				return nil, fmt.Errorf("run time snapshot conflict")
			}
			var snap runtimecontext.TimeSnapshot
			if json.Unmarshal(current.TimeSnapshotJSON, &snap) != nil {
				return nil, fmt.Errorf("invalid concurrent time snapshot")
			}
			return s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: run.TenantID, PrincipalID: run.UserID, PrincipalKind: run.PrincipalKind, ClientSurface: run.ClientSurface, Audience: run.Audience, ConversationID: conversation.ID, ConversationCreatedAt: conversation.CreatedAt, AgentID: agentRow.ID, AgentVersionID: run.AgentVersionID, RunID: run.ID, TimeSnapshot: &snap})
		}
		run.EvaluatedAtUTC = &trusted.Time.EvaluatedAtUTC
		run.Timezone = trusted.Time.Timezone
		run.TimeSnapshotJSON = body
	}
	return trusted, nil
}

func injectTrustedRuntimeContext(messages []aimodelruntime.Message, trusted *runtimecontext.TrustedRuntimeContext) []aimodelruntime.Message {
	if trusted == nil {
		return messages
	}
	payload := map[string]any{"preferred_address": trusted.PreferredAddress, "preferred_address_source": trusted.PreferredAddressSource, "display_name": trusted.DisplayName, "username": trusted.Username, "locale": trusted.Locale, "locale_source": trusted.LocaleSource, "timezone": trusted.Time.Timezone, "timezone_source": trusted.Time.TimezoneSource, "evaluated_at_utc": trusted.Time.EvaluatedAtUTC, "current_local_datetime": trusted.Time.CurrentLocalDateTime, "current_local_date": trusted.Time.CurrentLocalDate, "weekday": trusted.Time.Weekday, "context_version": trusted.ContextVersion, "policy_version": trusted.PolicyVersion}
	body, _ := json.Marshal(payload)
	section := aimodelruntime.Message{Role: aimodelruntime.RoleDeveloper, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: prompt.Render("ai/trusted-runtime-context", map[string]any{"Payload": string(body)})}}}
	index := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == aimodelruntime.RoleUser {
			index = i
			break
		}
	}
	out := make([]aimodelruntime.Message, 0, len(messages)+1)
	out = append(out, messages[:index]...)
	out = append(out, section)
	out = append(out, messages[index:]...)
	return out
}

func (s *AgentRuntimeService) attachRuntimeContextRefs(ctx context.Context, snapshotID string, trusted *runtimecontext.TrustedRuntimeContext) error {
	refs := runtimecontext.SnapshotRefs{RunID: trusted.RunID, ContextVersion: trusted.ContextVersion, PolicyVersion: trusted.PolicyVersion, Timezone: trusted.Time.Timezone, TimezoneSource: trusted.Time.TimezoneSource, EvaluatedAtUTC: trusted.Time.EvaluatedAtUTC}
	body, _ := json.Marshal(refs)
	res := s.db.WithContext(ctx).Model(&ContextSnapshot{}).Where("id=? AND runtime_context_refs = '{}'::jsonb", snapshotID).Update("runtime_context_refs", body)
	if res.Error != nil || res.RowsAffected == 1 {
		return res.Error
	}
	var snapshot ContextSnapshot
	if err := s.db.WithContext(ctx).Select("runtime_context_refs").Where("id=?", snapshotID).First(&snapshot).Error; err != nil {
		return err
	}
	if string(snapshot.RuntimeContextRefs) != string(body) {
		return fmt.Errorf("snapshot runtime context refs conflict")
	}
	return nil
}

func (s *AgentRuntimeService) injectMemoryRecall(ctx context.Context, run *AgentRun, version agentpkg.AgentVersion, messages []aimodelruntime.Message, trusted *runtimecontext.TrustedRuntimeContext) ([]aimodelruntime.Message, []MemoryProvenanceRef) {
	if s.memoryRetriever == nil || trusted == nil || !trusted.AutoMemoryEnabled {
		return messages, nil
	}
	var policy aimemory.Policy
	if json.Unmarshal(version.MemoryPolicy, &policy) != nil || !policy.Enabled {
		return messages, nil
	}
	query := ""
	for i := len(messages) - 1; i >= 0 && query == ""; i-- {
		if messages[i].Role != aimodelruntime.RoleUser {
			continue
		}
		for _, part := range messages[i].Content {
			if part.Type == aimodelruntime.PartInputText {
				query = strings.TrimSpace(part.Text)
				if query != "" {
					break
				}
			}
		}
	}
	if query == "" {
		return messages, nil
	}
	response, err := s.memoryRetriever(ctx, MemoryRecallRequest{Query: query, AgentID: trusted.AgentID, Types: []aimemory.ItemType{aimemory.TypeFact, aimemory.TypeContext, aimemory.TypeEvent}, Limit: policy.MaxRecallItems, MaxTokens: policy.MaxRecallTokens, NormalOnly: true})
	if err != nil || len(response.Items) == 0 {
		return messages, nil
	}
	refs := make([]MemoryProvenanceRef, len(response.Items))
	safe := make([]map[string]any, len(response.Items))
	for i, item := range response.Items {
		refs[i] = item.Provenance
		safe[i] = map[string]any{"type": item.Type, "summary": item.Summary, "content": item.Content, "occurred_at": item.OccurredAt, "occurred_end_at": item.OccurredEndAt, "occurred_timezone": item.OccurredTimezone, "local_date": item.LocalDate, "date_precision": item.DatePrecision, "score": item.Score, "provenance": item.Provenance}
	}
	body, _ := json.Marshal(safe)
	section := aimodelruntime.Message{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: prompt.Render("ai/untrusted-user-memory", map[string]any{"Payload": string(body)})}}}
	index := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == aimodelruntime.RoleUser {
			index = i
			break
		}
	}
	out := make([]aimodelruntime.Message, 0, len(messages)+1)
	out = append(out, messages[:index]...)
	out = append(out, section)
	out = append(out, messages[index:]...)
	return out, refs
}

func (s *AgentRuntimeService) attachMemoryRecallRefs(ctx context.Context, snapshotID string, refs []MemoryProvenanceRef) error {
	body, _ := json.Marshal(refs)
	return s.db.WithContext(ctx).Model(&ContextSnapshot{}).Where("id=? AND memory_recall_refs='[]'::jsonb", snapshotID).Update("memory_recall_refs", body).Error
}

func resultStatus(result *aiconversation.CompactionResult) string {
	if result == nil {
		return "failed"
	}
	return result.Status
}

func (s *AgentRuntimeService) streamModel(ctx context.Context, run *AgentRun, request aimodelruntime.GenerateRequest) (aimodelruntime.GenerateResponse, error) {
	value, err := s.invocations.Stream(ctx, request)
	if err != nil {
		return aimodelruntime.GenerateResponse{}, err
	}
	stream, ok := value.(aimodelruntime.ResultStream)
	if !ok {
		return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{
			Category: aimodelruntime.ErrorStreamProtocol,
			Message:  fmt.Sprintf("invalid model stream result: %T", value),
		}
	}
	return aimodelruntime.CollectStream(stream, func(event aimodelruntime.StreamEvent) error {
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.appendEvent(ctx, tx, run, "run.delta", nil, map[string]any{"delta": event.Delta})
		})
	})
}

func (s *AgentRuntimeService) attachModelInvocation(ctx context.Context, run *AgentRun, step *AgentStep, invocationID string) error {
	return s.attachModelInvocationMode(ctx, run, step, invocationID, "stream")
}

func (s *AgentRuntimeService) attachModelInvocationMode(ctx context.Context, run *AgentRun, step *AgentStep, invocationID, mode string) error {
	if invocationID == "" {
		var invocation modelruntime.ModelInvocation
		if err := s.db.WithContext(ctx).Where("agent_run_id=? AND mode=?", run.ID, mode).Order("created_at DESC").First(&invocation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		invocationID = invocation.ID
	}
	step.ModelInvocationID = &invocationID
	return s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=? AND run_id=?", step.ID, run.ID).Update("model_invocation_id", invocationID).Error
}

func modelFailureCategory(err error) string {
	var runtimeErr *aimodelruntime.RuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.Category != "" {
		return string(runtimeErr.Category)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return string(aimodelruntime.ErrorTimeout)
	}
	if errors.Is(err, context.Canceled) {
		return string(aimodelruntime.ErrorCancelled)
	}
	return "model_failed"
}

func (s *AgentRuntimeService) terminateModelError(ctx context.Context, run *AgentRun, category string) error {
	if category == string(aimodelruntime.ErrorCancelled) {
		return s.terminalLike(ctx, run, "cancelled", category, "", true)
	}
	return s.fail(ctx, run, category)
}

func asGenerateResponse(value any) (aimodelruntime.GenerateResponse, bool) {
	switch response := value.(type) {
	case aimodelruntime.GenerateResponse:
		return response, true
	case *aimodelruntime.GenerateResponse:
		if response != nil {
			return *response, true
		}
	}
	return aimodelruntime.GenerateResponse{}, false
}

func (s *AgentRuntimeService) ensureToolScope(ctx context.Context, run *AgentRun, agentRow *agentpkg.Agent) (string, error) {
	var existing tooling.ToolScope
	if err := s.db.WithContext(ctx).Where("principal_id=? AND agent_run_id=? AND expires_at>?", run.UserID, run.ID, s.now()).First(&existing).Error; err == nil {
		return existing.ID, nil
	}
	bindings, err := agentpkg.ListEnabledToolBindings(ctx, s.db.WithContext(ctx), agentRow.ID)
	if err != nil {
		return "", err
	}
	allowed := []string{}
	allowedSources := []string{}
	allowedCapabilities := []string{}
	for _, b := range bindings {
		if s.tools.Registry().IsBindable(b.ToolID) {
			allowed = append(allowed, b.ToolID)
		}
	}
	if s.contracts != nil && agentRow.ContractKey != "" {
		if contract, ok := s.contracts.Get(agentRow.ContractKey, agentRow.ContractVersion); ok {
			for _, id := range contract.RequiredToolSelectors {
				group := s.tools.Registry().BindingGroup(id)
				if len(group) == 0 {
					allowed = append(allowed, id)
					continue
				}
				for _, member := range group {
					allowed = append(allowed, member.ID)
				}
			}
			allowedSources = append(allowedSources, contract.AllowedToolSources...)
			allowedCapabilities = append(allowedCapabilities, contract.AllowedToolCapabilities...)
		}
	}
	rid := run.ID
	conversationID := run.ConversationID
	scope := tooling.ToolScope{ID: uuid.NewString(), TenantID: run.TenantID, PrincipalID: run.UserID, PrincipalKind: run.PrincipalKind, ClientSurface: run.ClientSurface, Audience: run.Audience, Realm: string(auth.RealmTenant), SessionID: run.SessionID, AuthVersion: run.AuthVersion, AgentRunID: &rid, ConversationID: &conversationID, AllowedToolsJSON: jsonBytes(allowed), AllowedSourcesJSON: jsonBytes(allowedSources), AllowedCapabilitiesJSON: jsonBytes(allowedCapabilities), LoadedToolsJSON: jsonBytes([]string{}), ReferenceEvidenceJSON: jsonBytes([]referenceEvidence{}), ExpiresAt: s.now().Add(30 * time.Minute)}
	candidates, err := s.tools.WorkingSetCandidates(ctx, run.ConversationID, conversationToolRestoreLimit)
	if err != nil {
		return "", err
	}
	if err := s.db.WithContext(ctx).Create(&scope).Error; err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if _, activateErr := s.tools.ActivateIntoScope(ctx, scope.ID, candidate.ToolID); activateErr != nil {
			var appErr *errs.AppError
			if errors.As(activateErr, &appErr) && (appErr.Code == errs.CodeForbidden || appErr.Code == errs.CodeValidation) {
				continue
			}
			_ = s.db.WithContext(ctx).Where("id=?", scope.ID).Delete(&tooling.ToolScope{}).Error
			return "", activateErr
		}
	}
	return scope.ID, nil
}

func (s *AgentRuntimeService) executeToolCall(ctx context.Context, run *AgentRun, scopeID string, call aimodelruntime.OutputItem) (bool, error) {
	stepType := aiagentruntime.StepToolExecute
	toolID := s.registryToolID(call.ToolName)
	if toolID == "meta:tool.search" {
		stepType = aiagentruntime.StepToolSearch
	}
	if toolID == "meta:tool.load" {
		stepType = aiagentruntime.StepToolLoad
	}
	if metadata, ok := s.tools.Registry().SlotMetadata(toolID); ok {
		stepType = aiagentruntime.StepType(metadata.StepType)
	}
	step, stepErr := s.newStep(ctx, run, stepType, map[string]any{"tool_name": call.ToolName, "tool_call_id": call.ToolCallID})
	if stepErr != nil {
		return false, stepErr
	}
	if toolID != "" && call.ToolName == toolID {
		return s.appendToolResult(ctx, run, step, call, nil, &toolRuntimeError{category: "tool_name_invalid", summary: fmt.Sprintf("tool name uses a registry ID; load it first and call the projected model tool name %s", tooling.ModelToolName(toolID)), recoverable: true})
	}
	if toolID == "meta:tool.search" {
		var input aitool.MetaSearchInput
		if json.Unmarshal(call.Arguments, &input) != nil {
			return s.appendToolResult(ctx, run, step, call, nil, errs.Validation("invalid tool arguments"))
		}
		value, err := s.tools.Search(ctx, tooling.ToolSearchRequest{ScopeID: scopeID, Query: input.Query, Capability: input.Capability, Source: input.Source, Limit: input.Limit})
		return s.appendToolResult(ctx, run, step, call, value, err)
	}
	if toolID == "meta:tool.load" {
		var input aitool.MetaLoadInput
		if json.Unmarshal(call.Arguments, &input) != nil {
			return s.appendToolResult(ctx, run, step, call, nil, errs.Validation("invalid tool arguments"))
		}
		value, err := s.tools.Load(ctx, tooling.ToolLoadRequest{ScopeID: scopeID, ToolIDs: input.ToolIDs})
		if err != nil {
			return s.appendToolResult(ctx, run, step, call, nil, err)
		}
		loaded := value.(tooling.ToolLoadResponse)
		return s.appendToolResult(ctx, run, step, call, agentToolLoadResult(loaded), nil)
	}
	if toolID == "" {
		return s.appendToolResult(ctx, run, step, call, nil, &toolRuntimeError{category: "tool_unavailable", summary: "tool is unavailable; call meta__tool_search and meta__tool_load before retrying", recoverable: true})
	}
	projectedInScope, err := s.tools.IsProjected(ctx, scopeID, toolID)
	if err != nil {
		return s.appendToolResult(ctx, run, step, call, nil, err)
	}
	if !projectedInScope {
		workingSet, workingSetErr := s.tools.WorkingSetEntry(ctx, run.ConversationID, toolID)
		if errors.Is(workingSetErr, gorm.ErrRecordNotFound) {
			return s.appendToolResult(ctx, run, step, call, nil, &toolRuntimeError{category: "tool_not_loaded", summary: "tool is not loaded; call meta__tool_load first", recoverable: true})
		}
		if workingSetErr != nil {
			return s.appendToolResult(ctx, run, step, call, nil, workingSetErr)
		}
		allowed, allowanceErr := s.allowToolRehydrate(ctx, run.ID, toolID)
		if allowanceErr != nil {
			return s.appendToolResult(ctx, run, step, call, nil, allowanceErr)
		}
		if !allowed {
			return s.appendToolResult(ctx, run, step, call, nil, &toolRuntimeError{category: "tool_rehydrate_limit", summary: "tool restore limit reached; use tool.search and tool.load before retrying", recoverable: false})
		}
		activation, activateErr := s.tools.ActivateIntoScope(ctx, scopeID, toolID)
		if activateErr != nil {
			return s.appendToolResult(ctx, run, step, call, nil, activateErr)
		}
		if workingSet.SchemaHash != activation.Definition.SchemaHash {
			return s.appendRehydratedToolResult(ctx, run, step, call, toolID)
		}
	}
	loadedValue, err := s.tools.Load(ctx, tooling.ToolLoadRequest{ScopeID: scopeID, ToolIDs: []string{toolID}})
	if err != nil {
		return s.appendToolResult(ctx, run, step, call, nil, err)
	}
	loaded := loadedValue.(tooling.ToolLoadResponse).Tools[0]
	value, err := s.tools.Execute(ctx, tooling.ToolExecuteRequest{Handle: loaded.Handle, ToolCallID: call.ToolCallID, Arguments: call.Arguments, IdempotencyKey: fmt.Sprintf("run:%s:tool:%s", run.ID, call.ToolCallID)})
	if err != nil {
		return s.appendToolResult(ctx, run, step, call, nil, err)
	}
	result := value.(*aitool.Result)
	if result.Status == "confirmation_required" {
		confirmation, ok := result.Data.(tooling.ToolConfirmation)
		if !ok {
			return false, fmt.Errorf("invalid confirmation")
		}
		step.ConfirmationID = &confirmation.ID
		step.ToolInvocationID = &confirmation.InvocationID
		_ = s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=?", step.ID).Updates(map[string]any{"status": "waiting", "confirmation_id": confirmation.ID, "tool_invocation_id": confirmation.InvocationID}).Error
		confirmationStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepConfirmation, map[string]any{"confirmation_id": confirmation.ID, "risk": confirmation.Risk, "side_effect": confirmation.SideEffect})
		if stepErr != nil {
			return false, stepErr
		}
		confirmationStep.ParentStepID = &step.ID
		confirmationStep.ConfirmationID = &confirmation.ID
		confirmationStep.ToolInvocationID = &confirmation.InvocationID
		_ = s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=?", confirmationStep.ID).Updates(map[string]any{"status": "waiting", "parent_step_id": step.ID, "confirmation_id": confirmation.ID, "tool_invocation_id": confirmation.InvocationID}).Error
		return true, nil
	}
	return s.appendToolResult(ctx, run, step, call, result, nil)
}

func (s *AgentRuntimeService) finalizeConfirmationWait(ctx context.Context, run *AgentRun) error {
	var confirmationStep AgentStep
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=? AND type=? AND confirmation_id IS NOT NULL", run.ID, "waiting", string(aiagentruntime.StepConfirmation)).Order("step_index DESC").First(&confirmationStep).Error; err != nil {
		return err
	}
	var conversationRevision uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", run.ConversationID).First(&conversation).Error; err != nil {
			return err
		}
		if conversation.Status != "active" {
			return errs.New(errs.CodeConflict, "conversation is not active")
		}
		conversationRevision = conversation.Revision
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", run.ID, "running", run.LeaseOwner).Updates(map[string]any{
			"status":                "waiting_confirmation",
			"conversation_revision": conversationRevision,
			"lease_owner":           "",
			"lease_expires_at":      nil,
			"revision":              gorm.Expr("revision+1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "run lease changed")
		}
		return s.appendEvent(ctx, tx, run, "run.waiting_confirmation", &confirmationStep.ID, map[string]any{"confirmation_id": *confirmationStep.ConfirmationID})
	})
	if err != nil {
		return err
	}
	run.Status = "waiting_confirmation"
	run.ConversationRevision = conversationRevision
	return nil
}

func (s *AgentRuntimeService) registryToolID(name string) string {
	return resolveRegistryToolID(s.tools.Registry().List(), name)
}

func resolveRegistryToolID(definitions []aitool.Definition, name string) string {
	resolved := ""
	for _, definition := range definitions {
		if definition.ID == name || tooling.ModelToolName(definition.ID) == name {
			if resolved != "" && resolved != definition.ID {
				return ""
			}
			resolved = definition.ID
		}
	}
	return resolved
}

func (s *AgentRuntimeService) allowToolRehydrate(ctx context.Context, runID, toolID string) (bool, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&AgentStep{}).Where("run_id=? AND error_category=?", runID, "rehydrated").Count(&total).Error; err != nil {
		return false, err
	}
	if total >= toolRehydrateLimitPerRun {
		return false, nil
	}
	var repeated int64
	if err := s.db.WithContext(ctx).Model(&AgentStep{}).
		Where("run_id=? AND error_category=? AND output_summary->>'tool_id'=?", runID, "rehydrated", toolID).
		Count(&repeated).Error; err != nil {
		return false, err
	}
	return repeated == 0, nil
}

func (s *AgentRuntimeService) appendRehydratedToolResult(ctx context.Context, run *AgentRun, step *AgentStep, call aimodelruntime.OutputItem, toolID string) (bool, error) {
	result := &aitool.Result{CallID: call.ToolCallID, Status: "rehydrated", Summary: "tool restored; retry after its current schema is projected"}
	encoded, _ := json.Marshal(result)
	_, messageErr := s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "tool", fmt.Sprintf("run:%s:tool-result:%s", run.ID, call.ToolCallID), []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: call.ToolCallID, Result: encoded}})
	stepErr := s.finishStep(ctx, run, step, "completed", map[string]any{"tool_call_id": call.ToolCallID, "tool_id": toolID, "status": "rehydrated"}, "rehydrated")
	return false, errors.Join(messageErr, stepErr)
}

type modelToolLoadResult struct {
	ScopeID              string              `json:"scope_id"`
	Tools                []aitool.Definition `json:"tools"`
	NewlyLoadedToolIDs   []string            `json:"newly_loaded_tool_ids"`
	AlreadyLoadedToolIDs []string            `json:"already_loaded_tool_ids"`
}

func agentToolLoadResult(loaded tooling.ToolLoadResponse) modelToolLoadResult {
	newlyLoaded := make(map[string]bool, len(loaded.NewlyLoadedToolIDs))
	for _, id := range loaded.NewlyLoadedToolIDs {
		newlyLoaded[id] = true
	}
	definitions := make([]aitool.Definition, 0, len(loaded.NewlyLoadedToolIDs))
	for _, tool := range loaded.Tools {
		if newlyLoaded[tool.Definition.ID] {
			definitions = append(definitions, tool.Definition)
		}
	}
	return modelToolLoadResult{
		ScopeID:              loaded.ScopeID,
		Tools:                definitions,
		NewlyLoadedToolIDs:   loaded.NewlyLoadedToolIDs,
		AlreadyLoadedToolIDs: loaded.AlreadyLoadedToolIDs,
	}
}

func (s *AgentRuntimeService) appendAssistantToolCalls(ctx context.Context, run *AgentRun, step *AgentStep, text string, toolCalls []aimodelruntime.OutputItem) error {
	parts := make([]aimodelruntime.ContentPart, 0, len(toolCalls)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, aimodelruntime.ContentPart{Type: aimodelruntime.PartOutputText, Text: text})
	}
	for _, call := range toolCalls {
		args := call.Arguments
		if len(args) == 0 || !json.Valid(args) {
			args = json.RawMessage(`{}`)
		}
		part := aimodelruntime.ContentPart{
			Type:       aimodelruntime.PartToolCall,
			ToolCallID: call.ToolCallID,
			ToolName:   call.ToolName,
			Arguments:  args,
		}
		if toolID := s.registryToolID(call.ToolName); toolID != "" {
			if definition, ok := s.tools.Registry().Get(toolID); ok {
				presentation := aitool.PresentationFor(definition)
				part.Presentation = &presentation
			}
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return fmt.Errorf("empty assistant tool call message")
	}
	_, err := s.conversations.AppendRuntimeMessage(
		ctx,
		run.ConversationID,
		run.ID,
		"assistant",
		fmt.Sprintf("run:%s:assistant-tools:%s", run.ID, step.ID),
		parts,
	)
	return err
}

func (s *AgentRuntimeService) appendToolResult(ctx context.Context, run *AgentRun, step *AgentStep, call aimodelruntime.OutputItem, value any, err error) (bool, error) {
	if err != nil {
		failedResult, recoverable := toolExecutionFailure(call.ToolCallID, err)
		category := failedResult.ErrorCategory
		encoded, _ := json.Marshal(failedResult)
		_, messageErr := s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "tool", fmt.Sprintf("run:%s:tool-result:%s", run.ID, call.ToolCallID), []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: call.ToolCallID, Result: encoded}})
		stepErr := s.finishStep(ctx, run, step, "failed", map[string]any{"tool_call_id": call.ToolCallID, "reason": category}, category)
		if recoverable && messageErr == nil && stepErr == nil {
			return false, nil
		}
		return false, errors.Join(err, messageErr, stepErr)
	}
	encoded, _ := json.Marshal(value)
	var invocation tooling.ToolInvocation
	if s.db.WithContext(ctx).Where("agent_run_id=? AND tool_call_id=?", run.ID, call.ToolCallID).Order("created_at DESC").First(&invocation).Error == nil {
		step.ToolInvocationID = &invocation.ID
		_ = s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=?", step.ID).Update("tool_invocation_id", invocation.ID).Error
	}
	_, err = s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "tool", fmt.Sprintf("run:%s:tool-result:%s", run.ID, call.ToolCallID), []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: call.ToolCallID, Result: encoded}})
	if err == nil {
		summary := map[string]any{"tool_call_id": call.ToolCallID}
		category := ""
		status := "completed"
		toolID := ""
		for _, d := range s.tools.Registry().List() {
			if tooling.ModelToolName(d.ID) == call.ToolName {
				toolID = d.ID
				break
			}
		}
		if result, resultOK := value.(*aitool.Result); resultOK {
			category = result.ErrorCategory
			if result.Status == aitool.ResultStatusFailed {
				status = "failed"
			}
			if metadata, exists := s.tools.Registry().SlotMetadata(toolID); exists {
				data, _ := result.Data.(map[string]any)
				for _, observation := range metadata.Observations {
					switch observation.Source {
					case "result_data":
						if item, ok := data[observation.Field]; ok {
							summary[observation.Field] = item
						}
					case "resource_ref_count":
						summary[observation.Field] = len(result.ResourceRefs)
					}
				}
			}
			_ = s.finishStep(ctx, run, step, status, summary, category)
			return false, nil
		}
		_ = s.finishStep(ctx, run, step, status, summary, category)
	}
	return false, err
}

func (s *AgentRuntimeService) appendSyntheticToolFailure(ctx context.Context, run *AgentRun, call aimodelruntime.OutputItem, category, summary string) error {
	result := syntheticToolFailure(call.ToolCallID, category, summary)
	encoded, _ := json.Marshal(result)
	_, err := s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "tool", fmt.Sprintf("run:%s:tool-result:%s", run.ID, call.ToolCallID), []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: call.ToolCallID, Result: encoded}})
	return err
}

type DeferredConfirmationQueueItem struct {
	OriginalCallID string
	RetryCallID    string
	ToolName       string
	Arguments      json.RawMessage
	RetryPersisted bool
}

type DeferredConfirmationQueueState struct {
	Next          *DeferredConfirmationQueueItem
	Blocked       bool
	ExecutedCalls int
}

type canonicalToolCallBatch struct {
	Calls []aimodelruntime.ContentPart
}

type canonicalToolResult struct {
	Status        string `json:"status"`
	ErrorCategory string `json:"error_category"`
}

func DeferredConfirmationRetryCallID(runID, originalCallID string) string {
	digest := sha256.Sum256([]byte(runID + "\x00" + originalCallID))
	return fmt.Sprintf("confirmation_retry_%x", digest[:16])
}

func BuildDeferredConfirmationQueue(runID string, messages []ConversationMessage) DeferredConfirmationQueueState {
	batches := make([]canonicalToolCallBatch, 0)
	callsByID := make(map[string]aimodelruntime.ContentPart)
	retriesByOriginal := make(map[string][]aimodelruntime.ContentPart)
	resultsByID := make(map[string][]canonicalToolResult)
	for _, message := range messages {
		var parts []aimodelruntime.ContentPart
		if json.Unmarshal(message.Content, &parts) != nil {
			return DeferredConfirmationQueueState{Blocked: true}
		}
		if message.Role == string(aimodelruntime.RoleAssistant) {
			batch := canonicalToolCallBatch{}
			for _, part := range parts {
				if part.Type != aimodelruntime.PartToolCall || part.ToolCallID == "" {
					continue
				}
				if _, exists := callsByID[part.ToolCallID]; exists {
					return DeferredConfirmationQueueState{Blocked: true}
				}
				callsByID[part.ToolCallID] = part
				if part.RetryOfToolCallID != "" {
					retriesByOriginal[part.RetryOfToolCallID] = append(retriesByOriginal[part.RetryOfToolCallID], part)
					continue
				}
				batch.Calls = append(batch.Calls, part)
			}
			if len(batch.Calls) > 0 {
				batches = append(batches, batch)
			}
		}
		if message.Role == string(aimodelruntime.RoleTool) {
			for _, part := range parts {
				if part.Type != aimodelruntime.PartToolResult || part.ToolCallID == "" {
					continue
				}
				var result canonicalToolResult
				if json.Unmarshal(part.Result, &result) != nil {
					return DeferredConfirmationQueueState{Blocked: true}
				}
				resultsByID[part.ToolCallID] = append(resultsByID[part.ToolCallID], result)
			}
		}
	}
	for originalCallID, retries := range retriesByOriginal {
		original, exists := callsByID[originalCallID]
		if !exists || original.RetryOfToolCallID != "" || len(retries) != 1 {
			return DeferredConfirmationQueueState{Blocked: true}
		}
	}
	executedCalls := 0
	for callID, results := range resultsByID {
		if _, exists := callsByID[callID]; !exists || len(results) != 1 {
			return DeferredConfirmationQueueState{Blocked: true}
		}
		if results[0].ErrorCategory != "deferred_for_confirmation" {
			executedCalls++
		}
	}

batchLoop:
	for _, batch := range batches {
		firstDeferred := -1
		for index, call := range batch.Calls {
			results := resultsByID[call.ToolCallID]
			if len(results) == 1 && results[0].ErrorCategory == "deferred_for_confirmation" {
				firstDeferred = index
				break
			}
		}
		if firstDeferred < 0 {
			continue
		}
		if firstDeferred == 0 {
			return DeferredConfirmationQueueState{Blocked: true}
		}
		for _, call := range batch.Calls[:firstDeferred] {
			results := resultsByID[call.ToolCallID]
			if len(results) != 1 || results[0].Status != "completed" || results[0].ErrorCategory != "" {
				continue batchLoop
			}
		}
		for _, call := range batch.Calls[firstDeferred:] {
			results := resultsByID[call.ToolCallID]
			if len(results) != 1 || results[0].ErrorCategory != "deferred_for_confirmation" {
				return DeferredConfirmationQueueState{Blocked: true}
			}
			retryCallID := DeferredConfirmationRetryCallID(runID, call.ToolCallID)
			retries := retriesByOriginal[call.ToolCallID]
			if len(retries) > 1 {
				return DeferredConfirmationQueueState{Blocked: true}
			}
			if len(retries) == 0 {
				if _, collision := callsByID[retryCallID]; collision {
					return DeferredConfirmationQueueState{Blocked: true}
				}
				return DeferredConfirmationQueueState{Next: &DeferredConfirmationQueueItem{OriginalCallID: call.ToolCallID, RetryCallID: retryCallID, ToolName: call.ToolName, Arguments: append(json.RawMessage(nil), call.Arguments...)}, ExecutedCalls: executedCalls}
			}
			retry := retries[0]
			if retry.ToolCallID != retryCallID || retry.ToolName != call.ToolName || !sameJSON(retry.Arguments, call.Arguments) {
				return DeferredConfirmationQueueState{Blocked: true}
			}
			retryResults := resultsByID[retryCallID]
			if len(retryResults) == 0 {
				return DeferredConfirmationQueueState{Next: &DeferredConfirmationQueueItem{OriginalCallID: call.ToolCallID, RetryCallID: retryCallID, ToolName: call.ToolName, Arguments: append(json.RawMessage(nil), call.Arguments...), RetryPersisted: true}, ExecutedCalls: executedCalls}
			}
			if len(retryResults) != 1 || retryResults[0].Status != "completed" {
				continue batchLoop
			}
		}
	}
	return DeferredConfirmationQueueState{ExecutedCalls: executedCalls}
}

func sameJSON(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func (s *AgentRuntimeService) deferredConfirmationQueue(ctx context.Context, run *AgentRun) (DeferredConfirmationQueueState, error) {
	var messages []ConversationMessage
	err := s.db.WithContext(ctx).Where("conversation_id=? AND run_id=? AND status=?", run.ConversationID, run.ID, "committed").Order("sequence ASC").Find(&messages).Error
	if err != nil {
		return DeferredConfirmationQueueState{}, err
	}
	return BuildDeferredConfirmationQueue(run.ID, messages), nil
}

func (s *AgentRuntimeService) appendDeferredConfirmationRetry(ctx context.Context, run *AgentRun, item DeferredConfirmationQueueItem) error {
	arguments := item.Arguments
	if len(arguments) == 0 || !json.Valid(arguments) {
		arguments = json.RawMessage(`{}`)
	}
	part := aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: item.RetryCallID, RetryOfToolCallID: item.OriginalCallID, ToolName: item.ToolName, Arguments: arguments}
	if toolID := s.registryToolID(item.ToolName); toolID != "" {
		if definition, ok := s.tools.Registry().Get(toolID); ok {
			presentation := aitool.PresentationFor(definition)
			part.Presentation = &presentation
		}
	}
	_, err := s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "assistant", fmt.Sprintf("run:%s:confirmation-retry:%s", run.ID, item.RetryCallID), []aimodelruntime.ContentPart{part})
	return err
}

func (s *AgentRuntimeService) continueDeferredConfirmationQueue(ctx context.Context, run *AgentRun, scopeID string, maxToolCalls, stepLimit int) (waiting, toolResultsPending bool, resultErr error) {
	for {
		queue, err := s.deferredConfirmationQueue(ctx, run)
		if err != nil {
			return false, toolResultsPending, err
		}
		if queue.Blocked {
			return false, toolResultsPending, errs.New(errs.CodeConflict, "deferred confirmation queue is inconsistent")
		}
		if err = s.persistToolCallFloor(ctx, run, queue.ExecutedCalls); err != nil {
			return false, toolResultsPending, err
		}
		if queue.Next == nil {
			return false, toolResultsPending, nil
		}
		item := *queue.Next
		if item.RetryPersisted {
			var invocation tooling.ToolInvocation
			err = s.db.WithContext(ctx).Where("agent_run_id=? AND tool_call_id=?", run.ID, item.RetryCallID).Order("created_at DESC").First(&invocation).Error
			if err == nil {
				if err = s.persistToolCallFloor(ctx, run, queue.ExecutedCalls+1); err != nil {
					return false, toolResultsPending, err
				}
				switch invocation.Status {
				case tooling.ToolInvocationWaitingConfirmation:
					return true, toolResultsPending, s.finalizeConfirmationWait(ctx, run)
				case tooling.ToolInvocationCompleted, tooling.ToolInvocationFailed:
					var result aitool.Result
					if json.Unmarshal(invocation.ResultJSON, &result) != nil {
						return false, toolResultsPending, errs.New(errs.CodeConflict, "persisted confirmation retry result is unavailable")
					}
					var step AgentStep
					if stepErr := s.db.WithContext(ctx).Where("run_id=? AND tool_invocation_id=? AND status IN ?", run.ID, invocation.ID, []string{"running", "waiting"}).Order("step_index DESC").First(&step).Error; stepErr != nil {
						return false, toolResultsPending, stepErr
					}
					call := aimodelruntime.OutputItem{Type: aimodelruntime.PartToolCall, ToolCallID: item.RetryCallID, ToolName: item.ToolName, Arguments: item.Arguments}
					if _, appendErr := s.appendToolResult(ctx, run, &step, call, &result, nil); appendErr != nil {
						return false, toolResultsPending, appendErr
					}
					toolResultsPending = true
					if step.ParentStepID != nil {
						var parent AgentStep
						if parentErr := s.db.WithContext(ctx).Where("id=?", *step.ParentStepID).First(&parent).Error; parentErr != nil {
							return false, toolResultsPending, parentErr
						}
						parentStatus, category := "completed", ""
						if invocation.Status == tooling.ToolInvocationFailed {
							parentStatus, category = "failed", result.ErrorCategory
						}
						if parentErr := s.finishStep(ctx, run, &parent, parentStatus, map[string]any{"confirmation_id": invocation.ConfirmationID, "executed": true, "recovered": true}, category); parentErr != nil {
							return false, toolResultsPending, parentErr
						}
					}
					continue
				default:
					return false, toolResultsPending, errs.New(errs.CodeConflict, "persisted confirmation retry execution is not recoverable")
				}
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return false, toolResultsPending, err
			}
		}
		if run.ToolCalls >= maxToolCalls {
			return false, toolResultsPending, errs.New(errs.CodeConflict, "tool budget exhausted while continuing confirmations")
		}
		if run.CurrentStep+2 > stepLimit {
			return false, toolResultsPending, errs.New(errs.CodeConflict, "step budget exhausted while continuing confirmations")
		}
		if !item.RetryPersisted {
			if err = s.appendDeferredConfirmationRetry(ctx, run, item); err != nil {
				return false, toolResultsPending, err
			}
		}
		call := aimodelruntime.OutputItem{Type: aimodelruntime.PartToolCall, ToolCallID: item.RetryCallID, ToolName: item.ToolName, Arguments: item.Arguments}
		waiting, executeErr := s.executeToolCall(ctx, run, scopeID, call)
		if executeErr != nil {
			return false, toolResultsPending, executeErr
		}
		run.ToolCalls++
		if err = s.persistCounters(ctx, run); err != nil {
			return false, toolResultsPending, err
		}
		if waiting {
			return true, toolResultsPending, s.finalizeConfirmationWait(ctx, run)
		}
		toolResultsPending = true
	}
}

func (s *AgentRuntimeService) persistToolCallFloor(ctx context.Context, run *AgentRun, floor int) error {
	if run.ToolCalls >= floor {
		return nil
	}
	run.ToolCalls = floor
	return s.persistCounters(ctx, run)
}

func syntheticToolFailure(callID, category, summary string) *aitool.Result {
	return &aitool.Result{CallID: callID, Status: "failed", Summary: summary, ErrorCategory: category}
}

type toolRuntimeError struct {
	category    string
	summary     string
	recoverable bool
}

func (e *toolRuntimeError) Error() string { return e.summary }

func toolExecutionFailure(callID string, err error) (*aitool.Result, bool) {
	var runtimeErr *toolRuntimeError
	if errors.As(err, &runtimeErr) {
		return &aitool.Result{CallID: callID, Status: "failed", Summary: runtimeErr.summary, ErrorCategory: runtimeErr.category}, runtimeErr.recoverable
	}
	category := toolExecutionErrorCategory(err)
	summary := "tool execution failed"
	recoverable := category == "invalid_arguments"
	var details *errs.SafeDetails
	if recoverable {
		var appErr *errs.AppError
		if errors.As(err, &appErr) && strings.TrimSpace(appErr.Message) != "" {
			summary = appErr.Message
			details = appErr.Details
			if appErr.Details != nil && appErr.Details.Reason != "" {
				category = appErr.Details.Reason
			}
		} else {
			summary = "tool arguments are invalid; correct them to match the input schema and retry"
		}
	}
	return &aitool.Result{CallID: callID, Status: "failed", Summary: summary, ErrorCategory: category, ErrorDetails: details}, recoverable
}

func toolExecutionErrorCategory(err error) string {
	var appErr *errs.AppError
	if !errors.As(err, &appErr) {
		return "execution_failed"
	}
	switch appErr.Code {
	case errs.CodeValidation:
		return "invalid_arguments"
	case errs.CodeUnauthorized, errs.CodeForbidden:
		return "authorization_failed"
	case errs.CodeConflict, errs.CodeRevisionConflict:
		return "authorization_changed"
	case errs.CodeNotFound, errs.CodeActionNotFound:
		return "tool_unavailable"
	default:
		return "execution_failed"
	}
}

func (s *AgentRuntimeService) resumeConfirmation(ctx context.Context, run *AgentRun) (bool, error) {
	var step AgentStep
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=? AND confirmation_id IS NOT NULL", run.ID, "waiting").Order("step_index DESC").First(&step).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var c tooling.ToolConfirmation
	if err := s.db.WithContext(ctx).Where("principal_id=? AND id=?", run.UserID, *step.ConfirmationID).First(&c).Error; err != nil {
		return false, s.fail(ctx, run, "confirmation_missing")
	}
	call := aimodelruntime.OutputItem{ToolCallID: c.ToolCallID, ToolName: tooling.ModelToolName(c.ToolID)}
	if c.Status == tooling.ToolConfirmationPending {
		if c.ExpiresAt.After(s.now()) {
			return false, s.waitForConfirmation(ctx, run, c.ID)
		}
		return false, s.finishRejectedConfirmation(ctx, run, &step, call, "confirmation_expired", "tool call was not executed because confirmation expired")
	}
	if c.Status == tooling.ToolConfirmationRejected {
		return false, s.finishRejectedConfirmation(ctx, run, &step, call, "confirmation_rejected", "tool call was not executed because confirmation was rejected")
	}
	if c.Status == tooling.ToolConfirmationExpired {
		return false, s.finishRejectedConfirmation(ctx, run, &step, call, "confirmation_expired", "tool call was not executed because confirmation expired")
	}
	if c.Status == tooling.ToolConfirmationCancelled {
		return false, s.finishRejectedConfirmation(ctx, run, &step, call, "confirmation_cancelled", "tool call was not executed because confirmation was cancelled")
	}
	if c.Status != tooling.ToolConfirmationConsumed {
		return false, s.fail(ctx, run, "confirmation_execution_unavailable")
	}
	if s.currentConversationRevision(ctx, run) != run.ConversationRevision {
		return false, s.fail(ctx, run, "conversation_revision_changed")
	}
	var inv tooling.ToolInvocation
	if err := s.db.WithContext(ctx).Where("id=? AND status IN ?", c.InvocationID, []string{tooling.ToolInvocationCompleted, tooling.ToolInvocationFailed}).First(&inv).Error; err != nil {
		return false, s.fail(ctx, run, "confirmation_execution_unavailable")
	}
	var result aitool.Result
	if json.Unmarshal(inv.ResultJSON, &result) != nil {
		return false, s.fail(ctx, run, "confirmation_result_unavailable")
	}
	_, err := s.appendToolResult(ctx, run, &step, call, &result, nil)
	if err == nil && step.ParentStepID != nil {
		var parent AgentStep
		if s.db.WithContext(ctx).Where("id=?", *step.ParentStepID).First(&parent).Error == nil {
			parentStatus, category := "completed", ""
			if inv.Status == tooling.ToolInvocationFailed {
				parentStatus, category = "failed", result.ErrorCategory
			}
			_ = s.finishStep(ctx, run, &parent, parentStatus, map[string]any{"confirmation_id": c.ID, "executed": true}, category)
		}
	}
	return err == nil && inv.Status == tooling.ToolInvocationCompleted, err
}

func (s *AgentRuntimeService) finishRejectedConfirmation(ctx context.Context, run *AgentRun, step *AgentStep, call aimodelruntime.OutputItem, category, summary string) error {
	messageErr := s.appendSyntheticToolFailure(ctx, run, call, category, summary)
	stepErr := s.finishStep(ctx, run, step, "failed", map[string]any{"tool_call_id": call.ToolCallID, "reason": category}, category)
	if step.ParentStepID != nil {
		var parent AgentStep
		if s.db.WithContext(ctx).Where("id=?", *step.ParentStepID).First(&parent).Error == nil {
			stepErr = errors.Join(stepErr, s.finishStep(ctx, run, &parent, "failed", map[string]any{"confirmation_id": step.ConfirmationID, "reason": category}, category))
		}
	}
	if err := errors.Join(messageErr, stepErr); err != nil {
		return err
	}
	return s.fail(ctx, run, category)
}

func stepLimitAfterConfirmation(configured, current int, resumed bool) int {
	if resumed && current+confirmationResumeStepAllowance > configured {
		return current + confirmationResumeStepAllowance
	}
	return configured
}
func (s *AgentRuntimeService) currentConversationRevision(ctx context.Context, run *AgentRun) uint64 {
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return 0
	}
	rows, err := tenantdata.TenantRaw[struct{ Revision uint64 }](tdb, `SELECT revision FROM ai_conversations WHERE tenant_id=? AND id=? LIMIT 1`, run.ConversationID)
	if err != nil || len(rows) != 1 {
		return 0
	}
	return rows[0].Revision
}
func (s *AgentRuntimeService) pause(ctx context.Context, run *AgentRun, reason string) error {
	return s.terminalLike(ctx, run, "paused", reason, "", false)
}
func (s *AgentRuntimeService) waitForConfirmation(ctx context.Context, run *AgentRun, confirmationID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", run.ID, "running", run.LeaseOwner).Updates(map[string]any{"status": "waiting_confirmation", "lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "run lease changed")
		}
		return s.appendEvent(ctx, tx, run, "run.waiting_confirmation", nil, map[string]any{"confirmation_id": confirmationID})
	})
}
func (s *AgentRuntimeService) fail(ctx context.Context, run *AgentRun, category string) error {
	return s.terminalLike(ctx, run, "failed", category, "", true)
}
func (s *AgentRuntimeService) complete(ctx context.Context, run *AgentRun) error {
	return s.completeWithReason(ctx, run, "")
}
func (s *AgentRuntimeService) completeWithReason(ctx context.Context, run *AgentRun, completionReason string) error {
	return s.terminalLike(ctx, run, "completed", "", completionReason, true)
}

type terminalOperationSummary struct {
	Completed  int `gorm:"column:completed"`
	Failed     int `gorm:"column:failed"`
	Unfinished int `gorm:"column:unfinished"`
}

func loadTerminalOperationSummary(tx *gorm.DB, runID string) (terminalOperationSummary, error) {
	var summary terminalOperationSummary
	err := tx.Model(&AgentStep{}).Where("run_id=? AND type=?", runID, string(aiagentruntime.StepToolExecute)).
		Select(`COUNT(*) FILTER (WHERE status='completed' AND tool_invocation_id IS NOT NULL) AS completed,
			COUNT(*) FILTER (WHERE status='failed' AND tool_invocation_id IS NOT NULL) AS failed,
			COUNT(*) FILTER (WHERE status NOT IN ('completed','failed','cancelled')) AS unfinished`).
		Scan(&summary).Error
	return summary, err
}

func terminalSafeArgs(decision aiagentruntime.TerminalDecision, operations terminalOperationSummary) json.RawMessage {
	value, _ := json.Marshal(map[string]any{
		"retryable":                  decision.Retryable,
		"completed_operation_count":  operations.Completed,
		"failed_operation_count":     operations.Failed,
		"unfinished_operation_count": operations.Unfinished,
	})
	return value
}

func terminalDecision(run *AgentRun, status, category, completionReason string) aiagentruntime.TerminalDecision {
	if status == string(aiagentruntime.RunCompleted) && category == "" {
		decision := aiagentruntime.TerminalDecision{
			ProtocolVersion:  aiagentruntime.TerminalProtocolVersion,
			Status:           aiagentruntime.RunCompleted,
			TerminalReason:   completionReason,
			CompletionReason: completionReason,
			Mode:             aiagentruntime.FinalizationNone,
		}
		if completionReason == "budget_limit" {
			decision.Mode = aiagentruntime.FinalizationModel
			decision.UserMessageKey = "ai.run.terminal.budget_limit"
			decision.SafeSummary = "run_budget_exhausted"
		}
		return decision
	}
	return aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{
		Status:        aiagentruntime.RunStatus(status),
		ErrorCategory: category,
	})
}

func (s *AgentRuntimeService) terminalLike(ctx context.Context, run *AgentRun, status, category, completionReason string, finished bool) error {
	return s.terminalWithDecision(ctx, run, status, category, completionReason, finished, terminalDecision(run, status, category, completionReason))
}

func (s *AgentRuntimeService) terminalWithDecision(ctx context.Context, run *AgentRun, status, category, completionReason string, finished bool, decision aiagentruntime.TerminalDecision) error {
	if err := aiagentruntime.ValidateRunTransition(aiagentruntime.RunStatus(run.Status), aiagentruntime.RunStatus(status)); err != nil {
		return errs.New(errs.CodeConflict, err.Error())
	}
	now := s.now()
	updates := map[string]any{
		"status": status, "error_category": category, "completion_reason": completionReason,
		"terminal_protocol_version": decision.ProtocolVersion, "terminal_reason": decision.TerminalReason,
		"finalization_mode": decision.Mode, "retryable": decision.Retryable,
		"user_message_key": decision.UserMessageKey, "safe_summary": decision.SafeSummary,
		"terminal_message_id": run.TerminalMessageID,
		"lease_owner":         "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1"),
	}
	if finished {
		updates["finished_at"] = now
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operations, e := loadTerminalOperationSummary(tx, run.ID)
		if e != nil {
			return e
		}
		updates["completed_operation_count"] = operations.Completed
		updates["failed_operation_count"] = operations.Failed
		updates["unfinished_operation_count"] = operations.Unfinished
		safeArgs := terminalSafeArgs(decision, operations)
		updates["safe_args"] = safeArgs
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", run.ID, run.Status, run.LeaseOwner).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run lease changed")
		}
		event := "run." + status
		payload := map[string]any{
			"error_category": category, "completion_reason": completionReason,
			"terminal_protocol_version": decision.ProtocolVersion, "terminal_reason": decision.TerminalReason,
			"finalization_mode": decision.Mode, "finalization_attempted": run.FinalizationAttempted,
			"retryable": decision.Retryable, "user_message_key": decision.UserMessageKey,
			"safe_args": safeArgs, "safe_summary": decision.SafeSummary, "terminal_message_id": run.TerminalMessageID,
			"completed_operation_count": operations.Completed, "failed_operation_count": operations.Failed,
			"unfinished_operation_count": operations.Unfinished,
		}
		if e := s.appendEvent(ctx, tx, run, event, nil, payload); e != nil {
			return e
		}
		if finished && s.writer != nil {
			payload["run_id"], payload["status"] = run.ID, status
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunTerminalEvent, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: int64(run.Revision + 1), Payload: payload})
			return e
		}
		return nil
	})
	if err != nil {
		return err
	}
	run.Status = status
	run.ErrorCategory = category
	run.CompletionReason = completionReason
	run.TerminalProtocolVersion = decision.ProtocolVersion
	run.TerminalReason = decision.TerminalReason
	run.FinalizationMode = string(decision.Mode)
	run.Retryable = decision.Retryable
	run.UserMessageKey = decision.UserMessageKey
	run.SafeSummary = decision.SafeSummary
	if summary, summaryErr := loadTerminalOperationSummary(s.db.WithContext(ctx), run.ID); summaryErr == nil {
		run.CompletedOperationCount = summary.Completed
		run.FailedOperationCount = summary.Failed
		run.UnfinishedOperationCount = summary.Unfinished
	}
	run.SafeArgsJSON = terminalSafeArgs(decision, terminalOperationSummary{
		Completed: run.CompletedOperationCount, Failed: run.FailedOperationCount, Unfinished: run.UnfinishedOperationCount,
	})
	run.LeaseOwner = ""
	run.LeaseExpiresAt = nil
	if finished {
		run.FinishedAt = &now
	}
	return nil
}

var _ = action.StreamEvent{}
