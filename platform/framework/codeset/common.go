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

	KeyAttachmentThumbStatus     = "system.attachment_thumb_status"
	NameAttachmentThumbStatus    = "codeset.system.attachment_thumb_status"
	LabelAttachmentThumbPending  = "codeset.system.attachment_thumb_status.pending"
	LabelAttachmentThumbReady    = "codeset.system.attachment_thumb_status.ready"
	LabelAttachmentThumbFailed   = "codeset.system.attachment_thumb_status.failed"
	LabelAttachmentThumbSkipped  = "codeset.system.attachment_thumb_status.skipped"

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

	KeyJobRunStatus         = "system.job_run_status"
	NameJobRunStatus        = "codeset.system.job_run_status"
	LabelJobPendingEnqueue  = "codeset.system.job_run_status.pending_enqueue"
	LabelJobQueued          = "codeset.system.job_run_status.queued"
	LabelJobRunning         = "codeset.system.job_run_status.running"
	LabelJobRetrying        = "codeset.system.job_run_status.retrying"
	LabelJobSucceeded       = "codeset.system.job_run_status.succeeded"
	LabelJobFailed          = "codeset.system.job_run_status.failed"
	LabelJobDead            = "codeset.system.job_run_status.dead"
	LabelJobCancelled       = "codeset.system.job_run_status.cancelled"
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

func AttachmentThumbStatus() CodeSet {
	return CodeSet{
		Key: KeyAttachmentThumbStatus, NameKey: NameAttachmentThumbStatus,
		Items: []CodeItem{
			{Value: string(status.AttachmentThumbPending), LabelKey: LabelAttachmentThumbPending, Semantic: SemanticWarning, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(status.AttachmentThumbReady), LabelKey: LabelAttachmentThumbReady, Semantic: SemanticSuccess, Icon: "mdi:image-outline", Order: 20},
			{Value: string(status.AttachmentThumbFailed), LabelKey: LabelAttachmentThumbFailed, Semantic: SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 30},
			{Value: string(status.AttachmentThumbSkipped), LabelKey: LabelAttachmentThumbSkipped, Semantic: SemanticMuted, Icon: "mdi:minus-circle-outline", Order: 40},
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

func JobRunStatus() CodeSet {
	return CodeSet{
		Key: KeyJobRunStatus, NameKey: NameJobRunStatus,
		Items: []CodeItem{
			{Value: string(status.JobPendingEnqueue), LabelKey: LabelJobPendingEnqueue, Semantic: SemanticWarning, Icon: "mdi:timer-sand", Order: 10},
			{Value: string(status.JobQueued), LabelKey: LabelJobQueued, Semantic: SemanticInfo, Icon: "mdi:playlist-plus", Order: 20},
			{Value: string(status.JobRunning), LabelKey: LabelJobRunning, Semantic: SemanticInfo, Icon: "mdi:play-circle-outline", Order: 30},
			{Value: string(status.JobRetrying), LabelKey: LabelJobRetrying, Semantic: SemanticWarning, Icon: "mdi:restart", Order: 40},
			{Value: string(status.JobSucceeded), LabelKey: LabelJobSucceeded, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 50},
			{Value: string(status.JobFailed), LabelKey: LabelJobFailed, Semantic: SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 60},
			{Value: string(status.JobDead), LabelKey: LabelJobDead, Semantic: SemanticMuted, Icon: "mdi:skull-outline", Order: 70},
			{Value: string(status.JobCancelled), LabelKey: LabelJobCancelled, Semantic: SemanticMuted, Icon: "mdi:cancel", Order: 80},
		},
	}
}
