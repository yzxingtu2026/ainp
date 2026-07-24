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
		ai          *action.AIMeta
	}
	definitions := []definition{
		{ActionKnowledgeBaseList, "/api/v1/ai/knowledge-bases/list", "知识库列表", "查询可见知识库", &auditOff, action.JSON(service.ListKnowledgeBases), readAIMeta("查询当前用户可见的知识库", "查询知识库列表", "查询当前用户可见的知识库", "已返回可见知识库", action.InputSchemaFor[KnowledgeBaseListRequest](), nil, "AI", "知识库", "列表")},
		{ActionKnowledgeBaseGet, "/api/v1/ai/knowledge-bases/get", "知识库详情", "查询可见知识库详情", &auditOff, action.JSON(service.GetKnowledgeBase), readAIMeta("按 ID 查询可见知识库详情", "查询知识库详情", "按 ID 查询可见知识库详情", "已返回知识库详情", action.InputSchemaFor[KnowledgeBaseIDRequest](), nil, "AI", "知识库", "详情")},
		{ActionKnowledgeBaseCreate, "/api/v1/ai/knowledge-bases/create", "创建知识库", "创建草稿知识库", &auditOn, action.JSON(service.CreateKnowledgeBase), mutationAIMeta("创建新的草稿知识库", "创建知识库", "创建新的草稿知识库", "知识库已创建", "high", "write", action.InputSchemaFor[KnowledgeBaseCreateRequest](), nil, "AI", "知识库", "创建")},
		{ActionKnowledgeBaseUpdate, "/api/v1/ai/knowledge-bases/update", "更新知识库", "更新知识库配置", &auditOn, action.JSON(service.UpdateKnowledgeBase), mutationAIMeta("更新知识库元数据和检索配置", "更新知识库", "更新知识库元数据和检索配置", "知识库已更新", "high", "write", action.InputSchemaFor[KnowledgeBaseUpdateRequest](), nil, "AI", "知识库", "更新")},
		{ActionKnowledgeBaseStatus, "/api/v1/ai/knowledge-bases/status/update", "更新知识库状态", "更新知识库生命周期状态", &auditOn, action.JSON(service.UpdateKnowledgeBaseStatus), mutationAIMeta("更新知识库的启用状态", "更新知识库状态", "更新知识库的启用状态", "知识库状态已更新", "high", "write", action.InputSchemaFor[KnowledgeBaseStatusRequest](), nil, "AI", "知识库", "状态")},
		{ActionKnowledgeBaseDelete, "/api/v1/ai/knowledge-bases/delete", "删除知识库", "删除未被引用的知识库", &auditOn, action.JSON(service.DeleteKnowledgeBase), mutationAIMeta("删除未被文档或智能体引用的知识库", "删除知识库", "删除未被文档或智能体引用的知识库", "知识库已删除", "high", "destructive", action.InputSchemaFor[KnowledgeBaseDeleteRequest](), nil, "AI", "知识库", "删除")},
		{ActionKnowledgeDocumentList, "/api/v1/ai/knowledge-documents/list", "知识文档列表", "查询知识库文档", &auditOff, action.JSON(service.ListDocuments), readAIMeta("查询指定知识库中的文档", "查询知识文档列表", "查询指定知识库中的文档", "已返回知识文档", action.InputSchemaFor[KnowledgeDocumentListRequest](), nil, "AI", "知识文档", "列表")},
		{ActionKnowledgeDocumentGet, "/api/v1/ai/knowledge-documents/get", "知识文档详情", "查询知识文档元数据", &auditOff, action.JSON(service.GetDocument), readAIMeta("按 ID 查询知识文档元数据", "查询知识文档详情", "按 ID 查询知识文档元数据", "已返回知识文档详情", action.InputSchemaFor[KnowledgeDocumentIDRequest](), nil, "AI", "知识文档", "详情")},
		{ActionKnowledgeDocumentCreate, "/api/v1/ai/knowledge-documents/create", "创建知识文档", "引用已就绪附件创建知识文档", &auditOn, action.JSON(service.CreateDocument), mutationAIMeta("将已就绪附件添加为知识文档", "创建知识文档", "将已就绪附件添加为知识文档", "知识文档已创建并等待索引", "high", "write", action.InputSchemaFor[KnowledgeDocumentCreateRequest](), nil, "AI", "知识文档", "创建")},
		{ActionKnowledgeDocumentUpdate, "/api/v1/ai/knowledge-documents/update", "更新知识文档", "替换知识文档附件快照", &auditOn, action.JSON(service.UpdateDocument), mutationAIMeta("使用已就绪附件替换知识文档内容", "更新知识文档", "使用已就绪附件替换知识文档内容", "知识文档已更新并等待索引", "high", "write", action.InputSchemaFor[KnowledgeDocumentUpdateRequest](), nil, "AI", "知识文档", "更新")},
		{ActionKnowledgeDocumentDelete, "/api/v1/ai/knowledge-documents/delete", "删除知识文档", "删除知识文档元数据", &auditOn, action.JSON(service.DeleteDocument), mutationAIMeta("删除指定知识文档及其索引数据", "删除知识文档", "删除指定知识文档及其索引数据", "知识文档已删除", "high", "destructive", action.InputSchemaFor[KnowledgeDocumentDeleteRequest](), nil, "AI", "知识文档", "删除")},
		{ActionKnowledgeRetrievalTest, "/api/v1/ai/knowledge-retrieval/test", "测试知识检索", "使用已保存策略测试检索", &auditOff, action.JSON(service.TestRetrieval), readAIMeta("测试指定知识库的检索结果，不修改知识数据", "测试知识检索", "测试指定知识库的检索结果，不修改知识数据", "已返回知识检索测试结果", action.InputSchemaFor[KnowledgeRetrievalTestRequest](), nil, "AI", "知识库", "检索测试")},
		{ActionKnowledgeCitationResolve, "/api/v1/ai/knowledge-citations/resolve", "解析知识引用", "重新授权并解析引用可用性", &auditOff, action.JSON(service.ResolveCitation), readAIMeta("校验并解析知识引用是否仍可访问", "解析知识引用", "校验并解析知识引用是否仍可访问", "已返回知识引用解析结果", action.InputSchemaFor[KnowledgeCitationResolveRequest](), nil, "AI", "知识", "引用")},
		{ActionAgentKnowledgeList, "/api/v1/ai/agent-knowledge/list", "智能体知识列表", "查询智能体绑定的知识候选", &auditOff, action.JSON(service.ListAgentKnowledge), readAIMeta("查询指定智能体已绑定和可见的知识库", "查询智能体知识绑定", "查询指定智能体已绑定和可见的知识库", "已返回智能体知识绑定", action.InputSchemaFor[AgentKnowledgeListRequest](), nil, "AI", "智能体", "知识绑定")},
		{ActionAgentKnowledgeBind, "/api/v1/ai/agent-knowledge/bind", "绑定智能体知识", "为智能体绑定可见知识库", &auditOn, action.JSON(service.BindAgentKnowledge), mutationAIMeta("将可见知识库绑定为智能体知识候选", "绑定智能体知识库", "将可见知识库绑定为智能体知识候选", "知识库已绑定到智能体", "high", "write", action.InputSchemaFor[AgentKnowledgeBindRequest](), nil, "AI", "智能体", "知识绑定")},
		{ActionAgentKnowledgeUnbind, "/api/v1/ai/agent-knowledge/unbind", "解绑智能体知识", "移除智能体知识库绑定", &auditOn, action.JSON(service.UnbindAgentKnowledge), mutationAIMeta("移除智能体与知识库的绑定", "解绑智能体知识库", "移除智能体与知识库的绑定", "智能体知识库已解绑", "high", "write", action.InputSchemaFor[AgentKnowledgeUnbindRequest](), nil, "AI", "智能体", "知识解绑")},
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
				AI:                     definition.ai,
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
