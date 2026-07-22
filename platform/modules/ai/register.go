package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	aihelp "github.com/yzxingtu2026/ainp/platform/modules/ai/help"
	aitools "github.com/yzxingtu2026/ainp/platform/modules/ai/tools"
	knowledgetool "github.com/yzxingtu2026/ainp/platform/modules/ai/tools/knowledge"
	memorytool "github.com/yzxingtu2026/ainp/platform/modules/ai/tools/memory"
	navigationtool "github.com/yzxingtu2026/ainp/platform/modules/ai/tools/navigation"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"gorm.io/gorm"
)

const (
	ActionProviderList                 = "ai.provider_list"
	ActionProviderGet                  = "ai.provider_get"
	ActionProviderCreate               = "ai.provider_create"
	ActionProviderUpdate               = "ai.provider_update"
	ActionProviderStatus               = "ai.provider_status_update"
	ActionProviderDiscover             = "ai.provider_discover_models"
	ActionDeploymentList               = "ai.deployment_list"
	ActionDeploymentGet                = "ai.deployment_get"
	ActionDeploymentCreate             = "ai.deployment_create"
	ActionDeploymentImport             = "ai.deployment_import"
	ActionDeploymentUpdate             = "ai.deployment_update"
	ActionDeploymentStatus             = "ai.deployment_status_update"
	ActionDeploymentDelete             = "ai.deployment_delete"
	ActionAliasList                    = "ai.alias_list"
	ActionAliasUpsert                  = "ai.alias_upsert"
	ActionAliasDelete                  = "ai.alias_delete"
	ActionProviderTypes                = "ai.provider_type_list"
	ActionToolSearch                   = "ai.tool_search"
	ActionToolLoad                     = "ai.tool_load"
	ActionToolExecute                  = "ai.tool_execute"
	ActionToolConfirmationList         = "ai.tool_confirmation_list"
	ActionToolConfirmationApprove      = "ai.tool_confirmation_approve"
	ActionToolConfirmationReject       = "ai.tool_confirmation_reject"
	ActionToolCatalogRebuild           = "ai.tool_catalog_rebuild"
	ActionToolIndexStatus              = "ai.tool_index_status"
	ActionToolIndexRebuild             = "ai.tool_index_rebuild"
	ActionNavigationIndexStatus        = "ai.navigation_index_status"
	ActionNavigationIndexRebuild       = "ai.navigation_index_rebuild"
	ActionAgentList                    = "ai.agent_list"
	ActionAgentGet                     = "ai.agent_get"
	ActionAgentCreate                  = "ai.agent_create"
	ActionAgentUpdate                  = "ai.agent_update"
	ActionAgentDuplicate               = "ai.agent_duplicate"
	ActionAgentStatus                  = "ai.agent_status_update"
	ActionAgentVersionList             = "ai.agent_version_list"
	ActionAgentVersionGet              = "ai.agent_version_get"
	ActionAgentVersionCreate           = "ai.agent_version_create"
	ActionAgentVersionPublish          = "ai.agent_version_publish"
	ActionAgentPreviewMaterialize      = "ai.agent_preview_materialize"
	ActionAgentToolSearch              = "ai.agent_tool_search"
	ActionAgentToolList                = "ai.agent_tool_list"
	ActionAgentToolBind                = "ai.agent_tool_bind"
	ActionAgentToolUnbind              = "ai.agent_tool_unbind"
	ActionAgentDiagnostics             = "ai.agent_diagnostics_get"
	ActionAgentBuiltinReconcile        = "ai.agent_builtin_reconcile"
	ActionAgentSlotList                = "ai.agent_slot_list"
	ActionAgentSlotGet                 = "ai.agent_slot_get"
	ActionAgentSlotUpdate              = "ai.agent_slot_update"
	ActionAgentSlotReset               = "ai.agent_slot_reset"
	ActionAgentSlotReconcile           = "ai.agent_slot_reconcile"
	ActionAgentSlotPreferenceGet       = "ai.agent_slot_preference_get"
	ActionAgentSlotPreferenceUpdate    = "ai.agent_slot_preference_update"
	ActionAgentSlotPreferenceReset     = "ai.agent_slot_preference_reset"
	ActionConversationList             = "ai.conversation_list"
	ActionConversationGet              = "ai.conversation_get"
	ActionConversationCreate           = "ai.conversation_create"
	ActionConversationCreateForSlot    = "ai.conversation_create_for_slot"
	ActionConversationUpdate           = "ai.conversation_update"
	ActionConversationArchive          = "ai.conversation_archive"
	ActionConversationRestore          = "ai.conversation_restore"
	ActionMessageList                  = "ai.message_list"
	ActionMessageGet                   = "ai.message_get"
	ActionMessageCreate                = "ai.message_create"
	ActionContextCompactionDiagnostics = "ai.context_compaction_diagnostics"
	ActionAgentRunStart                = "ai.agent_run_start"
	ActionAgentRunGet                  = "ai.agent_run_get"
	ActionAgentRunList                 = "ai.agent_run_list"
	ActionAgentRunCancel               = "ai.agent_run_cancel"
	ActionAgentRunResume               = "ai.agent_run_resume"
	ActionAgentRunRetry                = "ai.agent_run_retry"
	ActionAgentRunEvents               = "ai.agent_run_events"
	ActionAgentRunSubscribe            = "ai.agent_run_subscribe"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{ID: "ai", Name: "AI 中心", NameKey: "capability.ai.name", Description: "模型资产与 AI 基础能力", DescriptionKey: "capability.ai.description", Module: "ai", Order: 20}); err != nil {
		return err
	}
	registry := aimodel.NewProviderTypeRegistry()
	if err := aimodel.RegisterBuiltinProviderTypes(registry); err != nil {
		return err
	}
	a.AIProviderTypes = registry
	credentialCipher, err := aimodel.NewAESGCMCredentialCipher(a.Config.AI.CredentialEncryptionKey)
	if err != nil {
		return err
	}
	svc := NewService(a.DB, registry, credentialCipher)
	a.ModelResolver = svc
	adapters := aimodelruntime.NewAdapterRegistry()
	if err := adapters.Register(aimodel.ProtocolOpenAI, aimodelruntime.NewOpenAIAdapter(nil)); err != nil {
		return err
	}
	if err := adapters.Register(aimodel.ProtocolAnthropic, aimodelruntime.NewAnthropicAdapter(nil)); err != nil {
		return err
	}
	if err := adapters.Register(aimodel.ProtocolOpenAICompatible, aimodelruntime.NewOpenAICompatibleAdapter(nil)); err != nil {
		return err
	}
	embeddingAdapters := aimodelruntime.NewEmbeddingAdapterRegistry()
	if err := embeddingAdapters.Register(aimodel.ProtocolOpenAI, aimodelruntime.NewOpenAIEmbeddingAdapter(nil)); err != nil {
		return err
	}
	if err := embeddingAdapters.Register(aimodel.ProtocolOpenAICompatible, aimodelruntime.NewOpenAICompatibleEmbeddingAdapter(nil)); err != nil {
		return err
	}
	var writer outbox.Writer
	if a.Outbox != nil {
		if err := a.Outbox.RegisterDefinition(outbox.Definition{EventType: invocationFinishedEvent, SchemaVersion: 1}); err != nil {
			return err
		}
		writer = a.Outbox.Writer
		for _, eventType := range []string{toolCatalogChangedEvent, toolInvocationFinishedEvent, navigationDocumentChangedEvent} {
			if err := a.Outbox.RegisterDefinition(outbox.Definition{EventType: eventType, SchemaVersion: 1}); err != nil {
				return err
			}
		}
	}
	invocations := NewInvocationService(a.DB, svc, adapters, writer)
	invocations.SetEmbeddingAdapters(embeddingAdapters)
	invocations.SetAssetResolver(a.AIAssetResolver)
	personalization := NewPersonalizationService(a.DB, a.SettingValues, writer, invocations)
	if err := RegisterPersonalization(a, personalization); err != nil {
		return err
	}
	if err := a.SettingChanges.Register(AssistantPersonalizationSettingKey, func(ctx context.Context, _ setting.Change) error {
		_, err := personalization.Rebuild(ctx, struct{}{})
		return err
	}); err != nil {
		return err
	}
	a.EmbeddingRuntime = invocations
	if err := invocations.RecoverStale(context.Background(), 10*time.Minute); err != nil {
		return err
	}
	toolRegistry := a.ToolRegistry
	if toolRegistry == nil {
		return fmt.Errorf("tool registry is required")
	}
	handleSigner, err := aitool.NewHandleSigner(a.Config.Auth.JWT.Secret)
	if err != nil {
		return err
	}
	tools := NewToolService(a.DB, toolRegistry, handleSigner, writer)
	tools.SetEmbeddingRuntime(a.EmbeddingRuntime)
	tools.SetSettingProvider(a.SettingValues)
	tools.SetCatalogProjectionSynchronous(a.Outbox == nil || !a.Outbox.Enabled)
	if err := RegisterToolCatalogEmbedding(a, tools); err != nil {
		return err
	}
	navigationSearch := NewNavigationSearchService(a.DB, a.Menus, a.PageHelp, a.SettingValues, a.EmbeddingRuntime, writer)
	a.Menus.SetSemanticSearcher(navigationSearch)
	if err := RegisterNavigationEmbedding(a, navigationSearch); err != nil {
		return err
	}
	invocations.SetToolProjector(tools)
	contracts, err := NewBuiltinAgentContractRegistry()
	if err != nil {
		return err
	}
	a.AgentContracts = contracts
	agents := NewAgentService(a.DB, contracts, svc, toolRegistry)
	slotRegistry := agentslot.NewRegistry(contracts)
	if err := slotRegistry.Register(agentslot.Slot{Key: WorkspaceDefaultSlot, Version: "1.0.0", Module: "workspace", Title: "默认 AI 工作台", Description: "AI 工作台新会话使用的默认智能体", DefaultContractKey: "system.operations", DefaultContractVersion: "1.0.0", RequiredCapabilities: []string{"chat", "tools"}, TenantOverride: true, UserOverride: true, FallbackPolicy: agentslot.FallbackFailClosed, UpgradePolicy: agentslot.UpgradeCompatible}); err != nil {
		return err
	}
	slots := NewAgentSlotService(a.DB, slotRegistry, agents)
	personalization.SetContextSources(slots, a.RuntimeContextProvider)
	slots.SetChangedHook(func(ctx context.Context, slotKey string) error {
		if slotKey != WorkspaceDefaultSlot {
			return nil
		}
		return personalization.InvalidateAndRequest(ctx, auth.MustFromContext(ctx).UserID, uint64(time.Now().UTC().Unix()))
	})
	agents.SetMemoryPolicyProvider(a.SettingValues)
	knowledge := NewKnowledgeService(a.DB, svc, agents, a.AttachmentResourceLinker)
	parserRegistry, err := aiknowledge.NewDefaultParserRegistry(aiknowledge.DefaultParseLimits())
	if err != nil {
		return err
	}
	a.KnowledgeSourceResolver = system.NewKnowledgeSourceResolver(a.DB, a.Storage, knowledge)
	a.KnowledgeParser = parserRegistry
	a.KnowledgeChunker = aiknowledge.NewFixedTokenChunker()
	ingestion := NewKnowledgeIngestionService(a.DB, a.KnowledgeSourceResolver, a.KnowledgeParser, a.KnowledgeChunker, svc, a.EmbeddingRuntime)
	retriever := NewKnowledgeRetrievalService(a.DB, knowledge, a.EmbeddingRuntime)
	knowledgeTool := NewKnowledgeTool(a.DB, knowledge, retriever)
	knowledge.SetRetrievalServices(retriever, retriever)
	a.KnowledgeRetriever = retriever
	conversations := NewConversationService(a.DB, a.AIAssetResolver)
	compactions := NewContextCompactionService(a.DB, conversations, svc, invocations, toolRegistry)
	memoryExtraction := NewMemoryExtractionService(a.DB, invocations)
	memoryGovernance := NewMemoryGovernanceService(a.DB, writer)
	memoryManagement := NewMemoryManagementService(a.DB, memoryGovernance, writer)
	memoryProjection := NewMemoryProjectionService(a.DB, a.EmbeddingRuntime)
	memoryProjection.SetCompletedHook(func(ctx context.Context, userID string, revision uint64) error {
		return personalization.InvalidateAndRequest(ctx, userID, revision)
	})
	memoryRetriever := NewMemoryRetrievalService(a.DB, a.EmbeddingRuntime)
	memoryRuntimeTool := NewMemoryTool(memoryRetriever)
	memoryConsolidation := NewMemoryConsolidationService(a.DB, invocations)
	compactions.SetMemoryFlushHook(memoryExtraction)
	a.ContextCompactor = compactions
	agentRuntime := NewAgentRuntimeService(a.DB, conversations, invocations, tools, compactions, writer, a.Authorization, a.PageContexts, a.RuntimeContextProvider)
	agentRuntime.SetBudgetProvider(NewRuntimeBudgetProvider(a.SettingValues))
	agentRuntime.SetAgentContracts(contracts)
	agentRuntime.SetMemoryRetriever(memoryRetriever)
	tools.SetConfirmationHooks(agentRuntime.OnConfirmationPending, agentRuntime.OnConfirmationDecided)
	if err := RegisterAgentRuntime(a, agentRuntime); err != nil {
		return err
	}
	if err := RegisterMemoryGovernance(a, memoryProjection, memoryConsolidation); err != nil {
		return err
	}
	if err := RegisterMemoryManagement(a, memoryManagement); err != nil {
		return err
	}
	if err := RegisterMemoryExtraction(a, memoryExtraction, memoryGovernance); err != nil {
		return err
	}
	a.ToolCatalogInitializer = func(ctx context.Context, tenantID, actorID string) error {
		_, err := tools.ReconcileCatalogForTenant(ctx, tenantID, actorID)
		return err
	}
	a.NavigationInitializer = func(ctx context.Context, tenantID, actorID string) error {
		if err := navigationSearch.ReconcileForTenant(ctx, tenantID); err != nil {
			return err
		}
		if a.Outbox == nil || a.Outbox.Enabled {
			return nil
		}
		// Indexing is best-effort when background runtimes are disabled; lexical
		// navigation must remain available even if the embedding provider fails.
		_ = navigationSearch.IndexPending(auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: tenantID, UserID: actorID, Realm: auth.RealmTenant, Client: "navigation-reconciler"}), tenantID)
		return nil
	}
	if err := a.SettingChanges.Register(menu.SettingKeySemanticSearch, func(ctx context.Context, change setting.Change) error {
		return a.NavigationInitializer(ctx, change.TenantID, change.UserID)
	}); err != nil {
		return err
	}
	if err := a.SettingChanges.Register(aitool.SettingKeySearch, func(ctx context.Context, change setting.Change) error {
		_, err := tools.ReconcileCatalogForTenant(ctx, change.TenantID, change.UserID)
		return err
	}); err != nil {
		return err
	}
	a.AgentTenantInitializer = func(ctx context.Context, db *gorm.DB, tenantID, userID string) error {
		if err := agents.ReconcileBuiltinForTenant(ctx, db, tenantID, userID); err != nil {
			return err
		}
		_, err := tools.ReconcileCatalogForTenant(ctx, tenantID, userID)
		if err != nil {
			return err
		}
		return a.NavigationInitializer(ctx, tenantID, userID)
	}
	auditOn := true
	type def struct {
		id, path, name, group, groupName string
		handler                          action.Handler
		ai                               *action.AIMeta
	}
	defs := []def{
		{ActionProviderTypes, "/api/v1/ai/provider-types/list", "供应商协议列表", "ai.provider_management", "模型供应商", action.JSON(svc.ProviderTypes), readAIMeta("查询系统支持的模型供应商协议类型", "查询供应商协议类型", "已返回供应商协议类型", action.InputSchemaFor[struct{}](), nil, "AI", "模型供应商", "协议")},
		{ActionProviderList, "/api/v1/ai/providers/list", "供应商列表", "ai.provider_management", "模型供应商", action.JSON(svc.ListProviders), readAIMeta("查询已脱敏的模型供应商列表", "查询模型供应商", "已返回模型供应商", action.InputSchemaFor[ListRequest](), nil, "AI", "模型供应商", "列表")}, {ActionProviderGet, "/api/v1/ai/providers/get", "供应商详情", "ai.provider_management", "模型供应商", action.JSON(svc.GetProvider), readAIMeta("查询已脱敏的模型供应商详情", "查询模型供应商详情", "已返回模型供应商详情", action.InputSchemaFor[IDRequest](), nil, "AI", "模型供应商", "详情")}, {ActionProviderCreate, "/api/v1/ai/providers/create", "创建供应商", "ai.provider_management", "模型供应商", action.JSON(svc.CreateProvider), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)}, {ActionProviderUpdate, "/api/v1/ai/providers/update", "更新供应商", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProvider), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)}, {ActionProviderStatus, "/api/v1/ai/providers/status/update", "更新供应商状态", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProviderStatus), mutationAIMeta("启用或停用已配置的模型供应商，不接触凭证", "更新模型供应商状态", "模型供应商状态已更新", "high", "write", action.InputSchemaFor[StatusRequest](), nil, "AI", "模型供应商", "状态")}, {ActionProviderDiscover, "/api/v1/ai/providers/discover-models", "发现远端模型", "ai.provider_management", "模型供应商", action.JSON(svc.Discover), mutationAIMeta("使用已保存凭证向供应商发现远端模型，不接收或回显凭证", "发现供应商远端模型", "已返回远端模型发现结果", "high", "external", action.InputSchemaFor[DiscoverRequest](), nil, "AI", "模型供应商", "模型发现")},
		{ActionDeploymentList, "/api/v1/ai/deployments/list", "模型目录", "ai.model_catalog", "模型目录", action.JSON(svc.ListDeployments), readAIMeta("查询模型部署目录", "查询模型目录", "已返回模型目录", action.InputSchemaFor[ListRequest](), nil, "AI", "模型", "目录")}, {ActionDeploymentGet, "/api/v1/ai/deployments/get", "模型详情", "ai.model_catalog", "模型目录", action.JSON(svc.GetDeployment), readAIMeta("按 ID 查询模型部署详情", "查询模型详情", "已返回模型详情", action.InputSchemaFor[IDRequest](), nil, "AI", "模型", "详情")}, {ActionDeploymentCreate, "/api/v1/ai/deployments/create", "创建模型", "ai.model_catalog", "模型目录", action.JSON(svc.CreateDeployment), mutationAIMeta("在已配置供应商下创建模型部署", "创建模型部署", "模型部署已创建", "high", "write", action.InputSchemaFor[DeploymentCreateRequest](), nil, "AI", "模型", "创建")}, {ActionDeploymentImport, "/api/v1/ai/deployments/import", "导入模型", "ai.model_catalog", "模型目录", action.JSON(svc.ImportDeployments), mutationAIMeta("批量导入已发现的模型部署", "导入模型部署", "模型部署已导入", "high", "write", action.InputSchemaFor[ImportRequest](), nil, "AI", "模型", "导入")}, {ActionDeploymentUpdate, "/api/v1/ai/deployments/update", "更新模型", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeployment), mutationAIMeta("更新模型部署的能力与限制配置", "更新模型部署", "模型部署已更新", "high", "write", action.InputSchemaFor[DeploymentUpdateRequest](), nil, "AI", "模型", "更新")}, {ActionDeploymentStatus, "/api/v1/ai/deployments/status/update", "更新模型状态", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeploymentStatus), mutationAIMeta("启用或停用模型部署", "更新模型状态", "模型状态已更新", "high", "write", action.InputSchemaFor[StatusRequest](), nil, "AI", "模型", "状态")}, {ActionDeploymentDelete, "/api/v1/ai/deployments/delete", "删除模型", "ai.model_catalog", "模型目录", action.JSON(svc.DeleteDeployment), mutationAIMeta("删除未被 Alias 引用的模型部署", "删除模型部署", "模型部署已删除", "high", "destructive", action.InputSchemaFor[IDRequest](), nil, "AI", "模型", "删除")},
		{ActionAliasList, "/api/v1/ai/aliases/list", "Alias 列表", "ai.model_catalog", "模型目录", action.JSON(svc.ListAliases), readAIMeta("查询模型 Alias 绑定", "查询模型 Alias", "已返回模型 Alias", action.InputSchemaFor[struct{}](), nil, "AI", "模型", "Alias")}, {ActionAliasUpsert, "/api/v1/ai/aliases/upsert", "绑定 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.UpsertAlias), mutationAIMeta("创建或更新模型 Alias 绑定", "绑定模型 Alias", "模型 Alias 已绑定", "high", "write", action.InputSchemaFor[AliasUpsertRequest](), nil, "AI", "模型", "Alias", "绑定")}, {ActionAliasDelete, "/api/v1/ai/aliases/delete", "删除 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.DeleteAlias), mutationAIMeta("删除模型 Alias 绑定", "删除模型 Alias", "模型 Alias 已删除", "high", "destructive", action.InputSchemaFor[AliasDeleteRequest](), nil, "AI", "模型", "Alias", "删除")},
		{ActionToolSearch, "/api/v1/ai/tools/search", "搜索工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Search), nil},
		{ActionToolLoad, "/api/v1/ai/tools/load", "加载工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Load), nil},
		{ActionToolExecute, "/api/v1/ai/tools/execute", "执行已加载工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Execute), nil},
		{ActionToolConfirmationList, "/api/v1/ai/tool-confirmations/list", "待确认工具列表", "ai.tool_runtime", "工具运行时", action.JSON(tools.ListConfirmations), nil},
		{ActionToolConfirmationApprove, "/api/v1/ai/tool-confirmations/approve", "批准本次工具执行", "ai.tool_runtime", "工具运行时", action.JSON(tools.Approve), nil},
		{ActionToolConfirmationReject, "/api/v1/ai/tool-confirmations/reject", "拒绝本次工具执行", "ai.tool_runtime", "工具运行时", action.JSON(tools.Reject), nil},
		{ActionToolCatalogRebuild, "/api/v1/ai/tools/catalog/rebuild", "重建工具目录", "ai.tool_runtime", "工具运行时", action.JSON(func(ctx context.Context, _ struct{}) (any, error) { return tools.ReconcileCatalog(ctx, true) }), nil},
		{ActionToolIndexStatus, "/api/v1/ai/tools/index/status", "工具索引状态", "ai.tool_runtime", "工具运行时", action.JSON(func(ctx context.Context, _ struct{}) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.HasPermission(system.ActionSettingList) {
				return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			return tools.IndexStatus(ctx)
		}), nil},
		{ActionToolIndexRebuild, "/api/v1/ai/tools/index/rebuild", "重建工具索引", "ai.tool_runtime", "工具运行时", action.JSON(func(ctx context.Context, _ struct{}) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.HasPermission(system.ActionSettingUpdate) {
				return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			if _, err := tools.ReconcileCatalog(ctx, true); err != nil {
				return nil, err
			}
			return tools.IndexStatus(ctx)
		}), nil},
		{ActionNavigationIndexStatus, "/api/v1/ai/navigation/index/status", "导航索引状态", "ai.navigation", "功能导航", action.JSON(func(ctx context.Context, _ struct{}) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.HasPermission(system.ActionSettingList) {
				return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			return navigationSearch.IndexStatus(ctx, ac.TenantID)
		}), nil},
		{ActionNavigationIndexRebuild, "/api/v1/ai/navigation/index/rebuild", "重建导航索引", "ai.navigation", "功能导航", action.JSON(func(ctx context.Context, _ struct{}) (any, error) {
			ac := auth.MustFromContext(ctx)
			if !ac.HasPermission(system.ActionSettingUpdate) {
				return nil, errs.NewKey(errs.CodeForbidden, "common.forbidden", "forbidden")
			}
			if err := navigationSearch.ReconcileForTenant(ctx, ac.TenantID, true); err != nil {
				return nil, err
			}
			if a.Outbox != nil && !a.Outbox.Enabled {
				if err := navigationSearch.IndexPending(ctx, ac.TenantID); err != nil {
					return nil, err
				}
			}
			return navigationSearch.IndexStatus(ctx, ac.TenantID)
		}), nil},
		{ActionAgentList, "/api/v1/ai/agents/list", "智能体列表", "ai.agent_management", "智能体", action.JSON(agents.List), readAIMeta("查询当前租户可见的智能体", "查询智能体列表", "已返回可见智能体", action.InputSchemaFor[AgentListRequest](), nil, "AI", "智能体", "列表")},
		{ActionAgentGet, "/api/v1/ai/agents/get", "智能体详情", "ai.agent_management", "智能体", action.JSON(agents.Get), readAIMeta("按 ID 查询智能体详情", "查询智能体详情", "已返回智能体详情", action.InputSchemaFor[IDRequest](), nil, "AI", "智能体", "详情")},
		{ActionAgentCreate, "/api/v1/ai/agents/create", "创建智能体", "ai.agent_management", "智能体", action.JSON(agents.Create), mutationAIMeta("创建租户智能体草稿", "创建智能体", "智能体已创建", "high", "write", action.InputSchemaFor[AgentCreateRequest](), nil, "AI", "智能体", "创建")},
		{ActionAgentUpdate, "/api/v1/ai/agents/update", "更新智能体", "ai.agent_management", "智能体", action.JSON(agents.Update), mutationAIMeta("更新智能体资料和草稿配置", "更新智能体", "智能体已更新", "high", "write", action.InputSchemaFor[AgentUpdateRequest](), nil, "AI", "智能体", "更新")},
		{ActionAgentDuplicate, "/api/v1/ai/agents/duplicate", "复制智能体", "ai.agent_management", "智能体", action.JSON(agents.Duplicate), mutationAIMeta("复制可见智能体为新草稿", "复制智能体", "智能体副本已创建", "high", "write", action.InputSchemaFor[AgentDuplicateRequest](), nil, "AI", "智能体", "复制")},
		{ActionAgentStatus, "/api/v1/ai/agents/status/update", "更新智能体状态", "ai.agent_management", "智能体", action.JSON(agents.Status), mutationAIMeta("更新智能体启用状态", "更新智能体状态", "智能体状态已更新", "high", "write", action.InputSchemaFor[AgentStatusRequest](), nil, "AI", "智能体", "状态")},
		{ActionAgentVersionList, "/api/v1/ai/agent-versions/list", "智能体版本列表", "ai.agent_management", "智能体", action.JSON(agents.ListVersions), readAIMeta("查询指定智能体的发布版本", "查询智能体版本", "已返回智能体版本", action.InputSchemaFor[IDRequest](), nil, "AI", "智能体", "版本")},
		{ActionAgentVersionGet, "/api/v1/ai/agent-versions/get", "智能体版本详情", "ai.agent_management", "智能体", action.JSON(agents.GetVersion), readAIMeta("按版本 ID 查询智能体版本详情", "查询智能体版本详情", "已返回智能体版本详情", action.InputSchemaFor[IDRequest](), nil, "AI", "智能体", "版本详情")},
		{ActionAgentVersionCreate, "/api/v1/ai/agent-versions/create", "创建智能体版本", "ai.agent_management", "智能体", action.JSON(agents.CreateVersion), mutationAIMeta("从智能体配置创建不可变版本", "创建智能体版本", "智能体版本已创建", "high", "write", action.InputSchemaFor[AgentVersionCreateRequest](), nil, "AI", "智能体", "版本创建")},
		{ActionAgentVersionPublish, "/api/v1/ai/agent-versions/publish", "发布智能体版本", "ai.agent_management", "智能体", action.JSON(agents.PublishVersion), mutationAIMeta("创建并发布新的智能体版本", "发布智能体版本", "智能体版本已发布", "high", "write", action.InputSchemaFor[AgentVersionCreateRequest](), nil, "AI", "智能体", "发布")},
		{ActionAgentPreviewMaterialize, "/api/v1/ai/agents/preview/materialize", "物化智能体预览版本", "ai.agent_management", "智能体", action.JSON(agents.MaterializePreview), nil},
		{ActionAgentToolSearch, "/api/v1/ai/agent-tools/search", "搜索可绑定工具", "ai.agent_management", "智能体", action.JSON(agents.SearchTools), readAIMeta("搜索当前租户可绑定到智能体的工具", "搜索智能体可用工具", "已返回可绑定工具", action.InputSchemaFor[AgentToolSearchRequest](), nil, "AI", "智能体", "工具搜索")},
		{ActionAgentToolList, "/api/v1/ai/agent-tools/list", "智能体工具列表", "ai.agent_management", "智能体", action.JSON(agents.ListTools), readAIMeta("查询指定智能体已绑定的工具", "查询智能体工具", "已返回智能体工具绑定", action.InputSchemaFor[IDRequest](), nil, "AI", "智能体", "工具")},
		{ActionAgentToolBind, "/api/v1/ai/agent-tools/bind", "绑定智能体工具", "ai.agent_management", "智能体", action.JSON(agents.BindTool), mutationAIMeta("将可见工具绑定到智能体", "绑定智能体工具", "工具已绑定到智能体", "high", "write", action.InputSchemaFor[AgentBindRequest](), nil, "AI", "智能体", "工具绑定")},
		{ActionAgentToolUnbind, "/api/v1/ai/agent-tools/unbind", "解绑智能体工具", "ai.agent_management", "智能体", action.JSON(agents.UnbindTool), mutationAIMeta("移除智能体的工具绑定", "解绑智能体工具", "智能体工具已解绑", "high", "write", action.InputSchemaFor[AgentUnbindRequest](), nil, "AI", "智能体", "工具解绑")},
		{ActionAgentDiagnostics, "/api/v1/ai/agents/diagnostics", "智能体诊断", "ai.agent_management", "智能体", action.JSON(agents.Diagnostics), nil},
		{ActionAgentBuiltinReconcile, "/api/v1/ai/agents/builtin/reconcile", "同步内置智能体", "ai.agent_management", "智能体", action.JSON(agents.ReconcileBuiltin), nil},
		{ActionAgentSlotList, "/api/v1/ai/agent-slots/list", "AI 功能配置列表", "ai.agent_slot_management", "AI 功能配置", action.JSON(slots.List), readAIMeta("查询业务功能与智能体配置", "查询 AI 功能配置", "已返回 AI 功能配置", action.InputSchemaFor[AgentSlotListRequest](), nil, "AI", "功能配置", "列表")},
		{ActionAgentSlotGet, "/api/v1/ai/agent-slots/get", "AI 功能配置详情", "ai.agent_slot_management", "AI 功能配置", action.JSON(slots.Get), readAIMeta("查询指定业务 Slot 的智能体配置", "查询 AI 功能配置详情", "已返回 AI 功能配置详情", action.InputSchemaFor[AgentSlotGetRequest](), nil, "AI", "功能配置", "详情")},
		{ActionAgentSlotUpdate, "/api/v1/ai/agent-slots/update", "配置业务功能智能体", "ai.agent_slot_management", "AI 功能配置", action.JSON(slots.Update), mutationAIMeta("为业务功能配置智能体", "配置业务功能智能体", "业务功能智能体已配置", "high", "write", action.InputSchemaFor[AgentSlotMutationRequest](), nil, "AI", "功能配置", "更新")},
		{ActionAgentSlotReset, "/api/v1/ai/agent-slots/reset", "恢复业务功能系统默认", "ai.agent_slot_management", "AI 功能配置", action.JSON(slots.Reset), mutationAIMeta("恢复业务功能的系统默认智能体", "恢复 AI 功能默认配置", "AI 功能已恢复默认配置", "high", "write", action.InputSchemaFor[AgentSlotResetRequest](), nil, "AI", "功能配置", "重置")},
		{ActionAgentSlotReconcile, "/api/v1/ai/agent-slots/reconcile", "同步内置 AI 功能", "ai.agent_slot_management", "AI 功能配置", action.JSON(slots.Reconcile), nil},
		{ActionAgentSlotPreferenceGet, "/api/v1/ai/agent-slot-preferences/get", "读取个人默认智能体", "ai.agent_slot_preferences", "AI 功能偏好", action.JSON(slots.Get), readAIMeta("查询当前用户指定业务功能的默认智能体", "查询个人默认智能体", "已返回个人默认智能体", action.InputSchemaFor[AgentSlotGetRequest](), nil, "AI", "个人偏好", "智能体")},
		{ActionAgentSlotPreferenceUpdate, "/api/v1/ai/agent-slot-preferences/update", "设置个人默认智能体", "ai.agent_slot_preferences", "AI 功能偏好", action.JSON(slots.UpdatePreference), mutationAIMeta("设置当前用户指定业务功能的默认智能体", "设置个人默认智能体", "个人默认智能体已更新", "medium", "write", action.InputSchemaFor[AgentSlotMutationRequest](), nil, "AI", "个人偏好", "智能体")},
		{ActionAgentSlotPreferenceReset, "/api/v1/ai/agent-slot-preferences/reset", "恢复个人默认智能体", "ai.agent_slot_preferences", "AI 功能偏好", action.JSON(slots.ResetPreference), mutationAIMeta("恢复当前用户指定业务功能的默认智能体", "恢复个人默认智能体", "个人默认智能体已恢复", "medium", "write", action.InputSchemaFor[AgentSlotResetRequest](), nil, "AI", "个人偏好", "重置")},
		{ActionConversationList, "/api/v1/ai/conversations/list", "会话列表", "ai.conversation", "AI 会话", action.JSON(conversations.List), readAIMeta("查询当前用户可见的 AI 会话", "查询 AI 会话列表", "已返回可见 AI 会话", action.InputSchemaFor[ConversationListRequest](), nil, "AI", "会话", "列表")},
		{ActionConversationGet, "/api/v1/ai/conversations/get", "会话详情", "ai.conversation", "AI 会话", action.JSON(conversations.Get), readAIMeta("按 ID 查询可见 AI 会话详情", "查询 AI 会话详情", "已返回 AI 会话详情", action.InputSchemaFor[ConversationGetRequest](), nil, "AI", "会话", "详情")},
		{ActionConversationCreate, "/api/v1/ai/conversations/create", "创建会话", "ai.conversation", "AI 会话", action.JSON(conversations.Create), nil},
		{ActionConversationCreateForSlot, "/api/v1/ai/conversations/create-for-slot", "按业务功能创建会话", "ai.conversation", "AI 会话", action.JSON(conversations.CreateForSlot(slots)), nil},
		{ActionConversationUpdate, "/api/v1/ai/conversations/update", "更新会话", "ai.conversation", "AI 会话", action.JSON(conversations.Update), mutationAIMeta("更新当前用户可见会话的标题", "更新 AI 会话", "AI 会话已更新", "medium", "write", action.InputSchemaFor[ConversationUpdateRequest](), nil, "AI", "会话", "更新")},
		{ActionConversationArchive, "/api/v1/ai/conversations/archive", "归档会话", "ai.conversation", "AI 会话", action.JSON(conversations.Archive), mutationAIMeta("归档当前用户可见的 AI 会话", "归档 AI 会话", "AI 会话已归档", "medium", "write", action.InputSchemaFor[ConversationStateRequest](), nil, "AI", "会话", "归档")},
		{ActionConversationRestore, "/api/v1/ai/conversations/restore", "恢复会话", "ai.conversation", "AI 会话", action.JSON(conversations.Restore), mutationAIMeta("恢复当前用户已归档的 AI 会话", "恢复 AI 会话", "AI 会话已恢复", "medium", "write", action.InputSchemaFor[ConversationStateRequest](), nil, "AI", "会话", "恢复")},
		{ActionMessageList, "/api/v1/ai/messages/list", "消息列表", "ai.conversation", "AI 会话", action.JSON(conversations.ListMessages), readAIMeta("查询指定会话的消息列表", "查询 AI 会话消息", "已返回 AI 会话消息", action.InputSchemaFor[MessageListRequest](), nil, "AI", "会话", "消息")},
		{ActionMessageGet, "/api/v1/ai/messages/get", "消息详情", "ai.conversation", "AI 会话", action.JSON(conversations.GetMessage), readAIMeta("按 ID 查询可见会话消息详情", "查询 AI 消息详情", "已返回 AI 消息详情", action.InputSchemaFor[MessageGetRequest](), nil, "AI", "会话", "消息详情")},
		{ActionContextCompactionDiagnostics, "/api/v1/ai/context-compaction/diagnostics", "上下文压缩诊断", "ai.conversation", "AI 会话", action.JSON(compactions.Diagnostics), nil},
	}
	if err := a.RegisterAction(action.Action{ID: ActionMessageCreate, Handler: action.JSON(conversations.CreateMessage), Meta: action.Metadata{Capability: "ai", Name: "提交用户消息", NameKey: "action." + ActionMessageCreate + ".name", Description: "向会话幂等追加用户消息", DescriptionKey: "action." + ActionMessageCreate + ".description", PermissionGroup: "ai.conversation", PermissionGroupName: "AI 会话", PermissionGroupNameKey: "permission_group.ai.conversation", Audit: &auditOn, AI: action.ExcludeFromAI(action.AIExcludeInternalRuntime), Idempotency: &action.IdempotencyMeta{Mode: "required"}, HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/messages/create"}}}); err != nil {
		return err
	}
	for _, d := range defs {
		access := action.AccessMode("")
		if strings.HasPrefix(d.id, "ai.tool_") || strings.HasPrefix(d.id, "ai.navigation_index_") {
			access = action.AccessAuthenticated
		}
		if strings.HasPrefix(d.id, "ai.agent_slot_preference_") || d.id == ActionConversationCreateForSlot {
			access = action.AccessAuthenticated
		}
		aiMeta := d.ai
		if aiMeta == nil {
			aiMeta = action.ExcludeFromAI(action.AIExcludeInternalRuntime)
		}
		if err := a.RegisterAction(action.Action{ID: d.id, Handler: d.handler, Meta: action.Metadata{Capability: "ai", Access: access, Name: d.name, NameKey: "action." + d.id + ".name", Description: d.name, DescriptionKey: "action." + d.id + ".description", PermissionGroup: d.group, PermissionGroupName: d.groupName, PermissionGroupNameKey: "permission_group." + d.group, Audit: &auditOn, AI: aiMeta, HTTP: &action.HTTPMeta{Method: "POST", Path: d.path}}}); err != nil {
			return err
		}
	}
	if err := RegisterKnowledge(a, knowledge); err != nil {
		return err
	}
	if err := RegisterKnowledgeIngestion(a, knowledge, ingestion); err != nil {
		return err
	}
	if err := toolRegistry.RegisterProvider(&aitool.ActionProvider{Actions: a.Actions}, &aitool.ActionExecutor{Actions: a.Actions}); err != nil {
		return err
	}
	if err := aitools.RegisterBuiltins(toolRegistry, knowledgetool.New(knowledgeTool.Project, knowledgeTool), memorytool.New(memoryRuntimeTool.Project, memoryRuntimeTool), navigationtool.New(a.Menus), navigationtool.NewHelp(a.Menus, a.PageHelp)); err != nil {
		return err
	}
	meta := []aitool.Definition{{ID: "meta:tool.search", Source: aitool.SourceMeta, Name: "tool.search", TitleForHuman: "工具搜索", DescriptionForLLM: "Search visible tools by intent and return compact candidates", DescriptionForHuman: "按意图搜索当前智能体可用的工具", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaSearchInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.search", SchemaVersion: 1, Status: "enabled"}, {ID: "meta:tool.load", Source: aitool.SourceMeta, Name: "tool.load", TitleForHuman: "工具加载", DescriptionForLLM: "Load a small number of candidate tools into the current scope", DescriptionForHuman: "将少量候选工具加载到当前运行范围", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaLoadInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingDirect, BindingGroup: "tool_discovery", RuntimeRef: "tool.load", SchemaVersion: 1, Status: "enabled"}}
	metaExec := aitool.ExecuteFunc(func(ctx context.Context, t aitool.LoadedTool, args json.RawMessage) (*aitool.Result, error) {
		switch t.Definition.ID {
		case "meta:tool.search":
			var input aitool.MetaSearchInput
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, err
			}
			r := ToolSearchRequest{ScopeID: aitool.ScopeIDFromContext(ctx), Query: input.Query, Capability: input.Capability, Source: input.Source, Limit: input.Limit}
			if r.ScopeID == "" {
				return nil, fmt.Errorf("trusted tool scope is required")
			}
			v, err := tools.Search(ctx, r)
			return &aitool.Result{Status: "completed", Summary: "tool search completed", Data: v}, err
		case "meta:tool.load":
			var input aitool.MetaLoadInput
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, err
			}
			r := ToolLoadRequest{ScopeID: aitool.ScopeIDFromContext(ctx), ToolIDs: input.ToolIDs}
			if r.ScopeID == "" {
				return nil, fmt.Errorf("trusted tool scope is required")
			}
			v, err := tools.Load(ctx, r)
			return &aitool.Result{Status: "completed", Summary: "tool load completed", Data: v}, err
		default:
			return nil, fmt.Errorf("unknown meta tool")
		}
	})
	if err := toolRegistry.RegisterProvider(&aitool.StaticProvider{ProviderSource: aitool.SourceMeta, Definitions: meta}, metaExec); err != nil {
		return err
	}
	if err := toolRegistry.Refresh(context.Background()); err != nil {
		return err
	}
	for _, m := range []menu.Menu{
		{ID: "ai.root", Title: "AI 中心", TitleKey: "menu.ai.root.title", Icon: "mdi:creation-outline", Type: menu.TypeDirectory, Module: "ai", Order: 20},
		{ID: "ai.providers", ParentID: "ai.root", Title: "模型供应商", TitleKey: "menu.ai.providers.title", Icon: "mdi:server-network", Type: menu.TypeMenu, Path: "/ai/providers", Component: "ai/providers", Permission: ActionProviderList, BindAction: ActionProviderList, Module: "ai", Order: 1},
		{ID: "ai.models", ParentID: "ai.root", Title: "模型目录", TitleKey: "menu.ai.models.title", Icon: "mdi:database-search-outline", Type: menu.TypeMenu, Path: "/ai/models", Component: "ai/models", Permission: ActionDeploymentList, BindAction: ActionDeploymentList, Module: "ai", Order: 2},
		{ID: "ai.agents", ParentID: "ai.root", Title: "智能体", TitleKey: "menu.ai.agents.title", Icon: "mdi:robot-outline", Type: menu.TypeMenu, Path: "/ai/agents", Component: "ai/agents", Permission: ActionAgentList, BindAction: ActionAgentList, Module: "ai", Order: 4},
		{ID: "ai.agent_slots", ParentID: "ai.root", Title: "AI 功能配置", TitleKey: "menu.ai.agent_slots.title", Icon: "mdi:tune-variant", Type: menu.TypeMenu, Path: "/ai/agent-slots", Component: "ai/agent-slots", Permission: ActionAgentSlotList, BindAction: ActionAgentSlotList, Module: "ai", Order: 5},
		{ID: "ai.knowledge", ParentID: "ai.root", Title: "知识库", TitleKey: "menu.ai.knowledge.title", Icon: "mdi:bookshelf", Type: menu.TypeMenu, Path: "/ai/knowledge", Component: "ai/knowledge", Permission: ActionKnowledgeBaseList, BindAction: ActionKnowledgeBaseList, Module: "ai", Order: 6},
		{ID: "ai.workspace", ParentID: "ai.root", Title: "AI 工作台", TitleKey: "menu.ai.workspace.title", Icon: "mdi:creation", Type: menu.TypeMenu, Path: "/ai/workspace", Component: "ai/workspace", Permission: ActionConversationList, BindAction: ActionConversationList, Module: "ai", Order: 99, HideInMenu: true, Placement: menu.PlacementGlobalAILauncher, Immersive: true},
		// HideInMenu：不进侧栏，但必须出现在 /menus 结果中，供动态路由注册 Agent Builder
		{ID: "ai.agent_builder", ParentID: "ai.root", Title: "智能体编排工作台", TitleKey: "menu.ai.agent_builder.title", Icon: "mdi:application-edit-outline", Type: menu.TypeMenu, Path: "/ai/agent-builder", Component: "ai/agent-builder", Permission: ActionAgentList, BindAction: ActionAgentList, Module: "ai", Order: 6, HideInMenu: true},
	} {
		if err := a.RegisterMenu(m); err != nil {
			return err
		}
	}
	if err := aihelp.Register(a.PageHelp); err != nil {
		return fmt.Errorf("register AI page help: %w", err)
	}
	if err := a.PageHelp.ValidateMenus(a.Menus); err != nil {
		return err
	}
	return a.PageHelp.ValidateModuleCoverage(a.Menus, "ai", "zh-CN")
}

func NewBuiltinAgentContractRegistry() (*aiagent.Registry, error) {
	contracts := aiagent.NewRegistry()
	err := contracts.Register(aiagent.Contract{Key: "system.operations", Version: "1.0.0", Name: "系统运营助手", Description: "通过当前用户权限安全操作后台系统", BaseInstructions: prompt.MustGet("agent/system-operations"), RequiredModelCapabilities: []string{"chat", "tools"}, RequiredToolSelectors: []string{"meta:tool.search", "meta:tool.load", navigationtool.ToolID, navigationtool.HelpToolID}, AllowedToolSources: []string{"action"}, AllowedToolCapabilities: []string{"system", "operations"}, ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), SingletonScope: aiagent.SingletonTenant, ManagedFields: []string{"code", "source", "contract_key", "contract_version", "base_instructions", "visibility"}, UpgradePolicy: "managed-fields"})
	return contracts, err
}
