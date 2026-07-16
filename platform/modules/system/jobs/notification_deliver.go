package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
)

const (
	NotificationDeliverJobType = "notification.deliver"
)

// NotificationDeliverPayload is the job payload for notification delivery.
type NotificationDeliverPayload struct {
	DeliveryID string `json:"delivery_id"`
}

// NotificationDeliverHandler processes notification delivery jobs.
type NotificationDeliverHandler struct {
	store     NotificationDeliverStore
	providers map[notification.Channel]notification.ChannelProvider
	renderer  *notification.Renderer
	resolver  *notification.TemplateResolver
	defs      *notification.DefinitionRegistry
}

// NotificationDeliverStore abstracts the repository operations needed by the handler.
type NotificationDeliverStore interface {
	LoadDelivery(ctx context.Context, deliveryID string) (*DeliveryContext, error)
	UpdateDeliveryStatus(ctx context.Context, deliveryID string, status notification.NotificationDeliveryStatus, externalID, lastError string) error
}

// DeliveryContext holds everything the handler needs to process a delivery.
type DeliveryContext struct {
	DeliveryID    string
	RequestID     string
	TenantID      string
	DefinitionKey string
	RecipientType string
	RecipientID   string
	Channel       string
	Locale        string
	VariablesJSON []byte
}

func NewNotificationDeliverHandler(
	store NotificationDeliverStore,
	providers map[notification.Channel]notification.ChannelProvider,
	renderer *notification.Renderer,
	resolver *notification.TemplateResolver,
	defs *notification.DefinitionRegistry,
) *NotificationDeliverHandler {
	return &NotificationDeliverHandler{
		store:     store,
		providers: providers,
		renderer:  renderer,
		resolver:  resolver,
		defs:      defs,
	}
}

func (h *NotificationDeliverHandler) Type() string { return NotificationDeliverJobType }

func (h *NotificationDeliverHandler) Handle(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
	var p NotificationDeliverPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return job.Permanent(fmt.Errorf("notification.deliver: invalid payload: %w", err))
	}
	if p.DeliveryID == "" {
		return job.Permanent(fmt.Errorf("notification.deliver: delivery_id is required"))
	}

	logging.Info(ctx, "notification.deliver: starting",
		"delivery_id", p.DeliveryID,
		"job_id", env.ID,
	)

	// Load delivery context
	dc, err := h.store.LoadDelivery(ctx, p.DeliveryID)
	if err != nil {
		return job.Retryable(fmt.Errorf("notification.deliver: load delivery: %w", err))
	}
	if dc == nil {
		return job.Permanent(fmt.Errorf("notification.deliver: delivery %q not found", p.DeliveryID))
	}

	// Get definition
	def, err := h.defs.MustGet(dc.DefinitionKey)
	if err != nil {
		return job.Permanent(fmt.Errorf("notification.deliver: %w", err))
	}

	// Get provider
	ch := notification.Channel(dc.Channel)
	provider, ok := h.providers[ch]
	if !ok {
		return job.Permanent(fmt.Errorf("notification.deliver: no provider for channel %q", ch))
	}

	// Resolve template
	resolveResult, err := h.resolver.Resolve(dc.TenantID, def, ch, dc.Locale)
	if err != nil {
		return job.Permanent(fmt.Errorf("notification.deliver: template resolve: %w", err))
	}

	// Parse variables
	var variables map[string]any
	if len(dc.VariablesJSON) > 0 {
		if err := json.Unmarshal(dc.VariablesJSON, &variables); err != nil {
			return job.Permanent(fmt.Errorf("notification.deliver: parse variables: %w", err))
		}
	}
	if variables == nil {
		variables = make(map[string]any)
	}

	// Render template
	renderResult, err := h.resolver.Render(resolveResult, variables)
	if err != nil {
		return job.Permanent(fmt.Errorf("notification.deliver: render: %w", err))
	}

	// Build provider input
	providerInput := notification.ProviderInput{
		DeliveryID:     dc.DeliveryID,
		RequestID:      dc.RequestID,
		DefinitionKey:  dc.DefinitionKey,
		RecipientType:  notification.RecipientType(dc.RecipientType),
		RecipientID:    dc.RecipientID,
		Channel:        ch,
		Locale:         dc.Locale,
		Variables:      variables,
		IdempotencyKey: fmt.Sprintf("%s:%s", dc.RequestID, dc.DeliveryID),
		Content: notification.RenderedContent{
			Subject:        renderResult.Subject,
			Body:           renderResult.Body,
			Title:          renderResult.Title,
			Summary:        renderResult.Summary,
			Icon:           renderResult.Icon,
			LocalIcon:      renderResult.LocalIcon,
			Action:         renderResult.Action,
			TextBody:       renderResult.TextBody,
			HTMLBody:       renderResult.HTMLBody,
			SMSText:        renderResult.SMSText,
			WebhookPayload: renderResult.WebhookPayload,
			IMText:         renderResult.IMText,
			IMCardJSON:     renderResult.IMCardJSON,
		},
	}

	// Mark as sending
	_ = h.store.UpdateDeliveryStatus(ctx, dc.DeliveryID, notification.DeliverySending, "", "")

	// Call provider
	receipt, sendErr := provider.Send(ctx, providerInput)
	if sendErr != nil {
		retryable := notification.IsRetryable(sendErr)
		if retryable {
			_ = h.store.UpdateDeliveryStatus(ctx, dc.DeliveryID, notification.DeliveryFailed, "", sendErr.Error())
			logging.Info(ctx, "notification.deliver: retryable failure",
				"delivery_id", dc.DeliveryID,
				"provider", provider.Name(),
				"error", sendErr.Error(),
			)
			return job.Retryable(sendErr)
		}
		_ = h.store.UpdateDeliveryStatus(ctx, dc.DeliveryID, notification.DeliveryDead, "", sendErr.Error())
		logging.Info(ctx, "notification.deliver: permanent failure",
			"delivery_id", dc.DeliveryID,
			"provider", provider.Name(),
			"error", sendErr.Error(),
		)
		return job.Permanent(sendErr)
	}

	// Success
	externalID := ""
	if receipt != nil {
		externalID = receipt.ExternalID
	}
	_ = h.store.UpdateDeliveryStatus(ctx, dc.DeliveryID, notification.DeliveryDelivered, externalID, "")

	logging.Info(ctx, "notification.deliver: completed",
		"delivery_id", dc.DeliveryID,
		"provider", provider.Name(),
		"external_id", externalID,
	)
	return nil
}

// JobDefinition returns the job definition for registration.
func NotificationDeliverJobDefinition() job.Definition {
	return job.Definition{
		Type:           NotificationDeliverJobType,
		SchemaVersion:  1,
		Queue:          "notification",
		MaxAttempts:    5,
		Timeout:        30 * time.Second,
		MaxPayloadSize: 4096,
		Description:    "Delivers a notification through the appropriate channel provider",
	}
}
