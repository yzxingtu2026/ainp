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

	KeyOutboxStatus       = "system.outbox_status"
	NameOutboxStatus      = "codeset.system.outbox_status"
	LabelOutboxPending    = "codeset.system.outbox_status.pending"
	LabelOutboxProcessing = "codeset.system.outbox_status.processing"
	LabelOutboxPublished  = "codeset.system.outbox_status.published"
	LabelOutboxFailed     = "codeset.system.outbox_status.failed"
	LabelOutboxDead       = "codeset.system.outbox_status.dead"

	KeyDeliveryStatus       = "system.delivery_status"
	NameDeliveryStatus      = "codeset.system.delivery_status"
	LabelDeliveryPending    = "codeset.system.delivery_status.pending"
	LabelDeliveryProcessing = "codeset.system.delivery_status.processing"
	LabelDeliverySucceeded  = "codeset.system.delivery_status.succeeded"
	LabelDeliveryFailed     = "codeset.system.delivery_status.failed"
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

func OutboxStatus() CodeSet {
	return CodeSet{
		Key: KeyOutboxStatus, NameKey: NameOutboxStatus,
		Items: []CodeItem{
			{Value: string(status.OutboxPending), LabelKey: LabelOutboxPending, Semantic: SemanticWarning, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(status.OutboxProcessing), LabelKey: LabelOutboxProcessing, Semantic: SemanticInfo, Icon: "mdi:progress-clock", Order: 20},
			{Value: string(status.OutboxPublished), LabelKey: LabelOutboxPublished, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 30},
			{Value: string(status.OutboxFailed), LabelKey: LabelOutboxFailed, Semantic: SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 40},
			{Value: string(status.OutboxDead), LabelKey: LabelOutboxDead, Semantic: SemanticMuted, Icon: "mdi:skull-outline", Order: 50},
		},
	}
}

func DeliveryStatus() CodeSet {
	return CodeSet{
		Key: KeyDeliveryStatus, NameKey: NameDeliveryStatus,
		Items: []CodeItem{
			{Value: string(status.DeliveryPending), LabelKey: LabelDeliveryPending, Semantic: SemanticWarning, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(status.DeliveryProcessing), LabelKey: LabelDeliveryProcessing, Semantic: SemanticInfo, Icon: "mdi:progress-clock", Order: 20},
			{Value: string(status.DeliverySucceeded), LabelKey: LabelDeliverySucceeded, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 30},
			{Value: string(status.DeliveryFailed), LabelKey: LabelDeliveryFailed, Semantic: SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 40},
		},
	}
}
