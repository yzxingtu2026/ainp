package system

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/modules/system/notifications"
	"gorm.io/gorm"
)

type (
	NotificationRequest          = notifications.NotificationRequest
	NotificationDelivery         = notifications.NotificationDelivery
	NotificationTemplateOverride = notifications.NotificationTemplateOverride
	UserNotification             = notifications.UserNotification
	NotificationRepository       = notifications.NotificationRepository
	DeliveryDetailRow            = notifications.DeliveryDetailRow
	GormOverrideProvider         = notifications.GormOverrideProvider
	InAppProvider                = notifications.InAppProvider
	InAppProviderStore           = notifications.InAppProviderStore
	InAppNotificationService     = notifications.InAppNotificationService
	NotificationTemplateService  = notifications.NotificationTemplateService
	NotificationDeliveryService  = notifications.NotificationDeliveryService
	NotificationService          = notifications.NotificationService
	GormDeliverStore             = notifications.GormDeliverStore
	NotificationJobEnqueuer      = notifications.NotificationJobEnqueuer
	NotificationOutboxHandler    = notifications.NotificationOutboxHandler
	NotificationOutboxPayload    = notifications.NotificationOutboxPayload
)

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return notifications.NewNotificationRepository(db)
}
func NewGormOverrideProvider(repo *NotificationRepository) *GormOverrideProvider {
	return notifications.NewGormOverrideProvider(repo)
}
func NewInAppProvider(repo *NotificationRepository) *InAppProvider {
	return notifications.NewInAppProvider(repo)
}
func NewInAppProviderStore(db *gorm.DB, repo *NotificationRepository) *InAppProviderStore {
	return notifications.NewInAppProviderStore(db, repo)
}
func NewInAppNotificationService(repo *NotificationRepository) *InAppNotificationService {
	return notifications.NewInAppNotificationService(repo)
}
func NewNotificationTemplateService(repo *NotificationRepository, registry *notification.DefinitionRegistry, renderer *notification.Renderer, override notification.OverrideProvider) *NotificationTemplateService {
	return notifications.NewNotificationTemplateService(repo, registry, renderer, override)
}
func NewNotificationDeliveryService(repo *NotificationRepository, enqueuer ...job.Enqueuer) *NotificationDeliveryService {
	return notifications.NewNotificationDeliveryService(repo, enqueuer...)
}
func NewNotificationService(db *gorm.DB, repo *NotificationRepository, defs *notification.DefinitionRegistry, renderer *notification.Renderer, resolver *notification.TemplateResolver, recipients notification.RecipientResolver, enqueuer notification.OrchestratorEnqueuer, providers map[notification.Channel]notification.ChannelProvider) *NotificationService {
	return notifications.NewNotificationService(db, repo, defs, renderer, resolver, recipients, enqueuer, providers)
}
func NewGormDeliverStore(repo *NotificationRepository) *GormDeliverStore {
	return notifications.NewGormDeliverStore(repo)
}
func NewOperationsNotificationDeliveryService(a *app.App) *NotificationDeliveryService {
	return notifications.NewOperationsNotificationDeliveryService(a)
}
func NewNotificationJobEnqueuer(fn func(context.Context, string) error) *NotificationJobEnqueuer {
	return notifications.NewNotificationJobEnqueuer(fn)
}
func NewNotificationOutboxHandler(service *NotificationService) *NotificationOutboxHandler {
	return notifications.NewNotificationOutboxHandler(service)
}
func UserExistsFn(db *gorm.DB) func(context.Context, string, string) (bool, error) {
	return notifications.UserExistsFn(db)
}
func registerNotificationActions(a *app.App) error { return notifications.RegisterActions(a) }
