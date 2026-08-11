package attachments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/dblock"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/jobs"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/model"
	"gorm.io/gorm"
)

type AttachmentService struct {
	db       *gorm.DB
	store    storage.ObjectStore
	tokens   *attachment.TransferTokenService
	policies *attachment.PolicyRegistry
	policy   attachment.PolicyProvider
	outbox   outbox.Writer
	baseURL  string // public API base for transfer URLs, e.g. empty => relative paths
}

func NewAttachmentService(
	db *gorm.DB,
	store storage.ObjectStore,
	tokens *attachment.TransferTokenService,
	policies *attachment.PolicyRegistry,
	policy attachment.PolicyProvider,
) *AttachmentService {
	registerAttachmentTenantModels()
	if policy == nil {
		policy = attachment.StaticPolicyProvider{Policy: attachment.DefaultUploadPolicy()}
	}
	if policies == nil {
		policies = attachment.NewPolicyRegistry()
	}
	return &AttachmentService{db: db, store: store, tokens: tokens, policies: policies, policy: policy}
}

// SetOutboxWriter enables reliable thumb-job enqueue after upload ready (optional).
func (s *AttachmentService) SetOutboxWriter(w outbox.Writer) { s.outbox = w }

func (s *AttachmentService) Policies() *attachment.PolicyRegistry { return s.policies }

type attachmentPrepareRequest struct {
	OriginalName string `json:"original_name" validate:"required,max=255"`
	ContentType  string `json:"content_type,omitempty" validate:"omitempty"`
	Size         int64  `json:"size" validate:"required,min=1"`
	Transport    string `json:"transport,omitempty" validate:"omitempty,oneof=raw multipart"`
}

type attachmentIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type attachmentDownloadPrepareRequest struct {
	ID      string `json:"id" validate:"required"`
	Variant string `json:"variant,omitempty" validate:"omitempty,oneof=original thumb preview"` // 空 = original
}

type attachmentBindRequest struct {
	AttachmentID string `json:"attachment_id" validate:"required"`
	ResourceType string `json:"resource_type" validate:"required"`
	ResourceID   string `json:"resource_id" validate:"required"`
	Field        string `json:"field,omitempty" validate:"omitempty"`
}

type attachmentListRequest struct {
	ResourceType string `json:"resource_type" validate:"required"`
	ResourceID   string `json:"resource_id" validate:"required"`
	Field        string `json:"field,omitempty" validate:"omitempty"`
}

type AttachmentView struct {
	ID           string `json:"id"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256,omitempty"`
	Status       string `json:"status"`
	ThumbStatus  string `json:"thumb_status,omitempty"`
	UploaderID   string `json:"uploader_id"`
	UploaderName string `json:"uploader_name,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type TransferPrepareView struct {
	Attachment  AttachmentView `json:"attachment"`
	UploadURL   string         `json:"upload_url,omitempty"`
	DownloadURL string         `json:"download_url,omitempty"`
	Token       string         `json:"token"`
	ExpiresAt   string         `json:"expires_at"`
	Method      string         `json:"method"`
	Transport   string         `json:"transport,omitempty"`
	FieldName   string         `json:"field_name,omitempty"`
}

func (s *AttachmentService) UploadPrepare(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentPrepareRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid upload prepare request")
	}
	req.OriginalName = strings.TrimSpace(req.OriginalName)
	req.ContentType = strings.TrimSpace(req.ContentType)
	req.Transport = strings.TrimSpace(strings.ToLower(req.Transport))
	if req.OriginalName == "" || req.Size <= 0 {
		return nil, errs.ValidationKey("attachment.invalid_meta", "original_name and size required")
	}
	if err := validateOriginalName(req.OriginalName); err != nil {
		return nil, errs.ValidationKey("attachment.invalid_name", err.Error())
	}
	pol, err := s.policy.UploadPolicy(ctx, tenantID)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	if req.Size > pol.MaxSizeBytes {
		return nil, errs.ValidationKey("attachment.size_exceeded", "file too large")
	}
	ext := strings.ToLower(path.Ext(req.OriginalName))
	if !extAllowed(pol.AllowedExt, ext) {
		return nil, errs.ValidationKey("attachment.ext_forbidden", "file extension not allowed")
	}
	if req.ContentType != "" && !mimeAllowed(pol.AllowedMIME, req.ContentType) {
		return nil, errs.ValidationKey("attachment.mime_forbidden", "content type not allowed")
	}
	if req.ContentType == "" {
		req.ContentType = "application/octet-stream"
	}
	if req.Transport == "" {
		req.Transport = attachment.TransferTransportRaw
	}
	if req.Transport != attachment.TransferTransportRaw && req.Transport != attachment.TransferTransportMultipart {
		return nil, errs.ValidationKey("attachment.transport_invalid", "invalid upload transport")
	}

	objectKey, err := storage.BuildKey(storage.KeyParts{
		TenantID: tenantID, Namespace: "attachments",
	})
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	id := uuid.NewString()
	exp := time.Now().UTC().Add(pol.TempRetain)
	provider := storage.ProviderLocal

	row := model.Attachment{
		ID: id, Provider: provider, ObjectKey: objectKey,
		OriginalName: req.OriginalName, ContentType: req.ContentType,
		Size: 0, Status: status.AttachmentPending, UploaderID: userID,
		ExpectedSize: req.Size, ExpiresAt: &exp,
	}
	token, jti, tokenExp, err := s.tokens.Issue(attachment.IssueTransferInput{
		TenantID: tenantID, PrincipalID: userID, AttachmentID: id,
		Op: attachment.OpUpload, Transport: req.Transport, Size: req.Size, TTL: pol.TransferTTL,
	})
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	row.UploadJTI = jti

	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	if err := repos.attachments.Create(&row); err != nil {
		return nil, mapAttachmentError(err)
	}
	setAttachmentAudit(ctx, id, "attachment upload prepare")
	view := TransferPrepareView{
		Attachment: toAttachmentView(row),
		UploadURL:  "/api/v1/attachments/transfer/" + id,
		Token:      token,
		ExpiresAt:  tokenExp.Format(time.RFC3339),
		Method:     http.MethodPut,
		Transport:  req.Transport,
	}
	if req.Transport == attachment.TransferTransportMultipart {
		view.Method = http.MethodPost
		view.FieldName = multipartFileFieldName
	}
	return view, nil
}

func (s *AttachmentService) DownloadPrepare(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentDownloadPrepareRequest
	if err := json.Unmarshal(input, &req); err != nil || req.ID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "attachment id required")
	}
	variant := strings.TrimSpace(strings.ToLower(req.Variant))
	if variant == "" {
		variant = "original"
	}
	switch variant {
	case "original", "thumb":
	case "preview":
		// Phase 1：仅 original + thumb；preview 预留
		return nil, errs.NewKey(errs.CodeNotFound, "attachment.variant_unavailable", "attachment variant not available")
	default:
		return nil, errs.ValidationKey("common.validation_failed", "invalid variant")
	}
	row, err := s.getOwned(ctx, tenantID, req.ID)
	if err != nil {
		return nil, err
	}
	if row.Status != status.AttachmentReady {
		return nil, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	if err := s.authorizeDownload(ctx, tenantID, userID, row); err != nil {
		return nil, err
	}
	size := row.Size
	if variant == "thumb" {
		if row.GetThumbStatus() != status.AttachmentThumbReady || row.ThumbObjectKey == "" {
			return nil, errs.NewKey(errs.CodeNotFound, "attachment.variant_unavailable", "attachment variant not available")
		}
		size = row.ThumbSize
	}
	pol, err := s.policy.UploadPolicy(ctx, tenantID)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	token, _, tokenExp, err := s.tokens.Issue(attachment.IssueTransferInput{
		TenantID: tenantID, PrincipalID: userID, AttachmentID: row.ID,
		Op: attachment.OpDownload, Variant: variant, Size: size, TTL: pol.TransferTTL,
	})
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	setAttachmentAudit(ctx, row.ID, "attachment download prepare")
	return TransferPrepareView{
		Attachment:  toAttachmentView(*row),
		DownloadURL: "/api/v1/attachments/transfer/" + row.ID,
		Token:       token,
		ExpiresAt:   tokenExp.Format(time.RFC3339),
		Method:      http.MethodGet,
	}, nil
}

func (s *AttachmentService) Bind(ctx context.Context, input json.RawMessage) (any, error) {
	var req attachmentBindRequest
	if err := json.Unmarshal(input, &req); err != nil || req.AttachmentID == "" || req.ResourceType == "" || req.ResourceID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "invalid bind request")
	}
	var link attachment.ResourceLink
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var bindErr error
		link, bindErr = s.BindResource(ctx, tx, attachment.ResourceLinkRequest{
			AttachmentID: req.AttachmentID, ResourceType: req.ResourceType,
			ResourceID: req.ResourceID, Field: req.Field,
		})
		return bindErr
	})
	if err != nil {
		return nil, err
	}
	tenantID, _, _ := tenantIdentity(ctx)
	row, err := s.getOwned(ctx, tenantID, req.AttachmentID)
	if err != nil {
		return nil, err
	}
	setAttachmentAudit(ctx, row.ID, "attachment bind")
	return map[string]any{"link_id": link.ID, "attachment": toAttachmentView(*row)}, nil
}

func (s *AttachmentService) Unbind(ctx context.Context, input json.RawMessage) (any, error) {
	var req attachmentBindRequest
	if err := json.Unmarshal(input, &req); err != nil || req.AttachmentID == "" || req.ResourceType == "" || req.ResourceID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "invalid unbind request")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.UnbindResource(ctx, tx, attachment.ResourceLinkRequest{
			AttachmentID: req.AttachmentID, ResourceType: req.ResourceType,
			ResourceID: req.ResourceID, Field: req.Field,
		})
	})
	if err != nil {
		return nil, err
	}
	setAttachmentAudit(ctx, req.AttachmentID, "attachment unbind")
	return map[string]any{"unbound": true}, nil
}

func (s *AttachmentService) BindResource(ctx context.Context, tx *gorm.DB, req attachment.ResourceLinkRequest) (attachment.ResourceLink, error) {
	if tx == nil {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeInternal, "attachment.transaction_required", "attachment resource transaction is required")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeInternal, "attachment.transaction_required", "attachment resource transaction is required")
	}
	if err := req.Validate(); err != nil {
		return attachment.ResourceLink{}, errs.ValidationKey("common.validation_failed", err.Error())
	}
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return attachment.ResourceLink{}, err
	}
	if err = dblock.WithTimeout(ctx, 10*time.Second, func(lockCtx context.Context) error {
		return dblock.LockForTenant(tx.WithContext(lockCtx), "attachments", tenantID, req.AttachmentID)
	}); err != nil {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	repos, repoErr := newAttachmentRepositories(ctx, tx)
	if repoErr != nil {
		return attachment.ResourceLink{}, repoErr
	}
	row, repoErr := repos.attachments.First("id=? AND status=?", req.AttachmentID, status.AttachmentReady)
	if repoErr != nil {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	if row.UploaderID != userID {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	policyCtx := attachment.WithResourceLinkOperation(attachment.WithResourceTransaction(ctx, tx), attachment.ResourceLinkBind)
	if err = s.policies.Allow(policyCtx, req.ResourceType, tenantID, userID, req.ResourceID, req.Field); err != nil {
		return attachment.ResourceLink{}, errs.NewKey(errs.CodeForbidden, "attachment.resource_forbidden", err.Error())
	}
	link := model.AttachmentLink{
		ID: uuid.NewString(), AttachmentID: row.ID,
		ResourceType: req.ResourceType, ResourceID: req.ResourceID, Field: req.Field,
	}
	if err = repos.links.Create(&link); err != nil {
		return attachment.ResourceLink{}, mapAttachmentError(err)
	}
	return attachment.ResourceLink{
		ID: link.ID, AttachmentID: link.AttachmentID, ResourceType: link.ResourceType,
		ResourceID: link.ResourceID, Field: link.Field,
	}, nil
}

func (s *AttachmentService) UnbindResource(ctx context.Context, tx *gorm.DB, req attachment.ResourceLinkRequest) error {
	if tx == nil {
		return errs.NewKey(errs.CodeInternal, "attachment.transaction_required", "attachment resource transaction is required")
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return errs.NewKey(errs.CodeInternal, "attachment.transaction_required", "attachment resource transaction is required")
	}
	if err := req.Validate(); err != nil {
		return errs.ValidationKey("common.validation_failed", err.Error())
	}
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return err
	}
	repos, repoErr := newAttachmentRepositories(ctx, tx)
	if repoErr != nil {
		return repoErr
	}
	_, repoErr = repos.attachments.First("id=? AND status=?", req.AttachmentID, status.AttachmentReady)
	if repoErr != nil {
		return errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	policyCtx := attachment.WithResourceLinkOperation(attachment.WithResourceTransaction(ctx, tx), attachment.ResourceLinkUnbind)
	if err = s.policies.Allow(policyCtx, req.ResourceType, tenantID, userID, req.ResourceID, req.Field); err != nil {
		return errs.NewKey(errs.CodeForbidden, "attachment.resource_forbidden", err.Error())
	}
	result := repos.links.DeleteWhere("attachment_id=? AND resource_type=? AND resource_id=? AND field=?", req.AttachmentID, req.ResourceType, req.ResourceID, req.Field)
	if result.Error != nil {
		return mapAttachmentError(result.Error)
	}
	if result.RowsAffected != 1 {
		return errs.NewKey(errs.CodeNotFound, "attachment.link_not_found", "attachment link not found")
	}
	return nil
}

func (s *AttachmentService) ListByResource(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentListRequest
	if err := json.Unmarshal(input, &req); err != nil || req.ResourceType == "" || req.ResourceID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "resource_type and resource_id required")
	}
	if err := s.policies.Allow(ctx, req.ResourceType, tenantID, userID, req.ResourceID, req.Field); err != nil {
		return nil, errs.NewKey(errs.CodeForbidden, "attachment.resource_forbidden", err.Error())
	}
	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	q := repos.links.Where("resource_type = ? AND resource_id = ?", req.ResourceType, req.ResourceID)
	if req.Field != "" {
		q = q.Where("field = ?", req.Field)
	}
	links, repoErr := q.Find("1=1")
	if repoErr != nil {
		return nil, mapAttachmentError(repoErr)
	}
	items := make([]AttachmentView, 0, len(links))
	for _, link := range links {
		row, err := s.getOwned(ctx, tenantID, link.AttachmentID)
		if err != nil {
			continue
		}
		if row.Status != status.AttachmentReady {
			continue
		}
		items = append(items, toAttachmentView(*row))
	}
	return map[string]any{"items": items}, nil
}

func (s *AttachmentService) Delete(ctx context.Context, input json.RawMessage) (any, error) {
	tenantID, userID, err := tenantIdentity(ctx)
	if err != nil {
		return nil, err
	}
	var req attachmentIDRequest
	if err := json.Unmarshal(input, &req); err != nil || req.ID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "attachment id required")
	}
	var row model.Attachment
	now := time.Now().UTC()
	err = dblock.WithTimeout(ctx, 10*time.Second, func(lockCtx context.Context) error {
		return dblock.WithinTx(lockCtx, s.db, func(tx *gorm.DB) error {
			if lockErr := dblock.LockForTenant(tx, "attachments", tenantID, req.ID); lockErr != nil {
				return errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
			}
			repos, repoErr := newAttachmentRepositories(lockCtx, tx)
			if repoErr != nil {
				return repoErr
			}
			loaded, queryErr := repos.attachments.GetByID(req.ID)
			if queryErr != nil {
				return errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
			}
			row = loaded
			linkCount, countErr := repos.links.Where("attachment_id=?", row.ID).Count()
			if countErr != nil {
				return mapAttachmentError(countErr)
			}
			if linkCount > 0 {
				return errs.ValidationKey("attachment.has_links", "unbind attachment before delete")
			}
			if row.UploaderID != userID && !auth.MustFromContext(lockCtx).HasPermission(ActionAttachmentDelete) &&
				row.Status == status.AttachmentReady {
				return errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			updates := map[string]any{"status": status.AttachmentDeleting, "upload_jti": "", "updated_at": now}
			if updateErr := repos.attachments.Where("id=?", row.ID).Updates(updates).Error; updateErr != nil {
				return mapAttachmentError(updateErr)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	objectPending := false
	if row.ThumbObjectKey != "" {
		if err := s.store.Delete(ctx, row.ThumbObjectKey); err != nil {
			logging.Error(ctx, "attachment thumb object delete failed", err, "attachment_id", row.ID, "key_digest", storage.KeyDigest(row.ThumbObjectKey))
			objectPending = true
		}
	}
	if err := s.store.Delete(ctx, row.ObjectKey); err != nil {
		logging.Error(ctx, "attachment object delete failed", err, "attachment_id", row.ID, "key_digest", storage.KeyDigest(row.ObjectKey))
		objectPending = true
		// keep deleting for CleanupExpired retry; never restore ready/download access
		setAttachmentAudit(ctx, row.ID, "attachment delete")
		return map[string]any{"deleted": true, "object_pending": true}, nil
	}
	if objectPending {
		setAttachmentAudit(ctx, row.ID, "attachment delete")
		return map[string]any{"deleted": true, "object_pending": true}, nil
	}
	repos, _ := newAttachmentRepositories(ctx, s.db)
	_ = repos.attachments.Where("id = ?", row.ID).Updates(map[string]any{
		"status": status.AttachmentDeleted, "thumb_object_key": "", "updated_at": time.Now().UTC(),
	}).Error
	_ = repos.attachments.Delete(&row)
	setAttachmentAudit(ctx, row.ID, "attachment delete")
	return map[string]any{"deleted": true, "object_pending": objectPending}, nil
}

// CleanupExpired removes expired pending/failed attachments and retries deleting-status object cleanup.
// DB and ObjectStore are not the same transaction; callers may invoke this periodically or manually.
func (s *AttachmentService) CleanupExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	tenantID, _, err := tenantIdentity(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return 0, repoErr
	}
	q := repos.attachments.Where(
		"(status IN ? AND expires_at IS NOT NULL AND expires_at < ?) OR status = ?",
		[]status.AttachmentStatus{status.AttachmentPending, status.AttachmentFailed},
		now,
		status.AttachmentDeleting,
	)
	rows, repoErr := q.List(0, limit)
	if repoErr != nil {
		return 0, mapAttachmentError(repoErr)
	}
	cleaned := 0
	for i := range rows {
		row := &rows[i]
		if row.TenantID != tenantID {
			continue
		}
		if row.ThumbObjectKey != "" {
			if err := s.store.Delete(ctx, row.ThumbObjectKey); err != nil {
				logging.Error(ctx, "attachment cleanup thumb delete failed", err, "attachment_id", row.ID, "key_digest", storage.KeyDigest(row.ThumbObjectKey))
				_ = repos.attachments.Where("id = ?", row.ID).Updates(map[string]any{
					"status": status.AttachmentDeleting, "upload_jti": "", "updated_at": time.Now().UTC(),
				}).Error
				continue
			}
		}
		if err := s.store.Delete(ctx, row.ObjectKey); err != nil {
			logging.Error(ctx, "attachment cleanup object delete failed", err, "attachment_id", row.ID, "key_digest", storage.KeyDigest(row.ObjectKey))
			_ = repos.attachments.Where("id = ?", row.ID).Updates(map[string]any{
				"status": status.AttachmentDeleting, "upload_jti": "", "updated_at": time.Now().UTC(),
			}).Error
			continue
		}
		_ = repos.attachments.Where("id = ?", row.ID).Updates(map[string]any{
			"status": status.AttachmentDeleted, "upload_jti": "", "thumb_object_key": "", "updated_at": time.Now().UTC(),
		}).Error
		_ = repos.attachments.Delete(row)
		cleaned++
	}
	return cleaned, nil
}

// CompleteUpload streams body into ObjectStore and marks the attachment ready.
// Called from the dedicated transfer HTTP adapter (not JSON Action).
func (s *AttachmentService) CompleteUpload(ctx context.Context, claims *attachment.TransferClaims, body io.Reader) (*model.Attachment, error) {
	if claims.Op != attachment.OpUpload {
		return nil, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "invalid transfer token")
	}
	if body == nil {
		return nil, errs.ValidationKey("attachment.size_mismatch", "missing upload body")
	}
	row, err := s.getOwned(ctx, claims.TenantID, claims.AttachmentID)
	if err != nil {
		return nil, err
	}
	if row.Status != status.AttachmentPending {
		return nil, errs.ValidationKey("attachment.not_pending", "attachment is not pending upload")
	}
	if row.UploaderID != claims.PrincipalID {
		return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
	}
	if row.UploadJTI == "" || row.UploadJTI != claims.JTI {
		return nil, errs.NewKey(errs.CodeForbidden, "attachment.token_replay", "upload token already used or mismatched")
	}
	if claims.Size > 0 && claims.Size != row.ExpectedSize {
		return nil, errs.ValidationKey("attachment.size_mismatch", "size mismatch")
	}

	pol, err := s.policy.UploadPolicy(ctx, claims.TenantID)
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	claimedJTI, err := s.claimUpload(ctx, row, claims)
	if err != nil {
		return nil, err
	}
	fail := func(category string, cause error) (*model.Attachment, error) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.markFailed(cleanupCtx, row, claimedJTI, category)
		return nil, mapAttachmentError(cause)
	}

	hasher := sha256.New()
	sniff := &sniffReader{r: io.TeeReader(io.LimitReader(body, row.ExpectedSize), hasher), limit: 512}
	info, err := s.store.Put(ctx, storage.PutRequest{
		Key: row.ObjectKey, Body: sniff, Size: row.ExpectedSize, ContentType: row.ContentType,
	})
	if err != nil {
		if readErr := uploadReaderError(body); readErr != nil {
			return fail("invalid_stream", readErr)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fail("cancelled", errs.ValidationKey("attachment.upload_cancelled", "attachment upload cancelled"))
		}
		return fail("storage_failed", err)
	}
	// Reject truncated or oversized streams: LimitReader stops at ExpectedSize; ensure no leftover byte.
	extra := make([]byte, 1)
	if n, readErr := body.Read(extra); n > 0 {
		return fail("size_exceeded", errs.ValidationKey("attachment.size_mismatch", "size mismatch"))
	} else if readErr != nil && !errors.Is(readErr, io.EOF) {
		if adapterErr := uploadReaderError(body); adapterErr != nil {
			return fail("invalid_stream", adapterErr)
		}
		return fail("read_failed", readErr)
	}
	if info.Size != row.ExpectedSize {
		return fail("size_mismatch", errs.ValidationKey("attachment.size_mismatch", "size mismatch"))
	}
	if ctx.Err() != nil {
		return fail("cancelled", errs.ValidationKey("attachment.upload_cancelled", "attachment upload cancelled"))
	}

	detected := http.DetectContentType(sniff.head)
	if !mimeAllowed(pol.AllowedMIME, detected) && !mimeAllowed(pol.AllowedMIME, row.ContentType) {
		return fail("mime_forbidden", errs.ValidationKey("attachment.mime_forbidden", "detected content type not allowed"))
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	contentType := row.ContentType
	if contentType == "application/octet-stream" && detected != "" {
		contentType = detected
	}
	now := time.Now().UTC()
	thumbStatus := status.AttachmentThumbSkipped
	if attachment.IsThumbMediaMIME(contentType) {
		thumbStatus = status.AttachmentThumbPending
	}
	updates := map[string]any{
		"status": status.AttachmentReady, "size": info.Size, "sha256": sum,
		"content_type": contentType, "upload_jti": "", "updated_at": now,
		"thumb_status": thumbStatus, "thumb_error": "", "thumb_error_category": "", "thumb_retryable": nil, "thumb_updated_at": now,
	}

	applyReady := func(tx *gorm.DB) error {
		repos, repoErr := newAttachmentRepositories(ctx, tx)
		if repoErr != nil {
			return repoErr
		}
		res := repos.attachments.Where("id = ? AND status = ? AND upload_jti = ?", row.ID, status.AttachmentPending, claimedJTI).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.ValidationKey("attachment.not_pending", "attachment is not pending upload")
		}
		return nil
	}

	if s.outbox != nil && thumbStatus == status.AttachmentThumbPending {
		err := outbox.WithinTx(ctx, s.db, s.outbox, func(tx *gorm.DB, w outbox.Writer) error {
			if err := applyReady(tx); err != nil {
				return err
			}
			_, err := w.Append(ctx, tx, outbox.AppendInput{
				EventType:     jobs.EventAttachmentReadyV1,
				AggregateType: "attachment",
				AggregateID:   row.ID,
				Payload: jobs.GenerateThumbPayload{
					AttachmentID: row.ID,
					ContentType:  contentType,
				},
			})
			return err
		})
		if err != nil {
			return fail("ready_commit_failed", err)
		}
	} else {
		if err := applyReady(s.db); err != nil {
			return fail("ready_commit_failed", err)
		}
	}
	row.Status = status.AttachmentReady
	row.Size = info.Size
	row.SHA256 = sum
	row.ContentType = contentType
	row.UploadJTI = ""
	row.SetThumbStatus(thumbStatus)
	row.ThumbUpdatedAt = &now
	return row, nil
}

func claimedUploadJTI(jti string) string { return "claimed:" + jti }

func (s *AttachmentService) claimUpload(ctx context.Context, row *model.Attachment, claims *attachment.TransferClaims) (string, error) {
	claimedJTI := claimedUploadJTI(claims.JTI)
	repos, err := newAttachmentRepositories(ctx, s.db)
	if err != nil {
		return "", err
	}
	res := repos.attachments.Where(
		"id = ? AND status = ? AND upload_jti = ? AND uploader_id = ? AND expected_size = ?",
		row.ID, status.AttachmentPending, claims.JTI, claims.PrincipalID, row.ExpectedSize,
	).Updates(map[string]any{"upload_jti": claimedJTI, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return "", mapAttachmentError(res.Error)
	}
	if res.RowsAffected != 1 {
		return "", errs.NewKey(errs.CodeForbidden, "attachment.token_replay", "upload token already used or mismatched")
	}
	row.UploadJTI = claimedJTI
	return claimedJTI, nil
}

type uploadErrorSource interface {
	UploadError() error
}

func uploadReaderError(body io.Reader) error {
	source, ok := body.(uploadErrorSource)
	if !ok {
		return nil
	}
	return source.UploadError()
}

type sniffReader struct {
	r     io.Reader
	limit int
	head  []byte
}

func (s *sniffReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 && len(s.head) < s.limit {
		need := s.limit - len(s.head)
		if need > n {
			need = n
		}
		s.head = append(s.head, p[:need]...)
	}
	return n, err
}

// OpenDownload returns a reader authorized by a validated download transfer token.
// Resource access was decided by DownloadPrepare; transfer only revalidates the
// signed token claims and the attachment's current transferable state.
func (s *AttachmentService) OpenDownload(ctx context.Context, claims *attachment.TransferClaims) (io.ReadCloser, *model.Attachment, error) {
	return s.openDownloadVariant(ctx, claims, false, 0, 0)
}

func (s *AttachmentService) OpenDownloadRange(ctx context.Context, claims *attachment.TransferClaims, offset, length int64) (io.ReadCloser, *model.Attachment, error) {
	return s.openDownloadVariant(ctx, claims, true, offset, length)
}

func (s *AttachmentService) openDownloadVariant(ctx context.Context, claims *attachment.TransferClaims, ranged bool, offset, length int64) (io.ReadCloser, *model.Attachment, error) {
	if claims == nil || claims.Op != attachment.OpDownload {
		return nil, nil, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "invalid transfer token")
	}
	row, err := s.getOwned(ctx, claims.TenantID, claims.AttachmentID)
	if err != nil {
		return nil, nil, err
	}
	if row.Status != status.AttachmentReady {
		return nil, nil, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	variant := strings.TrimSpace(strings.ToLower(claims.Variant))
	if variant == "" {
		variant = "original"
	}
	key := row.ObjectKey
	out := *row
	switch variant {
	case "original":
		// use original
	case "thumb":
		if row.GetThumbStatus() != status.AttachmentThumbReady || row.ThumbObjectKey == "" {
			return nil, nil, errs.NewKey(errs.CodeNotFound, "attachment.variant_unavailable", "attachment variant not available")
		}
		key = row.ThumbObjectKey
		out.ObjectKey = key
		out.ContentType = row.ThumbContentType
		if out.ContentType == "" {
			out.ContentType = "image/jpeg"
		}
		out.Size = row.ThumbSize
		out.SHA256 = ""
		out.OriginalName = thumbDownloadName(row.OriginalName)
	default:
		return nil, nil, errs.NewKey(errs.CodeNotFound, "attachment.variant_unavailable", "attachment variant not available")
	}
	if claims.Size > 0 && claims.Size != out.Size {
		return nil, nil, errs.NewKey(errs.CodeForbidden, "attachment.token_invalid", "attachment size does not match token")
	}
	var rc io.ReadCloser
	if ranged {
		rc, _, err = s.store.OpenRange(ctx, key, offset, length)
	} else {
		rc, _, err = s.store.Open(ctx, key)
	}
	if err != nil {
		return nil, nil, mapAttachmentError(err)
	}
	return rc, &out, nil
}

func thumbDownloadName(original string) string {
	base := path.Base(original)
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = "thumb"
	}
	return name + ".thumb.jpg"
}

func (s *AttachmentService) ParseTransferToken(token string) (*attachment.TransferClaims, error) {
	claims, err := s.tokens.Parse(token)
	if err != nil {
		return nil, errs.NewKey(errs.CodeUnauthorized, "attachment.token_invalid", "invalid transfer token")
	}
	return claims, nil
}

func (s *AttachmentService) authorizeDownload(ctx context.Context, tenantID, userID string, row *model.Attachment) error {
	switch row.Status {
	case status.AttachmentReady:
		repos, repoErr := newAttachmentRepositories(ctx, s.db)
		if repoErr != nil {
			return repoErr
		}
		links, repoErr := repos.links.Find("attachment_id = ?", row.ID)
		if repoErr != nil {
			return mapAttachmentError(repoErr)
		}
		if len(links) == 0 {
			if row.UploaderID != userID {
				return errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			return nil
		}
		for _, link := range links {
			if err := s.policies.Allow(ctx, link.ResourceType, tenantID, userID, link.ResourceID, link.Field); err == nil {
				return nil
			}
		}
		return errs.NewKey(errs.CodeForbidden, "attachment.resource_forbidden", "resource access denied")
	case status.AttachmentPending, status.AttachmentFailed:
		if row.UploaderID != userID {
			return errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
		}
		if row.ExpiresAt != nil && time.Now().UTC().After(*row.ExpiresAt) {
			return errs.NewKey(errs.CodeForbidden, "attachment.expired", "attachment expired")
		}
		return nil
	default:
		return errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
}

func (s *AttachmentService) getOwned(ctx context.Context, tenantID, id string) (*model.Attachment, error) {
	repos, repoErr := newAttachmentRepositories(ctx, s.db)
	if repoErr != nil {
		return nil, repoErr
	}
	row, err := repos.attachments.GetByID(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	if err != nil {
		return nil, mapAttachmentError(err)
	}
	if row.TenantID != tenantID {
		return nil, errs.NewKey(errs.CodeNotFound, "attachment.not_found", "attachment not found")
	}
	return &row, nil
}

func (s *AttachmentService) markFailed(ctx context.Context, row *model.Attachment, claimedJTI, category string) error {
	logging.Info(ctx, "attachment upload failed", "attachment_id", row.ID, "result_category", category, "expected_size", row.ExpectedSize)
	repos, err := newAttachmentRepositories(ctx, s.db)
	if err != nil {
		return err
	}
	res := repos.attachments.Where("id = ? AND status = ? AND upload_jti = ?", row.ID, status.AttachmentPending, claimedJTI).Updates(map[string]any{
		"status": status.AttachmentFailed, "upload_jti": "", "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 1 {
		_ = s.store.Delete(ctx, row.ObjectKey)
	}
	return nil
}

func toAttachmentView(row model.Attachment) AttachmentView {
	v := AttachmentView{
		ID: row.ID, OriginalName: row.OriginalName, ContentType: row.ContentType,
		Size: row.Size, SHA256: row.SHA256, Status: string(row.Status), UploaderID: row.UploaderID,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if s := row.GetThumbStatus(); s != "" {
		v.ThumbStatus = string(s)
	}
	return v
}

func validateOriginalName(name string) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("invalid utf-8 name")
	}
	if strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("illegal characters in name")
	}
	if len(name) > 255 {
		return fmt.Errorf("name too long")
	}
	return nil
}

func extAllowed(allowed []string, ext string) bool {
	for _, item := range allowed {
		if strings.EqualFold(item, ext) {
			return true
		}
	}
	return false
}

func mimeAllowed(allowed []string, mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	for _, item := range allowed {
		if strings.EqualFold(item, mime) {
			return true
		}
	}
	return false
}

func setAttachmentAudit(ctx context.Context, attachmentID, summary string) {
	if ac, ok := auth.FromContext(ctx); ok && ac != nil {
		ac.AuditResourceType = "attachment"
		ac.AuditResourceID = attachmentID
		ac.AuditSummary = summary
	}
}

func mapAttachmentError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, tenant.ErrTenantContextRequired) {
		return errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	if errors.Is(err, storage.ErrNotFound) {
		return errs.NewKey(errs.CodeNotFound, "attachment.object_missing", "attachment object missing")
	}
	if errors.Is(err, storage.ErrSizeMismatch) {
		return errs.ValidationKey("attachment.size_mismatch", "size mismatch")
	}
	if errors.Is(err, storage.ErrInvalidKey) {
		return errs.ValidationKey("attachment.invalid_key", err.Error())
	}
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	return errs.Wrap(errs.CodeInternal, "attachment operation failed", err)
}

// ContentDisposition builds a safe Content-Disposition header value.
func ContentDisposition(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '\\' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, filename)
	if safe == "" {
		safe = "download"
	}
	escaped := strings.ReplaceAll(safe, `"`, "")
	return fmt.Sprintf(`attachment; filename="%s"`, escaped)
}

var _ attachment.ResourceLinker = (*AttachmentService)(nil)
