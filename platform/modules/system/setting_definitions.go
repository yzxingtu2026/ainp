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
	if err := registerPreferencesSetting(a); err != nil {
		return err
	}
	return registerVisibilityDemoSetting(a)
}

func registerPreferencesSetting(a *app.App) error {
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
				VisibleWhen: &setting.VisibleWhen{Field: "dark_mode", Eq: true},
			},
			{
				Key: "notes", Type: setting.FieldTextarea,
				Label: "备注", LabelKey: "setting.field.general.preferences.notes.label",
				Default: "", MaxLength: intPtr(200),
			},
		},
	})
}

// registerVisibilityDemoSetting 注册专用于验收 VisibleWhen（eq / in / 传递依赖）的测试设置组。
func registerVisibilityDemoSetting(a *app.App) error {
	return a.RegisterSettingDefinition(setting.Definition{
		Key: "general.visibility_demo", CategoryKey: "general",
		Title: "显隐联调", TitleKey: "setting.definition.general.visibility_demo.title",
		Description: "用于验收 VisibleWhen：布尔 eq、select in、以及传递隐藏", DescriptionKey: "setting.definition.general.visibility_demo.description",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant,
		Permission: ActionSettingList, Order: 20, SchemaVersion: 1,
		Fields: []setting.Field{
			{
				Key: "feature_enabled", Type: setting.FieldBoolean,
				Label: "启用实验功能", LabelKey: "setting.field.general.visibility_demo.feature_enabled.label",
				Description: "总开关：关闭后其子字段全部隐藏", DescriptionKey: "setting.field.general.visibility_demo.feature_enabled.description",
				Default: false,
			},
			{
				Key: "feature_mode", Type: setting.FieldSelect,
				Label: "功能模式", LabelKey: "setting.field.general.visibility_demo.feature_mode.label",
				Description: "eq：仅当启用实验功能时显示", DescriptionKey: "setting.field.general.visibility_demo.feature_mode.description",
				Required: true, Default: "basic",
				Options: []setting.FieldOption{
					{Value: "basic", Label: "基础", LabelKey: "setting.option.visibility_demo.mode.basic"},
					{Value: "advanced", Label: "高级", LabelKey: "setting.option.visibility_demo.mode.advanced"},
					{Value: "custom", Label: "自定义", LabelKey: "setting.option.visibility_demo.mode.custom"},
				},
				VisibleWhen: &setting.VisibleWhen{Field: "feature_enabled", Eq: true},
			},
			{
				Key: "advanced_tip", Type: setting.FieldTextarea,
				Label: "高级说明", LabelKey: "setting.field.general.visibility_demo.advanced_tip.label",
				Description: "in：模式为 advanced 或 custom 时显示", DescriptionKey: "setting.field.general.visibility_demo.advanced_tip.description",
				Default: "", MaxLength: intPtr(200),
				VisibleWhen: &setting.VisibleWhen{Field: "feature_mode", In: []any{"advanced", "custom"}},
			},
			{
				Key: "custom_webhook", Type: setting.FieldText,
				Label: "自定义 Webhook", LabelKey: "setting.field.general.visibility_demo.custom_webhook.label",
				Description: "eq：模式为 custom 时显示且必填（可验证隐藏时跳过必填）", DescriptionKey: "setting.field.general.visibility_demo.custom_webhook.description",
				Required: true, Default: "https://example.com/hook", MinLength: intPtr(8), MaxLength: intPtr(256),
				VisibleWhen: &setting.VisibleWhen{Field: "feature_mode", Eq: "custom"},
			},
			{
				Key: "custom_secret_hint", Type: setting.FieldText,
				Label: "密钥提示", LabelKey: "setting.field.general.visibility_demo.custom_secret_hint.label",
				Description: "传递依赖：依赖 custom 模式（其父级总开关关闭时一并隐藏）", DescriptionKey: "setting.field.general.visibility_demo.custom_secret_hint.description",
				Default: "", MaxLength: intPtr(64),
				VisibleWhen: &setting.VisibleWhen{Field: "feature_mode", Eq: "custom"},
			},
		},
	})
}

func intPtr(v int) *int { return &v }
