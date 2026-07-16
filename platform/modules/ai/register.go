package ai

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

const (
	ActionProviderList     = "ai.provider_list"
	ActionProviderGet      = "ai.provider_get"
	ActionProviderCreate   = "ai.provider_create"
	ActionProviderUpdate   = "ai.provider_update"
	ActionProviderStatus   = "ai.provider_status_update"
	ActionProviderDiscover = "ai.provider_discover_models"
	ActionDeploymentList   = "ai.deployment_list"
	ActionDeploymentGet    = "ai.deployment_get"
	ActionDeploymentCreate = "ai.deployment_create"
	ActionDeploymentImport = "ai.deployment_import"
	ActionDeploymentUpdate = "ai.deployment_update"
	ActionDeploymentStatus = "ai.deployment_status_update"
	ActionAliasList        = "ai.alias_list"
	ActionAliasUpsert      = "ai.alias_upsert"
	ActionAliasDelete      = "ai.alias_delete"
	ActionProviderTypes    = "ai.provider_type_list"
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
	svc := NewService(a.DB, registry, aimodel.EnvSecretResolver{})
	a.ModelResolver = svc
	auditOn := true
	type def struct {
		id, path, name, group, groupName string
		handler                          action.Handler
	}
	defs := []def{
		{ActionProviderTypes, "/api/v1/ai/provider-types/list", "供应商协议列表", "ai.provider_management", "模型供应商", action.JSON(svc.ProviderTypes)},
		{ActionProviderList, "/api/v1/ai/providers/list", "供应商列表", "ai.provider_management", "模型供应商", action.JSON(svc.ListProviders)}, {ActionProviderGet, "/api/v1/ai/providers/get", "供应商详情", "ai.provider_management", "模型供应商", action.JSON(svc.GetProvider)}, {ActionProviderCreate, "/api/v1/ai/providers/create", "创建供应商", "ai.provider_management", "模型供应商", action.JSON(svc.CreateProvider)}, {ActionProviderUpdate, "/api/v1/ai/providers/update", "更新供应商", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProvider)}, {ActionProviderStatus, "/api/v1/ai/providers/status/update", "更新供应商状态", "ai.provider_management", "模型供应商", action.JSON(svc.UpdateProviderStatus)}, {ActionProviderDiscover, "/api/v1/ai/providers/discover-models", "发现远端模型", "ai.provider_management", "模型供应商", action.JSON(svc.Discover)},
		{ActionDeploymentList, "/api/v1/ai/deployments/list", "模型目录", "ai.model_catalog", "模型目录", action.JSON(svc.ListDeployments)}, {ActionDeploymentGet, "/api/v1/ai/deployments/get", "模型详情", "ai.model_catalog", "模型目录", action.JSON(svc.GetDeployment)}, {ActionDeploymentCreate, "/api/v1/ai/deployments/create", "创建模型", "ai.model_catalog", "模型目录", action.JSON(svc.CreateDeployment)}, {ActionDeploymentImport, "/api/v1/ai/deployments/import", "导入模型", "ai.model_catalog", "模型目录", action.JSON(svc.ImportDeployments)}, {ActionDeploymentUpdate, "/api/v1/ai/deployments/update", "更新模型", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeployment)}, {ActionDeploymentStatus, "/api/v1/ai/deployments/status/update", "更新模型状态", "ai.model_catalog", "模型目录", action.JSON(svc.UpdateDeploymentStatus)},
		{ActionAliasList, "/api/v1/ai/aliases/list", "Alias 列表", "ai.model_catalog", "模型目录", action.JSON(svc.ListAliases)}, {ActionAliasUpsert, "/api/v1/ai/aliases/upsert", "绑定 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.UpsertAlias)}, {ActionAliasDelete, "/api/v1/ai/aliases/delete", "删除 Alias", "ai.model_catalog", "模型目录", action.JSON(svc.DeleteAlias)},
	}
	for _, d := range defs {
		if err := a.RegisterAction(action.Action{ID: d.id, Handler: d.handler, Meta: action.Metadata{Capability: "ai", Name: d.name, NameKey: "action." + d.id + ".name", Description: d.name, DescriptionKey: "action." + d.id + ".description", PermissionGroup: d.group, PermissionGroupName: d.groupName, Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: d.path}}}); err != nil {
			return err
		}
	}
	for _, m := range []menu.Menu{{ID: "ai.root", Title: "AI 中心", TitleKey: "menu.ai.root.title", Icon: "mdi:creation-outline", Type: menu.TypeDirectory, Module: "ai", Order: 20}, {ID: "ai.providers", ParentID: "ai.root", Title: "模型供应商", TitleKey: "menu.ai.providers.title", Icon: "mdi:server-network", Type: menu.TypeMenu, Path: "/ai/providers", Component: "ai/providers", Permission: ActionProviderList, BindAction: ActionProviderList, Module: "ai", Order: 1}, {ID: "ai.models", ParentID: "ai.root", Title: "模型目录", TitleKey: "menu.ai.models.title", Icon: "mdi:database-search-outline", Type: menu.TypeMenu, Path: "/ai/models", Component: "ai/models", Permission: ActionDeploymentList, BindAction: ActionDeploymentList, Module: "ai", Order: 2}} {
		if err := a.RegisterMenu(m); err != nil {
			return err
		}
	}
	return nil
}
