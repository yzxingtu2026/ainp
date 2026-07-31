package notifications

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/modules/system/notifications/jobs"
	"gorm.io/gorm"
)

// InAppProvider implements ChannelProvider for in-app (site) notifications.
type InAppProvider struct {
	repo *NotificationRepository
}

func NewInAppProvider(repo *NotificationRepository) *InAppProvider {
	return &InAppProvider{repo: repo}
}

func (p *InAppProvider) Name() string                  { return "in_app" }
func (p *InAppProvider) Channel() notification.Channel { return notification.ChannelInApp }

func (p *InAppProvider) Send(ctx context.Context, input notification.ProviderInput) (*notification.ProviderReceipt, error) {
	if input.Channel != notification.ChannelInApp {
		return nil, notification.NewPermanentError("wrong_channel", "InAppProvider only handles in_app channel")
	}

	// Build action_json
	actionJSON := []byte("{}")
	if input.Content.Action != nil {
		if err := input.Content.Action.Validate(); err != nil {
			return nil, notification.NewPermanentError("invalid_action", err.Error())
		}
		b, err := json.Marshal(input.Content.Action)
		if err != nil {
			return nil, notification.NewPermanentError("marshal_action", err.Error())
		}
		actionJSON = b
	}

	// Create user_notification (idempotent via delivery_id + user_id unique constraint)
	n := UserNotification{
		ID:         uuid.NewString(),
		UserID:     input.RecipientID,
		RequestID:  input.RequestID,
		DeliveryID: input.DeliveryID,
		Title:      input.Content.Title,
		Summary:    input.Content.Summary,
		Body:       input.Content.Body,
		Icon:       input.Content.Icon,
		LocalIcon:  input.Content.LocalIcon,
		ActionJSON: actionJSON,
		CreatedAt:  time.Now().UTC(),
	}

	if tenantIDFromCtx(ctx) == "" {
		return nil, notification.NewPermanentError("no_tenant", "tenant context required for in-app notification")
	}
	err := p.repo.CreateUserNotification(ctx, &n)
	if err != nil {
		if isUniqueViolation(err) {
			// Already created (idempotent), return success
			logging.Info(ctx, "in_app: duplicate delivery, idempotent skip",
				"delivery_id", input.DeliveryID,
				"user_id", input.RecipientID,
			)
			return &notification.ProviderReceipt{
				Provider:   "in_app",
				ExternalID: n.ID,
				AcceptedAt: time.Now().UTC(),
				Retryable:  false,
			}, nil
		}
		return nil, notification.NewRetryableError("db_error", err.Error())
	}

	logging.Info(ctx, "in_app: notification created",
		"delivery_id", input.DeliveryID,
		"user_id", input.RecipientID,
		"notification_id", n.ID,
	)

	return &notification.ProviderReceipt{
		Provider:   "in_app",
		ExternalID: n.ID,
		AcceptedAt: time.Now().UTC(),
		Retryable:  false,
	}, nil
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// GORM wraps the error; check the message
	msg := err.Error()
	return contains(msg, "duplicate key") || contains(msg, "23505") || contains(msg, "UNIQUE constraint")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Compile-time check
var _ notification.ChannelProvider = (*InAppProvider)(nil)

// InAppProviderStore wraps the InAppProvider for the Job handler's store interface.
type InAppProviderStore struct {
	db   *gorm.DB
	repo *NotificationRepository
}

func NewInAppProviderStore(db *gorm.DB, repo *NotificationRepository) *InAppProviderStore {
	return &InAppProviderStore{db: db, repo: repo}
}

// LoadDeliveryForJob implements the jobs.NotificationDeliverStore interface.
func (s *InAppProviderStore) LoadDelivery(ctx context.Context, deliveryID string) (*jobs.DeliveryContext, error) {
	del, err := s.repo.GetDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if del == nil {
		return nil, nil
	}
	req, err := s.repo.GetRequest(ctx, del.RequestID)
	if err != nil {
		return nil, err
	}
	varsJSON := []byte("{}")
	defKey := ""
	if req != nil {
		varsJSON = req.VariablesJSON
		defKey = req.DefinitionKey
	}
	return &jobs.DeliveryContext{
		DeliveryID:    del.ID,
		RequestID:     del.RequestID,
		TenantID:      del.TenantID,
		DefinitionKey: defKey,
		RecipientType: del.RecipientType,
		RecipientID:   del.RecipientID,
		Channel:       del.Channel,
		Locale:        del.Locale,
		VariablesJSON: varsJSON,
	}, nil
}

func (s *InAppProviderStore) UpdateDeliveryStatus(ctx context.Context, deliveryID string, status notification.NotificationDeliveryStatus, externalID, lastError string) error {
	return s.repo.UpdateDeliveryStatus(ctx, deliveryID, status, externalID, lastError)
}
