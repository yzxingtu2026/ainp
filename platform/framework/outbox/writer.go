package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

const maxErrorLen = 512

// AppendInput is the business-facing event append request.
// Tenant/realm/actor/correlation are taken from trusted context, not from this struct
// (except when EventContext is explicitly constructed for Job/system callers).
type AppendInput struct {
	EventType        string
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	Payload          any // marshaled to JSON; must be a DTO, never a GORM model
	Metadata         map[string]string
	CausationID      string
	OccurredAt       time.Time // optional; defaults to now
}

// Writer persists Event Envelopes inside an existing business GORM transaction.
type Writer interface {
	Append(ctx context.Context, tx *gorm.DB, in AppendInput) (Envelope, error)
}

type gormWriter struct {
	defs *DefinitionRegistry
}

func NewWriter(defs *DefinitionRegistry) Writer {
	if defs == nil {
		defs = NewDefinitionRegistry()
	}
	return &gormWriter{defs: defs}
}

func (w *gormWriter) Append(ctx context.Context, tx *gorm.DB, in AppendInput) (Envelope, error) {
	if tx == nil {
		return Envelope{}, fmt.Errorf("outbox: tx is required")
	}
	def, err := w.defs.MustGet(in.EventType)
	if err != nil {
		return Envelope{}, err
	}
	ec, err := resolveEventContext(ctx)
	if err != nil {
		return Envelope{}, err
	}
	if ec.Realm == auth.RealmTenant && strings.TrimSpace(ec.TenantID) == "" {
		return Envelope{}, fmt.Errorf("outbox: tenant_id is required for tenant realm events")
	}
	if in.AggregateType == "" || in.AggregateID == "" {
		return Envelope{}, fmt.Errorf("outbox: aggregate_type and aggregate_id are required")
	}
	payload, err := marshalPayload(in.Payload)
	if err != nil {
		return Envelope{}, err
	}
	metaBytes, err := marshalMetadata(in.Metadata)
	if err != nil {
		return Envelope{}, err
	}
	now := time.Now().UTC()
	occurred := in.OccurredAt
	if occurred.IsZero() {
		occurred = now
	}
	id := uuid.NewString()
	row := Event{
		ID:               id,
		TenantID:         ec.TenantID,
		Realm:            string(auth.NormalizeRealm(ec.Realm)),
		EventType:        def.EventType,
		SchemaVersion:    def.SchemaVersion,
		AggregateType:    in.AggregateType,
		AggregateID:      in.AggregateID,
		AggregateVersion: in.AggregateVersion,
		PayloadJSON:      payload,
		MetadataJSON:     metaBytes,
		Status:           status.OutboxPending,
		Attempts:         0,
		AvailableAt:      now,
		OccurredAt:       occurred,
		CorrelationID:    ec.CorrelationID,
		CausationID:      firstNonEmpty(in.CausationID, ec.CausationID),
		ActorID:          ec.ActorID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return Envelope{}, fmt.Errorf("outbox: insert event: %w", err)
	}
	return rowToEnvelope(row), nil
}

// WithinTx runs fn inside a GORM transaction with an EventWriter bound to that tx's registry.
func WithinTx(ctx context.Context, db *gorm.DB, writer Writer, fn func(tx *gorm.DB, w Writer) error) error {
	if db == nil {
		return fmt.Errorf("outbox: db is required")
	}
	if writer == nil {
		return fmt.Errorf("outbox: writer is required")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, writer)
	})
}

func marshalPayload(payload any) ([]byte, error) {
	if payload == nil {
		return []byte("{}"), nil
	}
	switch v := payload.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("outbox: payload is not valid JSON")
		}
		return append([]byte(nil), v...), nil
	case []byte:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("outbox: payload is not valid JSON")
		}
		return append([]byte(nil), v...), nil
	default:
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("outbox: marshal payload: %w", err)
		}
		return b, nil
	}
}

func marshalMetadata(meta map[string]string) ([]byte, error) {
	if len(meta) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("outbox: marshal metadata: %w", err)
	}
	return b, nil
}

func rowToEnvelope(row Event) Envelope {
	meta := map[string]string{}
	if len(row.MetadataJSON) > 0 {
		_ = json.Unmarshal(row.MetadataJSON, &meta)
	}
	return Envelope{
		ID:               row.ID,
		EventType:        row.EventType,
		SchemaVersion:    row.SchemaVersion,
		TenantID:         row.TenantID,
		Realm:            auth.Realm(row.Realm),
		AggregateType:    row.AggregateType,
		AggregateID:      row.AggregateID,
		AggregateVersion: row.AggregateVersion,
		OccurredAt:       row.OccurredAt,
		CorrelationID:    row.CorrelationID,
		CausationID:      row.CausationID,
		ActorID:          row.ActorID,
		Payload:          json.RawMessage(append([]byte(nil), row.PayloadJSON...)),
		Metadata:         meta,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > maxErrorLen {
		return msg[:maxErrorLen]
	}
	return msg
}
