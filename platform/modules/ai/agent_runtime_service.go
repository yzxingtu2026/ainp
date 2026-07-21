package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	agentRunRequestedEvent           = "ai.agent_run.requested.v1"
	agentRunResumeEvent              = "ai.agent_run.resume_requested.v1"
	agentRunTerminalEvent            = "ai.agent_run.terminal.v1"
	agentRunConfirmationTimeoutEvent = "ai.agent_run.confirmation_timeout.v1"
)

type AgentRuntimeService struct {
	db              tenantdata.GuardedDatabase
	conversations   *ConversationService
	invocations     *InvocationService
	tools           *ToolService
	compactor       aiconversation.ContextCompactor
	writer          outbox.Writer
	authorization   auth.AuthorizationProvider
	runtimeContext  runtimecontext.Provider
	memoryRetriever *MemoryRetrievalService
	contracts       *aiagent.Registry
	leaseDuration   time.Duration
	now             func() time.Time
}

func (s *AgentRuntimeService) SetMemoryRetriever(retriever *MemoryRetrievalService) {
	s.memoryRetriever = retriever
}

func (s *AgentRuntimeService) SetAgentContracts(contracts *aiagent.Registry) {
	s.contracts = contracts
}

func NewAgentRuntimeService(db *gorm.DB, c *ConversationService, i *InvocationService, t *ToolService, compactor aiconversation.ContextCompactor, w outbox.Writer, authorization auth.AuthorizationProvider, runtimeContext ...runtimecontext.Provider) *AgentRuntimeService {
	if err := registerAgentRuntimeTenantModels(); err != nil {
		panic(err)
	}
	var provider runtimecontext.Provider
	if len(runtimeContext) > 0 {
		provider = runtimeContext[0]
	}
	return &AgentRuntimeService{db: tenantdata.GuardedDB(db), conversations: c, invocations: i, tools: t, compactor: compactor, writer: w, authorization: authorization, runtimeContext: provider, leaseDuration: 30 * time.Second, now: func() time.Time { return time.Now().UTC() }}
}

type AgentRunStartRequest struct {
	ConversationID   string                `json:"conversation_id" validate:"required"`
	TriggerMessageID string                `json:"trigger_message_id" validate:"required"`
	Budget           aiagentruntime.Budget `json:"budget"`
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
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
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
	if err = s.db.WithContext(ctx).Where("user_id=? AND id=?", ac.UserID, id).First(&row).Error; err != nil {
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
	budget := r.Budget.Normalize()
	if err = budget.Validate(); err != nil {
		return nil, errs.New(errs.CodeValidation, err.Error())
	}
	budgetJSON, _ := json.Marshal(budget)
	traceID := strings.TrimSpace(ac.TraceID)
	if traceID == "" {
		traceID = ac.RequestID
	}
	run := AgentRun{ID: uuid.NewString(), TenantID: ac.TenantID, UserID: ac.UserID, ConversationID: conversation.ID, ConversationRevision: conversation.Revision, TriggerMessageID: trigger.ID, AgentVersionID: conversation.AgentVersionID, SessionID: ac.SessionID, AuthVersion: ac.AuthVersion, OriginRequestID: ac.RequestID, TraceID: traceID, Status: string(aiagentruntime.RunQueued), BudgetJSON: budgetJSON, IdempotencyKey: key, Revision: 1}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("user_id=? AND idempotency_key=?", ac.UserID, key).First(&run).Error; e == nil {
			return nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e := tx.Create(&run).Error; e != nil {
			return e
		}
		if e := s.appendEvent(ctx, tx, &run, "run.queued", nil, map[string]any{"conversation_revision": conversation.Revision}); e != nil {
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
	return map[string]any{"run": run, "steps": steps}, nil
}
func (s *AgentRuntimeService) List(ctx context.Context, r AgentRunListRequest) (any, error) {
	ac, err := runActor(ctx)
	if err != nil {
		return nil, err
	}
	r.Normalize()
	q := s.db.WithContext(ctx).Where("user_id=?", ac.UserID)
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRun{}).Where("id=? AND revision=? AND status=?", run.ID, r.Revision, run.Status).Updates(map[string]any{"status": string(to), "finished_at": now, "lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run revision conflict")
		}
		if e := s.appendEvent(ctx, tx, run, "run.cancelled", nil, map[string]any{}); e != nil {
			return e
		}
		if s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunTerminalEvent, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: int64(run.Revision + 1), Payload: map[string]any{"run_id": run.ID, "status": "cancelled", "error_category": ""}})
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

func (s *AgentRuntimeService) OnConfirmationPending(ctx context.Context, tx *gorm.DB, c *ToolConfirmation) error {
	if s.writer == nil {
		return nil
	}
	var scope ToolScope
	if err := tx.WithContext(ctx).Where("id=?", c.ScopeID).First(&scope).Error; err != nil || scope.AgentRunID == nil {
		return err
	}
	_, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunConfirmationTimeoutEvent, AggregateType: "ai_agent_run", AggregateID: *scope.AgentRunID, AggregateVersion: 1, Payload: map[string]any{"run_id": *scope.AgentRunID, "confirmation_id": c.ID, "expires_at": c.ExpiresAt}})
	return err
}
func (s *AgentRuntimeService) OnConfirmationDecided(ctx context.Context, c *ToolConfirmation) error {
	var scope ToolScope
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
	var c ToolConfirmation
	if err := s.db.WithContext(ctx).Where("id=?", *step.ConfirmationID).First(&c).Error; err != nil {
		return false, err
	}
	if c.Status == "pending" && c.ExpiresAt.After(s.now()) {
		return false, nil
	}
	if auth.MustFromContext(ctx).TenantID == "" {
		ctx = auth.WithContext(ctx, &auth.Context{TenantID: run.TenantID, UserID: run.UserID, Realm: auth.RealmTenant, Authenticated: true})
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
	snapshot, authErr := s.authorization.LoadAuthorization(ctx, auth.AuthorizationRequest{TenantID: run.TenantID, UserID: run.UserID, SessionID: run.SessionID, Realm: auth.RealmTenant, AuthVersion: run.AuthVersion})
	if authErr != nil || snapshot.AuthVersion != run.AuthVersion {
		return s.fail(ctx, run, "authorization_changed")
	}
	ctx = auth.WithContext(ctx, &auth.Context{RequestID: run.OriginRequestID, TraceID: run.TraceID, TenantID: snapshot.TenantID, UserID: snapshot.UserID, SessionID: snapshot.SessionID, OrgID: snapshot.OrgID, DeptID: snapshot.DeptID, Roles: snapshot.Roles, Permissions: snapshot.Permissions, AuthVersion: snapshot.AuthVersion, Realm: auth.RealmTenant, Client: "agent", AgentRunID: run.ID, Authenticated: true})
	var budget aiagentruntime.Budget
	if json.Unmarshal(run.BudgetJSON, &budget) != nil {
		return s.fail(ctx, run, "invalid_budget")
	}
	budget = budget.Normalize()
	deadline := s.now().Add(time.Duration(budget.MaxDurationSeconds) * time.Second)
	repos, repoErr := newAgentRepositories(ctx, s.db.WithContext(ctx))
	if repoErr != nil {
		return s.fail(ctx, run, "tenant_repository_unavailable")
	}
	version, err := repos.versions.GetByID(run.AgentVersionID)
	if err != nil {
		return s.fail(ctx, run, "agent_version_unavailable")
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
	if conversation.Purpose == ConversationPurposeWorkspace && version.Kind == AgentVersionKindPreview {
		return s.fail(ctx, run, "preview_version_forbidden")
	}
	agentQuery := repos.agents.Where("id=?", version.AgentID)
	if conversation.Purpose == ConversationPurposeBuilderPreview {
		agentQuery = agentQuery.Where("status IN ?", []string{"enabled", "draft"})
	} else {
		agentQuery = agentQuery.Where("status = ?", "enabled")
	}
	agent, err := agentQuery.First("1=1")
	if err != nil {
		return s.fail(ctx, run, "agent_unavailable")
	}
	if agent.Status == "disabled" {
		return s.fail(ctx, run, "agent_unavailable")
	}
	trustedContext, err := s.resolveRuntimeContext(ctx, run, &conversation, &agent)
	if err != nil {
		return s.fail(ctx, run, "runtime_context_failed")
	}
	ctx = runtimecontext.WithTrusted(ctx, trustedContext)
	scopeID, err := s.ensureToolScope(ctx, run, &agent)
	if err != nil {
		return s.fail(ctx, run, "tool_scope_failed")
	}
	if err = s.resumeConfirmation(ctx, run); err != nil {
		return err
	}
	for run.CurrentStep < budget.MaxSteps && s.now().Before(deadline) {
		if run.ModelCalls >= budget.MaxModelCalls {
			return s.fail(ctx, run, "model_budget_exhausted")
		}
		messages, snapshotID, err := s.runtimeMessages(ctx, run, version.ModelAlias, budget, trustedContext)
		if err != nil {
			return s.fail(ctx, run, "context_failed")
		}
		run.ContextSnapshotID = snapshotID
		if snapshotID != nil {
			_ = s.attachRuntimeContextRefs(ctx, *snapshotID, trustedContext)
		}
		messages, recallRefs := s.injectMemoryRecall(ctx, run, version, messages, trustedContext)
		if snapshotID != nil && len(recallRefs) > 0 {
			_ = s.attachMemoryRecallRefs(ctx, *snapshotID, recallRefs)
		}
		step, err := s.newStep(ctx, run, aiagentruntime.StepModel, map[string]any{"message_count": len(messages), "context_snapshot_id": snapshotID})
		if err != nil {
			return err
		}
		request := aimodelruntime.GenerateRequest{ModelAlias: version.ModelAlias, ToolScopeID: scopeID, EnableTools: true, Instructions: version.TenantInstructions, Messages: messages, Metadata: map[string]string{"agent_id": agent.ID, "agent_run_id": run.ID}}
		response, callErr := s.streamModel(ctx, run, request)
		run.ModelCalls++
		if e := s.persistCounters(ctx, run); e != nil {
			return e
		}
		if callErr != nil {
			var re *aimodelruntime.RuntimeError
			if errors.As(callErr, &re) && re.Category == aimodelruntime.ErrorContextLength && run.Compactions < budget.MaxCompactions {
				_ = s.finishStep(ctx, run, step, "failed", map[string]any{}, string(re.Category))
				continue
			}
			_ = s.finishStep(ctx, run, step, "failed", map[string]any{}, "model_failed")
			return s.fail(ctx, run, "model_failed")
		}
		var invocation ModelInvocation
		if s.db.WithContext(ctx).Where("agent_run_id=?", run.ID).Order("created_at DESC").First(&invocation).Error == nil {
			step.ModelInvocationID = &invocation.ID
			_ = s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=?", step.ID).Update("model_invocation_id", invocation.ID).Error
		}
		if response.Usage.InputTokens != nil {
			run.InputTokens += *response.Usage.InputTokens
		}
		if response.Usage.OutputTokens != nil {
			run.OutputTokens += *response.Usage.OutputTokens
		}
		if e := s.persistCounters(ctx, run); e != nil {
			return e
		}
		if run.InputTokens+run.OutputTokens > budget.MaxTokens {
			_ = s.finishStep(ctx, run, step, "failed", map[string]any{}, "token_budget_exhausted")
			return s.fail(ctx, run, "token_budget_exhausted")
		}
		_ = s.finishStep(ctx, run, step, "completed", map[string]any{"finish_reason": response.FinishReason, "output_items": len(response.Output)}, "")
		toolCalls := []aimodelruntime.OutputItem{}
		text := ""
		for _, item := range response.Output {
			if item.Type == aimodelruntime.PartOutputText {
				text += item.Text
			}
			if item.Type == aimodelruntime.PartToolCall {
				toolCalls = append(toolCalls, item)
			}
		}
		if len(toolCalls) == 0 {
			finalStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepFinalize, map[string]any{"text_bytes": len(text)})
			if stepErr != nil {
				return stepErr
			}
			streamID, e := s.conversations.Begin(ctx, run.ConversationID, "run-final:"+run.ID, run.ID)
			if e != nil {
				return s.fail(ctx, run, "finalize_failed")
			}
			if e = s.conversations.Append(ctx, streamID, aiconversation.StreamDelta{Cursor: 1, Type: "output_text.delta", Data: []byte(text)}); e != nil {
				return s.fail(ctx, run, "finalize_failed")
			}
			_, e = s.conversations.Commit(ctx, streamID, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartOutputText, Text: text}})
			if e != nil {
				return s.fail(ctx, run, "finalize_failed")
			}
			_ = s.finishStep(ctx, run, finalStep, "completed", map[string]any{"message_committed": true}, "")
			return s.complete(ctx, run)
		}
		// Provider 要求 tool_result 前必须有配对的 assistant tool_call；先落库再执行工具。
		if e := s.appendAssistantToolCalls(ctx, run, step, text, toolCalls); e != nil {
			return s.fail(ctx, run, "assistant_tool_call_persist_failed")
		}
		for _, call := range toolCalls {
			if run.ToolCalls >= budget.MaxToolCalls {
				return s.fail(ctx, run, "tool_budget_exhausted")
			}
			waiting, err := s.executeToolCall(ctx, run, scopeID, call)
			if err != nil {
				return s.fail(ctx, run, "tool_failed")
			}
			run.ToolCalls++
			if err := s.persistCounters(ctx, run); err != nil {
				return err
			}
			if waiting {
				return nil
			}
		}
	}
	if !s.now().Before(deadline) {
		return s.fail(ctx, run, "timeout")
	}
	return s.fail(ctx, run, "step_budget_exhausted")
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

func (s *AgentRuntimeService) runtimeMessages(ctx context.Context, run *AgentRun, modelAlias string, budget aiagentruntime.Budget, trusted *runtimecontext.TrustedRuntimeContext) ([]aimodelruntime.Message, *string, error) {
	assembleStep, stepErr := s.newStep(ctx, run, aiagentruntime.StepContextAssemble, map[string]any{"conversation_revision": s.currentConversationRevision(ctx, run)})
	if stepErr != nil {
		return nil, nil, stepErr
	}
	assembly, err := s.conversations.Assemble(ctx, aiconversation.AssemblyRequest{ConversationID: run.ConversationID, MaxTokens: int(budget.MaxTokens), MaxBytes: int(budget.MaxTokens) * 8})
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
		result, err := s.compactor.Compact(ctx, aiconversation.CompactionRequest{ConversationID: run.ConversationID, ModelAlias: modelAlias, IdempotencyKey: fmt.Sprintf("run:%s:compact:%d", run.ID, run.Compactions), ParentSnapshotID: run.ContextSnapshotID})
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
		_ = s.finishStep(ctx, run, compactStep, "completed", map[string]any{"status": result.Status, "snapshot_id": result.SnapshotID, "input_tokens": result.InputTokens, "output_tokens": result.OutputTokens}, "")
		if result.Status == "compacted" {
			if run.Compactions >= budget.MaxCompactions {
				return nil, nil, fmt.Errorf("compaction budget exhausted")
			}
			run.Compactions++
			id := result.SnapshotID
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
			return injectTrustedRuntimeContext(messages, trusted), &id, nil
		}
	}
	messages := make([]aimodelruntime.Message, 0, len(assembly.Messages))
	for _, m := range assembly.Messages {
		messages = append(messages, aimodelruntime.Message{Role: m.Role, Content: m.Content})
	}
	return injectTrustedRuntimeContext(messages, trusted), nil, nil
}

func (s *AgentRuntimeService) resolveRuntimeContext(ctx context.Context, run *AgentRun, conversation *Conversation, agent *Agent) (*runtimecontext.TrustedRuntimeContext, error) {
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
	trusted, err := s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: run.TenantID, UserID: run.UserID, ConversationID: conversation.ID, ConversationCreatedAt: conversation.CreatedAt, AgentID: agent.ID, AgentVersionID: run.AgentVersionID, RunID: run.ID, TimeSnapshot: existing})
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
			return s.runtimeContext.Build(ctx, runtimecontext.BuildRequest{TenantID: run.TenantID, UserID: run.UserID, ConversationID: conversation.ID, ConversationCreatedAt: conversation.CreatedAt, AgentID: agent.ID, AgentVersionID: run.AgentVersionID, RunID: run.ID, TimeSnapshot: &snap})
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

func (s *AgentRuntimeService) injectMemoryRecall(ctx context.Context, run *AgentRun, version AgentVersion, messages []aimodelruntime.Message, trusted *runtimecontext.TrustedRuntimeContext) ([]aimodelruntime.Message, []MemoryProvenanceRef) {
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
	response, err := s.memoryRetriever.Retrieve(ctx, MemoryRecallRequest{Query: query, AgentID: trusted.AgentID, Types: []aimemory.ItemType{aimemory.TypeFact, aimemory.TypeContext, aimemory.TypeEvent}, Limit: policy.MaxRecallItems, MaxTokens: policy.MaxRecallTokens, NormalOnly: true})
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
	stream, ok := value.(action.StreamResult)
	if !ok {
		return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{
			Category: aimodelruntime.ErrorStreamProtocol,
			Message:  fmt.Sprintf("invalid model stream result: %T", value),
		}
	}
	if stream.Cancel != nil {
		defer stream.Cancel()
	}

	response := aimodelruntime.GenerateResponse{FinishReason: aimodelruntime.FinishError}
	itemIndexes := map[string]int{}
	terminal := false

	addItem := func(event aimodelruntime.StreamEvent, fallbackType aimodelruntime.PartType) int {
		if index, exists := itemIndexes[event.ItemID]; exists {
			return index
		}
		item := aimodelruntime.OutputItem{ID: event.ItemID, Type: fallbackType}
		if event.Item != nil {
			item = *event.Item
			if item.ID == "" {
				item.ID = event.ItemID
			}
		}
		response.Output = append(response.Output, item)
		index := len(response.Output) - 1
		itemIndexes[event.ItemID] = index
		return index
	}

	for wrapped := range stream.Events {
		event, ok := wrapped.Data.(aimodelruntime.StreamEvent)
		if !ok {
			return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{
				Category: aimodelruntime.ErrorStreamProtocol,
				Message:  fmt.Sprintf("invalid model stream event: %T", wrapped.Data),
			}
		}
		if event.ResponseID != "" {
			response.ID = event.ResponseID
			response.ProviderRequestID = event.ResponseID
		}
		switch event.Type {
		case aimodelruntime.EventOutputItemStarted:
			addItem(event, aimodelruntime.PartOutputText)
		case aimodelruntime.EventOutputTextDelta:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorStreamProtocol, Message: "text delta without output item"}
			}
			response.Output[index].Text += event.Delta
			if event.Delta != "" {
				if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					return s.appendEvent(ctx, tx, run, "run.delta", nil, map[string]any{"delta": event.Delta})
				}); err != nil {
					return aimodelruntime.GenerateResponse{}, err
				}
			}
		case aimodelruntime.EventToolCallStarted:
			addItem(event, aimodelruntime.PartToolCall)
		case aimodelruntime.EventToolArgumentsDelta:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorStreamProtocol, Message: "tool arguments without tool call"}
			}
			response.Output[index].Arguments = append(response.Output[index].Arguments, event.Delta...)
		case aimodelruntime.EventToolCallCompleted:
			index, exists := itemIndexes[event.ItemID]
			if !exists {
				return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorStreamProtocol, Message: "completed tool call without start"}
			}
			if event.Item != nil {
				if event.Item.ToolCallID != "" {
					response.Output[index].ToolCallID = event.Item.ToolCallID
				}
				if event.Item.ToolName != "" {
					response.Output[index].ToolName = event.Item.ToolName
				}
				if len(event.Item.Arguments) > 0 {
					response.Output[index].Arguments = append(json.RawMessage(nil), event.Item.Arguments...)
				}
			}
		case aimodelruntime.EventUsageUpdated:
			mergeStreamUsage(&response.Usage, event.Usage)
			if event.FinishReason != "" {
				response.FinishReason = event.FinishReason
			}
		case aimodelruntime.EventResponseCompleted:
			terminal = true
			response.FinishReason = event.FinishReason
			mergeStreamUsage(&response.Usage, event.Usage)
		case aimodelruntime.EventResponseFailed:
			if event.Error != nil {
				return aimodelruntime.GenerateResponse{}, event.Error
			}
			return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorProviderUnavailable, Message: "model stream failed"}
		}
	}
	if !terminal {
		return aimodelruntime.GenerateResponse{}, &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorStreamProtocol, Message: "model stream ended without terminal event"}
	}
	return response, nil
}

func mergeStreamUsage(target *aimodelruntime.Usage, update *aimodelruntime.Usage) {
	if update == nil {
		return
	}
	if update.InputTokens != nil {
		target.InputTokens = update.InputTokens
	}
	if update.OutputTokens != nil {
		target.OutputTokens = update.OutputTokens
	}
	if update.TotalTokens != nil {
		target.TotalTokens = update.TotalTokens
	}
	if update.CachedInputTokens != nil {
		target.CachedInputTokens = update.CachedInputTokens
	}
	if update.ReasoningTokens != nil {
		target.ReasoningTokens = update.ReasoningTokens
	}
	target.Reported = target.Reported || update.Reported
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

func (s *AgentRuntimeService) ensureToolScope(ctx context.Context, run *AgentRun, agent *Agent) (string, error) {
	var existing ToolScope
	if err := s.db.WithContext(ctx).Where("user_id=? AND agent_run_id=? AND expires_at>?", run.UserID, run.ID, s.now()).First(&existing).Error; err == nil {
		return existing.ID, nil
	}
	repos, repoErr := newAgentRepositories(ctx, s.db.WithContext(ctx))
	if repoErr != nil {
		return "", repoErr
	}
	bindings, err := repos.tools.Find("agent_id=? AND status=?", agent.ID, "enabled")
	if err != nil {
		return "", err
	}
	allowed := []string{}
	allowedSources := []string{}
	allowedCapabilities := []string{}
	for _, b := range bindings {
		if s.tools.registry.IsBindable(b.ToolID) {
			allowed = append(allowed, b.ToolID)
		}
	}
	if s.contracts != nil && agent.ContractKey != "" {
		if contract, ok := s.contracts.Get(agent.ContractKey, agent.ContractVersion); ok {
			for _, id := range contract.RequiredToolSelectors {
				group := s.tools.registry.BindingGroup(id)
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
	scope := ToolScope{ID: uuid.NewString(), TenantID: run.TenantID, UserID: run.UserID, Realm: string(auth.RealmTenant), SessionID: run.SessionID, AuthVersion: run.AuthVersion, AgentRunID: &rid, AllowedToolsJSON: jsonBytes(allowed), AllowedSourcesJSON: jsonBytes(allowedSources), AllowedCapabilitiesJSON: jsonBytes(allowedCapabilities), LoadedToolsJSON: jsonBytes([]string{}), ExpiresAt: s.now().Add(30 * time.Minute)}
	return scope.ID, s.db.WithContext(ctx).Create(&scope).Error
}

func (s *AgentRuntimeService) executeToolCall(ctx context.Context, run *AgentRun, scopeID string, call aimodelruntime.OutputItem) (bool, error) {
	stepType := aiagentruntime.StepToolExecute
	if call.ToolName == modelToolName("meta:tool.search") {
		stepType = aiagentruntime.StepToolSearch
	}
	if call.ToolName == modelToolName("meta:tool.load") {
		stepType = aiagentruntime.StepToolLoad
	}
	toolID := ""
	for _, d := range s.tools.registry.List() {
		if modelToolName(d.ID) == call.ToolName {
			toolID = d.ID
			break
		}
	}
	if metadata, ok := s.tools.registry.SlotMetadata(toolID); ok {
		stepType = aiagentruntime.StepType(metadata.StepType)
	}
	step, stepErr := s.newStep(ctx, run, stepType, map[string]any{"tool_name": call.ToolName, "tool_call_id": call.ToolCallID})
	if stepErr != nil {
		return false, stepErr
	}
	if call.ToolName == modelToolName("meta:tool.search") {
		var input aitool.MetaSearchInput
		if json.Unmarshal(call.Arguments, &input) != nil {
			return false, fmt.Errorf("invalid search")
		}
		value, err := s.tools.Search(ctx, ToolSearchRequest{ScopeID: scopeID, Query: input.Query, Capability: input.Capability, Source: input.Source, Limit: input.Limit})
		return s.appendToolResult(ctx, run, step, call, value, err)
	}
	if call.ToolName == modelToolName("meta:tool.load") {
		var input aitool.MetaLoadInput
		if json.Unmarshal(call.Arguments, &input) != nil {
			return false, fmt.Errorf("invalid load")
		}
		value, err := s.tools.Load(ctx, ToolLoadRequest{ScopeID: scopeID, ToolIDs: input.ToolIDs})
		if err != nil {
			return s.appendToolResult(ctx, run, step, call, nil, err)
		}
		loaded := value.(ToolLoadResponse)
		definitions := make([]aitool.Definition, 0, len(loaded.Tools))
		for _, tool := range loaded.Tools {
			definitions = append(definitions, tool.Definition)
		}
		return s.appendToolResult(ctx, run, step, call, map[string]any{"scope_id": loaded.ScopeID, "tools": definitions}, nil)
	}
	if toolID == "" {
		return false, fmt.Errorf("unknown model tool")
	}
	loadedValue, err := s.tools.Load(ctx, ToolLoadRequest{ScopeID: scopeID, ToolIDs: []string{toolID}})
	if err != nil {
		return false, err
	}
	loaded := loadedValue.(ToolLoadResponse).Tools[0]
	value, err := s.tools.Execute(ctx, ToolExecuteRequest{Handle: loaded.Handle, ToolCallID: call.ToolCallID, Arguments: call.Arguments, IdempotencyKey: fmt.Sprintf("run:%s:tool:%s", run.ID, call.ToolCallID)})
	if err != nil {
		return false, err
	}
	result := value.(*aitool.Result)
	if result.Status == "confirmation_required" {
		confirmation, ok := result.Data.(ToolConfirmation)
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
		run.Status = "waiting_confirmation"
		run.ConversationRevision = s.currentConversationRevision(ctx, run)
		_ = s.db.WithContext(ctx).Model(&AgentRun{}).Where("id=? AND lease_owner=?", run.ID, run.LeaseOwner).Updates(map[string]any{"status": "waiting_confirmation", "conversation_revision": run.ConversationRevision, "lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1")}).Error
		_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.appendEvent(ctx, tx, run, "run.waiting_confirmation", &confirmationStep.ID, map[string]any{"confirmation_id": confirmation.ID})
		})
		return true, nil
	}
	return s.appendToolResult(ctx, run, step, call, result, nil)
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
		parts = append(parts, aimodelruntime.ContentPart{
			Type:       aimodelruntime.PartToolCall,
			ToolCallID: call.ToolCallID,
			ToolName:   call.ToolName,
			Arguments:  args,
		})
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
		_ = s.finishStep(ctx, run, step, "failed", map[string]any{}, "tool_failed")
		return false, err
	}
	encoded, _ := json.Marshal(value)
	var invocation ToolInvocation
	if s.db.WithContext(ctx).Where("agent_run_id=? AND tool_call_id=?", run.ID, call.ToolCallID).Order("created_at DESC").First(&invocation).Error == nil {
		step.ToolInvocationID = &invocation.ID
		_ = s.db.WithContext(ctx).Model(&AgentStep{}).Where("id=?", step.ID).Update("tool_invocation_id", invocation.ID).Error
	}
	_, err = s.conversations.AppendRuntimeMessage(ctx, run.ConversationID, run.ID, "tool", fmt.Sprintf("run:%s:tool-result:%s", run.ID, call.ToolCallID), []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: call.ToolCallID, Result: encoded}})
	if err == nil {
		summary := map[string]any{"tool_call_id": call.ToolCallID}
		category := ""
		toolID := ""
		for _, d := range s.tools.registry.List() {
			if modelToolName(d.ID) == call.ToolName {
				toolID = d.ID
				break
			}
		}
		if metadata, exists := s.tools.registry.SlotMetadata(toolID); exists {
			if result, resultOK := value.(*aitool.Result); resultOK {
				category = result.ErrorCategory
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
				status := "completed"
				if result.Status == "failed" {
					status = "failed"
				}
				_ = s.finishStep(ctx, run, step, status, summary, category)
				return false, nil
			}
		}
		_ = s.finishStep(ctx, run, step, "completed", summary, category)
	}
	return false, err
}

func (s *AgentRuntimeService) resumeConfirmation(ctx context.Context, run *AgentRun) error {
	var step AgentStep
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=? AND confirmation_id IS NOT NULL", run.ID, "waiting").Order("step_index DESC").First(&step).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	var c ToolConfirmation
	if err := s.db.WithContext(ctx).Where("user_id=? AND id=?", run.UserID, *step.ConfirmationID).First(&c).Error; err != nil {
		return s.fail(ctx, run, "confirmation_missing")
	}
	if c.Status == "pending" && c.ExpiresAt.After(s.now()) {
		return s.waitForConfirmation(ctx, run, c.ID)
	}
	if c.Status == "rejected" || c.ExpiresAt.Before(s.now()) {
		return s.fail(ctx, run, "confirmation_rejected")
	}
	if s.currentConversationRevision(ctx, run) != run.ConversationRevision {
		return s.fail(ctx, run, "conversation_revision_changed")
	}
	var inv ToolInvocation
	if err := s.db.WithContext(ctx).Where("id=? AND status=?", c.InvocationID, "completed").First(&inv).Error; err != nil {
		return s.fail(ctx, run, "confirmation_execution_unavailable")
	}
	var result aitool.Result
	if json.Unmarshal(inv.ResultJSON, &result) != nil {
		return s.fail(ctx, run, "confirmation_result_unavailable")
	}
	call := aimodelruntime.OutputItem{ToolCallID: c.ToolCallID, ToolName: modelToolName(c.ToolID)}
	_, err := s.appendToolResult(ctx, run, &step, call, &result, nil)
	if err == nil && step.ParentStepID != nil {
		var parent AgentStep
		if s.db.WithContext(ctx).Where("id=?", *step.ParentStepID).First(&parent).Error == nil {
			_ = s.finishStep(ctx, run, &parent, "completed", map[string]any{"confirmation_id": c.ID, "executed": true}, "")
		}
	}
	return err
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
	return s.terminalLike(ctx, run, "paused", reason, false)
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
	return s.terminalLike(ctx, run, "failed", category, true)
}
func (s *AgentRuntimeService) complete(ctx context.Context, run *AgentRun) error {
	return s.terminalLike(ctx, run, "completed", "", true)
}
func (s *AgentRuntimeService) terminalLike(ctx context.Context, run *AgentRun, status, category string, finished bool) error {
	if err := aiagentruntime.ValidateRunTransition(aiagentruntime.RunStatus(run.Status), aiagentruntime.RunStatus(status)); err != nil {
		return errs.New(errs.CodeConflict, err.Error())
	}
	now := s.now()
	updates := map[string]any{"status": status, "error_category": category, "lease_owner": "", "lease_expires_at": nil, "revision": gorm.Expr("revision+1")}
	if finished {
		updates["finished_at"] = now
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&AgentRun{}).Where("id=? AND status=? AND lease_owner=?", run.ID, run.Status, run.LeaseOwner).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "agent run lease changed")
		}
		event := "run." + status
		if e := s.appendEvent(ctx, tx, run, event, nil, map[string]any{"error_category": category}); e != nil {
			return e
		}
		if finished && s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: agentRunTerminalEvent, AggregateType: "ai_agent_run", AggregateID: run.ID, AggregateVersion: int64(run.Revision + 1), Payload: map[string]any{"run_id": run.ID, "status": status, "error_category": category}})
			return e
		}
		return nil
	})
}

var _ = action.StreamEvent{}
