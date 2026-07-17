package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const (
	ActionProviderList            = "ai.provider_list"
	ActionProviderGet             = "ai.provider_get"
	ActionProviderCreate          = "ai.provider_create"
	ActionProviderUpdate          = "ai.provider_update"
	ActionProviderStatus          = "ai.provider_status_update"
	ActionProviderDiscover        = "ai.provider_discover_models"
	ActionDeploymentList          = "ai.deployment_list"
	ActionDeploymentGet           = "ai.deployment_get"
	ActionDeploymentCreate        = "ai.deployment_create"
	ActionDeploymentImport        = "ai.deployment_import"
	ActionDeploymentUpdate        = "ai.deployment_update"
	ActionDeploymentStatus        = "ai.deployment_status_update"
	ActionDeploymentDelete        = "ai.deployment_delete"
	ActionAliasList               = "ai.alias_list"
	ActionAliasUpsert             = "ai.alias_upsert"
	ActionAliasDelete             = "ai.alias_delete"
	ActionProviderTypes           = "ai.provider_type_list"
	ActionModelGenerate           = "ai.model_generate"
	ActionModelStream             = "ai.model_stream"
	ActionToolSearch              = "ai.tool_search"
	ActionToolLoad                = "ai.tool_load"
	ActionToolExecute             = "ai.tool_execute"
	ActionToolConfirmationList    = "ai.tool_confirmation_list"
	ActionToolConfirmationApprove = "ai.tool_confirmation_approve"
	ActionToolConfirmationReject  = "ai.tool_confirmation_reject"
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
	invocations.SetAssetResolver(a.AIAssetResolver)
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
	for _, m := range []menu.Menu{{ID: "ai.root", Title: "AI 中心", TitleKey: "menu.ai.root.title", Icon: "mdi:creation-outline", Type: menu.TypeDirectory, Module: "ai", Order: 20}, {ID: "ai.providers", ParentID: "ai.root", Title: "模型供应商", TitleKey: "menu.ai.providers.title", Icon: "mdi:server-network", Type: menu.TypeMenu, Path: "/ai/providers", Component: "ai/providers", Permission: ActionProviderList, BindAction: ActionProviderList, Module: "ai", Order: 1}, {ID: "ai.models", ParentID: "ai.root", Title: "模型目录", TitleKey: "menu.ai.models.title", Icon: "mdi:database-search-outline", Type: menu.TypeMenu, Path: "/ai/models", Component: "ai/models", Permission: ActionDeploymentList, BindAction: ActionDeploymentList, Module: "ai", Order: 2}, {ID: "ai.runtime", ParentID: "ai.root", Title: "模型调试", TitleKey: "menu.ai.runtime.title", Icon: "mdi:message-text-fast-outline", Type: menu.TypeMenu, Path: "/ai/runtime", Component: "ai/runtime", Permission: ActionModelGenerate, BindAction: ActionModelGenerate, Module: "ai", Order: 3}} {
		if err := a.RegisterMenu(m); err != nil {
			return err
		}
	}
	return nil
}
