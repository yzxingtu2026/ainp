package operations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
)

const (
	ActionLogSourceList = "system.log_source_list"
	ActionLogQuery      = "system.log_query"
	ActionLogGet        = "system.log_get"

	logDefaultRange = 15 * time.Minute
	logMaxRange     = 24 * time.Hour
	logQueryTimeout = 5 * time.Second
	logMaxBytes     = 1024 * 1024
)

type logQueryRequest struct {
	Source    string `json:"source" validate:"required"`
	StartAt   string `json:"start_at"`
	EndAt     string `json:"end_at"`
	Level     string `json:"level" validate:"omitempty,oneof=debug info warn error"`
	Service   string `json:"service"`
	Component string `json:"component"`
	Keyword   string `json:"keyword" validate:"max=256"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id"`
	ActionID  string `json:"action_id"`
	JobType   string `json:"job_type"`
	JobRunID  string `json:"job_run_id"`
	EventID   string `json:"event_id"`
	Page      int    `json:"page" validate:"min=1"`
	PageSize  int    `json:"page_size" validate:"min=1,max=100"`
}

func (r *logQueryRequest) Normalize() {
	if r.Page == 0 {
		r.Page = 1
	}
	if r.PageSize == 0 {
		r.PageSize = 20
	}
}

type logGetRequest struct {
	Source string `json:"source" validate:"required"`
	ID     string `json:"id" validate:"required"`
}

type LogService struct{ providers *logcenter.Registry }

func NewLogService(providers *logcenter.Registry) *LogService {
	return &LogService{providers: providers}
}

func (s *LogService) sources(ctx context.Context, _ struct{}) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	sources := s.providers.List()
	visible := sources[:0]
	for _, source := range sources {
		if source.Capabilities.TenantFilter {
			visible = append(visible, source)
		}
	}
	return map[string]any{"sources": visible}, nil
}

func (s *LogService) query(ctx context.Context, req logQueryRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := s.tenantProvider(req.Source)
	if err != nil {
		return nil, err
	}
	end, start, err := logRange(req.StartAt, req.EndAt)
	if err != nil {
		return nil, err
	}
	query := logcenter.Query{StartAt: start, EndAt: end, Level: req.Level, Service: req.Service, Component: req.Component, Keyword: req.Keyword, RequestID: req.RequestID, TraceID: req.TraceID, ActionID: req.ActionID, JobType: req.JobType, JobRunID: req.JobRunID, EventID: req.EventID, Page: req.Page, PageSize: req.PageSize, TenantID: ac.TenantID}
	queryCtx, cancel := context.WithTimeout(ctx, logQueryTimeout)
	defer cancel()
	page, err := provider.Query(queryCtx, query)
	if err != nil {
		return nil, mapLogError(err)
	}
	for i := range page.Items {
		if page.Items[i].TenantID != ac.TenantID {
			return nil, errs.NewKey(errs.CodeForbidden, "logs.tenant_filter_failed", "provider returned data outside tenant scope")
		}
		page.Items[i] = logcenter.Sanitize(page.Items[i])
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, err
	}
	if len(encoded) > logMaxBytes {
		return nil, errs.NewKey(errs.CodeValidation, "logs.result_too_large", "log result too large")
	}
	return page, nil
}

func (s *LogService) get(ctx context.Context, req logGetRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := s.tenantProvider(req.Source)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, logQueryTimeout)
	defer cancel()
	entry, err := provider.Get(queryCtx, logcenter.Ref{ID: req.ID, TenantID: ac.TenantID})
	if err != nil {
		return nil, mapLogError(err)
	}
	if entry == nil || entry.TenantID != ac.TenantID {
		return nil, errs.NewKey(errs.CodeNotFound, "logs.not_found", "log entry not found")
	}
	clean := logcenter.Sanitize(*entry)
	encoded, err := json.Marshal(clean)
	if err != nil {
		return nil, err
	}
	if len(encoded) > logMaxBytes {
		return nil, errs.NewKey(errs.CodeValidation, "logs.result_too_large", "log result too large")
	}
	return map[string]any{"log_entry": clean}, nil
}

func (s *LogService) tenantProvider(name string) (logcenter.Provider, error) {
	provider, ok := s.providers.Get(name)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotFound, "logs.source_not_found", "log source not found")
	}
	if !provider.Capabilities().TenantFilter {
		return nil, errs.NewKey(errs.CodeForbidden, "logs.tenant_filter_unsupported", "log source does not support tenant filtering")
	}
	return provider, nil
}

func logRange(startRaw, endRaw string) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	var err error
	if endRaw != "" {
		end, err = time.Parse(time.RFC3339, endRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "logs.invalid_time", "invalid end_at")
		}
	}
	start := end.Add(-logDefaultRange)
	if startRaw != "" {
		start, err = time.Parse(time.RFC3339, startRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "logs.invalid_time", "invalid start_at")
		}
	}
	if !start.Before(end) || end.Sub(start) > logMaxRange {
		return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "logs.invalid_range", "log range must be positive and at most 24 hours")
	}
	return end.UTC(), start.UTC(), nil
}

func mapLogError(err error) error {
	switch {
	case errors.Is(err, logcenter.ErrNotFound):
		return errs.NewKey(errs.CodeNotFound, "logs.not_found", "log entry not found")
	case errors.Is(err, logcenter.ErrForbidden):
		return errs.NewKey(errs.CodeForbidden, "logs.forbidden", "log query forbidden")
	case errors.Is(err, logcenter.ErrInvalid), errors.Is(err, logcenter.ErrTooLarge):
		return errs.NewKey(errs.CodeValidation, "logs.invalid_query", err.Error())
	case errors.Is(err, logcenter.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return errs.NewKey(errs.CodeInternal, "logs.timeout", "log provider timeout")
	default:
		return errs.NewKey(errs.CodeInternal, "logs.unavailable", "log provider unavailable")
	}
}

func registerLogActions(a *app.App) error {
	svc := NewLogService(a.LogProviders)
	auditOn := true
	for _, spec := range []struct {
		id, path, name, nameKey, desc, descKey string
		handler                                action.Handler
	}{
		{ActionLogSourceList, "/api/v1/operations/logs/sources", "日志来源", "action.system.log_source_list.name", "列出当前租户可用日志来源", "action.system.log_source_list.description", action.JSON(svc.sources)},
		{ActionLogQuery, "/api/v1/operations/logs/query", "日志查询", "action.system.log_query.name", "查询当前租户结构化运行日志", "action.system.log_query.description", action.JSON(svc.query)},
		{ActionLogGet, "/api/v1/operations/logs/get", "日志详情", "action.system.log_get.name", "查询当前租户结构化日志详情", "action.system.log_get.description", action.JSON(svc.get)},
	} {
		if err := a.RegisterAction(action.Action{ID: spec.id, Handler: spec.handler, Meta: action.Metadata{Capability: "operations", Name: spec.name, NameKey: spec.nameKey, Description: spec.desc, DescriptionKey: spec.descKey, PermissionGroup: "system.operations.logs", PermissionGroupName: "日志中心", PermissionGroupNameKey: "permission_group.system.operations.logs", Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: spec.path}}}); err != nil {
			return err
		}
	}
	return nil
}
