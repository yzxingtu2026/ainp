package codeset

import "github.com/yzxingtu2026/ainp/platform/framework/status"

const (
	KeyActivationStatus     = "common.activation_status"
	NameActivationStatus    = "codeset.common.activation_status"
	LabelActivationEnabled  = "codeset.common.activation_status.enabled"
	LabelActivationDisabled = "codeset.common.activation_status.disabled"

	KeyAttachmentStatus     = "system.attachment_status"
	NameAttachmentStatus    = "codeset.system.attachment_status"
	LabelAttachmentPending  = "codeset.system.attachment_status.pending"
	LabelAttachmentReady    = "codeset.system.attachment_status.ready"
	LabelAttachmentDeleting = "codeset.system.attachment_status.deleting"
	LabelAttachmentDeleted  = "codeset.system.attachment_status.deleted"
	LabelAttachmentFailed   = "codeset.system.attachment_status.failed"
)

func ActivationStatus() CodeSet {
	return CodeSet{
		Key: KeyActivationStatus, NameKey: NameActivationStatus,
		Items: []CodeItem{
			{Value: string(status.ActivationEnabled), LabelKey: LabelActivationEnabled, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 10},
			{Value: string(status.ActivationDisabled), LabelKey: LabelActivationDisabled, Semantic: SemanticMuted, Icon: "mdi:cancel", Order: 20},
		},
	}
}

func AttachmentStatus() CodeSet {
	return CodeSet{
		Key: KeyAttachmentStatus, NameKey: NameAttachmentStatus,
		Items: []CodeItem{
			{Value: string(status.AttachmentPending), LabelKey: LabelAttachmentPending, Semantic: SemanticWarning, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(status.AttachmentReady), LabelKey: LabelAttachmentReady, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 20},
			{Value: string(status.AttachmentDeleting), LabelKey: LabelAttachmentDeleting, Semantic: SemanticInfo, Icon: "mdi:delete-clock-outline", Order: 30},
			{Value: string(status.AttachmentDeleted), LabelKey: LabelAttachmentDeleted, Semantic: SemanticMuted, Icon: "mdi:delete-outline", Order: 40},
			{Value: string(status.AttachmentFailed), LabelKey: LabelAttachmentFailed, Semantic: SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 50},
		},
	}
}
