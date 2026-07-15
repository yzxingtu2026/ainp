package job

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// Envelope is the stable job identity delivered to workers.
// Asynq messages should carry primarily JobRun ID; workers load trust from PostgreSQL.
type Envelope struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	SchemaVersion int        `json:"schema_version"`
	TenantID      string     `json:"tenant_id"`
	Realm         auth.Realm `json:"realm"`
	ActorID       string     `json:"actor_id,omitempty"`
	CorrelationID string     `json:"correlation_id,omitempty"`
	CausationID   string     `json:"causation_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	Attempt       int        `json:"attempt"`
}

// Queue names (stable resource classes; do not create one queue per job type).
const (
	QueueCritical     = "critical"
	QueueDefault      = "default"
	QueueFile         = "file"
	QueueNotification = "notification"
	QueueAI           = "ai"
)

func ValidQueue(q string) bool {
	switch q {
	case QueueCritical, QueueDefault, QueueFile, QueueNotification, QueueAI:
		return true
	default:
		return false
	}
}
