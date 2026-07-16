package operations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

type jobIDRequest struct {
	ID string `json:"id" validate:"required"`
}

const (
	ActionJobList   = "operations.job_list"
	ActionJobGet    = "operations.job_get"
	ActionJobCancel = "operations.job_cancel"
	ActionJobRetry  = "operations.job_retry"
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
		JobType string `json:"job_type"`
		Status  string `json:"status"`
		Current int    `json:"current"`
		Size    int    `json:"size"`
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

func (s *JobService) get(ctx context.Context, req jobIDRequest) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
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

func (s *JobService) cancel(ctx context.Context, req jobIDRequest) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
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

func (s *JobService) retry(ctx context.Context, req jobIDRequest) (any, error) {
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
		{ActionJobList, "/api/v1/operations/jobs/list", "任务列表", "action.operations.job_list.name", "分页查询当前租户 Job", "action.operations.job_list.description", &auditOff, svc.List},
		{ActionJobGet, "/api/v1/operations/jobs/get", "任务详情", "action.operations.job_get.name", "查询 JobRun 详情", "action.operations.job_get.description", &auditOff, action.JSON(svc.get)},
		{ActionJobCancel, "/api/v1/operations/jobs/cancel", "取消任务", "action.operations.job_cancel.name", "协作式请求取消", "action.operations.job_cancel.description", &auditOn, action.JSON(svc.cancel)},
		{ActionJobRetry, "/api/v1/operations/jobs/retry", "重试任务", "action.operations.job_retry.name", "对 failed/dead 新建重试 JobRun", "action.operations.job_retry.description", &auditOn, action.JSON(svc.retry)},
	}
	for _, spec := range specs {
		if err := a.RegisterAction(action.Action{
			ID:      spec.id,
			Handler: spec.handler,
			Meta: action.Metadata{
				Capability:             "operations",
				Name:                   spec.name,
				NameKey:                spec.nameKey,
				Description:            spec.desc,
				DescriptionKey:         spec.descKey,
				PermissionGroup:        "operations.jobs",
				PermissionGroupName:    "任务运维",
				PermissionGroupNameKey: "permission_group.operations.jobs",
				Audit:                  spec.audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: spec.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
