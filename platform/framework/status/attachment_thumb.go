package status

// AttachmentThumbStatus is the lifecycle of an attachment derivative (e.g. thumb).
// Independent from AttachmentStatus — original ready must not depend on thumb success.
type AttachmentThumbStatus string

const (
	AttachmentThumbPending AttachmentThumbStatus = "pending"
	AttachmentThumbReady   AttachmentThumbStatus = "ready"
	AttachmentThumbFailed  AttachmentThumbStatus = "failed"
	AttachmentThumbSkipped AttachmentThumbStatus = "skipped"
)

func (s AttachmentThumbStatus) Valid() bool {
	switch s {
	case AttachmentThumbPending, AttachmentThumbReady, AttachmentThumbFailed, AttachmentThumbSkipped:
		return true
	default:
		return false
	}
}
