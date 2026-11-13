package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const agentRunPageCaptureTimeoutEvent = "ai.agent_run.page_capture_timeout.v1"

type PageCaptureSubmitRequest struct {
	CaptureRequestID   string `json:"capture_request_id" validate:"required,max=64"`
	Status             string `json:"status,omitempty" validate:"omitempty,oneof=completed failed"`
	AttachmentID       string `json:"attachment_id,omitempty" validate:"omitempty,max=64"`
	AttachmentRevision string `json:"attachment_revision,omitempty" validate:"omitempty,max=128"`
	FailureReason      string `json:"failure_reason,omitempty" validate:"omitempty,oneof=render_failed upload_failed unavailable cancelled"`
}

func (r PageCaptureSubmitRequest) Validate(context.Context) error {
	mode := strings.TrimSpace(r.Status)
	if mode == "" || mode == "completed" {
		if strings.TrimSpace(r.AttachmentID) == "" || strings.TrimSpace(r.AttachmentRevision) == "" {
			return errs.Validation("attachment_id and attachment_revision are required when status is completed")
		}
		return nil
	}
	if mode == "failed" {
		if strings.TrimSpace(r.AttachmentID) != "" || strings.TrimSpace(r.AttachmentRevision) != "" {
			return errs.Validation("attachment fields must be omitted when status is failed")
		}
		return nil
	}
	return errs.Validation("invalid page capture submit status")
}

type PageCaptureSubmitResponse struct {
	CaptureRequestID string `json:"capture_request_id"`
	RunID            string `json:"run_id"`
	Status           string `json:"status"`
}

type pageCaptureAttachmentPolicy struct {
	db *gorm.DB
}

func NewPageCaptureAttachmentPolicy(db *gorm.DB) attachment.AccessPolicy {
	if err := registerAgentRuntimeTenantModels(); err != nil {
		panic(err)
	}
	return pageCaptureAttachmentPolicy{db: db}
}

func (pageCaptureAttachmentPolicy) ResourceType() string { return PageCaptureAttachmentResourceType }

func (p pageCaptureAttachmentPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	ac := action.NormalizeActor(auth.MustFromContext(ctx))
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" || ac.TenantID != tenantID || ac.PrincipalID != userID || field != PageCaptureAttachmentField || strings.TrimSpace(resourceID) == "" || p.db == nil {
		return errs.New(errs.CodeNotFound, "page capture not found")
	}
	tdb, err := tenantdata.TenantDB(ctx, attachment.ResourceTransaction(ctx, p.db))
	if err != nil {
		return errs.New(errs.CodeNotFound, "page capture not found")
	}
	rows, err := tenantdata.TenantRaw[PageCaptureRequest](tdb, `SELECT * FROM ai_page_capture_requests WHERE tenant_id=? AND principal_id=? AND id=? LIMIT 1`, userID, resourceID)
	if err != nil || len(rows) != 1 {
		return errs.New(errs.CodeNotFound, "page capture not found")
	}
	return nil
}

func (s *AgentRuntimeService) SetPageCaptureDependencies(assets aimodelruntime.AssetResolver, linker attachment.ResourceLinker) {
	s.captureAssets = assets
	s.attachmentLinker = linker
}

func (s *AgentRuntimeService) LookupRunModelAlias(ctx context.Context, runID string) (string, error) {
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return "", err
	}
	rows, err := tenantdata.TenantRaw[struct {
		ModelAlias string `gorm:"column:model_alias"`
	}](tdb, `SELECT v.model_alias FROM ai_agent_runs r JOIN ai_agent_versions v ON v.tenant_id=r.tenant_id AND v.id=r.agent_version_id WHERE r.tenant_id=? AND r.id=? LIMIT 1`, runID)
	if err != nil || len(rows) != 1 {
		return "", gorm.ErrRecordNotFound
	}
	return strings.TrimSpace(rows[0].ModelAlias), nil
}

func (s *AgentRuntimeService) beginClientFulfillment(ctx context.Context, run *AgentRun, step *AgentStep, pending tooling.ToolClientFulfillmentPending) error {
	if pending.Kind != PageCaptureFulfillmentKind {
		return errs.New(errs.CodeValidation, "unsupported client fulfillment kind")
	}
	step.ToolInvocationID = &pending.InvocationID
	now := s.now()
	timeoutSeconds := pending.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	expiresAt := now.Add(time.Duration(timeoutSeconds) * time.Second)
	paramsJSON, _ := json.Marshal(pending.Parameters)
	if len(paramsJSON) == 0 || string(paramsJSON) == "null" {
		paramsJSON = json.RawMessage(`{}`)
	}
	capture := PageCaptureRequest{
		ID:             uuid.NewString(),
		TenantID:       run.TenantID,
		PrincipalID:    run.UserID,
		PrincipalKind:  run.PrincipalKind,
		ClientSurface:  run.ClientSurface,
		Audience:       run.Audience,
		SessionID:      run.SessionID,
		AuthVersion:    run.AuthVersion,
		RunID:          run.ID,
		StepID:         step.ID,
		ScopeID:        pending.ScopeID,
		InvocationID:   pending.InvocationID,
		ToolCallID:     pending.ToolCallID,
		ToolID:         pending.ToolID,
		ParametersJSON: paramsJSON,
		Status:         PageCapturePending,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&AgentStep{}).Where("id=? AND status=?", step.ID, "running").Updates(map[string]any{
			"status":             "waiting",
			"tool_invocation_id": pending.InvocationID,
		}).Error; e != nil {
			return e
		}
		if e := tx.Create(&capture).Error; e != nil {
			return e
		}
		eventPayload := map[string]any{
			"capture_request_id": capture.ID,
			"kind":               pending.Kind,
			"tool_call_id":       pending.ToolCallID,
			"expires_at":         expiresAt,
		}
		if len(pending.Parameters) > 0 {
			eventPayload["parameters"] = pending.Parameters
		}
		if e := s.appendEvent(ctx, tx, run, aiagentruntime.RunEventCaptureRequest, &step.ID, eventPayload); e != nil {
			return e
		}
		if s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{
				EventType:        agentRunPageCaptureTimeoutEvent,
				AggregateType:    "ai_agent_run",
				AggregateID:      run.ID,
				AggregateVersion: 1,
				Payload: map[string]any{
					"run_id":             run.ID,
					"capture_request_id": capture.ID,
					"expires_at":         expiresAt,
				},
			})
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	step.Status = "waiting"
	return nil
}

func (s *AgentRuntimeService) finalizePageCaptureWait(ctx context.Context, run *AgentRun) error {
	var capture PageCaptureRequest
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=?", run.ID, PageCapturePending).Order("created_at DESC").First(&capture).Error; err != nil {
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
			"status":                string(aiagentruntime.RunPaused),
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
		return s.appendEvent(ctx, tx, run, "run.paused", &capture.StepID, map[string]any{
			"reason":             "waiting_page_capture",
			"capture_request_id": capture.ID,
		})
	})
	if err != nil {
		return err
	}
	run.Status = string(aiagentruntime.RunPaused)
	run.ConversationRevision = conversationRevision
	return nil
}

func (s *AgentRuntimeService) SubmitPageCapture(ctx context.Context, r PageCaptureSubmitRequest) (any, error) {
	ac, err := runActor(ctx)
	if err != nil {
		return nil, err
	}
	normalized := action.NormalizeActor(ac)
	requestID := strings.TrimSpace(r.CaptureRequestID)
	var capture PageCaptureRequest
	if err := s.db.WithContext(ctx).Where("id=? AND principal_id=?", requestID, normalized.PrincipalID).First(&capture).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "page capture request not found")
	}
	if capture.PrincipalKind != string(normalized.Principal.Kind) ||
		capture.ClientSurface != normalized.ClientSurface ||
		(capture.Audience != "" && normalized.Audience != "" && capture.Audience != normalized.Audience) ||
		capture.AuthVersion != normalized.AuthVersion {
		return nil, errs.New(errs.CodeForbidden, "page capture authorization changed")
	}
	mode := strings.TrimSpace(r.Status)
	if mode == "" {
		mode = "completed"
	}
	attachmentID := strings.TrimSpace(r.AttachmentID)
	attachmentRevision := strings.TrimSpace(r.AttachmentRevision)

	// Idempotent replay: same terminal submission returns no-op success.
	if capture.Status == PageCaptureSubmitted || capture.Status == PageCaptureConsumed {
		if mode == "completed" && capture.AttachmentID != nil && *capture.AttachmentID == attachmentID && capture.AttachmentRevision == attachmentRevision {
			return PageCaptureSubmitResponse{CaptureRequestID: capture.ID, RunID: capture.RunID, Status: capture.Status}, nil
		}
		return nil, errs.New(errs.CodeConflict, "page capture request has already been handled")
	}
	if capture.Status == PageCaptureFailed {
		if mode == "failed" {
			return PageCaptureSubmitResponse{CaptureRequestID: capture.ID, RunID: capture.RunID, Status: capture.Status}, nil
		}
		return nil, errs.New(errs.CodeConflict, "page capture request has already been handled")
	}
	if capture.Status != PageCapturePending {
		return nil, errs.New(errs.CodeConflict, "page capture request is no longer pending")
	}
	now := s.now()
	if !capture.ExpiresAt.After(now) {
		return nil, errs.New(errs.CodeConflict, "page capture request has expired")
	}
	var run AgentRun
	if err := s.db.WithContext(ctx).Where("id=? AND user_id=?", capture.RunID, normalized.PrincipalID).First(&run).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "agent run not found")
	}
	if run.Status != string(aiagentruntime.RunPaused) && run.Status != string(aiagentruntime.RunRunning) && run.Status != string(aiagentruntime.RunQueued) {
		return nil, errs.New(errs.CodeConflict, "agent run is no longer active")
	}

	if mode == "completed" {
		if s.captureAssets == nil {
			return nil, errs.New(errs.CodeInternal, "image asset resolver is unavailable")
		}
		resolved, resolveErr := s.captureAssets.Resolve(ctx, aimodelruntime.AssetResolveRequest{
			AttachmentID: attachmentID,
			Purpose:      "model_input",
		})
		if resolveErr != nil || resolved.Revision != attachmentRevision {
			if resolved != nil {
				clear(resolved.Data)
			}
			return nil, errs.New(errs.CodeValidation, "captured image attachment is unavailable or invalid")
		}
		clear(resolved.Data)
	}

	alreadyHandled := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked PageCaptureRequest
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND principal_id=?", capture.ID, normalized.PrincipalID).First(&locked).Error; e != nil {
			return e
		}
		if locked.Status == PageCaptureSubmitted || locked.Status == PageCaptureConsumed {
			if mode == "completed" && locked.AttachmentID != nil && *locked.AttachmentID == attachmentID && locked.AttachmentRevision == attachmentRevision {
				capture = locked
				alreadyHandled = true
				return nil
			}
			return errs.New(errs.CodeConflict, "page capture request has already been handled")
		}
		if locked.Status == PageCaptureFailed {
			if mode == "failed" {
				capture = locked
				alreadyHandled = true
				return nil
			}
			return errs.New(errs.CodeConflict, "page capture request has already been handled")
		}
		if locked.Status != PageCapturePending || !locked.ExpiresAt.After(now) {
			return errs.New(errs.CodeConflict, "page capture request is no longer pending")
		}

		var toolResult *aitool.Result
		nextCaptureStatus := PageCaptureSubmitted
		updates := map[string]any{
			"submitted_at": now,
			"updated_at":   now,
		}
		if mode == "completed" {
			if s.attachmentLinker != nil {
				if _, linkErr := s.attachmentLinker.BindResource(ctx, tx, attachment.ResourceLinkRequest{
					AttachmentID: attachmentID,
					ResourceType: PageCaptureAttachmentResourceType,
					ResourceID:   locked.ID,
					Field:        PageCaptureAttachmentField,
				}); linkErr != nil {
					return linkErr
				}
			}
			updates["status"] = PageCaptureSubmitted
			updates["attachment_id"] = attachmentID
			updates["attachment_revision"] = attachmentRevision
			toolResult = &aitool.Result{
				CallID:  locked.ToolCallID,
				Status:  aitool.ResultStatusCompleted,
				Summary: "captured current business page view",
				Data: map[string]any{
					"capture_status":      "ready",
					"attachment_id":       attachmentID,
					"attachment_revision": attachmentRevision,
					"resource_type":       PageCaptureAttachmentResourceType,
				},
				ResourceRefs: []string{"attachment:" + attachmentID},
			}
		} else {
			nextCaptureStatus = PageCaptureFailed
			updates["status"] = PageCaptureFailed
			updates["error_category"] = "capture_failed"
			toolResult = &aitool.Result{
				CallID:        locked.ToolCallID,
				Status:        aitool.ResultStatusFailed,
				Summary:       "client page capture failed; ask the user to describe the current page or retry",
				ErrorCategory: "capture_failed",
				Data: map[string]any{
					"capture_status": "failed",
				},
			}
		}

		res := tx.Model(&PageCaptureRequest{}).Where("id=? AND status=?", locked.ID, PageCapturePending).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "page capture request state changed")
		}
		if e := s.tools.CompleteClientFulfillment(ctx, tx, locked.InvocationID, toolResult); e != nil {
			return e
		}

		var lockedRun AgentRun
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", locked.RunID).First(&lockedRun).Error; e != nil {
			return e
		}
		if e := s.appendEvent(ctx, tx, &lockedRun, aiagentruntime.RunEventCaptureSettled, &locked.StepID, map[string]any{
			"capture_request_id": locked.ID,
			"status":             nextCaptureStatus,
		}); e != nil {
			return e
		}
		if lockedRun.Status == string(aiagentruntime.RunPaused) {
			requeued := tx.Model(&AgentRun{}).Where("id=? AND revision=? AND status=?", lockedRun.ID, lockedRun.Revision, string(aiagentruntime.RunPaused)).Updates(map[string]any{
				"status":           string(aiagentruntime.RunQueued),
				"error_category":   "",
				"lease_owner":      "",
				"lease_expires_at": nil,
				"revision":         gorm.Expr("revision+1"),
			})
			if requeued.Error != nil {
				return requeued.Error
			}
			if requeued.RowsAffected == 1 {
				if e := s.appendEvent(ctx, tx, &lockedRun, "run.queued", nil, map[string]any{"resume": true, "reason": "page_capture_settled"}); e != nil {
					return e
				}
				if s.writer != nil {
					if _, e := s.writer.Append(ctx, tx, outbox.AppendInput{
						EventType:        agentRunResumeEvent,
						AggregateType:    "ai_agent_run",
						AggregateID:      lockedRun.ID,
						AggregateVersion: int64(lockedRun.Revision + 1),
						Payload:          map[string]any{"run_id": lockedRun.ID},
					}); e != nil {
						return e
					}
				}
			}
		}
		capture.Status = nextCaptureStatus
		return nil
	})
	if err != nil {
		return nil, err
	}
	if alreadyHandled {
		return PageCaptureSubmitResponse{CaptureRequestID: capture.ID, RunID: capture.RunID, Status: capture.Status}, nil
	}
	return PageCaptureSubmitResponse{CaptureRequestID: capture.ID, RunID: capture.RunID, Status: capture.Status}, nil
}

func (s *AgentRuntimeService) WakeExpiredPageCapture(ctx context.Context, runID string) (bool, error) {
	var run AgentRun
	if err := s.db.WithContext(ctx).Where("id=? AND status=?", runID, string(aiagentruntime.RunPaused)).First(&run).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var capture PageCaptureRequest
	if err := s.db.WithContext(ctx).Where("run_id=? AND status=?", run.ID, PageCapturePending).Order("created_at DESC").First(&capture).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if capture.ExpiresAt.After(s.now()) {
		return false, nil
	}
	if auth.MustFromContext(ctx).TenantID == "" {
		ctx = auth.WithContext(ctx, &auth.Context{
			TenantID:      run.TenantID,
			PrincipalID:   run.UserID,
			Principal:     auth.PrincipalRef{Kind: auth.PrincipalKind(run.PrincipalKind), ID: run.UserID},
			ClientSurface: run.ClientSurface,
			Audience:      run.Audience,
			Realm:         auth.RealmTenant,
			Authenticated: true,
		})
	}
	_, err := s.setQueued(ctx, &run, run.Revision, agentRunResumeEvent)
	return err == nil, err
}

func (s *AgentRuntimeService) resumePageCapture(ctx context.Context, run *AgentRun) (bool, error) {
	var capture PageCaptureRequest
	err := s.db.WithContext(ctx).Where("run_id=? AND status IN ?", run.ID, []string{PageCapturePending, PageCaptureSubmitted, PageCaptureFailed}).Order("created_at DESC").First(&capture).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var step AgentStep
	if err := s.db.WithContext(ctx).Where("id=? AND run_id=?", capture.StepID, run.ID).First(&step).Error; err != nil {
		return false, err
	}
	if step.Status != "waiting" && step.Status != "running" {
		if capture.Status == PageCaptureSubmitted {
			now := s.now()
			_ = s.db.WithContext(ctx).Model(&PageCaptureRequest{}).Where("id=? AND status=?", capture.ID, PageCaptureSubmitted).Updates(map[string]any{"status": PageCaptureConsumed, "consumed_at": now})
		}
		return false, nil
	}
	call := aimodelruntime.OutputItem{
		Type:       aimodelruntime.PartToolCall,
		ToolCallID: capture.ToolCallID,
		ToolName:   tooling.ModelToolName(capture.ToolID),
	}
	now := s.now()
	if capture.Status == PageCapturePending {
		if capture.ExpiresAt.After(now) {
			return false, s.finalizePageCaptureWait(ctx, run)
		}
		timeoutResult := &aitool.Result{
			CallID:        capture.ToolCallID,
			Status:        aitool.ResultStatusFailed,
			Summary:       "page capture timed out before the client responded; continue by asking the user or using available tools",
			ErrorCategory: "capture_timeout",
			Data: map[string]any{
				"capture_status": "timeout",
			},
		}
		if txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&PageCaptureRequest{}).Where("id=? AND status=?", capture.ID, PageCapturePending).Updates(map[string]any{
				"status":         PageCaptureExpired,
				"error_category": "capture_timeout",
				"updated_at":     now,
			})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				if e := s.tools.CompleteClientFulfillment(ctx, tx, capture.InvocationID, timeoutResult); e != nil {
					return e
				}
				if e := s.appendEvent(ctx, tx, run, aiagentruntime.RunEventCaptureSettled, &step.ID, map[string]any{
					"capture_request_id": capture.ID,
					"status":             PageCaptureExpired,
					"error_category":     "capture_timeout",
				}); e != nil {
					return e
				}
			}
			return nil
		}); txErr != nil {
			return false, txErr
		}
		_, appendErr := s.appendToolResult(ctx, run, &step, call, timeoutResult, nil)
		return appendErr == nil, appendErr
	}

	if s.currentConversationRevision(ctx, run) != run.ConversationRevision {
		return false, s.fail(ctx, run, "conversation_revision_changed")
	}
	var inv tooling.ToolInvocation
	if err := s.db.WithContext(ctx).Where("id=? AND status IN ?", capture.InvocationID, []string{tooling.ToolInvocationCompleted, tooling.ToolInvocationFailed}).First(&inv).Error; err != nil {
		return false, s.fail(ctx, run, "capture_execution_unavailable")
	}
	var result aitool.Result
	if json.Unmarshal(inv.ResultJSON, &result) != nil {
		return false, s.fail(ctx, run, "capture_result_unavailable")
	}
	if capture.Status == PageCaptureSubmitted {
		_ = s.db.WithContext(ctx).Model(&PageCaptureRequest{}).Where("id=? AND status=?", capture.ID, PageCaptureSubmitted).Updates(map[string]any{
			"status":      PageCaptureConsumed,
			"consumed_at": now,
			"updated_at":  now,
		}).Error
	}
	_, appendErr := s.appendToolResult(ctx, run, &step, call, &result, nil)
	return appendErr == nil, appendErr
}

func extractPageCaptureImagePart(part aimodelruntime.ContentPart) (attachmentID, attachmentRevision string, ok bool) {
	if part.Type != aimodelruntime.PartToolResult || len(part.Result) == 0 {
		return "", "", false
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			CaptureStatus      string `json:"capture_status"`
			AttachmentID       string `json:"attachment_id"`
			AttachmentRevision string `json:"attachment_revision"`
			ResourceType       string `json:"resource_type"`
		} `json:"data"`
	}
	if json.Unmarshal(part.Result, &result) != nil {
		return "", "", false
	}
	if result.Status != aitool.ResultStatusCompleted || result.Data.CaptureStatus != "ready" || result.Data.ResourceType != PageCaptureAttachmentResourceType {
		return "", "", false
	}
	id := strings.TrimSpace(result.Data.AttachmentID)
	rev := strings.TrimSpace(result.Data.AttachmentRevision)
	if id == "" || rev == "" {
		return "", "", false
	}
	return id, rev, true
}
