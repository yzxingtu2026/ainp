package ai

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
)

const (
	ActionKnowledgeBaseList   = "ai.knowledge_base_list"
	ActionKnowledgeBaseGet    = "ai.knowledge_base_get"
	ActionKnowledgeBaseCreate = "ai.knowledge_base_create"
	ActionKnowledgeBaseUpdate = "ai.knowledge_base_update"
	ActionKnowledgeBaseStatus = "ai.knowledge_base_status_update"
	ActionKnowledgeBaseDelete = "ai.knowledge_base_delete"

	ActionKnowledgeDocumentList    = "ai.knowledge_document_list"
	ActionKnowledgeDocumentGet     = "ai.knowledge_document_get"
	ActionKnowledgeDocumentCreate  = "ai.knowledge_document_create"
	ActionKnowledgeDocumentUpdate  = "ai.knowledge_document_update"
	ActionKnowledgeDocumentDelete  = "ai.knowledge_document_delete"
	ActionKnowledgeRetrievalTest   = "ai.knowledge_retrieval_test"
	ActionKnowledgeCitationResolve = "ai.knowledge_citation_resolve"

	ActionAgentKnowledgeList   = "ai.agent_knowledge_list"
	ActionAgentKnowledgeBind   = "ai.agent_knowledge_bind"
	ActionAgentKnowledgeUnbind = "ai.agent_knowledge_unbind"
)

const (
	CodeSetKnowledgeBaseStatus         = "ai.knowledge_base_status"
	CodeSetDocumentIngestionStatus     = "ai.knowledge_document_ingestion_status"
	CodeSetKnowledgeIndexVersionStatus = "ai.knowledge_index_version_status"
)

func RegisterKnowledge(a *app.App, service *KnowledgeService) error {
	if a.AttachmentPolicies == nil {
		return fmt.Errorf("attachment policy registry is required")
	}
	if err := a.AttachmentPolicies.Register(knowledgeAttachmentPolicy{service: service}); err != nil {
		return err
	}
	a.KnowledgeService = service
	a.KnowledgeAccessPolicy = service

	for _, set := range knowledgeCodeSets() {
		if err := a.RegisterCodeSet(set); err != nil {
			return err
		}
	}

	auditOn, auditOff := true, false
	type definition struct {
		id          string
		path        string
		name        string
		description string
		audit       *bool
		handler     action.Handler
	}
	definitions := []definition{
		{ActionKnowledgeBaseList, "/api/v1/ai/knowledge-bases/list", "Knowledge base list", "List visible knowledge bases", &auditOff, action.JSON(service.ListKnowledgeBases)},
		{ActionKnowledgeBaseGet, "/api/v1/ai/knowledge-bases/get", "Knowledge base details", "Get a visible knowledge base", &auditOff, action.JSON(service.GetKnowledgeBase)},
		{ActionKnowledgeBaseCreate, "/api/v1/ai/knowledge-bases/create", "Create knowledge base", "Create a draft knowledge base", &auditOn, action.JSON(service.CreateKnowledgeBase)},
		{ActionKnowledgeBaseUpdate, "/api/v1/ai/knowledge-bases/update", "Update knowledge base", "Update knowledge base metadata and typed configuration", &auditOn, action.JSON(service.UpdateKnowledgeBase)},
		{ActionKnowledgeBaseStatus, "/api/v1/ai/knowledge-bases/status/update", "Update knowledge base status", "Update knowledge base lifecycle status", &auditOn, action.JSON(service.UpdateKnowledgeBaseStatus)},
		{ActionKnowledgeBaseDelete, "/api/v1/ai/knowledge-bases/delete", "Delete knowledge base", "Delete an unreferenced knowledge base", &auditOn, action.JSON(service.DeleteKnowledgeBase)},
		{ActionKnowledgeDocumentList, "/api/v1/ai/knowledge-documents/list", "Knowledge document list", "List documents in a visible knowledge base", &auditOff, action.JSON(service.ListDocuments)},
		{ActionKnowledgeDocumentGet, "/api/v1/ai/knowledge-documents/get", "Knowledge document details", "Get visible knowledge document metadata", &auditOff, action.JSON(service.GetDocument)},
		{ActionKnowledgeDocumentCreate, "/api/v1/ai/knowledge-documents/create", "Create knowledge document", "Reference a ready attachment as a knowledge document", &auditOn, action.JSON(service.CreateDocument)},
		{ActionKnowledgeDocumentUpdate, "/api/v1/ai/knowledge-documents/update", "Update knowledge document", "Replace a knowledge document attachment snapshot", &auditOn, action.JSON(service.UpdateDocument)},
		{ActionKnowledgeDocumentDelete, "/api/v1/ai/knowledge-documents/delete", "Delete knowledge document", "Delete knowledge document metadata", &auditOn, action.JSON(service.DeleteDocument)},
		{ActionKnowledgeRetrievalTest, "/api/v1/ai/knowledge-retrieval/test", "Test knowledge retrieval", "Test retrieval with the persisted knowledge base policy", &auditOff, action.JSON(service.TestRetrieval)},
		{ActionKnowledgeCitationResolve, "/api/v1/ai/knowledge-citations/resolve", "Resolve knowledge citation", "Reauthorize and resolve citation availability", &auditOff, action.JSON(service.ResolveCitation)},
		{ActionAgentKnowledgeList, "/api/v1/ai/agent-knowledge/list", "Agent knowledge list", "List currently visible knowledge candidates bound to an agent", &auditOff, action.JSON(service.ListAgentKnowledge)},
		{ActionAgentKnowledgeBind, "/api/v1/ai/agent-knowledge/bind", "Bind agent knowledge", "Bind a visible knowledge base as an agent candidate", &auditOn, action.JSON(service.BindAgentKnowledge)},
		{ActionAgentKnowledgeUnbind, "/api/v1/ai/agent-knowledge/unbind", "Unbind agent knowledge", "Remove an agent knowledge candidate binding", &auditOn, action.JSON(service.UnbindAgentKnowledge)},
	}
	for _, definition := range definitions {
		if err := a.RegisterAction(action.Action{
			ID:      definition.id,
			Handler: definition.handler,
			Meta: action.Metadata{
				Capability:             "ai",
				Name:                   definition.name,
				NameKey:                "action." + definition.id + ".name",
				Description:            definition.description,
				DescriptionKey:         "action." + definition.id + ".description",
				PermissionGroup:        "ai.knowledge_management",
				PermissionGroupName:    "Knowledge management",
				PermissionGroupNameKey: "permission_group.ai.knowledge_management",
				Audit:                  definition.audit,
				AI:                     action.ExcludeFromAI(action.AIExcludePendingSafetyReview),
				HTTP: &action.HTTPMeta{
					Method: "POST",
					Path:   definition.path,
				},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func knowledgeCodeSets() []codeset.CodeSet {
	return []codeset.CodeSet{
		{
			Key: CodeSetKnowledgeBaseStatus, NameKey: "codeset.ai.knowledge_base_status", Order: 230,
			Items: []codeset.CodeItem{
				{Value: "draft", LabelKey: "codeset.ai.knowledge_base_status.draft", Semantic: codeset.SemanticMuted, Icon: "mdi:file-edit-outline", Order: 10},
				{Value: "enabled", LabelKey: "codeset.ai.knowledge_base_status.enabled", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 20},
				{Value: "disabled", LabelKey: "codeset.ai.knowledge_base_status.disabled", Semantic: codeset.SemanticWarning, Icon: "mdi:cancel", Order: 30},
			},
		},
		{
			Key: CodeSetDocumentIngestionStatus, NameKey: "codeset.ai.knowledge_document_ingestion_status", Order: 231,
			Items: []codeset.CodeItem{
				{Value: "pending", LabelKey: "codeset.ai.knowledge_document_ingestion_status.pending", Semantic: codeset.SemanticMuted, Icon: "mdi:clock-outline", Order: 10},
				{Value: "processing", LabelKey: "codeset.ai.knowledge_document_ingestion_status.processing", Semantic: codeset.SemanticInfo, Icon: "mdi:progress-clock", Order: 20},
				{Value: "ready", LabelKey: "codeset.ai.knowledge_document_ingestion_status.ready", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 30},
				{Value: "failed", LabelKey: "codeset.ai.knowledge_document_ingestion_status.failed", Semantic: codeset.SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 40},
			},
		},
		{
			Key: CodeSetKnowledgeIndexVersionStatus, NameKey: "codeset.ai.knowledge_index_version_status", Order: 232,
			Items: []codeset.CodeItem{
				{Value: "pending", LabelKey: "codeset.ai.knowledge_index_version_status.pending", Semantic: codeset.SemanticMuted, Icon: "mdi:clock-outline", Order: 10},
				{Value: "building", LabelKey: "codeset.ai.knowledge_index_version_status.building", Semantic: codeset.SemanticInfo, Icon: "mdi:progress-clock", Order: 20},
				{Value: "ready", LabelKey: "codeset.ai.knowledge_index_version_status.ready", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 30},
				{Value: "failed", LabelKey: "codeset.ai.knowledge_index_version_status.failed", Semantic: codeset.SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 40},
				{Value: "superseded", LabelKey: "codeset.ai.knowledge_index_version_status.superseded", Semantic: codeset.SemanticMuted, Icon: "mdi:archive-outline", Order: 50},
				{Value: "aborted", LabelKey: "codeset.ai.knowledge_index_version_status.aborted", Semantic: codeset.SemanticWarning, Icon: "mdi:cancel", Order: 60},
			},
		},
	}
}
