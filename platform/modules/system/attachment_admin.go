package system

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

type attachmentAdminListRequest struct {
	Current  int    `json:"current"`
	Size     int    `json:"size"`
	Keyword  string `json:"keyword"`
	Status   string `json:"status"`
	Scope    string `json:"scope"` // all | mine
	Uploader string `json:"uploader_id"`
}

type AttachmentLinkView struct {
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Field        string `json:"field"`
	CreatedAt    string `json:"created_at"`
}

type AttachmentDetailView struct {
	AttachmentView
	Provider  string               `json:"provider"`
	ObjectKey string               `json:"object_key,omitempty"`
	Links     []AttachmentLinkView `json:"links"`
}

// PolicyGet returns effective upload policy for the current tenant (frontend precheck).
func (s *AttachmentService) PolicyGet(ctx context.Context, _ json.RawMessage) (any, error) {
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	pol, err := s.policy.UploadPolicy(ctx, tenantID)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	return toUploadPolicyView(pol), nil
}

// List supports attachment center (scope=all) and picker recent uploads (scope=mine).
func (s *AttachmentService) List(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentAdminListRequest
	_ = json.Unmarshal(input, &req)
	if req.Current < 1 {
		req.Current = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "mine"
	}
	ac := auth.MustFromContext(ctx)
	query := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&model.Attachment{})
	switch scope {
	case "all":
		if !ac.HasPermission(ActionAttachmentList) {
			return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
		}
		if uploader := strings.TrimSpace(req.Uploader); uploader != "" {
			query = query.Where("uploader_id = ?", uploader)
		}
	case "mine":
		query = query.Where("uploader_id = ?", userID)
	default:
		return nil, errs.ValidationKey("common.validation_failed", "invalid scope")
	}
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		query = query.Where("original_name ILIKE ?", "%"+keyword+"%")
	}
	if st := strings.TrimSpace(req.Status); st != "" {
		if !status.AttachmentStatus(st).Valid() {
			return nil, errs.ValidationKey("common.validation_failed", "invalid status")
		}
		query = query.Where("status = ?", st)
	} else if scope == "mine" {
		query = query.Where("status IN ?", []status.AttachmentStatus{status.AttachmentReady, status.AttachmentPending})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, mapAttachmentError(err)
	}
	var rows []model.Attachment
	if err := query.Order("created_at DESC, id DESC").
		Offset((req.Current - 1) * req.Size).Limit(req.Size).Find(&rows).Error; err != nil {
		return nil, mapAttachmentError(err)
	}
	records := make([]AttachmentView, 0, len(rows))
	for _, row := range rows {
		if row.TenantID != tenantID {
			continue
		}
		records = append(records, toAttachmentView(row))
	}
	return map[string]any{
		"current": req.Current, "size": req.Size, "total": total, "records": records,
	}, nil
}

// Get returns attachment detail and links for attachment center.
func (s *AttachmentService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentIDRequest
	if err := json.Unmarshal(input, &req); err != nil || req.ID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "attachment id required")
	}
	row, err := s.getOwned(ctx, tenantID, req.ID)
	if err != nil {
		return nil, err
	}
	ac := auth.MustFromContext(ctx)
	canManage := ac.HasPermission(ActionAttachmentList) || ac.HasPermission(ActionAttachmentGet)
	if row.UploaderID != userID && !canManage {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	var links []model.AttachmentLink
	if err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Where("attachment_id = ?", row.ID).Find(&links).Error; err != nil {
		return nil, mapAttachmentError(err)
	}
	linkViews := make([]AttachmentLinkView, 0, len(links))
	for _, link := range links {
		linkViews = append(linkViews, AttachmentLinkView{
			ID: link.ID, AttachmentID: link.AttachmentID,
			ResourceType: link.ResourceType, ResourceID: link.ResourceID, Field: link.Field,
			CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	view := AttachmentDetailView{
		AttachmentView: toAttachmentView(*row),
		Provider:       row.Provider,
		Links:          linkViews,
	}
	if canManage {
		view.ObjectKey = row.ObjectKey
	}
	return view, nil
}
