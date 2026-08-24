// Package platformresource defines the stable, provider-neutral platform
// resource contracts available to downstream modules.
package platformresource

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

var (
	ErrUnavailable = errors.New("platform resource unavailable")
	ErrForbidden   = errors.New("platform resource forbidden")
	ErrConflict    = errors.New("platform resource revision conflict")
)

type AttachmentVariant string

const (
	AttachmentVariantOriginal AttachmentVariant = "original"
	AttachmentVariantThumb    AttachmentVariant = "thumb"
)

type AttachmentBindingQuery struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Field        string `json:"field,omitempty"`
}

type AttachmentOpenRequest struct {
	AttachmentID string                 `json:"attachment_id"`
	Binding      AttachmentBindingQuery `json:"binding"`
	Variant      AttachmentVariant      `json:"variant,omitempty"`
}

type AttachmentMetadata struct {
	ID           string            `json:"id"`
	OriginalName string            `json:"original_name"`
	ContentType  string            `json:"content_type"`
	Size         int64             `json:"size"`
	SHA256       string            `json:"sha256,omitempty"`
	Variant      AttachmentVariant `json:"variant"`
	CreatedAt    time.Time         `json:"created_at"`
}

type AttachmentContent struct {
	Metadata AttachmentMetadata `json:"metadata"`
	Reader   io.ReadCloser      `json:"-"`
}

type AttachmentBinding struct {
	Attachment   AttachmentMetadata `json:"attachment"`
	ResourceType string             `json:"resource_type"`
	ResourceID   string             `json:"resource_id"`
	Field        string             `json:"field,omitempty"`
}

type AttachmentReader interface {
	Open(context.Context, AttachmentOpenRequest) (*AttachmentContent, error)
	BindingExists(context.Context, AttachmentOpenRequest) (bool, error)
	ListBindings(context.Context, AttachmentBindingQuery) ([]AttachmentBinding, error)
}

type PrincipalProjection struct {
	Principal   auth.PrincipalRef `json:"principal"`
	DisplayName string            `json:"display_name"`
	Active      bool              `json:"active"`
	Revision    uint64            `json:"revision"`
}

type PrincipalInvalidation struct {
	Principal          auth.PrincipalRef `json:"principal"`
	ExpectedRevision   uint64            `json:"expected_revision"`
	RequiredPermission string            `json:"-"`
	Reason             string            `json:"reason"`
}

type PrincipalService interface {
	Get(context.Context, auth.PrincipalRef) (*PrincipalProjection, error)
	Invalidate(context.Context, PrincipalInvalidation) (*PrincipalProjection, error)
}

// Services is the immutable platform-resource surface injected into API and
// Worker module registration through app.App.
type Services struct {
	Attachments AttachmentReader
	Principals  PrincipalService
}

func (s Services) Validate() error {
	if s.Attachments == nil || s.Principals == nil {
		return ErrUnavailable
	}
	return nil
}
