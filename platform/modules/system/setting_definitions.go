package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

func registerExampleSettings(a *app.App) error {
	if err := a.RegisterSettingCategory(setting.Category{
		Key: "general", Title: "通用", TitleKey: "setting.category.general.title",
		Description: "通用业务偏好", DescriptionKey: "setting.category.general.description",
		Icon: "mdi:cog-outline", Order: 10, Surface: auth.RealmTenant,
	}); err != nil {
		return err
	}

	minLen := 1
	maxLen := 64
	minPage := 5.0
	maxPage := 100.0
	step := 5.0
	return a.RegisterSettingDefinition(setting.Definition{
		Key: "general.preferences", CategoryKey: "general",
		Title: "偏好设置", TitleKey: "setting.definition.general.preferences.title",
		Description: "租户界面与列表偏好", DescriptionKey: "setting.definition.general.preferences.description",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant,
		Permission: ActionSettingList, Order: 10, SchemaVersion: 1,
		Fields: []setting.Field{
			{
				Key: "display_name", Type: setting.FieldText,
				Label: "显示名称", LabelKey: "setting.field.general.preferences.display_name.label",
				Required: true, Default: "AINP", MinLength: &minLen, MaxLength: &maxLen,
			},
			{
				Key: "page_size", Type: setting.FieldNumber,
				Label: "默认分页大小", LabelKey: "setting.field.general.preferences.page_size.label",
				Required: true, Default: 20.0, Min: &minPage, Max: &maxPage, Step: &step,
			},
			{
				Key: "dark_mode", Type: setting.FieldBoolean,
				Label: "深色模式", LabelKey: "setting.field.general.preferences.dark_mode.label",
				Default: false,
			},
			{
				Key: "language", Type: setting.FieldSelect,
				Label: "默认语言", LabelKey: "setting.field.general.preferences.language.label",
				Required: true, Default: "zh-CN",
				Options: []setting.FieldOption{
					{Value: "zh-CN", Label: "中文", LabelKey: "setting.option.language.zh_cn"},
					{Value: "en-US", Label: "English", LabelKey: "setting.option.language.en_us"},
				},
			},
			{
				Key: "tags", Type: setting.FieldMultiSelect,
				Label: "标签", LabelKey: "setting.field.general.preferences.tags.label",
				Default: []any{"ops"},
				Options: []setting.FieldOption{
					{Value: "ops", Label: "运维", LabelKey: "setting.option.tags.ops"},
					{Value: "biz", Label: "业务", LabelKey: "setting.option.tags.biz"},
					{Value: "dev", Label: "研发", LabelKey: "setting.option.tags.dev"},
				},
			},
			{
				Key: "brand_color", Type: setting.FieldColor,
				Label: "品牌色", LabelKey: "setting.field.general.preferences.brand_color.label",
				Default: "#18A058",
			},
			{
				Key: "notes", Type: setting.FieldTextarea,
				Label: "备注", LabelKey: "setting.field.general.preferences.notes.label",
				Default: "", MaxLength: intPtr(200),
			},
		},
	})
}

func intPtr(v int) *int { return &v }
