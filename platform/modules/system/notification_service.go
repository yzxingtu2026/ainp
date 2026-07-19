package system

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
	"gorm.io/gorm"
)

// NotificationService is the module-level service that bridges framework contracts to DB.
type NotificationService struct {
	repo         *NotificationRepository
	orchestrator *notification.Orchestrator
	providers    map[notification.Channel]notification.ChannelProvider
	db           *gorm.DB
}

func NewNotificationService(
	db *gorm.DB,
	repo *NotificationRepository,
	defs *notification.DefinitionRegistry,
	renderer *notification.Renderer,
	resolver *notification.TemplateResolver,
	recipients notification.RecipientResolver,
	enqueuer notification.OrchestratorEnqueuer,
	providers map[notification.Channel]notification.ChannelProvider,
) *NotificationService {
	store := &gormOrchestratorStore{repo: repo}
	orch := notification.NewOrchestrator(defs, resolver, recipients, store, enqueuer, &svcLogger{})
	return &NotificationService{
		repo:         repo,
		orchestrator: orch,
		providers:    providers,
		db:           db,
	}
}

// Orchestrator returns the orchestrator for direct use (e.g., from outbox handlers).
func (s *NotificationService) Orchestrator() *notification.Orchestrator {
	return s.orchestrator
}

// Repo returns the repository.
func (s *NotificationService) Repo() *NotificationRepository {
	return s.repo
}

// Providers returns the provider map.
func (s *NotificationService) Providers() map[notification.Channel]notification.ChannelProvider {
	return s.providers
}

// --- OrchestratorStore implementation ---

type gormOrchestratorStore struct {
	repo *NotificationRepository
}

func (s *gormOrchestratorStore) FindByIdempotencyKey(ctx context.Context, tenantID, definitionKey, idempotencyKey string) (string, bool, error) {
	if idempotencyKey == "" {
		return "", false, nil
	}
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || ac.TenantID != tenantID {
		return "", false, tenant.ErrTenantConflict
	}
	repo, repoErr := s.repo.requestRepository(ctx)
	if repoErr != nil {
		return "", false, repoErr
	}
	req, err := repo.First("definition_key = ? AND idempotency_key = ?", definitionKey, idempotencyKey)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return req.ID, true, nil
}

func (s *gormOrchestratorStore) CreateRequest(ctx context.Context, req notification.OrchestratorRequest) error {
	row := NotificationRequest{
		ID:             req.ID,
		TenantID:       req.TenantID,
		DefinitionKey:  req.DefinitionKey,
		Category:       req.Category,
		Priority:       req.Priority,
		IdempotencyKey: req.IdempotencyKey,
		VariablesJSON:  req.VariablesJSON,
		ActorID:        req.ActorID,
		CorrelationID:  req.CorrelationID,
		CausationID:    req.CausationID,
	}
	return s.repo.CreateRequest(ctx, &row)
}

func (s *gormOrchestratorStore) CreateDelivery(ctx context.Context, del notification.OrchestratorDelivery) error {
	row := NotificationDelivery{
		ID:              del.ID,
		TenantID:        del.TenantID,
		RequestID:       del.RequestID,
		RecipientType:   del.RecipientType,
		RecipientID:     del.RecipientID,
		Channel:         del.Channel,
		Provider:        del.Provider,
		TemplateVersion: del.TemplateVersion,
		Locale:          del.Locale,
		Status:          del.Status,
		MaxAttempts:     del.MaxAttempts,
	}
	return s.repo.CreateDelivery(ctx, &row)
}

func (s *gormOrchestratorStore) FindExistingDeliveries(ctx context.Context, requestID string) ([]notification.OrchestratorDelivery, error) {
	deliveries, err := s.repo.ListDeliveriesByRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]notification.OrchestratorDelivery, len(deliveries))
	for i, d := range deliveries {
		out[i] = notification.OrchestratorDelivery{
			ID:            d.ID,
			TenantID:      d.TenantID,
			RequestID:     d.RequestID,
			RecipientType: d.RecipientType,
			RecipientID:   d.RecipientID,
			Channel:       d.Channel,
			Status:        d.Status,
		}
	}
	return out, nil
}

// --- NotificationDeliverStore implementation for Job handler ---

type GormDeliverStore struct {
	repo *NotificationRepository
}

func NewGormDeliverStore(repo *NotificationRepository) *GormDeliverStore {
	return &GormDeliverStore{repo: repo}
}

func (s *GormDeliverStore) LoadDelivery(ctx context.Context, deliveryID string) (*jobs.DeliveryContext, error) {
	del, err := s.repo.GetDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if del == nil {
		return nil, nil
	}
	// Load request for variables
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

func (s *GormDeliverStore) UpdateDeliveryStatus(ctx context.Context, deliveryID string, status notification.NotificationDeliveryStatus, externalID, lastError string) error {
	return s.repo.UpdateDeliveryStatus(ctx, deliveryID, status, externalID, lastError)
}

// Compile-time check: GormDeliverStore implements jobs.NotificationDeliverStore
var _ jobs.NotificationDeliverStore = (*GormDeliverStore)(nil)

// --- OrchestratorLogger implementation ---

type svcLogger struct{}

func (l *svcLogger) Info(ctx context.Context, msg string, args ...any) {
	logging.Info(ctx, msg, args...)
}

func (l *svcLogger) Error(ctx context.Context, msg string, err error, args ...any) {
	logging.Error(ctx, msg, err, args...)
}

// --- Job Enqueuer bridge ---

// NotificationJobEnqueuer bridges job.Runtime.Enqueue to OrchestratorEnqueuer.
type NotificationJobEnqueuer struct {
	enqueueFn func(ctx context.Context, deliveryID string) error
}

func NewNotificationJobEnqueuer(fn func(ctx context.Context, deliveryID string) error) *NotificationJobEnqueuer {
	return &NotificationJobEnqueuer{enqueueFn: fn}
}

func (e *NotificationJobEnqueuer) EnqueueDeliveryJob(ctx context.Context, deliveryID string) error {
	return e.enqueueFn(ctx, deliveryID)
}

// --- Outbox Handler ---

// NotificationOutboxHandler processes outbox events and triggers notifications.
type NotificationOutboxHandler struct {
	service *NotificationService
}

func NewNotificationOutboxHandler(service *NotificationService) *NotificationOutboxHandler {
	return &NotificationOutboxHandler{service: service}
}

func (h *NotificationOutboxHandler) Name() string { return "notification.outbox_handler" }

func (h *NotificationOutboxHandler) EventTypes() []string {
	return []string{"notification.requested.v1"}
}

// NotificationOutboxPayload is the expected outbox event payload.
type NotificationOutboxPayload struct {
	DefinitionKey  string                       `json:"definition_key"`
	Recipients     []notification.RecipientSpec `json:"recipients"`
	Variables      map[string]any               `json:"variables"`
	Channels       []notification.Channel       `json:"channels,omitempty"`
	Locale         string                       `json:"locale,omitempty"`
	IdempotencyKey string                       `json:"idempotency_key,omitempty"`
}

func (h *NotificationOutboxHandler) Handle(ctx context.Context, env outbox.Envelope) error {
	var payload NotificationOutboxPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return fmt.Errorf("notification outbox: invalid payload: %w", err)
	}

	// Inject auth context from Envelope (Outbox Dispatcher goroutine has no auth)
	ctx = auth.WithContext(ctx, &auth.Context{
		TenantID:      env.TenantID,
		Realm:         auth.NormalizeRealm(env.Realm),
		UserID:        env.ActorID,
		RequestID:     env.CorrelationID,
		Authenticated: true,
		Client:        "outbox",
	})

	input := notification.NotificationInput{
		DefinitionKey:  payload.DefinitionKey,
		Recipients:     payload.Recipients,
		Variables:      payload.Variables,
		Channels:       payload.Channels,
		Locale:         payload.Locale,
		IdempotencyKey: payload.IdempotencyKey,
		ActorID:        env.ActorID,
		CorrelationID:  env.CorrelationID,
		CausationID:    env.CausationID,
		TenantID:       env.TenantID,
	}

	_, _, err := h.service.Orchestrator().Process(ctx, input)
	if err != nil {
		logging.Error(ctx, "notification outbox: orchestrator process failed", err,
			"event_id", env.ID,
			"definition_key", payload.DefinitionKey,
			"tenant_id", env.TenantID,
		)
	}
	return err
}

// --- User lookup for recipient resolver ---

// UserExistsFn returns a function that checks if a user exists and is enabled in a tenant.
func UserExistsFn(db *gorm.DB) func(ctx context.Context, tenantID, userID string) (bool, error) {
	return func(ctx context.Context, tenantID, userID string) (bool, error) {
		ac := auth.MustFromContext(ctx)
		if ac.TenantID != tenantID {
			return false, tenant.ErrTenantConflict
		}
		tdb, err := tenant.TenantDB(ctx, db)
		if err != nil {
			return false, err
		}
		type userRow struct {
			ID     string
			Status string
		}
		rows, err := tenant.TenantRaw[userRow](tdb, "SELECT id, status FROM users WHERE tenant_id = ? AND id = ? LIMIT 1", userID)
		if err != nil {
			return false, err
		}
		return len(rows) > 0 && rows[0].Status == "enabled", nil
	}
}

// Helper to ensure uuid is used
var _ = uuid.NewString
