package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// Orchestrator is the central coordinator for notification processing.
// It validates definitions, resolves recipients, selects channels/templates,
// creates request/delivery records, and enqueues delivery jobs.
type Orchestrator struct {
	defs       *DefinitionRegistry
	resolver   *TemplateResolver
	recipients RecipientResolver
	store      OrchestratorStore
	enqueuer   OrchestratorEnqueuer
	logger     OrchestratorLogger
}

// OrchestratorStore abstracts DB operations for the orchestrator (avoid importing GORM).
type OrchestratorStore interface {
	// FindByIdempotencyKey looks up an existing request in the trusted principal context.
	FindByIdempotencyKey(ctx context.Context, tenantID, definitionKey, idempotencyKey string) (existingID string, found bool, err error)
	// CreateRequest persists a notification request.
	CreateRequest(ctx context.Context, req OrchestratorRequest) error
	// CreateDelivery persists a notification delivery.
	CreateDelivery(ctx context.Context, del OrchestratorDelivery) error
	// FindExistingDeliveries returns deliveries for a request.
	FindExistingDeliveries(ctx context.Context, requestID string) ([]OrchestratorDelivery, error)
}

// OrchestratorEnqueuer abstracts job enqueue for delivery.
type OrchestratorEnqueuer interface {
	EnqueueDeliveryJob(ctx context.Context, deliveryID string) error
}

// OrchestratorLogger abstracts structured logging.
type OrchestratorLogger interface {
	Info(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, err error, args ...any)
}

// OrchestratorRequest is the data for creating a notification request.
type OrchestratorRequest struct {
	ID             string
	TenantID       string
	DefinitionKey  string
	Category       string
	Priority       string
	IdempotencyKey string
	VariablesJSON  []byte
	ActorID        string
	Principal      auth.PrincipalRef
	ClientSurface  string
	Audience       string
	CorrelationID  string
	CausationID    string
}

// OrchestratorDelivery is the data for creating a notification delivery.
type OrchestratorDelivery struct {
	ID              string
	TenantID        string
	RequestID       string
	RecipientType   string
	RecipientID     string
	Channel         string
	Provider        string
	TemplateVersion int
	Locale          string
	Status          string
	MaxAttempts     int
}

// NotificationInput is the business-facing input to the orchestrator.
type NotificationInput struct {
	DefinitionKey  string
	Recipients     []RecipientSpec
	Variables      map[string]any
	Channels       []Channel // optional override; if empty, uses Definition.DefaultChannels
	Locale         string    // optional; defaults to "en"
	Priority       Priority  // optional override
	IdempotencyKey string    // stable business key for dedup
	ActorID        string
	Principal      auth.PrincipalRef
	ClientSurface  string
	Audience       string
	CorrelationID  string
	CausationID    string
	TenantID       string // from trusted auth context
}

// NewOrchestrator creates a new notification orchestrator.
func NewOrchestrator(
	defs *DefinitionRegistry,
	resolver *TemplateResolver,
	recipients RecipientResolver,
	store OrchestratorStore,
	enqueuer OrchestratorEnqueuer,
	logger OrchestratorLogger,
) *Orchestrator {
	return &Orchestrator{
		defs:       defs,
		resolver:   resolver,
		recipients: recipients,
		store:      store,
		enqueuer:   enqueuer,
		logger:     logger,
	}
}

// Process handles a notification request end-to-end.
func (o *Orchestrator) Process(ctx context.Context, input NotificationInput) (requestID string, deliveryIDs []string, err error) {
	ac := auth.MustFromContext(ctx)
	trustedPrincipal := ac.Principal
	if trustedPrincipal == (auth.PrincipalRef{}) && ac.PrincipalID != "" {
		trustedPrincipal = auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: ac.PrincipalID}
	}
	if input.Principal != (auth.PrincipalRef{}) && !input.Principal.Valid() {
		return "", nil, fmt.Errorf("notification: invalid principal")
	}
	if input.Principal == (auth.PrincipalRef{}) && trustedPrincipal.Valid() {
		input.Principal = trustedPrincipal
	}
	if input.Principal == (auth.PrincipalRef{}) && input.ActorID != "" {
		input.Principal = auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: input.ActorID}
	}
	if input.ActorID == "" {
		input.ActorID = input.Principal.ID
	}
	if input.Principal.Valid() && input.ActorID != input.Principal.ID {
		return "", nil, fmt.Errorf("notification: actor does not match principal")
	}
	if ac.Authenticated && (!trustedPrincipal.Valid() || input.Principal != trustedPrincipal) {
		return "", nil, fmt.Errorf("notification: principal context mismatch")
	}
	trustedSurface, trustedAudience := ac.ClientSurface, ac.Audience
	if trustedPrincipal.Kind == auth.PrincipalKindAccount {
		if trustedSurface == "" {
			trustedSurface = auth.TenantConsoleSurface
		}
		if trustedAudience == "" {
			trustedAudience = auth.TenantAudience
		}
	}
	if input.ClientSurface == "" {
		input.ClientSurface = trustedSurface
	}
	if input.Audience == "" {
		input.Audience = trustedAudience
	}
	if ac.Authenticated && (trustedSurface == "" || input.ClientSurface != trustedSurface) {
		return "", nil, fmt.Errorf("notification: client surface context mismatch")
	}
	if ac.Authenticated && (trustedAudience == "" || input.Audience != trustedAudience) {
		return "", nil, fmt.Errorf("notification: audience context mismatch")
	}
	if ac.Authenticated && ac.TenantID != input.TenantID {
		return "", nil, fmt.Errorf("notification: tenant context mismatch")
	}
	if input.Principal.Kind == auth.PrincipalKindAccount {
		if input.ClientSurface == "" {
			input.ClientSurface = auth.TenantConsoleSurface
		}
		if input.Audience == "" {
			input.Audience = auth.TenantAudience
		}
	}
	if input.Principal.Valid() && (input.ClientSurface == "" || input.Audience == "") {
		return "", nil, fmt.Errorf("notification: principal surface context is required")
	}
	// 1. Validate definition
	def, err := o.defs.MustGet(input.DefinitionKey)
	if err != nil {
		return "", nil, err
	}

	// 2. Idempotency check
	if input.IdempotencyKey != "" && input.TenantID != "" {
		existingID, found, lookupErr := o.store.FindByIdempotencyKey(ctx, input.TenantID, input.DefinitionKey, input.IdempotencyKey)
		if lookupErr != nil {
			return "", nil, fmt.Errorf("notification: idempotency lookup: %w", lookupErr)
		}
		if found {
			// Return existing request and its deliveries
			existingDels, _ := o.store.FindExistingDeliveries(ctx, existingID)
			ids := make([]string, len(existingDels))
			for i, d := range existingDels {
				ids[i] = d.ID
			}
			o.logger.Info(ctx, "notification: idempotent replay",
				"definition_key", input.DefinitionKey,
				"request_id", existingID,
			)
			return existingID, ids, nil
		}
	}

	// 3. Validate variables
	if err := ValidateVariables(def.Variables, input.Variables); err != nil {
		return "", nil, err
	}

	// 4. Sanitize variables for persistence (remove ephemeral)
	sanitized := SanitizeVariables(def.Variables, input.Variables)
	varsJSON, err := json.Marshal(sanitized)
	if err != nil {
		return "", nil, fmt.Errorf("notification: marshal variables: %w", err)
	}

	// 5. Determine channels
	channels := input.Channels
	if len(channels) == 0 {
		channels = def.DefaultChannels
	}
	for _, ch := range channels {
		if !def.IsChannelAllowed(ch) {
			return "", nil, fmt.Errorf("notification: channel %q not allowed for definition %q", ch, input.DefinitionKey)
		}
	}

	// 6. Determine locale
	locale := input.Locale
	if locale == "" {
		locale = "en"
	}

	// 7. Determine priority
	priority := def.Priority
	if input.Priority.Valid() {
		priority = input.Priority
	}

	// 8. Resolve recipients and create request + deliveries
	requestID = uuid.NewString()
	now := time.Now().UTC()
	_ = now

	req := OrchestratorRequest{
		ID:             requestID,
		TenantID:       input.TenantID,
		DefinitionKey:  input.DefinitionKey,
		Category:       def.Category,
		Priority:       string(priority),
		IdempotencyKey: input.IdempotencyKey,
		VariablesJSON:  varsJSON,
		ActorID:        input.ActorID,
		Principal:      input.Principal,
		ClientSurface:  input.ClientSurface,
		Audience:       input.Audience,
		CorrelationID:  input.CorrelationID,
		CausationID:    input.CausationID,
	}
	if err := o.store.CreateRequest(ctx, req); err != nil {
		return "", nil, fmt.Errorf("notification: create request: %w", err)
	}

	// 9. For each recipient × channel, create a delivery
	for _, recipient := range input.Recipients {
		if err := recipient.Validate(); err != nil {
			o.logger.Error(ctx, "notification: invalid recipient", err,
				"definition_key", input.DefinitionKey,
				"recipient_type", recipient.Type,
			)
			continue
		}

		// Resolve recipient (validates tenant membership)
		resolved, resolveErr := o.recipients.Resolve(ctx, input.TenantID, recipient, channels[0])
		if resolveErr != nil {
			o.logger.Error(ctx, "notification: recipient resolution failed", resolveErr,
				"definition_key", input.DefinitionKey,
				"recipient_id", recipient.ID,
			)
			continue
		}

		for _, ch := range channels {
			// Resolve template (validates template exists)
			resolveResult, tplErr := o.resolver.Resolve(ctx, input.TenantID, def, ch, locale)
			if tplErr != nil {
				o.logger.Error(ctx, "notification: template resolution failed", tplErr,
					"definition_key", input.DefinitionKey,
					"channel", ch,
					"locale", locale,
				)
				continue
			}

			deliveryID := uuid.NewString()
			del := OrchestratorDelivery{
				ID:              deliveryID,
				TenantID:        input.TenantID,
				RequestID:       requestID,
				RecipientType:   string(resolved.Type),
				RecipientID:     resolved.ID,
				Channel:         string(ch),
				Provider:        "", // filled by job handler
				TemplateVersion: resolveResult.Version,
				Locale:          resolveResult.Locale,
				Status:          string(DeliveryQueued),
				MaxAttempts:     3,
			}
			if err := o.store.CreateDelivery(ctx, del); err != nil {
				o.logger.Error(ctx, "notification: create delivery failed", err,
					"definition_key", input.DefinitionKey,
					"delivery_id", deliveryID,
				)
				continue
			}

			// Enqueue delivery job
			if o.enqueuer != nil {
				if enqueueErr := o.enqueuer.EnqueueDeliveryJob(ctx, deliveryID); enqueueErr != nil {
					o.logger.Error(ctx, "notification: enqueue delivery job failed", enqueueErr,
						"definition_key", input.DefinitionKey,
						"delivery_id", deliveryID,
					)
				}
			}

			deliveryIDs = append(deliveryIDs, deliveryID)
		}
	}

	o.logger.Info(ctx, "notification: processed",
		"definition_key", input.DefinitionKey,
		"request_id", requestID,
		"deliveries", len(deliveryIDs),
	)
	return requestID, deliveryIDs, nil
}
