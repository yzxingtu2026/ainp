package notification

// NotificationRequestStatus is the lifecycle of a notification request.
type NotificationRequestStatus string

const (
	RequestPending   NotificationRequestStatus = "pending"
	RequestProcessed NotificationRequestStatus = "processed"
	RequestFailed    NotificationRequestStatus = "failed"
)

func (s NotificationRequestStatus) Valid() bool {
	switch s {
	case RequestPending, RequestProcessed, RequestFailed:
		return true
	default:
		return false
	}
}

// NotificationDeliveryStatus is the lifecycle of a single delivery attempt.
type NotificationDeliveryStatus string

const (
	DeliveryPending   NotificationDeliveryStatus = "pending"
	DeliveryQueued    NotificationDeliveryStatus = "queued"
	DeliverySending   NotificationDeliveryStatus = "sending"
	DeliverySent      NotificationDeliveryStatus = "sent"
	DeliveryDelivered NotificationDeliveryStatus = "delivered"
	DeliveryFailed    NotificationDeliveryStatus = "failed"
	DeliveryDead      NotificationDeliveryStatus = "dead"
	DeliveryCancelled NotificationDeliveryStatus = "cancelled"
	DeliverySkipped   NotificationDeliveryStatus = "skipped"
)

func (s NotificationDeliveryStatus) Valid() bool {
	switch s {
	case DeliveryPending, DeliveryQueued, DeliverySending, DeliverySent,
		DeliveryDelivered, DeliveryFailed, DeliveryDead, DeliveryCancelled, DeliverySkipped:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if the delivery is in a terminal state.
func (s NotificationDeliveryStatus) IsTerminal() bool {
	switch s {
	case DeliverySent, DeliveryDelivered, DeliveryDead, DeliveryCancelled, DeliverySkipped:
		return true
	default:
		return false
	}
}
