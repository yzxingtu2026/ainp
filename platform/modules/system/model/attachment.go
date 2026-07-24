package model

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

// Attachment is tenant-scoped binary object metadata. Bytes live in ObjectStore.
type Attachment struct {
	ID           string                  `gorm:"primaryKey;size:64"`
	TenantID     string                  `gorm:"size:64;index:idx_attachments_tenant;not null"`
	Provider     string                  `gorm:"size:32;not null"`
	ObjectKey    string                  `gorm:"size:512;not null"`
	OriginalName string                  `gorm:"size:512;not null"`
	ContentType  string                  `gorm:"size:128;not null"`
	Size         int64                   `gorm:"not null;default:0"`
	SHA256       string                  `gorm:"size:64"`
	Status       status.AttachmentStatus `gorm:"size:32;not null;index:idx_attachments_tenant_status"`
	UploaderID   string                  `gorm:"size:64;not null"`
	UploadJTI    string                  `gorm:"size:64"` // one-time upload token id; cleared after consume
	ExpectedSize int64                   `gorm:"not null;default:0"`
	ExpiresAt    *time.Time              // pending temp retain deadline
	// Thumb* holds the standard `thumb` derivative (cover ~200² JPEG). Independent of Status.
	// Pointer so upload_prepare inserts NULL (empty string violates ck_attachments_thumb_status).
	ThumbStatus      *status.AttachmentThumbStatus `gorm:"size:32;index:idx_attachments_thumb_status"`
	ThumbObjectKey   string                        `gorm:"size:512"`
	ThumbContentType string                        `gorm:"size:128"`
	ThumbSize        int64                         `gorm:"not null;default:0"`
	ThumbError       string                        `gorm:"size:512"`
	ThumbUpdatedAt   *time.Time
	CreatedAt        time.Time      `gorm:"autoCreateTime"`
	UpdatedAt        time.Time      `gorm:"autoUpdateTime"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

func (Attachment) TableName() string { return "attachments" }

// GetThumbStatus returns the derivative status, or empty when unset (NULL).
func (a Attachment) GetThumbStatus() status.AttachmentThumbStatus {
	if a.ThumbStatus == nil {
		return ""
	}
	return *a.ThumbStatus
}

// SetThumbStatus sets the derivative status pointer (nil clears to NULL).
func (a *Attachment) SetThumbStatus(s status.AttachmentThumbStatus) {
	if a == nil {
		return
	}
	if s == "" {
		a.ThumbStatus = nil
		return
	}
	v := s
	a.ThumbStatus = &v
}

// AttachmentLink binds an attachment to a business resource field.
type AttachmentLink struct {
	ID           string    `gorm:"primaryKey;size:64"`
	TenantID     string    `gorm:"size:64;uniqueIndex:ux_attachment_link;index:idx_attachment_links_resource;not null"`
	AttachmentID string    `gorm:"size:64;uniqueIndex:ux_attachment_link;index;not null"`
	ResourceType string    `gorm:"size:128;uniqueIndex:ux_attachment_link;index:idx_attachment_links_resource;not null"`
	ResourceID   string    `gorm:"size:128;uniqueIndex:ux_attachment_link;index:idx_attachment_links_resource;not null"`
	Field        string    `gorm:"size:64;uniqueIndex:ux_attachment_link;index:idx_attachment_links_resource;not null;default:''"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (AttachmentLink) TableName() string { return "attachment_links" }
