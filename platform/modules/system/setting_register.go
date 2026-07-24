package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerSettingActions(a *app.App, svc *SettingService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     func(context.Context, json.RawMessage) (any, error)
		ai          *action.AIMeta
	}{
		{ActionSettingList, "/api/v1/system/settings/list", "设置列表", "按分类返回当前租户可见设置元数据与有效值", svc.List, readAIMeta("查询当前租户可见的设置分类、字段定义和有效值", "查询系统设置列表", "查询当前租户可见的设置分类、字段定义和有效值", "已返回当前用户有权查看的系统设置", action.InputSchemaFor[struct{}](), action.SchemaFor[[]SettingCategoryView](), "设置", "系统配置", "查询")},
		{ActionSettingGet, "/api/v1/system/settings/get", "设置详情", "获取单个设置组详情与有效值", svc.Get, readAIMeta("按设置 Key 查询当前租户的字段定义、来源、版本和有效值", "查询系统设置详情", "按设置 Key 查询当前租户的字段定义、来源、版本和有效值", "已返回指定系统设置", action.InputSchemaFor[settingKeyRequest](), action.SchemaFor[SettingDefinitionView](), "设置", "详情", "查询")},
		{ActionSettingUpdate, "/api/v1/system/settings/update", "更新设置", "保存租户设置覆盖值", svc.Update, writeAIMeta("按当前 revision 更新指定租户设置的覆盖值", "更新系统设置", "按当前 revision 更新指定租户设置的覆盖值", "系统设置已更新", "high", action.InputSchemaFor[settingUpdateRequest](), action.SchemaFor[SettingDefinitionView](), "设置", "更新", "配置")},
		{ActionSettingReset, "/api/v1/system/settings/reset", "重置设置", "删除租户覆盖值并恢复代码默认值", svc.Reset, writeAIMeta("按当前 revision 删除指定租户设置覆盖并恢复代码默认值", "重置系统设置", "按当前 revision 删除指定租户设置覆盖并恢复代码默认值", "系统设置已恢复默认值", "high", action.InputSchemaFor[settingResetRequest](), action.SchemaFor[SettingDefinitionView](), "设置", "重置", "默认值")},
	}
	for _, definition := range definitions {
		definition := definition
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.setting_management", PermissionGroupName: "设置管理",
				PermissionGroupNameKey: "permission_group.system.setting_management",
				Description:            definition.description, DescriptionKey: "action." + definition.id + ".description",
				AI:    definition.ai,
				Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
