package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	systemmodel "github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type knowledgeResolver struct{}

func (knowledgeResolver) Resolve(_ context.Context, alias string, requirements aimodel.CapabilityRequirements) (*aimodel.ResolvedModel, error) {
	if alias != "embed" || requirements.ModelType != aimodel.ModelTypeEmbedding ||
		len(requirements.Features) != 1 || requirements.Features[0] != aimodel.FeatureEmbeddings {
		return nil, errs.New(errs.CodeValidation, "embedding requirements not satisfied")
	}
	return &aimodel.ResolvedModel{ModelType: aimodel.ModelTypeEmbedding}, nil
}

func TestKnowledgeContractsNormalizeValidateAndFakeDeterministically(t *testing.T) {
	chunking := aiknowledge.ChunkingConfig{}
	chunking.Normalize()
	if err := chunking.Validate(); err != nil || chunking.ChunkSize != 800 || chunking.ChunkOverlap != 100 {
		t.Fatalf("unexpected chunking defaults: %#v %v", chunking, err)
	}
	retrieval := aiknowledge.RetrievalConfig{}
	retrieval.Normalize()
	if err := retrieval.Validate(); err != nil || retrieval.TopK != 5 || retrieval.MaxContextTokens != 4000 {
		t.Fatalf("unexpected retrieval defaults: %#v %v", retrieval, err)
	}
	bad := aiknowledge.ChunkingConfig{Strategy: aiknowledge.ChunkingFixedTokens, ChunkSize: 100, ChunkOverlap: 100}
	if bad.Validate() == nil {
		t.Fatal("overlap equal to chunk size must fail")
	}

	fake := aiknowledge.NewFake()
	fake.ReadableKBs["kb"] = true
	fake.KnowledgeBases["kb"] = aiknowledge.KnowledgeBaseRef{ID: "kb", TenantID: "tenant-a"}
	fake.Retrieval = []aiknowledge.RetrievedChunk{
		{Citation: aiknowledge.Citation{KnowledgeBaseID: "kb", ChunkID: "z"}, Score: 0.8},
		{Citation: aiknowledge.Citation{KnowledgeBaseID: "kb", ChunkID: "a"}, Score: 0.8},
		{Citation: aiknowledge.Citation{KnowledgeBaseID: "other", ChunkID: "x"}, Score: 1},
	}
	items, err := fake.Retrieve(context.Background(), aiknowledge.RetrievalRequest{
		KnowledgeBaseIDs: []string{"kb"}, Config: aiknowledge.RetrievalConfig{TopK: 1, MinScore: 0.5},
	})
	if err != nil || len(items) != 1 || items[0].Citation.ChunkID != "a" {
		t.Fatalf("fake retrieval is not deterministic: %#v %v", items, err)
	}
	if empty, emptyErr := fake.Retrieve(context.Background(), aiknowledge.RetrievalRequest{Config: retrieval}); emptyErr == nil || len(empty) != 0 {
		t.Fatalf("empty knowledge scope must fail closed: %#v %v", empty, emptyErr)
	}
	fake.ParsedSections = []aiknowledge.ParsedSection{{Ordinal: 1, Text: "stable"}}
	sections, err := fake.Parse(context.Background(), aiknowledge.DocumentRef{}, strings.NewReader("ignored"))
	if err != nil || len(sections) != 1 || sections[0].Text != "stable" {
		t.Fatalf("fake parser is not configurable: %#v %v", sections, err)
	}
}

func TestKnowledgeMigrationModelsActionsAndI18n(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0025_ai_knowledge_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(migration)
	for _, required := range []string{
		"ai_knowledge_bases", "ai_knowledge_documents", "ai_knowledge_index_versions",
		"ai_knowledge_chunks", "ai_agent_knowledge_bindings",
		"fk_ai_knowledge_document_attachment_tenant", "ck_ai_knowledge_base_enabled_embedding",
		"embedding_deployment_id", "max_context_tokens", "WHERE deleted_at IS NULL",
		"source_revision", "position_start", "superseded", "aborted",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration is missing %s", required)
		}
	}
	for _, forbidden := range []string{"vector(", "tsvector", "object_key", "document_text", "chunk_text"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("foundation migration contains deferred payload/storage field %q", forbidden)
		}
	}
	if (ai.KnowledgeBase{}).TableName() != "ai_knowledge_bases" ||
		(ai.KnowledgeDocument{}).TableName() != "ai_knowledge_documents" ||
		(ai.KnowledgeChunk{}).TableName() != "ai_knowledge_chunks" {
		t.Fatal("knowledge model table names must use ai_ prefix")
	}
	encoded, err := json.Marshal(ai.KnowledgeDocument{
		ID: "document-a", TenantID: "tenant-a", AttachmentID: "attachment-a",
		SourceRevision: strings.Repeat("a", 64), ContentType: "text/plain", OriginalName: "safe.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "tenant-a") || strings.Contains(string(encoded), "object_key") {
		t.Fatalf("document API leaked tenant/storage data: %s", encoded)
	}

	application := app.New(&config.Config{}, nil, nil, nil)
	if err = application.RegisterCapability(capability.Capability{ID: "ai", NameKey: "capability.ai.name"}); err != nil {
		t.Fatal(err)
	}
	service := ai.NewKnowledgeService(nil, nil, nil, nil)
	if err = ai.RegisterKnowledge(application, service); err != nil {
		t.Fatal(err)
	}
	if application.KnowledgeService == nil || application.KnowledgeAccessPolicy == nil {
		t.Fatal("knowledge contracts were not injected into app")
	}
	for _, id := range []string{
		ai.ActionKnowledgeBaseList, ai.ActionKnowledgeBaseCreate,
		ai.ActionKnowledgeDocumentList, ai.ActionKnowledgeDocumentCreate,
		ai.ActionKnowledgeRetrievalTest, ai.ActionKnowledgeCitationResolve,
		ai.ActionAgentKnowledgeList, ai.ActionAgentKnowledgeBind,
	} {
		registered, ok := application.Actions.Get(id)
		if !ok || registered.Meta.PermissionGroup != "ai.knowledge_management" {
			t.Fatalf("knowledge action not registered correctly: %s", id)
		}
		wantAudit := !strings.HasSuffix(id, "_list") && !strings.HasSuffix(id, "_get") &&
			id != ai.ActionKnowledgeRetrievalTest && id != ai.ActionKnowledgeCitationResolve
		if registered.Meta.Audit == nil || *registered.Meta.Audit != wantAudit {
			t.Fatalf("unexpected audit metadata for %s", id)
		}
	}
	if _, ok := application.CodeSets.Get(ai.CodeSetKnowledgeBaseStatus); !ok {
		t.Fatal("knowledge base status CodeSet not registered")
	}
	if err = i18n.LoadDir("../i18n"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"permission_group.ai.knowledge_management",
		"codeset.ai.knowledge_base_status.draft",
		"action.ai.knowledge_base_create.name",
		"action.ai.knowledge_retrieval_test.name",
		"action.ai.knowledge_citation_resolve.name",
		"ai.knowledge.embedding_required",
	} {
		if i18n.T("en", key) == key || i18n.T("zh-CN", key) == key {
			t.Fatalf("missing bilingual i18n key: %s", key)
		}
	}
}

func openKnowledgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "knowledge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	db, err := database.Open(fmt.Sprintf("%s search_path=%s", dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(
		&systemmodel.Attachment{}, &systemmodel.AttachmentLink{}, &ai.Agent{}, &ai.AgentVersion{}, &ai.AgentToolBinding{},
	); err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrateKnowledge(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKnowledgeSQLMigrationExecutesWithTenantConstraints(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "knowledge_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
	db, err := database.Open(fmt.Sprintf("%s search_path=%s", dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(
		&tenant.Tenant{}, &systemmodel.User{}, &systemmodel.Attachment{},
		&ai.Provider{}, &ai.Deployment{}, &ai.Agent{},
	); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../migrations/0025_ai_knowledge_foundation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(migration)).Error; err != nil {
		t.Fatalf("execute knowledge migration: %v", err)
	}
	if err = ai.AutoMigrateKnowledge(db); err != nil {
		t.Fatalf("auto migrate knowledge after SQL migration: %v", err)
	}
	if err = db.Exec(string(migration)).Error; err != nil {
		t.Fatalf("knowledge migration must remain idempotent after AutoMigrate: %v", err)
	}
	for _, constraint := range []string{
		"fk_ai_knowledge_base_owner_tenant",
		"fk_ai_knowledge_document_attachment_tenant",
		"fk_ai_knowledge_index_embedding_deployment_tenant",
		"fk_ai_knowledge_chunk_document_tenant",
		"fk_ai_agent_knowledge_agent_tenant",
	} {
		var count int64
		if err = db.Raw("SELECT COUNT(*) FROM pg_constraint WHERE conname = ?", constraint).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("migration constraint %s missing", constraint)
		}
	}
}

func knowledgeContext(tenantID, userID string, permissions ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, TenantID: tenantID, UserID: userID, Realm: auth.RealmTenant,
		Permissions: permissions,
	})
}

func TestKnowledgeTenantAttachmentRevisionAndBindingBoundaries(t *testing.T) {
	db := openKnowledgeTestDB(t)
	agents := ai.NewAgentService(db, aiagent.NewRegistry(), nil, aitool.NewRegistry())
	application := app.New(&config.Config{}, db, nil, nil)
	attachSvc := system.NewAttachmentService(db, nil, nil, application.AttachmentPolicies, nil)
	service := ai.NewKnowledgeService(db, knowledgeResolver{}, agents, attachSvc)
	if err := application.RegisterCapability(capability.Capability{ID: "ai", NameKey: "capability.ai.name"}); err != nil {
		t.Fatal(err)
	}
	if err := ai.RegisterKnowledge(application, service); err != nil {
		t.Fatal(err)
	}
	ownerCtx := knowledgeContext("tenant-a", "owner")
	otherCtx := knowledgeContext("tenant-a", "other")
	mutationCtx := knowledgeContext("tenant-a", "other", ai.ActionKnowledgeBaseUpdate, ai.ActionAgentKnowledgeBind)

	create := func(ctx context.Context, code, visibility string) ai.KnowledgeBase {
		value, err := service.CreateKnowledgeBase(ctx, ai.KnowledgeBaseCreateRequest{
			Code: code, Name: code, Visibility: visibility, EmbeddingAlias: "embed",
			ChunkingConfig:  aiknowledge.ChunkingConfig{Strategy: aiknowledge.ChunkingFixedTokens, ChunkSize: 800, ChunkOverlap: 100},
			RetrievalConfig: aiknowledge.RetrievalConfig{TopK: 5, MaxContextTokens: 4000},
		})
		if err != nil {
			t.Fatal(err)
		}
		return value.(ai.KnowledgeBase)
	}
	privateKB := create(ownerCtx, "private-kb", aiknowledge.VisibilityPrivate)
	tenantKB := create(ownerCtx, "tenant-kb", aiknowledge.VisibilityTenant)
	indexID := "index-current"
	if err := db.Create(&ai.KnowledgeIndexVersion{
		ID: indexID, TenantID: tenantKB.TenantID, KnowledgeBaseID: tenantKB.ID, Version: 1,
		Status: aiknowledge.IndexReady, EmbeddingAlias: "embed", EmbeddingDeploymentID: "deployment-a",
		EmbeddingDimensions: 1536, PreprocessingVersion: "1", ParserVersion: "1", ChunkingVersion: "1",
		ConfigHash: strings.Repeat("b", 64),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ai.KnowledgeBase{}).Where("id=?", tenantKB.ID).Update("current_index_version_id", indexID).Error; err != nil {
		t.Fatal(err)
	}
	tenantKB.CurrentIndexVersionID = &indexID
	retrievalOnlyValue, err := service.UpdateKnowledgeBase(ownerCtx, ai.KnowledgeBaseUpdateRequest{
		ID: tenantKB.ID, Name: tenantKB.Name, Visibility: tenantKB.Visibility,
		EmbeddingAlias: tenantKB.EmbeddingAlias, ChunkingConfig: tenantKB.ChunkingConfig,
		RetrievalConfig: aiknowledge.RetrievalConfig{TopK: 8, MaxContextTokens: 5000}, Revision: tenantKB.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantKB = *retrievalOnlyValue.(*ai.KnowledgeBase)
	if tenantKB.CurrentIndexVersionID == nil || *tenantKB.CurrentIndexVersionID != indexID {
		t.Fatal("retrieval-only update must retain current index version")
	}
	changedChunking := tenantKB.ChunkingConfig
	changedChunking.ChunkSize++
	chunkingValue, err := service.UpdateKnowledgeBase(ownerCtx, ai.KnowledgeBaseUpdateRequest{
		ID: tenantKB.ID, Name: tenantKB.Name, Visibility: tenantKB.Visibility,
		EmbeddingAlias: tenantKB.EmbeddingAlias, ChunkingConfig: changedChunking,
		RetrievalConfig: tenantKB.RetrievalConfig, Revision: tenantKB.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantKB = *chunkingValue.(*ai.KnowledgeBase)
	if tenantKB.CurrentIndexVersionID != nil {
		t.Fatal("chunking update must invalidate current index version")
	}
	if _, err := service.GetKnowledgeBase(otherCtx, ai.KnowledgeBaseIDRequest{ID: privateKB.ID}); !isCode(err, errs.CodeNotFound) {
		t.Fatalf("private knowledge base leaked to another user: %v", err)
	}
	if _, err := service.GetKnowledgeBase(otherCtx, ai.KnowledgeBaseIDRequest{ID: tenantKB.ID}); err != nil {
		t.Fatalf("tenant knowledge base should be visible: %v", err)
	}
	if _, err := service.UpdateKnowledgeBase(mutationCtx, ai.KnowledgeBaseUpdateRequest{
		ID: tenantKB.ID, Name: "forbidden", Visibility: tenantKB.Visibility,
		EmbeddingAlias: tenantKB.EmbeddingAlias, ChunkingConfig: tenantKB.ChunkingConfig,
		RetrievalConfig: tenantKB.RetrievalConfig, Revision: tenantKB.Revision,
	}); !isCode(err, errs.CodeNotFound) {
		t.Fatalf("tenant visibility must not grant modification rights: %v", err)
	}
	if _, err := service.GetKnowledgeBase(mutationCtx, ai.KnowledgeBaseIDRequest{ID: privateKB.ID}); !isCode(err, errs.CodeNotFound) {
		t.Fatalf("mutation permission must not expose another owner's private knowledge base: %v", err)
	}
	platformCtx := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, TenantID: "tenant-a", UserID: "owner", Realm: auth.RealmPlatform,
	})
	if _, err := service.GetKnowledgeBase(platformCtx, ai.KnowledgeBaseIDRequest{ID: privateKB.ID}); !isCode(err, errs.CodeUnauthorized) {
		t.Fatalf("platform realm must fail closed on tenant knowledge path: %v", err)
	}
	if _, err := service.UpdateKnowledgeBase(ownerCtx, ai.KnowledgeBaseUpdateRequest{
		ID: privateKB.ID, Name: "conflict", Visibility: aiknowledge.VisibilityPrivate,
		EmbeddingAlias: "embed", ChunkingConfig: privateKB.ChunkingConfig,
		RetrievalConfig: privateKB.RetrievalConfig, Revision: privateKB.Revision + 1,
	}); !isCode(err, errs.CodeRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	deletableKB := create(ownerCtx, "reusable-kb", aiknowledge.VisibilityPrivate)
	if _, err := service.DeleteKnowledgeBase(ownerCtx, ai.KnowledgeBaseDeleteRequest{ID: deletableKB.ID, Revision: deletableKB.Revision}); err != nil {
		t.Fatal(err)
	}
	var retainedKBs int64
	if err := db.Unscoped().Model(&ai.KnowledgeBase{}).Where("id=?", deletableKB.ID).Count(&retainedKBs).Error; err != nil || retainedKBs != 1 {
		t.Fatalf("soft-deleted knowledge base was not retained: count=%d err=%v", retainedKBs, err)
	}
	_ = create(ownerCtx, "reusable-kb", aiknowledge.VisibilityPrivate)

	hash := strings.Repeat("a", 64)
	for _, attachment := range []systemmodel.Attachment{
		{ID: "owned", TenantID: "tenant-a", Provider: "local", ObjectKey: "private/path-a", OriginalName: "owned.txt", ContentType: "text/plain", SHA256: hash, Status: status.AttachmentReady, UploaderID: "owner"},
		{ID: "owned-next", TenantID: "tenant-a", Provider: "local", ObjectKey: "private/path-next", OriginalName: "owned-next.txt", ContentType: "text/plain", SHA256: hash, Status: status.AttachmentReady, UploaderID: "owner"},
		{ID: "other", TenantID: "tenant-a", Provider: "local", ObjectKey: "private/path-b", OriginalName: "other.txt", ContentType: "text/plain", SHA256: hash, Status: status.AttachmentReady, UploaderID: "other"},
		{ID: "cross", TenantID: "tenant-b", Provider: "local", ObjectKey: "private/path-c", OriginalName: "cross.txt", ContentType: "text/plain", SHA256: hash, Status: status.AttachmentReady, UploaderID: "owner"},
	} {
		if err := db.Create(&attachment).Error; err != nil {
			t.Fatal(err)
		}
	}
	documentValue, err := service.CreateDocument(ownerCtx, ai.KnowledgeDocumentCreateRequest{KnowledgeBaseID: privateKB.ID, AttachmentID: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	document := documentValue.(ai.KnowledgeDocument)
	if document.SourceRevision != hash || document.ContentType != "text/plain" || document.OriginalName != "owned.txt" {
		t.Fatalf("attachment snapshot is incomplete: %#v", document)
	}
	var links []systemmodel.AttachmentLink
	if err = db.Where("resource_type=? AND resource_id=?", ai.KnowledgeDocumentResourceType, document.ID).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].AttachmentID != "owned" || links[0].Field != "source" {
		t.Fatalf("knowledge source link was not created atomically: %#v", links)
	}
	if _, err = attachSvc.BindResource(ownerCtx, db, attachment.ResourceLinkRequest{
		AttachmentID: "owned-next", ResourceType: ai.KnowledgeDocumentResourceType,
		ResourceID: document.ID, Field: "source",
	}); !isCode(err, errs.CodeInternal) {
		t.Fatalf("resource linker must reject a non-transactional DB handle: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		_, bindErr := attachSvc.BindResource(ownerCtx, tx, attachment.ResourceLinkRequest{
			AttachmentID: "other", ResourceType: ai.KnowledgeDocumentResourceType,
			ResourceID: document.ID, Field: "source",
		})
		return bindErr
	})
	if !isCode(err, errs.CodeNotFound) {
		t.Fatalf("resource linker must not bind another user's attachment: %v", err)
	}
	if _, err = attachSvc.Delete(ownerCtx, json.RawMessage(`{"id":"owned"}`)); !isCode(err, errs.CodeValidation) {
		t.Fatalf("linked source attachment must not be deletable: %v", err)
	}
	for _, attachmentID := range []string{"other", "cross"} {
		if _, err = service.CreateDocument(ownerCtx, ai.KnowledgeDocumentCreateRequest{KnowledgeBaseID: privateKB.ID, AttachmentID: attachmentID}); !isCode(err, errs.CodeNotFound) {
			t.Fatalf("attachment boundary failed for %s: %v", attachmentID, err)
		}
	}
	if _, err = service.CreateDocument(mutationCtx, ai.KnowledgeDocumentCreateRequest{KnowledgeBaseID: privateKB.ID, AttachmentID: "other"}); !isCode(err, errs.CodeNotFound) {
		t.Fatalf("mutation permission must not manage another owner's private knowledge base: %v", err)
	}
	updatedDocumentValue, err := service.UpdateDocument(ownerCtx, ai.KnowledgeDocumentUpdateRequest{
		ID: document.ID, AttachmentID: "owned-next", Revision: document.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedDocument := updatedDocumentValue.(*ai.KnowledgeDocument)
	links = nil
	if err = db.Where("resource_type=? AND resource_id=?", ai.KnowledgeDocumentResourceType, document.ID).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].AttachmentID != "owned-next" {
		t.Fatalf("knowledge source link was not replaced atomically: %#v", links)
	}
	if _, err = service.DeleteDocument(ownerCtx, ai.KnowledgeDocumentDeleteRequest{ID: document.ID, Revision: updatedDocument.Revision}); err != nil {
		t.Fatal(err)
	}
	var activeDocumentCount, retainedDocumentCount, sourceLinkCount int64
	_ = db.Model(&ai.KnowledgeDocument{}).Where("id=?", document.ID).Count(&activeDocumentCount).Error
	_ = db.Unscoped().Model(&ai.KnowledgeDocument{}).Where("id=?", document.ID).Count(&retainedDocumentCount).Error
	_ = db.Model(&systemmodel.AttachmentLink{}).Where("resource_type=? AND resource_id=?", ai.KnowledgeDocumentResourceType, document.ID).Count(&sourceLinkCount).Error
	if activeDocumentCount != 0 || retainedDocumentCount != 1 || sourceLinkCount != 0 {
		t.Fatalf("unexpected soft-delete lifecycle: active=%d retained=%d links=%d", activeDocumentCount, retainedDocumentCount, sourceLinkCount)
	}
	if _, err = service.CreateDocument(ownerCtx, ai.KnowledgeDocumentCreateRequest{KnowledgeBaseID: privateKB.ID, AttachmentID: "owned-next"}); err != nil {
		t.Fatalf("partial unique index should allow reusing a soft-deleted document source: %v", err)
	}

	otherOwner := "other"
	agent := ai.Agent{
		ID: "agent-other", TenantID: "tenant-a", OwnerUserID: &otherOwner, Code: "agent-other",
		Name: "Other agent", Source: "personal", Visibility: "private", Status: "draft", Revision: 1,
	}
	if err = db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.BindAgentKnowledge(mutationCtx, ai.AgentKnowledgeBindRequest{AgentID: agent.ID, KnowledgeBaseID: tenantKB.ID}); err != nil {
		t.Fatalf("visible tenant knowledge base should be bindable: %v", err)
	}
	updatedValue, err := service.UpdateKnowledgeBase(ownerCtx, ai.KnowledgeBaseUpdateRequest{
		ID: tenantKB.ID, Name: tenantKB.Name, Visibility: aiknowledge.VisibilityPrivate,
		EmbeddingAlias: tenantKB.EmbeddingAlias, ChunkingConfig: tenantKB.ChunkingConfig,
		RetrievalConfig: tenantKB.RetrievalConfig, Revision: tenantKB.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedValue.(*ai.KnowledgeBase).Visibility != aiknowledge.VisibilityPrivate {
		t.Fatal("knowledge base visibility did not update")
	}
	bindingsValue, err := service.ListAgentKnowledge(mutationCtx, ai.AgentKnowledgeListRequest{
		Request: pagination.Request{Page: 1, PageSize: 20}, AgentID: agent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := bindingsValue.(pagination.Result[ai.AgentKnowledgeBinding])
	if len(bindings.Items) != 0 || bindings.Total != 0 {
		t.Fatal("agent binding must not expand knowledge access")
	}
	if _, err = service.UnbindAgentKnowledge(mutationCtx, ai.AgentKnowledgeUnbindRequest{AgentID: agent.ID, KnowledgeBaseID: tenantKB.ID}); err != nil {
		t.Fatalf("unbind must not require the knowledge base to remain visible: %v", err)
	}
}

func isCode(err error, code int) bool {
	var appErr *errs.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}
