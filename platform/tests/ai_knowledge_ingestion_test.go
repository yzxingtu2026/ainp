package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	systemmodel "github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type ingestionSource struct{ text string }

func (s ingestionSource) OpenSource(context.Context, aiknowledge.SourceOpenRequest) (aiknowledge.DocumentSource, error) {
	return aiknowledge.DocumentSource{Reader: io.NopCloser(strings.NewReader(s.text)), Size: int64(len(s.text))}, nil
}

type ingestionEmbedding struct {
	dimensions int
	vectorSize int
	err        error
}

func (f ingestionEmbedding) Embed(_ context.Context, request aimodelruntime.EmbeddingRequest) (*aimodelruntime.EmbeddingResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	vectors := make([]aimodelruntime.EmbeddingVector, len(request.Inputs))
	size := f.vectorSize
	if size == 0 {
		size = f.dimensions
	}
	for i, input := range request.Inputs {
		vector := make([]float32, size)
		if size > 0 {
			vector[0] = 1
		}
		vectors[i] = aimodelruntime.EmbeddingVector{ID: input.ID, Index: i, Vector: vector}
	}
	return &aimodelruntime.EmbeddingResponse{Model: aimodelruntime.ModelSummary{Alias: request.ModelAlias, DeploymentID: "deployment-v1"}, Vectors: vectors, Dimensions: f.dimensions, PreprocessingVersion: aimodelruntime.EmbeddingPreprocessingVersionNone}, nil
}

func ingestionContext() context.Context {
	return auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", UserID: "owner-a", Realm: auth.RealmTenant})
}

func seedIngestion(t *testing.T, db *gorm.DB) (ai.KnowledgeBase, ai.KnowledgeDocument) {
	t.Helper()
	attachment := systemmodel.Attachment{ID: "attachment-a", TenantID: "tenant-a", Provider: "local", ObjectKey: "tenant-a/attachment-a", OriginalName: "safe.txt", ContentType: "text/plain", Size: 64, SHA256: strings.Repeat("a", 64), Status: status.AttachmentReady, UploaderID: "owner-a"}
	if err := db.Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	base := ai.KnowledgeBase{ID: "kb-a", TenantID: "tenant-a", OwnerUserID: "owner-a", Code: "kb", Name: "Knowledge", Visibility: aiknowledge.VisibilityPrivate, Status: aiknowledge.KnowledgeBaseEnabled, EmbeddingAlias: "embed", ChunkingConfig: aiknowledge.ChunkingConfig{Strategy: aiknowledge.ChunkingFixedTokens, ChunkSize: 100, ChunkOverlap: 10}, RetrievalConfig: aiknowledge.RetrievalConfig{TopK: 5, MaxContextTokens: 4000}, Revision: 1}
	if err := db.Create(&base).Error; err != nil {
		t.Fatal(err)
	}
	doc := ai.KnowledgeDocument{ID: "doc-a", TenantID: "tenant-a", KnowledgeBaseID: base.ID, AttachmentID: "attachment-a", SourceRevision: strings.Repeat("a", 64), ContentType: "text/plain", OriginalName: "safe.txt", IngestionStatus: aiknowledge.DocumentPending, Revision: 1}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatal(err)
	}
	return base, doc
}

func TestKnowledgeIngestionPublishesAtomicallyAndIsIdempotent(t *testing.T) {
	db := openKnowledgeTestDB(t)
	_, doc := seedIngestion(t, db)
	secondAttachment := systemmodel.Attachment{ID: "attachment-b", TenantID: "tenant-a", Provider: "local", ObjectKey: "tenant-a/attachment-b", OriginalName: "second.txt", ContentType: "text/plain", Size: 64, SHA256: strings.Repeat("b", 64), Status: status.AttachmentReady, UploaderID: "owner-a"}
	if err := db.Create(&secondAttachment).Error; err != nil {
		t.Fatal(err)
	}
	second := ai.KnowledgeDocument{ID: "doc-b", TenantID: "tenant-a", KnowledgeBaseID: "kb-a", AttachmentID: secondAttachment.ID, SourceRevision: secondAttachment.SHA256, ContentType: "text/plain", OriginalName: "second.txt", IngestionStatus: aiknowledge.DocumentPending, Revision: 1}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	registry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	service := ai.NewKnowledgeIngestionService(db, ingestionSource{text: strings.Repeat("stable ", 220)}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{dimensions: 3})
	payload := ai.KnowledgeIngestionJobPayload{DocumentID: doc.ID, SourceRevision: doc.SourceRevision, Revision: doc.Revision}
	if err = service.Process(ingestionContext(), payload); err != nil {
		t.Fatal(err)
	}
	var current ai.KnowledgeDocument
	if err = db.First(&current, "id=?", doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.IngestionStatus != aiknowledge.DocumentReady || current.CurrentIndexVersionID == nil {
		t.Fatalf("document was not atomically published: %#v", current)
	}
	var version ai.KnowledgeIndexVersion
	if err = db.First(&version, "id=?", *current.CurrentIndexVersionID).Error; err != nil {
		t.Fatal(err)
	}
	if version.Status != aiknowledge.IndexReady || version.EmbeddingDimensions != 3 || version.EmbeddingDeploymentID != "deployment-v1" || version.DocumentCount != 2 || version.ChunkCount != 6 {
		t.Fatalf("unexpected version: %#v", version)
	}
	var chunks, payloads int64
	db.Model(&ai.KnowledgeChunk{}).Count(&chunks)
	db.Model(&ai.KnowledgeChunkPayload{}).Count(&payloads)
	if chunks != 6 || payloads != 6 {
		t.Fatalf("unexpected derived rows: %d %d", chunks, payloads)
	}
	var storedPayload ai.KnowledgeChunkPayload
	if err = db.First(&storedPayload).Error; err != nil {
		t.Fatal(err)
	}
	if len(storedPayload.Vector) != 3 || storedPayload.Dimensions != 3 {
		t.Fatalf("pgvector payload did not round-trip: %#v", storedPayload)
	}
	var secondCurrent ai.KnowledgeDocument
	db.First(&secondCurrent, "id=?", second.ID)
	if secondCurrent.CurrentIndexVersionID == nil || *secondCurrent.CurrentIndexVersionID != version.ID {
		t.Fatalf("whole-base publish did not switch second document: %#v", secondCurrent)
	}
	if err = service.Process(ingestionContext(), payload); err != nil {
		t.Fatal(err)
	}
	var versions int64
	db.Model(&ai.KnowledgeIndexVersion{}).Count(&versions)
	if versions != 1 {
		t.Fatalf("duplicate delivery created %d versions", versions)
	}
}

func TestKnowledgeIngestionReservesVersionWithResolvedDeployment(t *testing.T) {
	db := openKnowledgeTestDB(t)
	if err := db.AutoMigrate(&ai.Deployment{}); err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrateKnowledge(db); err != nil {
		t.Fatal(err)
	}
	deployment := ai.Deployment{
		ID: "deployment-v1", TenantID: "tenant-a", ProviderID: "provider-a", Code: "embed-v1",
		RemoteModelID: "embed-v1", DisplayName: "Embedding V1", ModelType: "embedding",
		CapabilitiesJSON: json.RawMessage(`{"input_modalities":["text"],"output_modalities":[],"features":["embeddings"]}`),
		LimitsJSON:       json.RawMessage(`{"dimensions":3}`), Status: "enabled", ConfigJSON: json.RawMessage(`{}`),
	}
	if err := db.Create(&deployment).Error; err != nil {
		t.Fatal(err)
	}
	_, document := seedIngestion(t, db)
	registry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	service := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "stable text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{dimensions: 3})
	if err = service.Process(ingestionContext(), ai.KnowledgeIngestionJobPayload{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision}); err != nil {
		t.Fatal(err)
	}
	var version ai.KnowledgeIndexVersion
	if err = db.First(&version, "knowledge_base_id=?", document.KnowledgeBaseID).Error; err != nil {
		t.Fatal(err)
	}
	if version.EmbeddingDeploymentID != deployment.ID {
		t.Fatalf("reserved deployment = %q, want %q", version.EmbeddingDeploymentID, deployment.ID)
	}
}

func TestKnowledgeIngestionReservationFailureMarksDocumentFailed(t *testing.T) {
	db := openKnowledgeTestDB(t)
	if err := db.AutoMigrate(&ai.Deployment{}); err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrateKnowledge(db); err != nil {
		t.Fatal(err)
	}
	_, document := seedIngestion(t, db)
	registry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	service := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "stable text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{dimensions: 3})
	err = service.Process(ingestionContext(), ai.KnowledgeIngestionJobPayload{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision})
	if err == nil || !job.IsRetryable(err) {
		t.Fatalf("foreign-key reservation failure must be retryable: %v", err)
	}
	var failed ai.KnowledgeDocument
	if queryErr := db.First(&failed, "id=?", document.ID).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if failed.IngestionStatus != aiknowledge.DocumentFailed || failed.IngestionErrorCategory != "processing_failed" {
		t.Fatalf("reservation failure did not close document state: %#v", failed)
	}
}

func TestKnowledgeIngestionResolutionFailureMarksDocumentFailed(t *testing.T) {
	db := openKnowledgeTestDB(t)
	_, document := seedIngestion(t, db)
	registry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	service := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "stable text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{err: errors.New("resolver unavailable")}, ingestionEmbedding{dimensions: 3})
	err = service.Process(ingestionContext(), ai.KnowledgeIngestionJobPayload{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision})
	if err == nil || !job.IsRetryable(err) {
		t.Fatalf("resolver failure must be retryable: %v", err)
	}
	var failed ai.KnowledgeDocument
	if queryErr := db.First(&failed, "id=?", document.ID).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if failed.IngestionStatus != aiknowledge.DocumentFailed {
		t.Fatalf("resolver failure left document in %q", failed.IngestionStatus)
	}
}

func TestKnowledgeIngestionNonRetryableEmbeddingFailureIsPermanent(t *testing.T) {
	db := openKnowledgeTestDB(t)
	_, document := seedIngestion(t, db)
	registry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	providerError := &aimodelruntime.RuntimeError{Category: aimodelruntime.ErrorInvalidRequest, Message: "provider rejected batch", Retryable: false}
	service := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "stable text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{err: providerError})
	err = service.Process(ingestionContext(), ai.KnowledgeIngestionJobPayload{DocumentID: document.ID, SourceRevision: document.SourceRevision, Revision: document.Revision})
	if err == nil || !job.IsPermanent(err) {
		t.Fatalf("non-retryable provider failure must be permanent: %v", err)
	}
}

func TestKnowledgeIngestionRejectsStaleRevisionAndPreservesPublishedVersionOnFailure(t *testing.T) {
	db := openKnowledgeTestDB(t)
	_, doc := seedIngestion(t, db)
	registry, _ := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	good := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "stable text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{dimensions: 2})
	old := ai.KnowledgeIngestionJobPayload{DocumentID: doc.ID, SourceRevision: doc.SourceRevision, Revision: 1}
	if err := good.Process(ingestionContext(), old); err != nil {
		t.Fatal(err)
	}
	var published ai.KnowledgeDocument
	db.First(&published, "id=?", doc.ID)
	oldIndex := *published.CurrentIndexVersionID
	newRevision := strings.Repeat("b", 64)
	if err := db.Model(&ai.KnowledgeDocument{}).Where("id=?", doc.ID).Updates(map[string]any{"source_revision": newRevision, "revision": 2, "ingestion_status": aiknowledge.DocumentPending}).Error; err != nil {
		t.Fatal(err)
	}
	if err := good.Process(ingestionContext(), old); err != nil {
		t.Fatalf("stale work should be ignored: %v", err)
	}
	bad := ai.NewKnowledgeIngestionService(db, ingestionSource{text: "changed text"}, registry, aiknowledge.NewFixedTokenChunker(), knowledgeResolver{}, ingestionEmbedding{dimensions: 3, vectorSize: 2})
	err := bad.Process(ingestionContext(), ai.KnowledgeIngestionJobPayload{DocumentID: doc.ID, SourceRevision: newRevision, Revision: 2})
	if err == nil || !job.IsPermanent(err) {
		t.Fatalf("dimension mismatch must fail permanently: %v", err)
	}
	var failed ai.KnowledgeDocument
	db.First(&failed, "id=?", doc.ID)
	if failed.CurrentIndexVersionID == nil || *failed.CurrentIndexVersionID != oldIndex || failed.IngestionStatus != aiknowledge.DocumentFailed {
		t.Fatalf("failed build replaced old index: %#v", failed)
	}
	if err = db.Delete(&ai.KnowledgeDocument{}, "id=?", doc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = bad.Cleanup(ingestionContext(), doc.ID); err != nil {
		t.Fatal(err)
	}
	var remaining int64
	db.Model(&ai.KnowledgeIndexVersion{}).Where("source_document_id=?", doc.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatalf("cleanup left %d derived index versions", remaining)
	}
}

func TestKnowledgeIngestionPayloadAndMigrationRemainSafe(t *testing.T) {
	payload, err := json.Marshal(ai.KnowledgeIngestionJobPayload{Operation: "ingest", DocumentID: "doc", SourceRevision: strings.Repeat("a", 64), Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{"document_text", "chunk_text", "vector", "object_key", "provider_error"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("job payload leaked %s: %s", forbidden, text)
		}
	}
	migration, err := os.ReadFile("../migrations/0026_ai_knowledge_ingestion.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(migration)
	for _, required := range []string{"CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public", "ai_knowledge_chunk_payloads", "vector public.vector NOT NULL", "ux_ai_knowledge_index_source_revision"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("ingestion migration missing %s", required)
		}
	}
}

var _ aiknowledge.DocumentSourceResolver = ingestionSource{}
var _ aimodelruntime.EmbeddingRuntime = ingestionEmbedding{}
