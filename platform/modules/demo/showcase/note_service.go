package showcase

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

// RevisionDoc is a demo resource with optimistic locking via revision.
type RevisionDoc struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string    `gorm:"size:64;index;not null" json:"tenant_id"`
	Title     string    `gorm:"size:256;not null" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Revision  int64     `gorm:"not null;default:1" json:"revision"`
	CreatedBy string    `gorm:"size:64" json:"created_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (RevisionDoc) TableName() string { return "demo_revision_docs" }

func (d RevisionDoc) GetRevision() int64 { return d.Revision }

var revisionDocRegistration sync.Once

// RevisionDocService provides CRUD for the demo RevisionDoc resource.
type RevisionDocService struct {
	db *gorm.DB
}

func NewRevisionDocService(db *gorm.DB) *RevisionDocService {
	revisionDocRegistration.Do(func() {
		if err := tenantdata.DefaultRegistry.Register(RevisionDoc{}, tenantdata.ModelOptions{Kind: tenantdata.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
			panic(err)
		}
	})
	return &RevisionDocService{db: db}
}

func (s *RevisionDocService) repository(ctx context.Context) (*tenantdata.Repository[RevisionDoc], error) {
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	return tenantdata.NewTenantRepository[RevisionDoc](tdb)
}

func (s *RevisionDocService) AutoMigrate() error {
	return s.db.AutoMigrate(&RevisionDoc{})
}

func (s *RevisionDocService) Create(ctx context.Context, input json.RawMessage) (any, error) {
	ac := auth.MustFromContext(ctx)
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &req); err != nil || req.Title == "" {
		return nil, errs.ValidationKey("common.validation_failed", "title is required")
	}
	doc := RevisionDoc{
		ID:        uuid.NewString(),
		Title:     req.Title,
		Content:   req.Content,
		Revision:  1,
		CreatedBy: ac.PrincipalID,
	}
	repo, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	if err := repo.Create(&doc); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create doc failed", err)
	}
	return doc, nil
}

func (s *RevisionDocService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || req.ID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id is required")
	}
	repo, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	doc, err := repo.GetByID(req.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewKey(errs.CodeNotFound, "demo.doc_not_found", "document not found")
	}
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "get doc failed", err)
	}
	return doc, nil
}

func (s *RevisionDocService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	var req struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(input, &req); err != nil || req.ID == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id is required")
	}

	updates := map[string]any{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if len(updates) == 0 {
		return nil, errs.ValidationKey("common.validation_failed", "no updates provided")
	}
	updates["revision"] = gorm.Expr("revision + 1")

	repo, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	result := repo.Where("id = ? AND revision = ?", req.ID, req.Revision).Updates(updates)
	if result.Error != nil {
		return nil, errs.Wrap(errs.CodeInternal, "update doc failed", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, errs.NewKey(errs.CodeRevisionConflict, "common.revision_conflict", "data was updated by another operation")
	}
	doc, _ := repo.GetByID(req.ID)
	return doc, nil
}

func (s *RevisionDocService) List(ctx context.Context, _ json.RawMessage) (any, error) {
	repo, err := s.repository(ctx)
	if err != nil {
		return nil, err
	}
	docs, err := repo.Order("created_at DESC").List(0, 50)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list docs failed", err)
	}
	return map[string]any{"items": docs}, nil
}

// RegisterRevisionDocActions registers the demo revision doc CRUD actions.
func RegisterRevisionDocActions(a *app.App, svc *RevisionDocService) error {
	if err := svc.AutoMigrate(); err != nil {
		return err
	}

	auditOn := true
	auditOff := false

	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		method      string
		handler     func(context.Context, json.RawMessage) (any, error)
		idem        *action.IdempotencyMeta
		audit       *bool
	}{
		{"demo.doc_list", "/api/v1/demo/docs", "文档列表", "查询演示文档列表", "GET", svc.List, nil, &auditOff},
		{"demo.doc_get", "/api/v1/demo/docs/get", "文档详情", "查询单个演示文档", "POST", svc.Get, nil, &auditOff},
		{"demo.doc_create", "/api/v1/demo/docs/create", "创建文档", "创建演示文档（幂等可选）", "POST", svc.Create, &action.IdempotencyMeta{Mode: "optional"}, &auditOn},
		{"demo.doc_update", "/api/v1/demo/docs/update", "更新文档", "更新演示文档（乐观锁）", "POST", svc.Update, nil, &auditOn},
	}

	for _, d := range definitions {
		d := d
		if err := a.RegisterAction(action.Action{
			ID:      d.id,
			Handler: d.handler,
			Meta: action.Metadata{
				Capability:     "demo",
				Name:           d.name,
				NameKey:        "action." + d.id + ".name",
				Description:    d.description,
				DescriptionKey: "action." + d.id + ".description",
				Audit:          d.audit,
				Idempotency:    d.idem,
				HTTP:           &action.HTTPMeta{Method: d.method, Path: d.path},
			},
		}); err != nil {
			return err
		}
	}

	return nil
}
