package system

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

type attachmentAdminListRequest struct {
	pagination.Request
	Keyword  string `json:"keyword,omitempty"`
	Status   string `json:"status,omitempty" validate:"omitempty,oneof=pending ready deleting deleted failed"`
	Scope    string `json:"scope,omitempty" validate:"omitempty,oneof=all mine"` // all | mine
	Uploader string `json:"uploader_id,omitempty"`
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
	return action.JSON(s.list)(ctx, input)
}

func (s *AttachmentService) list(ctx context.Context, req attachmentAdminListRequest) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "mine"
	}
	ac := auth.MustFromContext(ctx)
	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	query := repos.attachments.Where("1 = 1")
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

	total, err := query.Count()
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	rows, err := query.Order("created_at DESC, id DESC").List(req.Offset(), req.PageSize)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	records := make([]AttachmentView, 0, len(rows))
	uploaderIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		records = append(records, toAttachmentView(row))
		if row.UploaderID != "" {
			uploaderIDs = append(uploaderIDs, row.UploaderID)
		}
	}
	names := s.resolveUploaderNames(ctx, tenantID, uploaderIDs)
	for i := range records {
		if name := names[records[i].UploaderID]; name != "" {
			records[i].UploaderName = name
		}
	}
	return pagination.NewResult(records, total, req.Request), nil
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
	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	links, repoErr := repos.links.Find("attachment_id = ?", row.ID)
	if repoErr != nil {
		return nil, mapAttachmentError(repoErr)
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
	if names := s.resolveUploaderNames(ctx, tenantID, []string{row.UploaderID}); names[row.UploaderID] != "" {
		view.UploaderName = names[row.UploaderID]
	}
	if canManage {
		view.ObjectKey = row.ObjectKey
	}
	return view, nil
}

// resolveUploaderNames 按租户批量解析上传者展示名（优先 display_name，否则 username）。
func (s *AttachmentService) resolveUploaderNames(ctx context.Context, tenantID string, userIDs []string) map[string]string {
	out := make(map[string]string)
	uniq := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out
	}
	ac := auth.MustFromContext(ctx)
	if ac.TenantID != tenantID {
		return out
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return out
	}
	users, err := tenant.TenantRaw[struct {
		ID          string
		DisplayName string
		Username    string
	}](tdb, "SELECT id, display_name, username FROM users WHERE tenant_id = ? AND id IN ?", uniq)
	if err != nil {
		return out
	}
	for _, u := range users {
		name := strings.TrimSpace(u.DisplayName)
		if name == "" {
			name = strings.TrimSpace(u.Username)
		}
		if name != "" {
			out[u.ID] = name
		}
	}
	return out
}
