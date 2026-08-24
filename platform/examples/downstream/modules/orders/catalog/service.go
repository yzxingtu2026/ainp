package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
)

type Service struct{ Resources platformresource.Services }

func (Service) List(context.Context, json.RawMessage) (any, error) {
	return []string{}, nil
}

func (Service) BatchCreate(_ context.Context, request orderBatchCreateRequest) (any, error) {
	items := make([]orderCreateResponse, len(request.Items))
	for index, item := range request.Items {
		items[index] = orderCreateResponse{Reference: item.Reference}
	}
	return orderBatchCreateResponse{CreatedCount: len(items), Items: items}, nil
}

func (Service) Sync(context.Context, job.Envelope, json.RawMessage, job.Reporter) error {
	return nil
}

func (s Service) ReadAttachment(ctx context.Context, req attachmentResourceRequest) (any, error) {
	content, err := s.Resources.Attachments.Open(ctx, req.openRequest())
	if err != nil {
		return nil, err
	}
	defer content.Reader.Close()
	bytes, err := io.ReadAll(io.LimitReader(content.Reader, content.Metadata.Size+1))
	if err != nil || int64(len(bytes)) != content.Metadata.Size {
		return nil, fmt.Errorf("orders: read attachment: %w", platformresource.ErrUnavailable)
	}
	return attachmentResourceResponse{Attachment: content.Metadata, BytesRead: len(bytes)}, nil
}

func (s Service) AttachmentBindingExists(ctx context.Context, req attachmentResourceRequest) (any, error) {
	exists, err := s.Resources.Attachments.BindingExists(ctx, req.openRequest())
	if err != nil {
		return nil, err
	}
	return map[string]bool{"exists": exists}, nil
}

func (s Service) Principal(ctx context.Context, req principalResourceRequest) (any, error) {
	return s.Resources.Principals.Get(ctx, auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: req.PrincipalID})
}

func (s Service) InvalidatePrincipal(ctx context.Context, req principalInvalidationRequest) (any, error) {
	return s.Resources.Principals.Invalidate(ctx, platformresource.PrincipalInvalidation{
		Principal:        auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: req.PrincipalID},
		ExpectedRevision: req.ExpectedRevision, RequiredPermission: "orders:manage", Reason: req.Reason,
	})
}
