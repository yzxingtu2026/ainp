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
	}{
		{ActionSettingList, "/api/v1/system/settings/list", "设置列表", "按分类返回当前租户可见设置元数据与有效值", svc.List},
		{ActionSettingGet, "/api/v1/system/settings/get", "设置详情", "获取单个设置组详情与有效值", svc.Get},
		{ActionSettingUpdate, "/api/v1/system/settings/update", "更新设置", "保存租户设置覆盖值", svc.Update},
		{ActionSettingReset, "/api/v1/system/settings/reset", "重置设置", "删除租户覆盖值并恢复代码默认值", svc.Reset},
	}
	for _, definition := range definitions {
		definition := definition
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.setting_management", PermissionGroupName: "设置管理",
				PermissionGroupNameKey: "permission_group.system.setting_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
