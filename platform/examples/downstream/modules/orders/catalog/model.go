package catalog

import "github.com/yzxingtu2026/ainp/platform/framework/platformresource"

type orderCreateRequest struct {
	Reference string `json:"reference" validate:"required,min=1,max=64"`
}

type orderBatchCreateRequest struct {
	Items []orderCreateRequest `json:"items" validate:"required,min=1,max=50,dive"`
}

type orderCreateResponse struct {
	Reference string `json:"reference"`
}

type orderBatchCreateResponse struct {
	CreatedCount int                   `json:"created_count"`
	Items        []orderCreateResponse `json:"items"`
}

type attachmentResourceRequest struct {
	AttachmentID string                             `json:"attachment_id" validate:"required"`
	ResourceID   string                             `json:"resource_id" validate:"required"`
	Field        string                             `json:"field,omitempty"`
	Variant      platformresource.AttachmentVariant `json:"variant,omitempty"`
}

func (r attachmentResourceRequest) openRequest() platformresource.AttachmentOpenRequest {
	return platformresource.AttachmentOpenRequest{AttachmentID: r.AttachmentID, Variant: r.Variant, Binding: platformresource.AttachmentBindingQuery{ResourceType: "example.order", ResourceID: r.ResourceID, Field: r.Field}}
}

type attachmentResourceResponse struct {
	Attachment platformresource.AttachmentMetadata `json:"attachment"`
	BytesRead  int                                 `json:"bytes_read"`
}

type principalResourceRequest struct {
	PrincipalID string `json:"principal_id" validate:"required"`
}

type principalInvalidationRequest struct {
	PrincipalID      string `json:"principal_id" validate:"required"`
	ExpectedRevision uint64 `json:"expected_revision" validate:"required,min=1"`
	Reason           string `json:"reason" validate:"required,max=128"`
}
