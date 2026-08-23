package outbox

import (
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// Envelope is the stable business event contract stored in the outbox and
// delivered to handlers. It must not carry GORM models, secrets, file bytes,
// or full HTTP request/response bodies.
type Envelope struct {
	ID               string            `json:"id"`
	EventType        string            `json:"event_type"`
	SchemaVersion    int               `json:"schema_version"`
	TenantID         string            `json:"tenant_id"`
	Realm            auth.Realm        `json:"realm"`
	AggregateType    string            `json:"aggregate_type"`
	AggregateID      string            `json:"aggregate_id"`
	AggregateVersion int64             `json:"aggregate_version"`
	OccurredAt       time.Time         `json:"occurred_at"`
	CorrelationID    string            `json:"correlation_id,omitempty"`
	CausationID      string            `json:"causation_id,omitempty"`
	ActorID          string            `json:"actor_id,omitempty"`
	Principal        auth.PrincipalRef `json:"principal,omitempty"`
	ClientSurface    string            `json:"client_surface,omitempty"`
	Audience         string            `json:"audience,omitempty"`
	Payload          json.RawMessage   `json:"payload"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}
