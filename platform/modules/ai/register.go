package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
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
	ActionModelGenerate                = "ai.model_generate"
	ActionModelStream                  = "ai.model_stream"
	ActionToolSearch                   = "ai.tool_search"
	ActionToolLoad                     = "ai.tool_load"
	ActionToolExecute                  = "ai.tool_execute"
	ActionToolConfirmationList         = "ai.tool_confirmation_list"
	ActionToolConfirmationApprove      = "ai.tool_confirmation_approve"
	ActionToolConfirmationReject       = "ai.tool_confirmation_reject"
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
	ActionConversationList             = "ai.conversation_list"
	ActionConversationGet              = "ai.conversation_get"
	ActionConversationCreate           = "ai.conversation_create"
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
		for _, eventType := range []string{toolCatalogChangedEvent, toolInvocationFinishedEvent} {
			if err := a.Outbox.RegisterDefinition(outbox.Definition{EventType: eventType, SchemaVersion: 1}); err != nil {
				return err
			}
		}
	}
	invocations := NewInvocationService(a.DB, svc, adapters, writer)
	invocations.SetEmbeddingAdapters(embeddingAdapters)
	invocations.SetAssetResolver(a.AIAssetResolver)
	a.EmbeddingRuntime = invocations
	if err := invocations.RecoverStale(context.Background(), 10*time.Minute); err != nil {
		return err
	}
	toolRegistry := aitool.NewRegistry()
	handleSigner, err := aitool.NewHandleSigner(a.Config.Auth.JWT.Secret)
	if err != nil {
		return err
	}
	tools := NewToolService(a.DB, toolRegistry, handleSigner, writer)
	invocations.SetToolProjector(tools)
	a.ToolRegistry = toolRegistry
	contracts, err := NewBuiltinAgentContractRegistry()
	if err != nil {
		return err
	}
	a.AgentContracts = contracts
	agents := NewAgentService(a.DB, contracts, svc, toolRegistry)
	conversations := NewConversationService(a.DB, a.AIAssetResolver)
	compactions := NewContextCompactionService(a.DB, conversations, svc, invocations, toolRegistry)
	a.ContextCompactor = compactions
	agentRuntime := NewAgentRuntimeService(a.DB, conversations, invocations, tools, compactions, writer, a.Authorization)
	tools.SetConfirmationHooks(agentRuntime.OnConfirmationPending, agentRuntime.OnConfirmationDecided)
	if err := RegisterAgentRuntime(a, agentRuntime); err != nil {
		return err
	}
	a.AgentTenantInitializer = agents.ReconcileBuiltinForTenant
	auditOn := true
	type def struct {
		id, path, name, group, groupName string
		handler                          action.Handler
	}
	defs := []def{
		{ActionProviderTypes, "/api/v1/ai/provider-types/list", "供应商协议列表", "ai.provider_management", "模型供应商", action.JSON(svc.ProviderTypes)},
		{ActionProviderList, "/api/v1/ai/providers/list", "供应商列表", "ai.provider_management", "模型供应商", action.JSON(svc.ListProviders)}, {ActionProviderGet, "/api/v1/ai/providers/get", "供应商详情", "ai.provider_management", "模型供应商", action.JSON(svc.GetProvider)}, {ActionProviderCreate, "/api/v1/ai/providers/create", "创建供应商", "ai.provider_management", "模型供应商", action.JSON(svc.CreateProvider)}, {ActionProviderUpdate, "/api/v1/ai/providers/update", "更新供应商", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProvider)}, {ActionProviderStatus, "/api/v1/ai/providers/status/update", "更新供应商状态", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProviderStatus)}, {ActionProviderDiscover, "/api/v1/ai/providers/discover-models", "发现远端模型", "ai.provider_management", "模型供应商", action.JSON(svc.Discover)},
		{ActionDeploymentList, "/api/v1/ai/deployments/list", "模型目录", "ai.model_catalog", "模型目录", action.JSON(svc.ListDeployments)}, {ActionDeploymentGet, "/api/v1/ai/deployments/get", "模型详情", "ai.model_catalog", "模型目录", action.JSON(svc.GetDeployment)}, {ActionDeploymentCreate, "/api/v1/ai/deployments/create", "创建模型", "ai.model_catalog", "模型目录", action.JSON(svc.CreateDeployment)}, {ActionDeploymentImport, "/api/v1/ai/deployments/import", "导入模型", "ai.model_catalog", "模型目录", action.JSON(svc.ImportDeployments)}, {ActionDeploymentUpdate, "/api/v1/ai/deployments/update", "更新模型", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeployment)}, {ActionDeploymentStatus, "/api/v1/ai/deployments/status/update", "更新模型状态", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeploymentStatus)}, {ActionDeploymentDelete, "/api/v1/ai/deployments/delete", "删除模型", "ai.model_catalog", "模型目录", action.JSON(svc.DeleteDeployment)},
		{ActionAliasList, "/api/v1/ai/aliases/list", "Alias 列表", "ai.model_catalog", "模型目录", action.JSON(svc.ListAliases)}, {ActionAliasUpsert, "/api/v1/ai/aliases/upsert", "绑定 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.UpsertAlias)}, {ActionAliasDelete, "/api/v1/ai/aliases/delete", "删除 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.DeleteAlias)},
		{ActionModelGenerate, "/api/v1/ai/models/generate", "模型生成", "ai.model_runtime", "模型调用", action.JSON(invocations.Generate)},
		{ActionModelStream, "/api/v1/ai/models/stream", "模型流式生成", "ai.model_runtime", "模型调用", action.JSON(invocations.Stream)},
		{ActionToolSearch, "/api/v1/ai/tools/search", "搜索工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Search)},
		{ActionToolLoad, "/api/v1/ai/tools/load", "加载工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Load)},
		{ActionToolExecute, "/api/v1/ai/tools/execute", "执行已加载工具", "ai.tool_runtime", "工具运行时", action.JSON(tools.Execute)},
		{ActionToolConfirmationList, "/api/v1/ai/tool-confirmations/list", "待确认工具列表", "ai.tool_runtime", "工具运行时", action.JSON(tools.ListConfirmations)},
		{ActionToolConfirmationApprove, "/api/v1/ai/tool-confirmations/approve", "批准本次工具执行", "ai.tool_runtime", "工具运行时", action.JSON(tools.Approve)},
		{ActionToolConfirmationReject, "/api/v1/ai/tool-confirmations/reject", "拒绝本次工具执行", "ai.tool_runtime", "工具运行时", action.JSON(tools.Reject)},
		{ActionAgentList, "/api/v1/ai/agents/list", "智能体列表", "ai.agent_management", "智能体", action.JSON(agents.List)},
		{ActionAgentGet, "/api/v1/ai/agents/get", "智能体详情", "ai.agent_management", "智能体", action.JSON(agents.Get)},
		{ActionAgentCreate, "/api/v1/ai/agents/create", "创建智能体", "ai.agent_management", "智能体", action.JSON(agents.Create)},
		{ActionAgentUpdate, "/api/v1/ai/agents/update", "更新智能体", "ai.agent_management", "智能体", action.JSON(agents.Update)},
		{ActionAgentDuplicate, "/api/v1/ai/agents/duplicate", "复制智能体", "ai.agent_management", "智能体", action.JSON(agents.Duplicate)},
		{ActionAgentStatus, "/api/v1/ai/agents/status/update", "更新智能体状态", "ai.agent_management", "智能体", action.JSON(agents.Status)},
		{ActionAgentVersionList, "/api/v1/ai/agent-versions/list", "智能体版本列表", "ai.agent_management", "智能体", action.JSON(agents.ListVersions)},
		{ActionAgentVersionGet, "/api/v1/ai/agent-versions/get", "智能体版本详情", "ai.agent_management", "智能体", action.JSON(agents.GetVersion)},
		{ActionAgentVersionCreate, "/api/v1/ai/agent-versions/create", "创建智能体版本", "ai.agent_management", "智能体", action.JSON(agents.CreateVersion)},
		{ActionAgentVersionPublish, "/api/v1/ai/agent-versions/publish", "发布智能体版本", "ai.agent_management", "智能体", action.JSON(agents.PublishVersion)},
		{ActionAgentPreviewMaterialize, "/api/v1/ai/agents/preview/materialize", "物化智能体预览版本", "ai.agent_management", "智能体", action.JSON(agents.MaterializePreview)},
		{ActionAgentToolSearch, "/api/v1/ai/agent-tools/search", "搜索可绑定工具", "ai.agent_management", "智能体", action.JSON(agents.SearchTools)},
		{ActionAgentToolList, "/api/v1/ai/agent-tools/list", "智能体工具列表", "ai.agent_management", "智能体", action.JSON(agents.ListTools)},
		{ActionAgentToolBind, "/api/v1/ai/agent-tools/bind", "绑定智能体工具", "ai.agent_management", "智能体", action.JSON(agents.BindTool)},
		{ActionAgentToolUnbind, "/api/v1/ai/agent-tools/unbind", "解绑智能体工具", "ai.agent_management", "智能体", action.JSON(agents.UnbindTool)},
		{ActionAgentDiagnostics, "/api/v1/ai/agents/diagnostics", "智能体诊断", "ai.agent_management", "智能体", action.JSON(agents.Diagnostics)},
		{ActionAgentBuiltinReconcile, "/api/v1/ai/agents/builtin/reconcile", "同步内置智能体", "ai.agent_management", "智能体", action.JSON(agents.ReconcileBuiltin)},
		{ActionConversationList, "/api/v1/ai/conversations/list", "会话列表", "ai.conversation", "AI 会话", action.JSON(conversations.List)},
		{ActionConversationGet, "/api/v1/ai/conversations/get", "会话详情", "ai.conversation", "AI 会话", action.JSON(conversations.Get)},
		{ActionConversationCreate, "/api/v1/ai/conversations/create", "创建会话", "ai.conversation", "AI 会话", action.JSON(conversations.Create)},
		{ActionConversationUpdate, "/api/v1/ai/conversations/update", "更新会话", "ai.conversation", "AI 会话", action.JSON(conversations.Update)},
		{ActionConversationArchive, "/api/v1/ai/conversations/archive", "归档会话", "ai.conversation", "AI 会话", action.JSON(conversations.Archive)},
		{ActionConversationRestore, "/api/v1/ai/conversations/restore", "恢复会话", "ai.conversation", "AI 会话", action.JSON(conversations.Restore)},
		{ActionMessageList, "/api/v1/ai/messages/list", "消息列表", "ai.conversation", "AI 会话", action.JSON(conversations.ListMessages)},
		{ActionMessageGet, "/api/v1/ai/messages/get", "消息详情", "ai.conversation", "AI 会话", action.JSON(conversations.GetMessage)},
		{ActionContextCompactionDiagnostics, "/api/v1/ai/context-compaction/diagnostics", "上下文压缩诊断", "ai.conversation", "AI 会话", action.JSON(compactions.Diagnostics)},
	}
	if err := a.RegisterAction(action.Action{ID: ActionMessageCreate, Handler: action.JSON(conversations.CreateMessage), Meta: action.Metadata{Capability: "ai", Name: "提交用户消息", NameKey: "action." + ActionMessageCreate + ".name", Description: "向会话幂等追加用户消息", DescriptionKey: "action." + ActionMessageCreate + ".description", PermissionGroup: "ai.conversation", PermissionGroupName: "AI 会话", PermissionGroupNameKey: "permission_group.ai.conversation", Audit: &auditOn, Idempotency: &action.IdempotencyMeta{Mode: "required"}, HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/messages/create"}}}); err != nil {
		return err
	}
	for _, d := range defs {
		access := action.AccessMode("")
		if strings.HasPrefix(d.id, "ai.tool_") {
			access = action.AccessAuthenticated
		}
		if err := a.RegisterAction(action.Action{ID: d.id, Handler: d.handler, Meta: action.Metadata{Capability: "ai", Access: access, Name: d.name, NameKey: "action." + d.id + ".name", Description: d.name, DescriptionKey: "action." + d.id + ".description", PermissionGroup: d.group, PermissionGroupName: d.groupName, PermissionGroupNameKey: "permission_group." + d.group, Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: d.path}}}); err != nil {
			return err
		}
	}
	if err := toolRegistry.RegisterProvider(&aitool.ActionProvider{Actions: a.Actions}, &aitool.ActionExecutor{Actions: a.Actions}); err != nil {
		return err
	}
	meta := []aitool.Definition{{ID: "meta:tool.search", Source: aitool.SourceMeta, Name: "tool.search", DescriptionForLLM: "Search visible tools by intent and return compact candidates", DescriptionForHuman: "搜索当前用户可用的工具", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaSearchInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, RuntimeRef: "tool.search", SchemaVersion: 1, Status: "enabled"}, {ID: "meta:tool.load", Source: aitool.SourceMeta, Name: "tool.load", DescriptionForLLM: "Load a small number of candidate tools into the current scope", DescriptionForHuman: "将候选工具加载到当前调用范围", Capability: "ai", InputSchema: action.SchemaFor[aitool.MetaLoadInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, RuntimeRef: "tool.load", SchemaVersion: 1, Status: "enabled"}}
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
		{ID: "ai.runtime", ParentID: "ai.root", Title: "模型调试", TitleKey: "menu.ai.runtime.title", Icon: "mdi:message-text-fast-outline", Type: menu.TypeMenu, Path: "/ai/runtime", Component: "ai/runtime", Permission: ActionModelGenerate, BindAction: ActionModelGenerate, Module: "ai", Order: 3},
		{ID: "ai.agents", ParentID: "ai.root", Title: "智能体", TitleKey: "menu.ai.agents.title", Icon: "mdi:robot-outline", Type: menu.TypeMenu, Path: "/ai/agents", Component: "ai/agents", Permission: ActionAgentList, BindAction: ActionAgentList, Module: "ai", Order: 4},
		// HideInMenu：不进侧栏，但必须出现在 /menus 结果中，供动态路由注册 Agent Builder
		{ID: "ai.agent_builder", ParentID: "ai.root", Title: "智能体编排工作台", TitleKey: "menu.ai.agent_builder.title", Icon: "mdi:application-edit-outline", Type: menu.TypeMenu, Path: "/ai/agent-builder", Component: "ai/agent-builder", Permission: ActionAgentList, BindAction: ActionAgentList, Module: "ai", Order: 5, HideInMenu: true},
	} {
		if err := a.RegisterMenu(m); err != nil {
			return err
		}
	}
	return nil
}

func NewBuiltinAgentContractRegistry() (*aiagent.Registry, error) {
	contracts := aiagent.NewRegistry()
	err := contracts.Register(aiagent.Contract{Key: "system.operations", Version: "1.0.0", Name: "系统运营助手", Description: "通过当前用户权限安全操作后台系统", BaseInstructions: "始终继承当前用户的租户、身份和权限。不得绕过鉴权、工具确认、租户隔离或数据安全策略。先搜索并加载必要工具，只执行完成当前任务所需的最少操作。", RequiredModelCapabilities: []string{"chat", "tools"}, RequiredToolSelectors: []string{"meta:tool.search", "meta:tool.load"}, ConfigSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), SingletonScope: aiagent.SingletonTenant, ManagedFields: []string{"code", "source", "contract_key", "contract_version", "base_instructions", "visibility"}, UpgradePolicy: "managed-fields"})
	return contracts, err
}
