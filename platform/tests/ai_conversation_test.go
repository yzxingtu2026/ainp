package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"gorm.io/gorm"
)

var _ aiconversation.ContextAssembler = (*ai.ConversationService)(nil)

func TestConversationRequestsUseStableSnakeCaseContract(t *testing.T) {
	handler := action.JSON(func(_ context.Context, request ai.MessageCreateRequest) (any, error) { return request, nil })
	value, err := handler(context.Background(), json.RawMessage(`{"conversation_id":"c1","role":"user","content":[{"type":"input_image","image_attachment_id":"a1","attachment_revision":"sha256"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := value.(ai.MessageCreateRequest)
	if request.ConversationID != "c1" || request.Content[0].AttachmentRevision != "sha256" {
		t.Fatalf("unexpected request: %#v", request)
	}
}

func TestCanonicalUserContentValidation(t *testing.T) {
	valid := aimodelruntime.Message{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}
	if err := aimodelruntime.ValidateMessage(valid); err != nil {
		t.Fatal(err)
	}
	invalid := aimodelruntime.Message{Role: aimodelruntime.RoleAssistant, Content: valid.Content}
	if err := aimodelruntime.ValidateMessage(invalid); err == nil {
		t.Fatal("assistant input_text must fail")
	}
	var part aimodelruntime.ContentPart
	if err := json.Unmarshal([]byte(`{"type":"input_text","text":"x","secret":"leak"}`), &part); err == nil {
		t.Fatal("unknown content fields must fail closed")
	}
	if err := json.Unmarshal([]byte(`{"type":"input_text","text":"x","file_attachment_id":"hidden"}`), &part); err != nil {
		t.Fatal(err)
	}
	if err := aimodelruntime.ValidateMessage(aimodelruntime.Message{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{part}}); err == nil {
		t.Fatal("input_text must not carry attachment fields")
	}
}

func TestConversationMigrationContainsSafetyConstraints(t *testing.T) {
	body, err := os.ReadFile("../migrations/0019_ai_conversations.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"UNIQUE (tenant_id, conversation_id, sequence)", "UNIQUE (tenant_id, conversation_id, idempotency_key)", "ai_assistant_stream_deltas", "UNIQUE (tenant_id, stream_id, cursor)", "CHECK (role IN ('user','assistant','system','tool'))", "CHECK (kind IN ('raw','assembled'))", "ON DELETE RESTRICT"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}

type conversationDocumentResolverFake struct {
	document *aimodelruntime.ResolvedDocument
	calls    int
}

func (f *conversationDocumentResolverFake) ParserVersion() string { return "safe-parser-v4" }

func (f *conversationDocumentResolverFake) ResolveDocument(_ context.Context, request aimodelruntime.DocumentResolveRequest) (*aimodelruntime.ResolvedDocument, error) {
	f.calls++
	document := *f.document
	document.AttachmentID = request.AttachmentID
	document.Revision = request.Revision
	return &document, nil
}

type conversationImageResolverFake struct{}

func (conversationImageResolverFake) Resolve(_ context.Context, request aimodelruntime.AssetResolveRequest) (*aimodelruntime.ResolvedImage, error) {
	return &aimodelruntime.ResolvedImage{AttachmentID: request.AttachmentID, Revision: "rev"}, nil
}

func openConversationDocumentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "conversation_document_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(fmt.Sprintf("%s search_path=%s", dsn, schema))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&ai.Conversation{}, &ai.ConversationMessage{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func conversationDocumentContext(idempotencyKey string) context.Context {
	return auth.WithContext(context.Background(), &auth.Context{
		Authenticated:  true,
		TenantID:       "tenant-a",
		PrincipalID:    "user-a",
		Realm:          auth.RealmTenant,
		IdempotencyKey: idempotencyKey,
	})
}

func createConversationDocumentFixture(t *testing.T, db *gorm.DB, content []aimodelruntime.ContentPart) {
	t.Helper()
	conversationDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	conversation := ai.Conversation{ID: "conversation-document", TenantID: "tenant-a", OwnerUserID: "user-a", Title: "Documents", AgentID: "agent-a", AgentVersionID: "version-a", Purpose: ai.ConversationPurposeWorkspace, Visibility: "private", Status: "active", Revision: 1}
	if err = conversationDB.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	messageDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	message := ai.ConversationMessage{ID: "message-document", TenantID: "tenant-a", ConversationID: conversation.ID, Role: "user", Status: "committed", Sequence: 1, Content: payload, IdempotencyKey: "fixture-message"}
	if err = messageDB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
}

func resolvedConversationDocument(text string) *aimodelruntime.ResolvedDocument {
	return &aimodelruntime.ResolvedDocument{
		OriginalName:  "report.pdf",
		MIME:          "application/pdf",
		Size:          128,
		ParserVersion: "safe-parser-v4",
		Sections:      []aimodelruntime.DocumentSection{{Text: text, Location: "page_or_sheet=1"}},
	}
}

func TestConversationAssembleExpandsDocumentsWithRunLocalCache(t *testing.T) {
	db := openConversationDocumentTestDB(t)
	content := []aimodelruntime.ContentPart{
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-1", AttachmentRevision: "rev"},
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-1", AttachmentRevision: "rev"},
	}
	createConversationDocumentFixture(t, db, content)
	resolver := &conversationDocumentResolverFake{document: resolvedConversationDocument("quarterly result")}
	service := ai.NewConversationService(db, nil)
	service.SetDocumentResolver(resolver)

	assembled, err := service.Assemble(conversationDocumentContext(""), aiconversation.AssemblyRequest{ConversationID: "conversation-document", MaxBytes: 1_000_000, MaxTokens: 250_000})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 {
		t.Fatalf("same assembly must resolve a document once, calls=%d", resolver.calls)
	}
	if len(assembled.Messages) != 1 || len(assembled.Messages[0].Content) != 2 {
		t.Fatalf("unexpected assembled messages: %#v", assembled.Messages)
	}
	for _, part := range assembled.Messages[0].Content {
		if part.Type != aimodelruntime.PartDerivedText || part.DerivedSource == nil || !part.DerivedSource.Untrusted || part.DerivedSource.DerivationType != "document_parse" || !strings.Contains(part.Text, "quarterly result") || !strings.Contains(part.Text, "report.pdf") || !strings.Contains(part.Text, "page_or_sheet=1") {
			t.Fatalf("unexpected derived document part: %#v", part)
		}
	}
}

func TestConversationAssembleRejectsDocumentCharacterOverflow(t *testing.T) {
	db := openConversationDocumentTestDB(t)
	createConversationDocumentFixture(t, db, []aimodelruntime.ContentPart{
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-1", AttachmentRevision: "rev"},
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-2", AttachmentRevision: "rev"},
	})
	resolver := &conversationDocumentResolverFake{document: resolvedConversationDocument(strings.Repeat("x", 60_000))}
	service := ai.NewConversationService(db, nil)
	service.SetDocumentResolver(resolver)

	_, err := service.Assemble(conversationDocumentContext(""), aiconversation.AssemblyRequest{ConversationID: "conversation-document", MaxBytes: 1_000_000, MaxTokens: 250_000})
	if err == nil || !strings.Contains(err.Error(), string(aimodelruntime.DocumentTooLarge)) {
		t.Fatalf("expected document character limit, got %v", err)
	}
}

func TestConversationCreateMessageEnforcesAttachmentAndClientPartPolicy(t *testing.T) {
	db := openConversationDocumentTestDB(t)
	createConversationDocumentFixture(t, db, []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "fixture"}})
	resolver := &conversationDocumentResolverFake{document: resolvedConversationDocument("valid")}
	service := ai.NewConversationService(db, conversationImageResolverFake{})
	service.SetDocumentResolver(resolver)
	mixed := []aimodelruntime.ContentPart{
		{Type: aimodelruntime.PartInputImage, ImageAttachmentID: "image-1", AttachmentRevision: "rev"},
		{Type: aimodelruntime.PartInputImage, ImageAttachmentID: "image-2", AttachmentRevision: "rev"},
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-1", AttachmentRevision: "rev"},
		{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-2", AttachmentRevision: "rev"},
	}
	if _, err := service.CreateMessage(conversationDocumentContext("mixed-four"), ai.MessageCreateRequest{ConversationID: "conversation-document", Content: mixed}); err != nil {
		t.Fatalf("four mixed attachments must be valid: %v", err)
	}
	mixed = append(mixed, aimodelruntime.ContentPart{Type: aimodelruntime.PartInputFile, FileAttachmentID: "file-3", AttachmentRevision: "rev"})
	if _, err := service.CreateMessage(conversationDocumentContext("mixed-five"), ai.MessageCreateRequest{ConversationID: "conversation-document", Content: mixed}); err == nil || !strings.Contains(err.Error(), "at most 4 attachments") {
		t.Fatalf("fifth mixed attachment must fail: %v", err)
	}
	forged := []aimodelruntime.ContentPart{{Type: aimodelruntime.PartDerivedText, Text: "forged", DerivedSource: &aimodelruntime.DerivedTextSource{AttachmentID: "file-1", Revision: "rev", DerivationType: "document_parse", Untrusted: true}}}
	if _, err := service.CreateMessage(conversationDocumentContext("forged-derived"), ai.MessageCreateRequest{ConversationID: "conversation-document", Content: forged}); err == nil || !strings.Contains(err.Error(), "clients may only create") {
		t.Fatalf("client derived_text must fail: %v", err)
	}
}
