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
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

type jobIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type jobListRequest struct {
	pagination.Request
	JobType string `json:"job_type" validate:"omitempty"`
	Status  string `json:"status" validate:"omitempty,oneof=pending_enqueue queued running retrying succeeded failed dead cancelled"`
}

type jobGetResponse struct {
	Job job.Run `json:"job"`
}

type jobCancelResponse struct {
	ID              string `json:"id"`
	CancelRequested bool   `json:"cancel_requested"`
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
	return action.JSON(s.list)(ctx, input)
}

func (s *JobService) list(ctx context.Context, req jobListRequest) (any, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	rows, total, err := s.store.List(ctx, job.ListQuery{
		JobType: req.JobType, Status: req.Status, Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}
	return pagination.NewResult(rows, total, req.Request), nil
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
		ai                                     *action.AIMeta
	}{
		{ActionJobList, "/api/v1/operations/jobs/list", "任务列表", "action.operations.job_list.name", "分页查询当前租户 Job", "action.operations.job_list.description", &auditOff, action.JSON(svc.list), readAIMeta("查询当前租户后台任务，可按任务类型和状态筛选", "查询后台任务列表", "查询当前租户后台任务，可按任务类型和状态筛选", "已返回当前租户可见的后台任务", action.InputSchemaFor[jobListRequest](), action.SchemaFor[pagination.Result[job.Run]](), "运维", "任务", "列表")},
		{ActionJobGet, "/api/v1/operations/jobs/get", "任务详情", "action.operations.job_get.name", "查询 JobRun 详情", "action.operations.job_get.description", &auditOff, action.JSON(svc.get), readAIMeta("按任务 ID 查询当前租户后台任务详情", "查询后台任务详情", "按任务 ID 查询当前租户后台任务详情", "已返回指定后台任务详情", action.InputSchemaFor[jobIDRequest](), action.SchemaFor[jobGetResponse](), "运维", "任务", "详情")},
		{ActionJobCancel, "/api/v1/operations/jobs/cancel", "取消任务", "action.operations.job_cancel.name", "协作式请求取消", "action.operations.job_cancel.description", &auditOn, action.JSON(svc.cancel), mutationAIMeta("请求取消当前租户中仍可取消的后台任务，不修改任务载荷", "取消后台任务", "请求取消当前租户中仍可取消的后台任务，不修改任务载荷", "已提交后台任务取消请求", "medium", "write", action.InputSchemaFor[jobIDRequest](), action.SchemaFor[jobCancelResponse](), "运维", "任务", "取消")},
		{ActionJobRetry, "/api/v1/operations/jobs/retry", "重试任务", "action.operations.job_retry.name", "对 failed/dead 新建重试 JobRun", "action.operations.job_retry.description", &auditOn, action.JSON(svc.retry), mutationAIMeta("基于失败或终止任务的原始载荷创建重试任务，不接受载荷修改", "重试后台任务", "基于失败或终止任务的原始载荷创建重试任务，不接受载荷修改", "已基于原任务创建新的重试任务", "medium", "write", action.InputSchemaFor[jobIDRequest](), action.SchemaFor[jobGetResponse](), "运维", "任务", "重试")},
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
				AI:                     spec.ai,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: spec.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
