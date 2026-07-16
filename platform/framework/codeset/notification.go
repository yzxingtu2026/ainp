package codeset

import (
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
)

// NotificationRequestStatus returns the CodeSet for notification request lifecycle.
func NotificationRequestStatus() CodeSet {
	return CodeSet{
		Key:     "system.notification_request_status",
		NameKey: "codeset.notification_request_status.name",
		Items: []CodeItem{
			{Value: string(notification.RequestPending), Label: "Pending", LabelKey: "codeset.notification_request_status.pending", Semantic: SemanticDefault},
			{Value: string(notification.RequestProcessed), Label: "Processed", LabelKey: "codeset.notification_request_status.processed", Semantic: SemanticSuccess},
			{Value: string(notification.RequestFailed), Label: "Failed", LabelKey: "codeset.notification_request_status.failed", Semantic: SemanticDanger},
		},
	}
}

// NotificationDeliveryStatus returns the CodeSet for notification delivery lifecycle.
func NotificationDeliveryStatus() CodeSet {
	return CodeSet{
		Key:     "system.notification_delivery_status",
		NameKey: "codeset.notification_delivery_status.name",
		Items: []CodeItem{
			{Value: string(notification.DeliveryPending), Label: "Pending", LabelKey: "codeset.notification_delivery_status.pending", Semantic: SemanticDefault},
			{Value: string(notification.DeliveryQueued), Label: "Queued", LabelKey: "codeset.notification_delivery_status.queued", Semantic: SemanticInfo},
			{Value: string(notification.DeliverySending), Label: "Sending", LabelKey: "codeset.notification_delivery_status.sending", Semantic: SemanticInfo},
			{Value: string(notification.DeliverySent), Label: "Sent", LabelKey: "codeset.notification_delivery_status.sent", Semantic: SemanticSuccess},
			{Value: string(notification.DeliveryDelivered), Label: "Delivered", LabelKey: "codeset.notification_delivery_status.delivered", Semantic: SemanticSuccess},
			{Value: string(notification.DeliveryFailed), Label: "Failed", LabelKey: "codeset.notification_delivery_status.failed", Semantic: SemanticDanger},
			{Value: string(notification.DeliveryDead), Label: "Dead", LabelKey: "codeset.notification_delivery_status.dead", Semantic: SemanticDanger},
			{Value: string(notification.DeliveryCancelled), Label: "Cancelled", LabelKey: "codeset.notification_delivery_status.cancelled", Semantic: SemanticMuted},
			{Value: string(notification.DeliverySkipped), Label: "Skipped", LabelKey: "codeset.notification_delivery_status.skipped", Semantic: SemanticMuted},
		},
	}
}
