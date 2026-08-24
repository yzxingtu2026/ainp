package attachments

import (
	"context"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/model"
)

// ResourceReader is the narrow downstream attachment contract. It deliberately
// lives beside the implementation so the framework package stays provider-neutral.
type ResourceReader struct{ service *AttachmentService }

func (s *AttachmentService) ResourceReader() platformresource.AttachmentReader {
	return ResourceReader{service: s}
}

func (r ResourceReader) identity(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || strings.TrimSpace(ac.TenantID) == "" || !attachmentPrincipal(ctx).Valid() {
		return nil, platformresource.ErrForbidden
	}
	return ac, nil
}

func (r ResourceReader) authorizeBinding(ctx context.Context, q platformresource.AttachmentBindingQuery) (*auth.Context, []model.AttachmentLink, error) {
	ac, err := r.identity(ctx)
	if err != nil || strings.TrimSpace(q.ResourceType) == "" || strings.TrimSpace(q.ResourceID) == "" {
		return nil, nil, platformresource.ErrUnavailable
	}
	if err := r.service.policies.Allow(ctx, q.ResourceType, ac.TenantID, ac.PrincipalID, q.ResourceID, q.Field); err != nil {
		return nil, nil, platformresource.ErrForbidden
	}
	repos, err := newAttachmentRepositories(ctx, r.service.db)
	if err != nil {
		return nil, nil, platformresource.ErrUnavailable
	}
	query := repos.links.Where("resource_type = ? AND resource_id = ?", q.ResourceType, q.ResourceID)
	if q.Field != "" {
		query = query.Where("field = ?", q.Field)
	}
	links, err := query.Find("1=1")
	if err != nil {
		return nil, nil, platformresource.ErrUnavailable
	}
	return ac, links, nil
}

func attachmentMetadata(row model.Attachment, variant platformresource.AttachmentVariant) platformresource.AttachmentMetadata {
	if variant == platformresource.AttachmentVariantThumb {
		return platformresource.AttachmentMetadata{ID: row.ID, OriginalName: thumbDownloadName(row.OriginalName), ContentType: row.ThumbContentType, Size: row.ThumbSize, Variant: variant, CreatedAt: row.CreatedAt}
	}
	return platformresource.AttachmentMetadata{ID: row.ID, OriginalName: row.OriginalName, ContentType: row.ContentType, Size: row.Size, SHA256: row.SHA256, Variant: platformresource.AttachmentVariantOriginal, CreatedAt: row.CreatedAt}
}

func (r ResourceReader) Open(ctx context.Context, req platformresource.AttachmentOpenRequest) (*platformresource.AttachmentContent, error) {
	ac, links, err := r.authorizeBinding(ctx, req.Binding)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, platformresource.ErrUnavailable
	}
	for _, link := range links {
		if strings.TrimSpace(req.AttachmentID) == "" || link.AttachmentID != req.AttachmentID {
			continue
		}
		row, getErr := r.service.getOwned(ctx, ac.TenantID, link.AttachmentID)
		if getErr != nil || row.Status != status.AttachmentReady {
			continue
		}
		variant := req.Variant
		if variant == "" {
			variant = platformresource.AttachmentVariantOriginal
		}
		key := row.ObjectKey
		if variant == platformresource.AttachmentVariantThumb {
			if row.GetThumbStatus() != status.AttachmentThumbReady || row.ThumbObjectKey == "" {
				continue
			}
			key = row.ThumbObjectKey
		} else if variant != platformresource.AttachmentVariantOriginal {
			return nil, platformresource.ErrUnavailable
		}
		reader, info, openErr := r.service.store.Open(ctx, key)
		if openErr != nil {
			return nil, platformresource.ErrUnavailable
		}
		metadata := attachmentMetadata(*row, variant)
		if info.Size != metadata.Size {
			_ = reader.Close()
			return nil, platformresource.ErrUnavailable
		}
		return &platformresource.AttachmentContent{Metadata: metadata, Reader: reader}, nil
	}
	return nil, platformresource.ErrUnavailable
}

func (r ResourceReader) BindingExists(ctx context.Context, req platformresource.AttachmentOpenRequest) (bool, error) {
	if strings.TrimSpace(req.AttachmentID) == "" {
		return false, platformresource.ErrUnavailable
	}
	_, links, err := r.authorizeBinding(ctx, req.Binding)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if link.AttachmentID == req.AttachmentID {
			row, getErr := r.service.getOwned(ctx, auth.MustFromContext(ctx).TenantID, link.AttachmentID)
			if getErr == nil && row.Status == status.AttachmentReady {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r ResourceReader) ListBindings(ctx context.Context, q platformresource.AttachmentBindingQuery) ([]platformresource.AttachmentBinding, error) {
	_, links, err := r.authorizeBinding(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]platformresource.AttachmentBinding, 0, len(links))
	for _, link := range links {
		row, getErr := r.service.getOwned(ctx, auth.MustFromContext(ctx).TenantID, link.AttachmentID)
		if getErr != nil || row.Status != status.AttachmentReady {
			continue
		}
		items = append(items, platformresource.AttachmentBinding{Attachment: attachmentMetadata(*row, platformresource.AttachmentVariantOriginal), ResourceType: link.ResourceType, ResourceID: link.ResourceID, Field: link.Field})
	}
	return items, nil
}

var _ platformresource.AttachmentReader = ResourceReader{}
