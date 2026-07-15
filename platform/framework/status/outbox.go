package status

// OutboxStatus is the lifecycle of a transactional outbox event row.
type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxProcessing OutboxStatus = "processing"
	OutboxPublished  OutboxStatus = "published"
	OutboxFailed     OutboxStatus = "failed"
	OutboxDead       OutboxStatus = "dead"
)

func (s OutboxStatus) Valid() bool {
	switch s {
	case OutboxPending, OutboxProcessing, OutboxPublished, OutboxFailed, OutboxDead:
		return true
	default:
		return false
	}
}

// DeliveryStatus is the per-handler receipt for an outbox event.
type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryProcessing DeliveryStatus = "processing"
	DeliverySucceeded  DeliveryStatus = "succeeded"
	DeliveryFailed     DeliveryStatus = "failed"
)

func (s DeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryProcessing, DeliverySucceeded, DeliveryFailed:
		return true
	default:
		return false
	}
}
