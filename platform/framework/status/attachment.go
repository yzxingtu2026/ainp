package status

// AttachmentStatus is the lifecycle of an attachment metadata record.
type AttachmentStatus string

const (
	AttachmentPending  AttachmentStatus = "pending"
	AttachmentReady    AttachmentStatus = "ready"
	AttachmentDeleting AttachmentStatus = "deleting"
	AttachmentDeleted  AttachmentStatus = "deleted"
	AttachmentFailed   AttachmentStatus = "failed"
)

func (s AttachmentStatus) Valid() bool {
	switch s {
	case AttachmentPending, AttachmentReady, AttachmentDeleting, AttachmentDeleted, AttachmentFailed:
		return true
	default:
		return false
	}
}
