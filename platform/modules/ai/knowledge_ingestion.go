package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	KnowledgeIngestionRequestedEvent = "ai.knowledge_ingestion_requested.v1"
	KnowledgeCleanupRequestedEvent   = "ai.knowledge_cleanup_requested.v1"
	KnowledgeIngestionJobType        = "ai.knowledge_ingestion"
	knowledgeIngestionHandler        = "ai.enqueue_knowledge_ingestion"
)

var errStaleKnowledgeIngestion = errors.New("knowledge ingestion revision became stale")

type KnowledgeIngestionEvent struct {
	DocumentID     string `json:"document_id"`
	SourceRevision string `json:"source_revision"`
	Revision       uint64 `json:"revision"`
}

type KnowledgeIngestionJobPayload struct {
	Operation      string `json:"operation"`
	DocumentID     string `json:"document_id"`
	SourceRevision string `json:"source_revision"`
	Revision       uint64 `json:"revision"`
}

func (s *KnowledgeIngestionService) Cleanup(ctx context.Context, documentID string) error {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac.TenantID == "" {
		return job.Permanent(errors.New("knowledge cleanup identity unavailable"))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document KnowledgeDocument
		if err := tx.Unscoped().Where("id=?", documentID).First(&document).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if !document.DeletedAt.Valid {
			return nil
		}
		var versions []string
		if err := tx.Model(&KnowledgeIndexVersion{}).Where("source_document_id=?", documentID).Pluck("id", &versions).Error; err != nil {
			return err
		}
		if len(versions) == 0 {
			return nil
		}
		if err := tx.Where("index_version_id IN ?", versions).Delete(&KnowledgeChunkPayload{}).Error; err != nil {
			return err
		}
		if err := tx.Where("index_version_id IN ?", versions).Delete(&KnowledgeChunk{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&KnowledgeBase{}).Where("id=? AND current_index_version_id IN ?", document.KnowledgeBaseID, versions).Update("current_index_version_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Model(&KnowledgeDocument{}).Where("current_index_version_id IN ?", versions).Updates(map[string]any{"current_index_version_id": nil, "ingestion_status": aiknowledge.DocumentPending}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", versions).Delete(&KnowledgeIndexVersion{}).Error
	})
}

type KnowledgeIngestionService struct {
	db         *gorm.DB
	sources    aiknowledge.DocumentSourceResolver
	parser     aiknowledge.DocumentParser
	chunker    aiknowledge.Chunker
	models     aimodel.Resolver
	embeddings aimodelruntime.EmbeddingRuntime
}

type knowledgeBuiltChunk struct {
	Document KnowledgeDocument
	Chunk    aiknowledge.Chunk
}

func NewKnowledgeIngestionService(db *gorm.DB, sources aiknowledge.DocumentSourceResolver, parser aiknowledge.DocumentParser, chunker aiknowledge.Chunker, models aimodel.Resolver, embeddings aimodelruntime.EmbeddingRuntime) *KnowledgeIngestionService {
	if err := registerKnowledgeTenantModels(); err != nil {
		panic(err)
	}
	return &KnowledgeIngestionService{db: tenantdata.RuntimeDB(db), sources: sources, parser: parser, chunker: chunker, models: models, embeddings: embeddings}
}

func (s *KnowledgeIngestionService) Process(ctx context.Context, payload KnowledgeIngestionJobPayload) error {
	if s.db == nil || s.sources == nil || s.parser == nil || s.chunker == nil || s.models == nil || s.embeddings == nil {
		return job.Permanent(errors.New("knowledge ingestion dependencies unavailable"))
	}
	ac, ok := auth.FromContext(ctx)
	if !ok || !ac.Authenticated || ac.TenantID == "" {
		return job.Permanent(errors.New("knowledge ingestion identity unavailable"))
	}
	var document KnowledgeDocument
	if err := s.db.WithContext(ctx).Where("id=?", payload.DocumentID).First(&document).Error; err != nil {
		return job.Permanent(errors.New("knowledge document unavailable"))
	}
	if document.Revision != payload.Revision || document.SourceRevision != payload.SourceRevision {
		return nil
	}
	claimed := s.db.WithContext(ctx).Model(&KnowledgeDocument{}).
		Where("id=? AND revision=? AND source_revision=?", document.ID, document.Revision, document.SourceRevision).
		Updates(map[string]any{"ingestion_status": aiknowledge.DocumentProcessing, "ingestion_error_category": ""})
	if claimed.Error != nil {
		return job.Retryable(errors.New("knowledge document claim failed"))
	}
	if claimed.RowsAffected != 1 {
		return nil
	}
	completed := false
	versionID := ""
	defer func() {
		if completed {
			return
		}
		if versionID != "" {
			s.failVersion(context.WithoutCancel(ctx), document, versionID, "processing_failed")
			return
		}
		s.failDocument(context.WithoutCancel(ctx), document, "processing_failed")
	}()
	var base KnowledgeBase
	if err := s.db.WithContext(ctx).Where("id=?", document.KnowledgeBaseID).First(&base).Error; err != nil {
		return job.Permanent(errors.New("knowledge base unavailable"))
	}
	var documents []KnowledgeDocument
	if err := s.db.WithContext(ctx).Where("knowledge_base_id=?", base.ID).Order("id").Find(&documents).Error; err != nil {
		return job.Retryable(errors.New("knowledge document snapshot unavailable"))
	}
	if len(documents) == 0 {
		return job.Permanent(errors.New("knowledge base contains no documents"))
	}
	resolved, err := s.models.Resolve(ctx, base.EmbeddingAlias, aimodel.CapabilityRequirements{
		ModelType:       aimodel.ModelTypeEmbedding,
		InputModalities: []aimodel.Modality{aimodel.ModalityText},
		Features:        []aimodel.Feature{aimodel.FeatureEmbeddings},
	})
	if err != nil {
		return job.Retryable(errors.New("embedding model resolution failed"))
	}
	if resolved == nil || resolved.DeploymentID == "" {
		return job.Permanent(errors.New("embedding deployment unavailable"))
	}

	version, done, err := s.beginVersion(ctx, base, document, resolved.DeploymentID)
	if err != nil {
		return job.Retryable(errors.New("knowledge index reservation failed"))
	}
	if done {
		s.markDocumentReady(context.WithoutCancel(ctx), document, version.ID)
		completed = true
		return nil
	}
	versionID = version.ID

	processor := aiknowledge.Processor{Sources: s.sources, Parser: s.parser, Chunker: s.chunker, Limits: aiknowledge.DefaultParseLimits()}
	var chunks []knowledgeBuiltChunk
	for _, snapshot := range documents {
		parsed, parseErr := processor.Process(ctx, documentRef(snapshot), base.ChunkingConfig)
		if parseErr != nil {
			return classifyIngestionError(parseErr)
		}
		for _, chunk := range parsed {
			chunks = append(chunks, knowledgeBuiltChunk{Document: snapshot, Chunk: chunk})
		}
	}
	if len(chunks) == 0 {
		return job.Permanent(errors.New("knowledge base contains no indexable text"))
	}

	vectors := make(map[string]aimodelruntime.EmbeddingVector, len(chunks))
	deploymentID, preprocessingVersion, dimensions := resolved.DeploymentID, "", 0
	for start := 0; start < len(chunks); start += aimodelruntime.MaxEmbeddingItems {
		end := start + aimodelruntime.MaxEmbeddingItems
		if end > len(chunks) {
			end = len(chunks)
		}
		inputs := make([]aimodelruntime.EmbeddingItem, 0, end-start)
		for i := start; i < end; i++ {
			inputs = append(inputs, aimodelruntime.EmbeddingItem{ID: fmt.Sprintf("%d", i), Text: chunks[i].Chunk.Text})
		}
		response, embedErr := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: base.EmbeddingAlias, Inputs: inputs, Purpose: aimodelruntime.EmbeddingPurposeKnowledgeIngestion})
		if embedErr != nil {
			var runtimeErr *aimodelruntime.RuntimeError
			if errors.As(embedErr, &runtimeErr) && !runtimeErr.Retryable {
				return job.Permanent(errors.New("embedding runtime failed"))
			}
			return job.Retryable(errors.New("embedding runtime failed"))
		}
		if dimensions == 0 {
			dimensions = response.Dimensions
			preprocessingVersion = response.PreprocessingVersion
		}
		if response.Dimensions != dimensions || response.Model.DeploymentID != deploymentID || response.PreprocessingVersion != preprocessingVersion {
			return job.Permanent(errors.New("embedding model changed during index build"))
		}
		for _, vector := range response.Vectors {
			vectors[vector.ID] = vector
		}
	}
	if dimensions <= 0 || deploymentID == "" {
		return job.Permanent(errors.New("embedding provenance unavailable"))
	}
	if err = s.publish(ctx, base, documents, version, chunks, vectors, deploymentID, preprocessingVersion, dimensions); err != nil {
		if errors.Is(err, errStaleKnowledgeIngestion) {
			s.abortVersion(context.WithoutCancel(ctx), version.ID)
			s.markDocumentPending(context.WithoutCancel(ctx), document)
			completed = true
			return nil
		}
		return err
	}
	completed = true
	return nil
}

func (s *KnowledgeIngestionService) beginVersion(ctx context.Context, base KnowledgeBase, document KnowledgeDocument, deploymentID string) (KnowledgeIndexVersion, bool, error) {
	var version KnowledgeIndexVersion
	done := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", base.ID).First(&KnowledgeBase{}).Error; err != nil {
			return err
		}
		err := tx.Where("knowledge_base_id=? AND source_document_id=? AND source_revision=?", base.ID, document.ID, document.SourceRevision).First(&version).Error
		if err == nil {
			if version.Status == aiknowledge.IndexReady && version.ParserVersion == aiknowledge.ParserVersion && version.ChunkingVersion == aiknowledge.ChunkingVersion {
				done = true
				return nil
			}
			return tx.Model(&version).Updates(map[string]any{"status": aiknowledge.IndexBuilding, "embedding_alias": base.EmbeddingAlias, "embedding_deployment_id": deploymentID, "parser_version": aiknowledge.ParserVersion, "chunking_version": aiknowledge.ChunkingVersion, "error_category": "", "completed_at": nil}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var maxVersion int
		if err = tx.Model(&KnowledgeIndexVersion{}).Where("knowledge_base_id=?", base.ID).Select("COALESCE(MAX(version),0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		config, _ := json.Marshal(base.ChunkingConfig)
		sum := sha256.Sum256(append([]byte(base.EmbeddingAlias+":"), config...))
		sourceDocumentID, sourceRevision := document.ID, document.SourceRevision
		version = KnowledgeIndexVersion{ID: uuid.NewString(), TenantID: base.TenantID, KnowledgeBaseID: base.ID, SourceDocumentID: &sourceDocumentID, SourceRevision: &sourceRevision, Version: maxVersion + 1, Status: aiknowledge.IndexBuilding, EmbeddingAlias: base.EmbeddingAlias, EmbeddingDeploymentID: deploymentID, EmbeddingDimensions: 1, PreprocessingVersion: "pending", ParserVersion: aiknowledge.ParserVersion, ChunkingVersion: aiknowledge.ChunkingVersion, ConfigHash: hex.EncodeToString(sum[:])}
		return tx.Create(&version).Error
	})
	return version, done, err
}

func (s *KnowledgeIngestionService) publish(ctx context.Context, base KnowledgeBase, documents []KnowledgeDocument, version KnowledgeIndexVersion, chunks []knowledgeBuiltChunk, vectors map[string]aimodelruntime.EmbeddingVector, deploymentID, preprocessing string, dimensions int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var currentBase KnowledgeBase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", base.ID).First(&currentBase).Error; err != nil {
			return errStaleKnowledgeIngestion
		}
		if currentBase.EmbeddingAlias != base.EmbeddingAlias || currentBase.ChunkingConfig != base.ChunkingConfig {
			return errStaleKnowledgeIngestion
		}
		for _, snapshot := range documents {
			var current KnowledgeDocument
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", snapshot.ID).First(&current).Error; err != nil {
				return errStaleKnowledgeIngestion
			}
			if current.Revision != snapshot.Revision || current.SourceRevision != snapshot.SourceRevision {
				return errStaleKnowledgeIngestion
			}
		}
		var currentVersion KnowledgeIndexVersion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", version.ID).First(&currentVersion).Error; err != nil {
			return err
		}
		if currentVersion.Status == aiknowledge.IndexReady {
			return nil
		}
		if err := tx.Where("index_version_id=?", version.ID).Delete(&KnowledgeChunkPayload{}).Error; err != nil {
			return err
		}
		if err := tx.Where("index_version_id=?", version.ID).Delete(&KnowledgeChunk{}).Error; err != nil {
			return err
		}
		for i, chunk := range chunks {
			key := fmt.Sprintf("%d", i)
			vector, ok := vectors[key]
			if !ok || len(vector.Vector) != dimensions {
				return job.Permanent(errors.New("embedding result incomplete"))
			}
			id := uuid.NewString()
			row := KnowledgeChunk{ID: id, TenantID: chunk.Document.TenantID, KnowledgeBaseID: chunk.Document.KnowledgeBaseID, DocumentID: chunk.Document.ID, IndexVersionID: version.ID, Ordinal: i, SourceRevision: chunk.Document.SourceRevision, PageNumber: chunk.Chunk.PageNumber, PositionStart: chunk.Chunk.PositionStart, PositionEnd: chunk.Chunk.PositionEnd, HeadingPath: chunk.Chunk.HeadingPath, TokenCount: chunk.Chunk.TokenCount, ContentHash: chunk.Chunk.ContentHash}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			payload := KnowledgeChunkPayload{ChunkID: id, TenantID: chunk.Document.TenantID, IndexVersionID: version.ID, Text: chunk.Chunk.Text, Vector: PGVector(vector.Vector), Dimensions: dimensions}
			if err := tx.Create(&payload).Error; err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if err := tx.Model(&KnowledgeIndexVersion{}).Where("id=?", version.ID).Updates(map[string]any{"status": aiknowledge.IndexReady, "embedding_deployment_id": deploymentID, "embedding_dimensions": dimensions, "preprocessing_version": preprocessing, "document_count": len(documents), "chunk_count": len(chunks), "completed_at": now}).Error; err != nil {
			return err
		}
		if currentBase.CurrentIndexVersionID != nil && *currentBase.CurrentIndexVersionID != version.ID {
			_ = tx.Model(&KnowledgeIndexVersion{}).Where("id=? AND status=?", *currentBase.CurrentIndexVersionID, aiknowledge.IndexReady).Update("status", aiknowledge.IndexSuperseded).Error
		}
		for _, snapshot := range documents {
			if err := tx.Model(&KnowledgeDocument{}).Where("id=? AND revision=?", snapshot.ID, snapshot.Revision).Updates(map[string]any{"current_index_version_id": version.ID, "ingestion_status": aiknowledge.DocumentReady, "ingestion_error_category": ""}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&KnowledgeBase{}).Where("id=?", base.ID).Update("current_index_version_id", version.ID).Error
	})
}

func (s *KnowledgeIngestionService) abortVersion(ctx context.Context, versionID string) {
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&KnowledgeIndexVersion{}).Where("id=? AND status=?", versionID, aiknowledge.IndexBuilding).Updates(map[string]any{"status": aiknowledge.IndexAborted, "error_category": "stale_revision", "completed_at": now}).Error
}

func (s *KnowledgeIngestionService) failVersion(ctx context.Context, document KnowledgeDocument, versionID, category string) {
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&KnowledgeIndexVersion{}).Where("id=? AND status=?", versionID, aiknowledge.IndexBuilding).Updates(map[string]any{"status": aiknowledge.IndexFailed, "error_category": category, "completed_at": now}).Error
	_ = s.db.WithContext(ctx).Model(&KnowledgeDocument{}).Where("id=? AND revision=?", document.ID, document.Revision).Updates(map[string]any{"ingestion_status": aiknowledge.DocumentFailed, "ingestion_error_category": category}).Error
}

func (s *KnowledgeIngestionService) failDocument(ctx context.Context, document KnowledgeDocument, category string) {
	_ = s.db.WithContext(ctx).Model(&KnowledgeDocument{}).Where("id=? AND revision=?", document.ID, document.Revision).Updates(map[string]any{"ingestion_status": aiknowledge.DocumentFailed, "ingestion_error_category": category}).Error
}

func (s *KnowledgeIngestionService) markDocumentReady(ctx context.Context, document KnowledgeDocument, versionID string) {
	_ = s.db.WithContext(ctx).Model(&KnowledgeDocument{}).Where("id=? AND revision=?", document.ID, document.Revision).Updates(map[string]any{"current_index_version_id": versionID, "ingestion_status": aiknowledge.DocumentReady, "ingestion_error_category": ""}).Error
}

func (s *KnowledgeIngestionService) markDocumentPending(ctx context.Context, document KnowledgeDocument) {
	_ = s.db.WithContext(ctx).Model(&KnowledgeDocument{}).Where("id=? AND revision=?", document.ID, document.Revision).Updates(map[string]any{"ingestion_status": aiknowledge.DocumentPending, "ingestion_error_category": ""}).Error
}

func classifyIngestionError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return job.Cancelled(errors.New("knowledge ingestion cancelled"))
	case errors.Is(err, aiknowledge.ErrUnsupportedFormat), errors.Is(err, aiknowledge.ErrMIMEMismatch), errors.Is(err, aiknowledge.ErrUnsafeContent), errors.Is(err, aiknowledge.ErrCorruptDocument), errors.Is(err, aiknowledge.ErrLimitExceeded):
		return job.Permanent(errors.New("knowledge document rejected"))
	default:
		return job.Retryable(errors.New("knowledge parsing failed"))
	}
}
func documentRef(d KnowledgeDocument) aiknowledge.DocumentRef {
	return aiknowledge.DocumentRef{ID: d.ID, TenantID: d.TenantID, KnowledgeBaseID: d.KnowledgeBaseID, AttachmentID: d.AttachmentID, SourceRevision: d.SourceRevision, ContentType: d.ContentType, OriginalName: d.OriginalName, Revision: d.Revision}
}

func appendKnowledgeIngestion(ctx context.Context, tx *gorm.DB, writer outbox.Writer, document KnowledgeDocument) error {
	if writer == nil {
		return nil
	}
	_, err := writer.Append(ctx, tx, outbox.AppendInput{EventType: KnowledgeIngestionRequestedEvent, AggregateType: "ai.knowledge_document", AggregateID: document.ID, AggregateVersion: int64(document.Revision), Payload: KnowledgeIngestionEvent{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision}})
	return err
}

func appendKnowledgeCleanup(ctx context.Context, tx *gorm.DB, writer outbox.Writer, document KnowledgeDocument) error {
	if writer == nil {
		return nil
	}
	_, err := writer.Append(ctx, tx, outbox.AppendInput{EventType: KnowledgeCleanupRequestedEvent, AggregateType: "ai.knowledge_document", AggregateID: document.ID, AggregateVersion: int64(document.Revision), Payload: KnowledgeIngestionEvent{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision}})
	return err
}
