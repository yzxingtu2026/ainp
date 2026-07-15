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
	var run Run
	err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Where("id = ?", id).First(&run).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
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
	db := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&Run{})
	if q.JobType != "" {
		db = db.Where("job_type = ?", q.JobType)
	}
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}
	if q.From != nil {
		db = db.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("created_at <= ?", *q.To)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Run
	err := db.Order("created_at DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error
	return rows, total, err
}

func (s *gormRunStore) MarkCancelRequested(ctx context.Context, id string) error {
	now := time.Now().UTC()
	res := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&Run{}).
		Where("id = ? AND status IN ?", id, []status.JobRunStatus{
			status.JobPendingEnqueue, status.JobQueued, status.JobRunning, status.JobRetrying,
		}).Updates(map[string]any{
		"cancel_requested_at": now,
		"updated_at":          now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
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
