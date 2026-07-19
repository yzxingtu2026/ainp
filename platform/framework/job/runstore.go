package job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const maxErrorLen = 512

// RunStore persists and queries JobRun facts in PostgreSQL.
type RunStore interface {
	Create(ctx context.Context, run *Run) error
	Get(ctx context.Context, id string) (*Run, error)
	GetByIdempotency(ctx context.Context, tenantID, jobType, key string) (*Run, error)
	UpdateFields(ctx context.Context, id string, fields map[string]any) error
	List(ctx context.Context, q ListQuery) ([]Run, int64, error)
	MarkCancelRequested(ctx context.Context, id string) error
	ListPendingEnqueue(ctx context.Context, limit int) ([]Run, error)
}

type ListQuery struct {
	JobType  string
	Status   string
	Page     int
	PageSize int
	From     *time.Time
	To       *time.Time
}

type gormRunStore struct {
	db *gorm.DB
}

func NewRunStore(db *gorm.DB) RunStore {
	return &gormRunStore{db: db}
}

func (s *gormRunStore) Create(ctx context.Context, run *Run) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	return s.db.WithContext(ctx).Create(run).Error
}

func (s *gormRunStore) Get(ctx context.Context, id string) (*Run, error) {
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	rows, err := tenant.TenantRaw[Run](tdb, "SELECT * FROM job_runs WHERE tenant_id = ? AND id = ? LIMIT 1", id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

// GetTrusted loads a run by id without tenant scope (Worker uses JobRun as source of truth).
func GetTrusted(ctx context.Context, db *gorm.DB, id string) (*Run, error) {
	var run Run
	err := db.WithContext(ctx).Where("id = ?", id).First(&run).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *gormRunStore) GetByIdempotency(ctx context.Context, tenantID, jobType, key string) (*Run, error) {
	var run Run
	err := s.db.WithContext(ctx).
		Where("tenant_id = ? AND job_type = ? AND idempotency_key = ?", tenantID, jobType, key).
		First(&run).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *gormRunStore) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	return s.db.WithContext(ctx).Model(&Run{}).Where("id = ?", id).Updates(fields).Error
}

func (s *gormRunStore) List(ctx context.Context, q ListQuery) ([]Run, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 20
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, 0, err
	}
	whereSQL := " WHERE tenant_id = ?"
	args := make([]any, 0, 4)
	if q.JobType != "" {
		whereSQL += " AND job_type = ?"
		args = append(args, q.JobType)
	}
	if q.Status != "" {
		whereSQL += " AND status = ?"
		args = append(args, q.Status)
	}
	if q.From != nil {
		whereSQL += " AND created_at >= ?"
		args = append(args, *q.From)
	}
	if q.To != nil {
		whereSQL += " AND created_at <= ?"
		args = append(args, *q.To)
	}
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, "SELECT COUNT(*) AS count FROM job_runs"+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(counts) > 0 {
		total = counts[0].Count
	}
	listArgs := append(append([]any{}, args...), q.PageSize, (q.Page-1)*q.PageSize)
	rows, err := tenant.TenantRaw[Run](tdb, "SELECT * FROM job_runs"+whereSQL+" ORDER BY created_at DESC LIMIT ? OFFSET ?", listArgs...)
	return rows, total, err
}

func (s *gormRunStore) MarkCancelRequested(ctx context.Context, id string) error {
	now := time.Now().UTC()
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return err
	}
	rows, err := tenant.TenantRaw[struct{ ID string }](tdb, `WITH scoped AS (
  SELECT id FROM job_runs WHERE tenant_id = ? AND id = ? AND status IN (?, ?, ?, ?) FOR UPDATE
)
UPDATE job_runs AS r SET cancel_requested_at = ?, updated_at = ?
FROM scoped WHERE r.id = scoped.id RETURNING r.id`, id,
		status.JobPendingEnqueue, status.JobQueued, status.JobRunning, status.JobRetrying, now, now)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrConflict
	}
	return nil
}

func (s *gormRunStore) ListPendingEnqueue(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []Run
	err := s.db.WithContext(ctx).
		Where("status = ?", status.JobPendingEnqueue).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// CreateIdempotent inserts a run or returns the existing one for the same idempotency key.
func CreateIdempotent(ctx context.Context, db *gorm.DB, run *Run) (*Run, bool, error) {
	if run.IdempotencyKey == "" {
		if err := db.WithContext(ctx).Create(run).Error; err != nil {
			return nil, false, err
		}
		return run, true, nil
	}
	var existing Run
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND job_type = ? AND idempotency_key = ?", run.TenantID, run.JobType, run.IdempotencyKey).
		First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	if err := db.WithContext(ctx).Create(run).Error; err != nil {
		var again Run
		if e2 := db.WithContext(ctx).
			Where("tenant_id = ? AND job_type = ? AND idempotency_key = ?", run.TenantID, run.JobType, run.IdempotencyKey).
			First(&again).Error; e2 == nil {
			return &again, false, nil
		}
		return nil, false, err
	}
	return run, true, nil
}

func TruncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(msg) > maxErrorLen {
		return msg[:maxErrorLen]
	}
	return msg
}

func MarshalPayload(payload any) ([]byte, error) {
	if payload == nil {
		return []byte("{}"), nil
	}
	switch v := payload.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("job: payload is not valid JSON")
		}
		return append([]byte(nil), v...), nil
	case []byte:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("job: payload is not valid JSON")
		}
		return append([]byte(nil), v...), nil
	default:
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("job: marshal payload: %w", err)
		}
		return b, nil
	}
}

func ResolveTenantContext(ctx context.Context) (tenantID string, realm auth.Realm, actorID, correlationID string, err error) {
	ac := auth.MustFromContext(ctx)
	realm = auth.NormalizeRealm(ac.Realm)
	tenantID = strings.TrimSpace(ac.TenantID)
	if realm == auth.RealmTenant && tenantID == "" {
		return "", "", "", "", fmt.Errorf("job: tenant_id missing from AuthContext")
	}
	return tenantID, realm, strings.TrimSpace(ac.UserID), firstNonEmpty(ac.RequestID, ac.TraceID), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
