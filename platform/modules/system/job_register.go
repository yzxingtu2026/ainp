package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

const (
	ActionJobList   = "system.job_list"
	ActionJobGet    = "system.job_get"
	ActionJobCancel = "system.job_cancel"
	ActionJobRetry  = "system.job_retry"
)

type JobService struct {
	store    job.RunStore
	enqueuer job.Enqueuer
	registry *job.Registry
	enabled  bool
}

func NewJobService(rt *job.Runtime) *JobService {
	if rt == nil {
		return &JobService{}
	}
	return &JobService{store: rt.Store, enqueuer: rt.Enqueuer, registry: rt.Registry, enabled: rt.Enabled}
}

func (s *JobService) require() error {
	if s == nil || s.store == nil {
		return errs.NewKey(errs.CodeInternal, "job.unavailable", "jobs unavailable")
	}
	return nil
}

func (s *JobService) List(ctx context.Context, input json.RawMessage) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var req struct {
		JobType  string `json:"job_type"`
		Status   string `json:"status"`
		Current  int    `json:"current"`
		Size     int    `json:"size"`
	}
	_ = json.Unmarshal(input, &req)
	rows, total, err := s.store.List(ctx, job.ListQuery{
		JobType: req.JobType, Status: req.Status, Page: req.Current, PageSize: req.Size,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": rows, "total": total, "current": req.Current, "size": req.Size}, nil
}

func (s *JobService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	run, err := s.store.Get(ctx, req.ID)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			return nil, errs.NewKey(errs.CodeNotFound, "job.not_found", "job not found")
		}
		return nil, err
	}
	return map[string]any{"job": run}, nil
}

func (s *JobService) Cancel(ctx context.Context, input json.RawMessage) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	if err := s.store.MarkCancelRequested(ctx, req.ID); err != nil {
		if errors.Is(err, job.ErrConflict) {
			return nil, errs.NewKey(errs.CodeConflict, "job.cancel_not_allowed", "job cannot be cancelled")
		}
		return nil, err
	}
	ac.AuditResourceType = "job_run"
	ac.AuditResourceID = req.ID
	ac.AuditSummary = "request job cancel"
	return map[string]any{"id": req.ID, "cancel_requested": true}, nil
}

func (s *JobService) Retry(ctx context.Context, input json.RawMessage) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if !s.enabled || s.enqueuer == nil {
		return nil, errs.NewKey(errs.CodeInternal, "job.unavailable", "jobs unavailable")
	}
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	old, err := s.store.Get(ctx, req.ID)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			return nil, errs.NewKey(errs.CodeNotFound, "job.not_found", "job not found")
		}
		return nil, err
	}
	if old.Status != status.JobFailed && old.Status != status.JobDead {
		return nil, errs.NewKey(errs.CodeConflict, "job.retry_not_allowed", "only failed/dead jobs can be retried")
	}
	var payload any
	_ = json.Unmarshal(old.PayloadJSON, &payload)
	run, err := s.enqueuer.Enqueue(ctx, job.EnqueueInput{
		Type:           old.JobType,
		Payload:        payload,
		IdempotencyKey: old.ID + ":retry:" + time.Now().UTC().Format("20060102150405"),
		CausationID:    old.ID,
	})
	if err != nil {
		if errors.Is(err, job.ErrUnavailable) {
			return nil, errs.NewKey(errs.CodeInternal, "job.unavailable", "jobs unavailable")
		}
		return nil, err
	}
	ac.AuditResourceType = "job_run"
	ac.AuditResourceID = run.ID
	ac.AuditSummary = "retry job from " + old.ID
	return map[string]any{"job": run}, nil
}

func registerJobActions(a *app.App) error {
	svc := NewJobService(a.Jobs)
	auditOn := true
	auditOff := false
	specs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionJobList, "/api/v1/system/jobs/list", "任务列表", "action.system.job_list.name", "分页查询当前租户 Job", "action.system.job_list.description", &auditOff, svc.List},
		{ActionJobGet, "/api/v1/system/jobs/get", "任务详情", "action.system.job_get.name", "查询 JobRun 详情", "action.system.job_get.description", &auditOff, svc.Get},
		{ActionJobCancel, "/api/v1/system/jobs/cancel", "取消任务", "action.system.job_cancel.name", "协作式请求取消", "action.system.job_cancel.description", &auditOn, svc.Cancel},
		{ActionJobRetry, "/api/v1/system/jobs/retry", "重试任务", "action.system.job_retry.name", "对 failed/dead 新建重试 JobRun", "action.system.job_retry.description", &auditOn, svc.Retry},
	}
	for _, spec := range specs {
		if err := a.RegisterAction(action.Action{
			ID:      spec.id,
			Handler: spec.handler,
			Meta: action.Metadata{
				Capability:             "system",
				Name:                   spec.name,
				NameKey:                spec.nameKey,
				Description:            spec.desc,
				DescriptionKey:         spec.descKey,
				PermissionGroup:        "system.job_management",
				PermissionGroupName:    "任务运维",
				PermissionGroupNameKey: "permission_group.system.job_management",
				Audit:                  spec.audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: spec.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
