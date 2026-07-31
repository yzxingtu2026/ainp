package jobs

import (
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	notificationjobs "github.com/yzxingtu2026/ainp/platform/modules/system/notifications/jobs"
)

const NotificationDeliverJobType = notificationjobs.NotificationDeliverJobType

type (
	NotificationDeliverPayload = notificationjobs.NotificationDeliverPayload
	NotificationDeliverHandler = notificationjobs.NotificationDeliverHandler
	NotificationDeliverStore   = notificationjobs.NotificationDeliverStore
	DeliveryContext            = notificationjobs.DeliveryContext
)

func NewNotificationDeliverHandler(store NotificationDeliverStore, providers map[notification.Channel]notification.ChannelProvider, renderer *notification.Renderer, resolver *notification.TemplateResolver, defs *notification.DefinitionRegistry) *NotificationDeliverHandler {
	return notificationjobs.NewNotificationDeliverHandler(store, providers, renderer, resolver, defs)
}
