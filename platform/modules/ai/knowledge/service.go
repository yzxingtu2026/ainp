package knowledge

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/dblock"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/agent"
	"gorm.io/gorm"
)

const KnowledgeDocumentResourceType = "ai.knowledge_document"

const (
	auditKnowledgeBaseCreated     = "knowledge base created"
	auditKnowledgeBaseUpdated     = "knowledge base updated"
	auditKnowledgeBaseStatus      = "knowledge base status updated"
	auditKnowledgeBaseDeleted     = "knowledge base deleted"
	auditKnowledgeDocumentCreated = "knowledge document created"
	auditKnowledgeDocumentUpdated = "knowledge document updated"
	auditKnowledgeDocumentDeleted = "knowledge document deleted"
	auditAgentKnowledgeBound      = "agent knowledge binding created"
	auditAgentKnowledgeUnbound    = "agent knowledge binding deleted"
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type KnowledgeService struct {
	db        tenantdata.GuardedDatabase
	models    aimodel.Resolver
	agents    *agent.AgentService
	links     attachment.ResourceLinker
	events    outbox.Writer
	retriever aiknowledge.KnowledgeRetriever
	citations aiknowledge.CitationResolver
}

func (s *KnowledgeService) SetEventWriter(writer outbox.Writer) { s.events = writer }

func (s *KnowledgeService) SetRetrievalServices(retriever aiknowledge.KnowledgeRetriever, citations aiknowledge.CitationResolver) {
	s.retriever = retriever
	s.citations = citations
}

func NewKnowledgeService(db *gorm.DB, models aimodel.Resolver, agents *agent.AgentService, links attachment.ResourceLinker) *KnowledgeService {
	if err := registerKnowledgeTenantModels(); err != nil {
		panic(err)
	}
	return &KnowledgeService{db: tenantdata.GuardedDB(db), models: models, agents: agents, links: links}
}

type KnowledgeBaseListRequest struct {
	pagination.Request
	Keyword    string `json:"keyword" validate:"omitempty,max=128"`
	Visibility string `json:"visibility" validate:"omitempty,oneof=private tenant"`
	Status     string `json:"status" validate:"omitempty,oneof=draft enabled disabled"`
}

type KnowledgeBaseIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type KnowledgeBaseCreateRequest struct {
	Name            string                      `json:"name" validate:"required,max=128"`
	Description     string                      `json:"description" validate:"omitempty,max=1024"`
	Visibility      string                      `json:"visibility" validate:"required,oneof=private tenant"`
	EmbeddingAlias  string                      `json:"embedding_alias" validate:"omitempty,max=64"`
	ChunkingConfig  aiknowledge.ChunkingConfig  `json:"chunking_config" validate:"omitempty"`
	RetrievalConfig aiknowledge.RetrievalConfig `json:"retrieval_config" validate:"omitempty"`
}

func (r *KnowledgeBaseCreateRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Visibility = strings.TrimSpace(r.Visibility)
	r.EmbeddingAlias = strings.TrimSpace(r.EmbeddingAlias)
	r.ChunkingConfig.Normalize()
	r.RetrievalConfig.Normalize()
}

func (r KnowledgeBaseCreateRequest) Validate(context.Context) error {
	if err := r.ChunkingConfig.Validate(); err != nil {
		return fieldValidation("chunking_config", "knowledge.validation.invalid_chunking_config", err.Error())
	}
	if err := r.RetrievalConfig.Validate(); err != nil {
		return fieldValidation("retrieval_config", "knowledge.validation.invalid_retrieval_config", err.Error())
	}
	return nil
}

type KnowledgeBaseUpdateRequest struct {
	ID              string                      `json:"id" validate:"required"`
	Name            string                      `json:"name" validate:"required,max=128"`
	Description     string                      `json:"description" validate:"omitempty,max=1024"`
	Visibility      string                      `json:"visibility" validate:"required,oneof=private tenant"`
	EmbeddingAlias  string                      `json:"embedding_alias" validate:"omitempty,max=64"`
	ChunkingConfig  aiknowledge.ChunkingConfig  `json:"chunking_config" validate:"omitempty"`
	RetrievalConfig aiknowledge.RetrievalConfig `json:"retrieval_config" validate:"omitempty"`
	Revision        uint64                      `json:"revision" validate:"required"`
}

func (r *KnowledgeBaseUpdateRequest) Normalize() {
	r.ID = strings.TrimSpace(r.ID)
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Visibility = strings.TrimSpace(r.Visibility)
	r.EmbeddingAlias = strings.TrimSpace(r.EmbeddingAlias)
	r.ChunkingConfig.Normalize()
	r.RetrievalConfig.Normalize()
}

func (r KnowledgeBaseUpdateRequest) Validate(context.Context) error {
	if err := r.ChunkingConfig.Validate(); err != nil {
		return fieldValidation("chunking_config", "knowledge.validation.invalid_chunking_config", err.Error())
	}
	if err := r.RetrievalConfig.Validate(); err != nil {
		return fieldValidation("retrieval_config", "knowledge.validation.invalid_retrieval_config", err.Error())
	}
	return nil
}

type KnowledgeBaseStatusRequest struct {
	ID       string `json:"id" validate:"required"`
	Status   string `json:"status" validate:"required,oneof=draft enabled disabled"`
	Revision uint64 `json:"revision" validate:"required"`
}

type KnowledgeBaseDeleteRequest struct {
	ID       string `json:"id" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}

type KnowledgeDocumentListRequest struct {
	pagination.Request
	KnowledgeBaseID string `json:"knowledge_base_id" validate:"required"`
	Status          string `json:"status" validate:"omitempty,oneof=pending ready processing failed"`
}

type KnowledgeDocumentIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type KnowledgeDocumentCreateRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id" validate:"required"`
	AttachmentID    string `json:"attachment_id" validate:"required"`
}

type KnowledgeDocumentUpdateRequest struct {
	ID           string `json:"id" validate:"required"`
	AttachmentID string `json:"attachment_id" validate:"required"`
	Revision     uint64 `json:"revision" validate:"required"`
}

type KnowledgeDocumentDeleteRequest struct {
	ID       string `json:"id" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}

type KnowledgeRetrievalTestRequest struct {
	KnowledgeBaseID string   `json:"knowledge_base_id" validate:"required"`
	DocumentIDs     []string `json:"document_ids" validate:"omitempty,max=100,dive,required"`
	Query           string   `json:"query" validate:"required,max=4096"`
}

func (r *KnowledgeRetrievalTestRequest) Normalize() {
	r.KnowledgeBaseID = strings.TrimSpace(r.KnowledgeBaseID)
	r.Query = strings.TrimSpace(r.Query)
	for index := range r.DocumentIDs {
		r.DocumentIDs[index] = strings.TrimSpace(r.DocumentIDs[index])
	}
}

type KnowledgeCitationResolveRequest struct {
	Citation aiknowledge.Citation `json:"citation" validate:"required"`
}

type KnowledgeRetrievalTestItem struct {
	Citation  aiknowledge.Citation `json:"citation"`
	Score     float64              `json:"score"`
	Text      string               `json:"text"`
	Untrusted bool                 `json:"untrusted"`
}

type AgentKnowledgeListRequest struct {
	pagination.Request
	AgentID string `json:"agent_id" validate:"required"`
}

type AgentKnowledgeBindRequest struct {
	AgentID         string `json:"agent_id" validate:"required"`
	KnowledgeBaseID string `json:"knowledge_base_id" validate:"required"`
}

type AgentKnowledgeUnbindRequest = AgentKnowledgeBindRequest

func fieldValidation(field, key, fallback string) error {
	return errs.ValidationKey("common.validation_failed", "validation failed", errs.FieldError{
		Field: field, Message: fallback, MessageKey: key,
	})
}

func knowledgeActor(ctx context.Context) (*auth.Context, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" ||
		strings.TrimSpace(ac.PrincipalID) == "" || ac.Realm != auth.RealmTenant {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}

func (s *KnowledgeService) dbForContext(ctx context.Context) *gorm.DB {
	return attachment.ResourceTransaction(ctx, s.db.WithContext(ctx)).WithContext(ctx)
}

func knowledgeNotFound() error {
	return errs.NewKey(errs.CodeNotFound, "ai.knowledge.not_found", "knowledge resource not found")
}

func revisionConflict() error {
	return errs.NewKey(errs.CodeRevisionConflict, "common.revision_conflict", "data was updated by another operation")
}

func lockKnowledgeBase(ctx context.Context, tx *gorm.DB, tenantID, id string) error {
	return dblock.WithTimeout(ctx, 10*time.Second, func(lockCtx context.Context) error {
		if err := dblock.LockForTenant(tx.WithContext(lockCtx), "ai_knowledge_bases", tenantID, id); err != nil {
			return knowledgeNotFound()
		}
		return nil
	})
}

func (s *KnowledgeService) visibleKnowledgeBases(ctx context.Context) (*gorm.DB, *auth.Context, error) {
	ac, err := knowledgeActor(ctx)
	if err != nil {
		return nil, nil, err
	}
	query := s.dbForContext(ctx).Where("visibility = ? OR account_id = ?", aiknowledge.VisibilityTenant, ac.PrincipalID)
	return query, ac, nil
}

func (s *KnowledgeService) getKnowledgeBase(ctx context.Context, id string, manage bool) (*KnowledgeBase, *auth.Context, error) {
	query, ac, err := s.visibleKnowledgeBases(ctx)
	if err != nil {
		return nil, nil, err
	}
	var row KnowledgeBase
	if err = query.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, nil, knowledgeNotFound()
	}
	if manage && row.AccountID != ac.PrincipalID {
		return nil, nil, knowledgeNotFound()
	}
	return &row, ac, nil
}

func (s *KnowledgeService) ListKnowledgeBases(ctx context.Context, request KnowledgeBaseListRequest) (any, error) {
	query, _, err := s.visibleKnowledgeBases(ctx)
	if err != nil {
		return nil, err
	}
	if keyword := strings.TrimSpace(request.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}
	if request.Visibility != "" {
		query = query.Where("visibility = ?", request.Visibility)
	}
	if request.Status != "" {
		query = query.Where("status = ?", request.Status)
	}
	var total int64
	if err = query.Model(&KnowledgeBase{}).Count(&total).Error; err != nil {
		return nil, dbError(err, "list knowledge bases")
	}
	var rows []KnowledgeBase
	if err = query.Order("updated_at DESC").Offset(request.Offset()).Limit(request.PageSize).Find(&rows).Error; err != nil {
		return nil, dbError(err, "list knowledge bases")
	}
	return pagination.NewResult(rows, total, request.Request), nil
}

func (s *KnowledgeService) GetKnowledgeBase(ctx context.Context, request KnowledgeBaseIDRequest) (any, error) {
	row, _, err := s.getKnowledgeBase(ctx, request.ID, false)
	return row, err
}

func (s *KnowledgeService) CreateKnowledgeBase(ctx context.Context, request KnowledgeBaseCreateRequest) (any, error) {
	ac, err := knowledgeActor(ctx)
	if err != nil {
		return nil, err
	}
	if request.EmbeddingAlias != "" {
		if err = s.resolveEmbedding(ctx, request.EmbeddingAlias); err != nil {
			return nil, err
		}
	}
	identifier := uuid.New()
	row := KnowledgeBase{
		ID: identifier.String(), TenantID: ac.TenantID, AccountID: ac.PrincipalID,
		Code: "kb_" + strings.ReplaceAll(identifier.String(), "-", ""), Name: request.Name, Description: request.Description,
		Visibility: request.Visibility, Status: aiknowledge.KnowledgeBaseDraft,
		EmbeddingAlias: request.EmbeddingAlias, ChunkingConfig: request.ChunkingConfig,
		RetrievalConfig: request.RetrievalConfig, Revision: 1,
	}
	if err = s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, knowledgeDBError(err, "ai.knowledge.code_exists", "knowledge base code already exists")
	}
	setKnowledgeAudit(ctx, "ai_knowledge_base", row.ID, auditKnowledgeBaseCreated)
	return row, nil
}

func (s *KnowledgeService) UpdateKnowledgeBase(ctx context.Context, request KnowledgeBaseUpdateRequest) (any, error) {
	row, _, err := s.getKnowledgeBase(ctx, request.ID, true)
	if err != nil {
		return nil, err
	}
	if request.EmbeddingAlias != "" {
		if err = s.resolveEmbedding(ctx, request.EmbeddingAlias); err != nil {
			return nil, err
		}
	}
	if row.Status == aiknowledge.KnowledgeBaseEnabled && request.EmbeddingAlias == "" {
		return nil, errs.NewKey(errs.CodeValidation, "ai.knowledge.embedding_required", "embedding alias is required before enabling")
	}
	updates := map[string]any{
		"name": request.Name, "description": request.Description, "visibility": request.Visibility,
		"embedding_alias": request.EmbeddingAlias, "chunking_config": marshal(request.ChunkingConfig),
		"retrieval_config": marshal(request.RetrievalConfig), "revision": gorm.Expr("revision + 1"),
	}
	if row.EmbeddingAlias != request.EmbeddingAlias || row.ChunkingConfig != request.ChunkingConfig {
		updates["current_index_version_id"] = nil
	}
	result := s.db.WithContext(ctx).Model(&KnowledgeBase{}).
		Where("id=? AND revision=?", row.ID, request.Revision).
		Updates(updates)
	if result.Error != nil {
		return nil, dbError(result.Error, "update knowledge base")
	}
	if result.RowsAffected != 1 {
		return nil, revisionConflict()
	}
	setKnowledgeAudit(ctx, "ai_knowledge_base", row.ID, auditKnowledgeBaseUpdated)
	return s.GetKnowledgeBase(ctx, KnowledgeBaseIDRequest{ID: row.ID})
}

func (s *KnowledgeService) UpdateKnowledgeBaseStatus(ctx context.Context, request KnowledgeBaseStatusRequest) (any, error) {
	row, _, err := s.getKnowledgeBase(ctx, request.ID, true)
	if err != nil {
		return nil, err
	}
	if request.Status == aiknowledge.KnowledgeBaseEnabled {
		if strings.TrimSpace(row.EmbeddingAlias) == "" {
			return nil, errs.NewKey(errs.CodeValidation, "ai.knowledge.embedding_required", "embedding alias is required before enabling")
		}
		if err = s.resolveEmbedding(ctx, row.EmbeddingAlias); err != nil {
			return nil, err
		}
	}
	result := s.db.WithContext(ctx).Model(&KnowledgeBase{}).
		Where("id=? AND revision=?", row.ID, request.Revision).
		Updates(map[string]any{"status": request.Status, "revision": gorm.Expr("revision + 1")})
	if result.Error != nil {
		return nil, dbError(result.Error, "update knowledge base status")
	}
	if result.RowsAffected != 1 {
		return nil, revisionConflict()
	}
	setKnowledgeAudit(ctx, "ai_knowledge_base", row.ID, auditKnowledgeBaseStatus)
	return s.GetKnowledgeBase(ctx, KnowledgeBaseIDRequest{ID: row.ID})
}

func (s *KnowledgeService) DeleteKnowledgeBase(ctx context.Context, request KnowledgeBaseDeleteRequest) (any, error) {
	row, _, err := s.getKnowledgeBase(ctx, request.ID, true)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := lockKnowledgeBase(ctx, tx, row.TenantID, row.ID); lockErr != nil {
			return lockErr
		}
		var current KnowledgeBase
		if queryErr := tx.Where("id=? AND account_id=?", row.ID, row.AccountID).
			First(&current).Error; queryErr != nil {
			return knowledgeNotFound()
		}
		var documents, bindings int64
		if countErr := tx.Model(&KnowledgeDocument{}).
			Where("knowledge_base_id=?", row.ID).Count(&documents).Error; countErr != nil {
			return dbError(countErr, "check knowledge base documents")
		}
		if countErr := tx.Model(&AgentKnowledgeBinding{}).
			Where("knowledge_base_id=?", row.ID).Count(&bindings).Error; countErr != nil {
			return dbError(countErr, "check knowledge base bindings")
		}
		if documents > 0 || bindings > 0 {
			return errs.NewKey(errs.CodeConflict, "ai.knowledge.delete_referenced", "knowledge base still has documents or agent bindings")
		}
		result := tx.Where("id=? AND revision=?", row.ID, request.Revision).
			Delete(&KnowledgeBase{})
		if result.Error != nil {
			return dbError(result.Error, "delete knowledge base")
		}
		if result.RowsAffected != 1 {
			return revisionConflict()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	setKnowledgeAudit(ctx, "ai_knowledge_base", row.ID, auditKnowledgeBaseDeleted)
	return map[string]any{"deleted": true}, nil
}

func (s *KnowledgeService) resolveEmbedding(ctx context.Context, alias string) error {
	if s.models == nil {
		return errs.NewKey(errs.CodeValidation, "ai.knowledge.embedding_unavailable", "embedding model resolver is unavailable")
	}
	_, err := s.models.Resolve(ctx, alias, aimodel.CapabilityRequirements{
		ModelType: aimodel.ModelTypeEmbedding,
		Features:  []aimodel.Feature{aimodel.FeatureEmbeddings},
	})
	return err
}

func (s *KnowledgeService) getDocument(ctx context.Context, id string, manage bool) (*KnowledgeDocument, *KnowledgeBase, *auth.Context, error) {
	ac, err := knowledgeActor(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var document KnowledgeDocument
	if err = s.dbForContext(ctx).Where("id=?", id).First(&document).Error; err != nil {
		return nil, nil, nil, knowledgeNotFound()
	}
	knowledgeBase, _, err := s.getKnowledgeBase(ctx, document.KnowledgeBaseID, manage)
	if err != nil {
		return nil, nil, nil, knowledgeNotFound()
	}
	return &document, knowledgeBase, ac, nil
}

func (s *KnowledgeService) ListDocuments(ctx context.Context, request KnowledgeDocumentListRequest) (any, error) {
	knowledgeBase, _, err := s.getKnowledgeBase(ctx, request.KnowledgeBaseID, false)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("knowledge_base_id=?", knowledgeBase.ID)
	if request.Status != "" {
		query = query.Where("ingestion_status=?", request.Status)
	}
	var total int64
	if err = query.Model(&KnowledgeDocument{}).Count(&total).Error; err != nil {
		return nil, dbError(err, "list knowledge documents")
	}
	var rows []KnowledgeDocument
	if err = query.Order("created_at DESC").Offset(request.Offset()).Limit(request.PageSize).Find(&rows).Error; err != nil {
		return nil, dbError(err, "list knowledge documents")
	}
	return pagination.NewResult(rows, total, request.Request), nil
}

func (s *KnowledgeService) GetDocument(ctx context.Context, request KnowledgeDocumentIDRequest) (any, error) {
	document, _, _, err := s.getDocument(ctx, request.ID, false)
	return document, err
}

func (s *KnowledgeService) TestRetrieval(ctx context.Context, request KnowledgeRetrievalTestRequest) (any, error) {
	knowledgeBase, _, err := s.getKnowledgeBase(ctx, request.KnowledgeBaseID, false)
	if err != nil {
		return nil, err
	}
	if knowledgeBase.Status != aiknowledge.KnowledgeBaseEnabled {
		return nil, errs.NewKey(errs.CodeConflict, "ai.knowledge.retrieval_requires_enabled", "knowledge base must be enabled before retrieval")
	}
	if knowledgeBase.CurrentIndexVersionID == nil {
		return nil, errs.NewKey(errs.CodeConflict, "ai.knowledge.retrieval_index_not_ready", "knowledge index is not ready")
	}
	var readyVersions int64
	if err = s.dbForContext(ctx).Model(&KnowledgeIndexVersion{}).
		Where("id=? AND knowledge_base_id=? AND status=?", *knowledgeBase.CurrentIndexVersionID, knowledgeBase.ID, aiknowledge.IndexReady).
		Count(&readyVersions).Error; err != nil {
		return nil, dbError(err, "check knowledge retrieval index")
	}
	if readyVersions != 1 {
		return nil, errs.NewKey(errs.CodeConflict, "ai.knowledge.retrieval_index_not_ready", "knowledge index is not ready")
	}
	if s.retriever == nil {
		return nil, errs.NewKey(errs.CodeInternal, "ai.knowledge.retrieval_unavailable", "knowledge retrieval service is unavailable")
	}
	items, err := s.retriever.Retrieve(ctx, aiknowledge.RetrievalRequest{
		KnowledgeBaseIDs: []string{knowledgeBase.ID},
		DocumentIDs:      request.DocumentIDs,
		Query:            request.Query,
		Config:           knowledgeBase.RetrievalConfig,
	})
	if err != nil {
		return nil, err
	}
	result := make([]KnowledgeRetrievalTestItem, 0, len(items))
	for _, item := range items {
		result = append(result, KnowledgeRetrievalTestItem{
			Citation: item.Citation, Score: item.Score, Text: item.Text, Untrusted: item.Untrusted,
		})
	}
	return map[string]any{"items": result}, nil
}

func (s *KnowledgeService) ResolveCitation(ctx context.Context, request KnowledgeCitationResolveRequest) (any, error) {
	if s.citations == nil {
		return nil, errs.NewKey(errs.CodeInternal, "ai.knowledge.retrieval_unavailable", "knowledge citation service is unavailable")
	}
	// The resolver reauthorizes both the base and document. Its lookup deliberately
	// contains no chunk body or attachment access capability.
	return s.citations.ResolveCitation(ctx, request.Citation)
}

func (s *KnowledgeService) CreateDocument(ctx context.Context, request KnowledgeDocumentCreateRequest) (any, error) {
	knowledgeBase, ac, err := s.getKnowledgeBase(ctx, request.KnowledgeBaseID, true)
	if err != nil {
		return nil, err
	}
	source, err := s.attachmentSnapshot(ctx, ac, request.AttachmentID)
	if err != nil {
		return nil, err
	}
	row := KnowledgeDocument{
		ID: uuid.NewString(), TenantID: ac.TenantID, KnowledgeBaseID: knowledgeBase.ID,
		AttachmentID: source.ID, SourceRevision: source.SHA256, ContentType: source.ContentType,
		OriginalName: source.OriginalName, IngestionStatus: aiknowledge.DocumentPending, Revision: 1,
	}
	if s.links == nil {
		return nil, errs.NewKey(errs.CodeInternal, "ai.knowledge.attachment_linker_unavailable", "attachment resource linker is unavailable")
	}
	created := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := lockKnowledgeBase(ctx, tx, knowledgeBase.TenantID, knowledgeBase.ID); lockErr != nil {
			return lockErr
		}
		var current KnowledgeBase
		if queryErr := tx.Where("id=? AND account_id=?", knowledgeBase.ID, ac.PrincipalID).First(&current).Error; queryErr != nil {
			return knowledgeNotFound()
		}
		var existing KnowledgeDocument
		queryErr := tx.Where("knowledge_base_id=? AND attachment_id=?", knowledgeBase.ID, source.ID).First(&existing).Error
		if queryErr == nil {
			row = existing
			return nil
		}
		if !errors.Is(queryErr, gorm.ErrRecordNotFound) {
			return queryErr
		}
		if createErr := tx.Create(&row).Error; createErr != nil {
			return knowledgeDBError(createErr, "ai.knowledge.document_exists", "attachment is already referenced by this knowledge base")
		}
		_, linkErr := s.links.BindResource(ctx, tx, attachment.ResourceLinkRequest{
			AttachmentID: row.AttachmentID, ResourceType: KnowledgeDocumentResourceType,
			ResourceID: row.ID, Field: "source",
		})
		if linkErr != nil {
			return linkErr
		}
		created = true
		return appendKnowledgeIngestion(ctx, tx, s.events, row)
	})
	if err != nil {
		return nil, knowledgeDBError(err, "ai.knowledge.document_exists", "attachment is already referenced by this knowledge base")
	}
	if created {
		setKnowledgeAudit(ctx, "ai_knowledge_document", row.ID, auditKnowledgeDocumentCreated)
	}
	return row, nil
}

func (s *KnowledgeService) UpdateDocument(ctx context.Context, request KnowledgeDocumentUpdateRequest) (any, error) {
	document, _, ac, err := s.getDocument(ctx, request.ID, true)
	if err != nil {
		return nil, err
	}
	source, err := s.attachmentSnapshot(ctx, ac, request.AttachmentID)
	if err != nil {
		return nil, err
	}
	if s.links == nil {
		return nil, errs.NewKey(errs.CodeInternal, "ai.knowledge.attachment_linker_unavailable", "attachment resource linker is unavailable")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if document.AttachmentID != source.ID {
			if _, linkErr := s.links.BindResource(ctx, tx, attachment.ResourceLinkRequest{
				AttachmentID: source.ID, ResourceType: KnowledgeDocumentResourceType,
				ResourceID: document.ID, Field: "source",
			}); linkErr != nil {
				return linkErr
			}
			if unlinkErr := s.links.UnbindResource(ctx, tx, attachment.ResourceLinkRequest{
				AttachmentID: document.AttachmentID, ResourceType: KnowledgeDocumentResourceType,
				ResourceID: document.ID, Field: "source",
			}); unlinkErr != nil {
				return unlinkErr
			}
		}
		result := tx.Model(&KnowledgeDocument{}).
			Where("id=? AND revision=?", document.ID, request.Revision).
			Updates(map[string]any{
				"attachment_id": source.ID, "source_revision": source.SHA256,
				"content_type": source.ContentType, "original_name": source.OriginalName,
				"ingestion_status": aiknowledge.DocumentPending, "ingestion_error_category": "",
				"current_index_version_id": nil, "revision": gorm.Expr("revision + 1"),
			})
		if result.Error != nil {
			return knowledgeDBError(result.Error, "ai.knowledge.document_exists", "attachment is already referenced by this knowledge base")
		}
		if result.RowsAffected != 1 {
			return revisionConflict()
		}
		next := *document
		next.AttachmentID = source.ID
		next.SourceRevision = source.SHA256
		next.ContentType = source.ContentType
		next.OriginalName = source.OriginalName
		next.Revision = request.Revision + 1
		return appendKnowledgeIngestion(ctx, tx, s.events, next)
	})
	if err != nil {
		return nil, err
	}
	setKnowledgeAudit(ctx, "ai_knowledge_document", document.ID, auditKnowledgeDocumentUpdated)
	return s.GetDocument(ctx, KnowledgeDocumentIDRequest{ID: document.ID})
}

func (s *KnowledgeService) DeleteDocument(ctx context.Context, request KnowledgeDocumentDeleteRequest) (any, error) {
	document, _, _, err := s.getDocument(ctx, request.ID, true)
	if err != nil {
		return nil, err
	}
	if s.links == nil {
		return nil, errs.NewKey(errs.CodeInternal, "ai.knowledge.attachment_linker_unavailable", "attachment resource linker is unavailable")
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if unlinkErr := s.links.UnbindResource(ctx, tx, attachment.ResourceLinkRequest{
			AttachmentID: document.AttachmentID, ResourceType: KnowledgeDocumentResourceType,
			ResourceID: document.ID, Field: "source",
		}); unlinkErr != nil {
			return unlinkErr
		}
		result := tx.Where("id=? AND revision=?", document.ID, request.Revision).
			Delete(&KnowledgeDocument{})
		if result.Error != nil {
			return dbError(result.Error, "delete knowledge document")
		}
		if result.RowsAffected != 1 {
			return revisionConflict()
		}
		if err := appendKnowledgeCleanup(ctx, tx, s.events, *document); err != nil {
			return err
		}
		var remaining KnowledgeDocument
		err := tx.Where("knowledge_base_id=?", document.KnowledgeBaseID).Order("id").First(&remaining).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return appendKnowledgeIngestion(ctx, tx, s.events, remaining)
	})
	if err != nil {
		return nil, err
	}
	setKnowledgeAudit(ctx, "ai_knowledge_document", document.ID, auditKnowledgeDocumentDeleted)
	return map[string]any{"deleted": true}, nil
}

type attachmentSnapshot struct {
	ID, UploaderID, Status, OriginalName, ContentType, SHA256 string
	Size                                                      int64
}

func (s *KnowledgeService) attachmentSnapshot(ctx context.Context, ac *auth.Context, id string) (*attachmentSnapshot, error) {
	tdb, queryErr := s.db.Tenant(ctx)
	if queryErr != nil {
		return nil, queryErr
	}
	rows, queryErr := tenantdata.TenantRaw[attachmentSnapshot](tdb, `SELECT id,uploader_id,status,original_name,content_type,sha256,size FROM attachments WHERE tenant_id=? AND id=? AND status=? LIMIT 1`, id, status.AttachmentReady)
	if queryErr != nil || len(rows) != 1 {
		return nil, errs.NewKey(errs.CodeNotFound, "ai.knowledge.attachment_not_found", "ready attachment not found")
	}
	row := rows[0]
	if row.UploaderID != ac.PrincipalID {
		return nil, errs.NewKey(errs.CodeNotFound, "ai.knowledge.attachment_not_found", "ready attachment not found")
	}
	if !sha256Pattern.MatchString(strings.ToLower(row.SHA256)) {
		return nil, errs.NewKey(errs.CodeValidation, "ai.knowledge.attachment_revision_missing", "attachment SHA-256 is unavailable")
	}
	row.SHA256 = strings.ToLower(row.SHA256)
	return &row, nil
}

func (s *KnowledgeService) ListAgentKnowledge(ctx context.Context, request AgentKnowledgeListRequest) (any, error) {
	ac, err := knowledgeActor(ctx)
	if err != nil {
		return nil, err
	}
	agent, _, err := s.agents.GetVisible(ctx, request.AgentID, false)
	if err != nil {
		return nil, err
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := tenantdata.TenantRaw[struct{ Count int64 }](tdb, `SELECT COUNT(*) AS count
FROM ai_agent_knowledge_bindings b
JOIN ai_knowledge_bases kb ON kb.tenant_id=b.tenant_id AND kb.id=b.knowledge_base_id AND kb.deleted_at IS NULL
WHERE b.tenant_id=? AND b.agent_id=? AND (kb.visibility=? OR kb.account_id=?)`, agent.ID, aiknowledge.VisibilityTenant, ac.PrincipalID)
	if err != nil {
		return nil, dbError(err, "list agent knowledge bindings")
	}
	var total int64
	if len(counts) == 1 {
		total = counts[0].Count
	}
	bindings, err := tenantdata.TenantRaw[AgentKnowledgeBinding](tdb, `SELECT b.*
FROM ai_agent_knowledge_bindings b
JOIN ai_knowledge_bases kb ON kb.tenant_id=b.tenant_id AND kb.id=b.knowledge_base_id AND kb.deleted_at IS NULL
WHERE b.tenant_id=? AND b.agent_id=? AND (kb.visibility=? OR kb.account_id=?)
ORDER BY b.created_at ASC LIMIT ? OFFSET ?`, agent.ID, aiknowledge.VisibilityTenant, ac.PrincipalID, request.PageSize, request.Offset())
	if err != nil {
		return nil, dbError(err, "list agent knowledge bindings")
	}
	return pagination.NewResult(bindings, total, request.Request), nil
}

func (s *KnowledgeService) BindAgentKnowledge(ctx context.Context, request AgentKnowledgeBindRequest) (any, error) {
	if _, err := knowledgeActor(ctx); err != nil {
		return nil, err
	}
	agent, ac, err := s.agents.GetVisible(ctx, request.AgentID, true)
	if err != nil {
		return nil, err
	}
	knowledgeBase, _, err := s.getKnowledgeBase(ctx, request.KnowledgeBaseID, false)
	if err != nil {
		return nil, err
	}
	binding := AgentKnowledgeBinding{
		ID: uuid.NewString(), TenantID: ac.TenantID, AgentID: agent.ID,
		KnowledgeBaseID: knowledgeBase.ID, CreatedBy: ac.PrincipalID,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := lockKnowledgeBase(ctx, tx, knowledgeBase.TenantID, knowledgeBase.ID); lockErr != nil {
			return lockErr
		}
		var current KnowledgeBase
		if queryErr := tx.Where("id=?", knowledgeBase.ID).
			Where("visibility=? OR account_id=?", aiknowledge.VisibilityTenant, ac.PrincipalID).
			First(&current).Error; queryErr != nil {
			return knowledgeNotFound()
		}
		if createErr := tx.Create(&binding).Error; createErr != nil {
			return knowledgeDBError(createErr, "ai.knowledge.binding_exists", "knowledge base is already bound to the agent")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	setKnowledgeAudit(ctx, "ai_agent_knowledge_binding", binding.ID, auditAgentKnowledgeBound)
	return binding, nil
}

func (s *KnowledgeService) UnbindAgentKnowledge(ctx context.Context, request AgentKnowledgeUnbindRequest) (any, error) {
	if _, err := knowledgeActor(ctx); err != nil {
		return nil, err
	}
	agent, _, err := s.agents.GetVisible(ctx, request.AgentID, true)
	if err != nil {
		return nil, err
	}
	var binding AgentKnowledgeBinding
	if err = s.db.WithContext(ctx).Where("agent_id=? AND knowledge_base_id=?", agent.ID, request.KnowledgeBaseID).First(&binding).Error; err != nil {
		return nil, knowledgeNotFound()
	}
	result := s.db.WithContext(ctx).Delete(&binding)
	if result.Error != nil {
		return nil, dbError(result.Error, "unbind agent knowledge")
	}
	setKnowledgeAudit(ctx, "ai_agent_knowledge_binding", binding.ID, auditAgentKnowledgeUnbound)
	return map[string]any{"deleted": true}, nil
}

func (s *KnowledgeService) CanReadKnowledgeBase(ctx context.Context, id string) error {
	_, _, err := s.getKnowledgeBase(ctx, id, false)
	return err
}

func (s *KnowledgeService) CanManageKnowledgeBase(ctx context.Context, id string) error {
	_, _, err := s.getKnowledgeBase(ctx, id, true)
	return err
}

func (s *KnowledgeService) CanReadDocument(ctx context.Context, id string) error {
	_, _, _, err := s.getDocument(ctx, id, false)
	return err
}

func (s *KnowledgeService) GetKnowledgeBaseRef(ctx context.Context, id string) (aiknowledge.KnowledgeBaseRef, error) {
	row, _, err := s.getKnowledgeBase(ctx, id, false)
	if err != nil {
		return aiknowledge.KnowledgeBaseRef{}, err
	}
	return aiknowledge.KnowledgeBaseRef{
		ID: row.ID, TenantID: row.TenantID, AccountID: row.AccountID, Visibility: row.Visibility,
		Status: row.Status, EmbeddingAlias: row.EmbeddingAlias, Revision: row.Revision,
	}, nil
}

func (s *KnowledgeService) GetDocumentRef(ctx context.Context, id string) (aiknowledge.DocumentRef, error) {
	row, _, _, err := s.getDocument(ctx, id, false)
	if err != nil {
		return aiknowledge.DocumentRef{}, err
	}
	return aiknowledge.DocumentRef{
		ID: row.ID, TenantID: row.TenantID, KnowledgeBaseID: row.KnowledgeBaseID,
		AttachmentID: row.AttachmentID, SourceRevision: row.SourceRevision,
		ContentType: row.ContentType, OriginalName: row.OriginalName, Revision: row.Revision,
	}, nil
}

type knowledgeAttachmentPolicy struct {
	service *KnowledgeService
}

func (p knowledgeAttachmentPolicy) ResourceType() string { return KnowledgeDocumentResourceType }

func (p knowledgeAttachmentPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	ac, err := knowledgeActor(ctx)
	if err != nil || ac.TenantID != tenantID || ac.PrincipalID != userID || field != "source" {
		return knowledgeNotFound()
	}
	if attachment.ResourceLinkOperationFromContext(ctx) != "" {
		_, _, _, manageErr := p.service.getDocument(ctx, resourceID, true)
		return manageErr
	}
	return p.service.CanReadDocument(ctx, resourceID)
}

func knowledgeDBError(err error, conflictKey, conflictFallback string) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.AsAppError(err); ok {
		return err
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "duplicate") || strings.Contains(lower, "unique") {
		return errs.NewKey(errs.CodeConflict, conflictKey, conflictFallback)
	}
	return errs.Wrap(errs.CodeInternal, "knowledge persistence failed", err)
}

func setKnowledgeAudit(ctx context.Context, resourceType, resourceID, summary string) {
	if ac, ok := auth.FromContext(ctx); ok && ac != nil {
		ac.AuditResourceType = resourceType
		ac.AuditResourceID = resourceID
		ac.AuditSummary = summary
	}
}

var _ aiknowledge.Service = (*KnowledgeService)(nil)
