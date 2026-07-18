package system

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

// KnowledgeSourceResolver is the only bridge from knowledge parsing to attachment bytes.
// It reauthorizes and validates the persisted source snapshot on every open.
type KnowledgeSourceResolver struct {
	db        *gorm.DB
	store     storage.ObjectStore
	knowledge aiknowledge.Service
}

func NewKnowledgeSourceResolver(db *gorm.DB, store storage.ObjectStore, knowledge aiknowledge.Service) *KnowledgeSourceResolver {
	return &KnowledgeSourceResolver{db: db, store: store, knowledge: knowledge}
}

func (r *KnowledgeSourceResolver) OpenSource(ctx context.Context, req aiknowledge.SourceOpenRequest) (aiknowledge.DocumentSource, error) {
	if r.db == nil || r.store == nil || r.knowledge == nil || req.MaxBytes <= 0 {
		return aiknowledge.DocumentSource{}, aiknowledge.ErrUnsupportedFormat
	}
	authCtx, ok := auth.FromContext(ctx)
	if !ok || !authCtx.Authenticated || authCtx.TenantID == "" || authCtx.UserID == "" {
		return aiknowledge.DocumentSource{}, fmt.Errorf("knowledge source not found")
	}
	if err := r.knowledge.CanReadDocument(ctx, req.Document.ID); err != nil {
		return aiknowledge.DocumentSource{}, err
	}
	current, err := r.knowledge.GetDocumentRef(ctx, req.Document.ID)
	if err != nil {
		return aiknowledge.DocumentSource{}, err
	}
	if current.TenantID != authCtx.TenantID || current.AttachmentID != req.Document.AttachmentID || current.SourceRevision != req.Document.SourceRevision {
		return aiknowledge.DocumentSource{}, fmt.Errorf("knowledge source revision changed")
	}
	var attachmentRow model.Attachment
	err = r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Where("id = ?", current.AttachmentID).First(&attachmentRow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return aiknowledge.DocumentSource{}, fmt.Errorf("knowledge source not found")
	}
	if err != nil {
		return aiknowledge.DocumentSource{}, err
	}
	if attachmentRow.TenantID != current.TenantID || attachmentRow.Status != status.AttachmentReady || attachmentRow.SHA256 != current.SourceRevision || attachmentRow.Size > req.MaxBytes {
		return aiknowledge.DocumentSource{}, fmt.Errorf("knowledge source snapshot invalid")
	}
	var links int64
	err = r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&model.AttachmentLink{}).
		Where("attachment_id = ? AND resource_type = ? AND resource_id = ? AND field = ?", attachmentRow.ID, "ai.knowledge_document", current.ID, "source").Count(&links).Error
	if err != nil {
		return aiknowledge.DocumentSource{}, err
	}
	if links != 1 {
		return aiknowledge.DocumentSource{}, fmt.Errorf("knowledge source is no longer linked")
	}
	rc, info, err := r.store.Open(ctx, attachmentRow.ObjectKey)
	if err != nil {
		return aiknowledge.DocumentSource{}, err
	}
	if info.Size != attachmentRow.Size || info.Size > req.MaxBytes {
		_ = rc.Close()
		return aiknowledge.DocumentSource{}, aiknowledge.ErrLimitExceeded
	}
	return aiknowledge.DocumentSource{Reader: &contextReadCloser{ctx: ctx, reader: rc}, Size: info.Size}, nil
}

type contextReadCloser struct {
	ctx    context.Context
	reader io.ReadCloser
}

func (r *contextReadCloser) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func (r *contextReadCloser) Close() error { return r.reader.Close() }

var _ aiknowledge.DocumentSourceResolver = (*KnowledgeSourceResolver)(nil)
