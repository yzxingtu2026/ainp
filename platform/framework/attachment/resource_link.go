package attachment

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type ResourceLinkRequest struct {
	AttachmentID string `json:"attachment_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Field        string `json:"field"`
}

func (r ResourceLinkRequest) Validate() error {
	if r.AttachmentID == "" || r.ResourceType == "" || r.ResourceID == "" {
		return fmt.Errorf("attachment_id, resource_type and resource_id are required")
	}
	return nil
}

type ResourceLink struct {
	ID           string
	AttachmentID string
	ResourceType string
	ResourceID   string
	Field        string
}

// ResourceLinker binds business resources without exposing attachment persistence details.
// The supplied transaction is mandatory so the business row and link commit atomically.
type ResourceLinker interface {
	BindResource(context.Context, *gorm.DB, ResourceLinkRequest) (ResourceLink, error)
	UnbindResource(context.Context, *gorm.DB, ResourceLinkRequest) error
}

type resourceTransactionKey struct{}
type resourceLinkOperationKey struct{}

type ResourceLinkOperation string

const (
	ResourceLinkBind   ResourceLinkOperation = "bind"
	ResourceLinkUnbind ResourceLinkOperation = "unbind"
)

// WithResourceTransaction lets an AccessPolicy observe business rows written in the
// same transaction. Only ResourceLinker implementations should attach this value.
func WithResourceTransaction(ctx context.Context, tx *gorm.DB) context.Context {
	if tx == nil {
		return ctx
	}
	return context.WithValue(ctx, resourceTransactionKey{}, tx)
}

// ResourceTransaction returns the controlled transaction or the provided fallback.
func ResourceTransaction(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(resourceTransactionKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}

func WithResourceLinkOperation(ctx context.Context, operation ResourceLinkOperation) context.Context {
	return context.WithValue(ctx, resourceLinkOperationKey{}, operation)
}

func ResourceLinkOperationFromContext(ctx context.Context) ResourceLinkOperation {
	operation, _ := ctx.Value(resourceLinkOperationKey{}).(ResourceLinkOperation)
	return operation
}
