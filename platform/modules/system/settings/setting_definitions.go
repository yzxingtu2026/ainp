package settings

import (
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

func RegisterExampleSettings(a *app.App) error {
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
	if err := registerVisibilityDemoSetting(a); err != nil {
		return err
	}
	if err := registerAttachmentUploadPolicySetting(a); err != nil {
		return err
	}
	if err := registerMemoryPolicySetting(a); err != nil {
		return err
	}
	if err := registerAgentRuntimeBudgetSetting(a); err != nil {
		return err
	}
	if err := registerToolSearchSetting(a); err != nil {
		return err
	}
	if err := registerNavigationSearchSetting(a); err != nil {
		return err
	}
	return registerAssistantPersonalizationSetting(a)
}

func registerAgentRuntimeBudgetSetting(a *app.App) error {
	d := aiagentruntime.SystemDefaultBudget()
	minSteps, maxSteps := 1.0, 100.0
	minModelCalls, maxModelCalls := 1.0, 32.0
	minToolCalls, maxToolCalls := 0.0, 64.0
	minCompactions, maxCompactions := 0.0, 3.0
	minDuration, maxDuration := 10.0, 1800.0
	minTokens, maxTokens := 1.0, 10000000.0
	step := 1.0
	return a.RegisterSettingDefinition(setting.Definition{
		Key: "ai.agent_runtime_budget", CategoryKey: "ai",
		Title: "Agent 运行预算", Description: "控制单次 Agent Run 的步骤、调用、时长与累计 Token 硬上限",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 50, SchemaVersion: 1,
		Fields: []setting.Field{
			{Key: "max_steps", Type: setting.FieldNumber, Label: "最大步骤数", Required: true, Default: float64(d.MaxSteps), Min: &minSteps, Max: &maxSteps, Step: &step},
			{Key: "max_model_calls", Type: setting.FieldNumber, Label: "最大模型调用次数", Required: true, Default: float64(d.MaxModelCalls), Min: &minModelCalls, Max: &maxModelCalls, Step: &step},
			{Key: "max_tool_calls", Type: setting.FieldNumber, Label: "最大工具调用次数", Required: true, Default: float64(d.MaxToolCalls), Min: &minToolCalls, Max: &maxToolCalls, Step: &step},
			{Key: "max_compactions", Type: setting.FieldNumber, Label: "最大上下文压缩次数", Required: true, Default: float64(d.MaxCompactions), Min: &minCompactions, Max: &maxCompactions, Step: &step},
			{Key: "max_duration_seconds", Type: setting.FieldNumber, Label: "最长运行时间（秒）", Required: true, Default: float64(d.MaxDurationSeconds), Min: &minDuration, Max: &maxDuration, Step: &step},
			{Key: "run_token_budget", Type: setting.FieldNumber, Label: "累计 Token 预算", Required: true, Default: float64(d.RunTokenBudget), Min: &minTokens, Max: &maxTokens, Step: &step},
		},
	})
}

func registerToolSearchSetting(a *app.App) error {
	return a.RegisterSettingDefinition(setting.Definition{Key: aitool.SettingKeySearch, CategoryKey: "ai", Title: "工具语义检索", TitleKey: "setting.definition.ai.tool_search.title", Description: "配置工具目录和查询使用的 Embedding Model Alias；留空时降级为词法检索", DescriptionKey: "setting.definition.ai.tool_search.description", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 20, SchemaVersion: 1, Fields: []setting.Field{{Key: aitool.SettingFieldModelAlias, Type: setting.FieldRemoteSelect, Label: "Embedding 模型 Alias", LabelKey: "setting.field.ai.tool_search.embedding_model_alias.label", Default: "", RemoteSource: aliasRemoteSource("embedding")}}})
}

func registerNavigationSearchSetting(a *app.App) error {
	return a.RegisterSettingDefinition(setting.Definition{Key: menu.SettingKeySemanticSearch, CategoryKey: "ai", Title: "导航语义检索", Description: "配置功能导航索引和查询使用的 Embedding Model Alias；留空时无损降级为词法检索", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 30, SchemaVersion: 1, Fields: []setting.Field{{Key: menu.SettingFieldModelAlias, Type: setting.FieldRemoteSelect, Label: "Embedding 模型 Alias", Default: "", RemoteSource: aliasRemoteSource("embedding")}}})
}

func registerAssistantPersonalizationSetting(a *app.App) error {
	return a.RegisterSettingDefinition(setting.Definition{Key: "ai.assistant_personalization", CategoryKey: "ai", Title: "个性化欢迎语", Description: "为首页、AI 工作台和全局助手异步生成统一欢迎语与推荐问题", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 40, SchemaVersion: 1, Fields: []setting.Field{
		{Key: "enabled", Type: setting.FieldBoolean, Label: "启用个性化欢迎语", Default: false},
		{Key: "model_alias", Type: setting.FieldRemoteSelect, Label: "助手个性化模型 Alias", Default: "", Required: true, RemoteSource: aliasRemoteSource("chat"), VisibleWhen: &setting.VisibleWhen{Field: "enabled", Eq: true}},
	}})
}

func aliasRemoteSource(modelType string) *setting.RemoteOptionSource {
	return &setting.RemoteOptionSource{Endpoint: "/api/v1/ai/aliases/list", Params: map[string]any{"model_type": modelType}, LabelField: "code", ValueField: "code"}
}

func registerMemoryPolicySetting(a *app.App) error {
	if err := a.RegisterSettingCategory(setting.Category{Key: "ai", Title: "AI", Description: "AI 运行策略", Icon: "mdi:brain", Order: 30, Surface: auth.RealmTenant}); err != nil {
		return err
	}
	minOne, maxCandidates, maxBatch, maxRecall, minTokens, maxTokens := 1.0, 100.0, 1000.0, 100.0, 128.0, 100000.0
	d := aimemory.SystemDefaultPolicy()
	whenEnabled := func() *setting.VisibleWhen { return &setting.VisibleWhen{Field: "enabled", Eq: true} }
	return a.RegisterSettingDefinition(setting.Definition{Key: aimemory.SettingKeyPolicyDefaults, CategoryKey: "ai", Title: "长期记忆默认策略", Description: "Agent Version 未覆盖时使用的租户默认值", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 10, SchemaVersion: 1, Fields: []setting.Field{
		{Key: "enabled", Type: setting.FieldBoolean, Label: "启用长期记忆", Default: d.Enabled},
		{Key: "extraction_model_alias", Type: setting.FieldRemoteSelect, Label: "记忆处理模型 Alias", LabelKey: "setting.field.ai.memory_policy.extraction_model_alias.label", Default: d.ExtractionModelAlias, Required: true, RemoteSource: aliasRemoteSource("chat"), VisibleWhen: whenEnabled()},
		{Key: "consolidation_model_alias", Type: setting.FieldRemoteSelect, Label: "整理模型 Alias", LabelKey: "setting.field.ai.memory_policy.consolidation_model_alias.label", Default: "", RemoteSource: aliasRemoteSource("chat"), VisibleWhen: whenEnabled()},
		{Key: "auto_save_low_sensitive", Type: setting.FieldBoolean, Label: "低敏候选自动保存", Default: d.AutoSaveLowSensitive, VisibleWhen: whenEnabled()},
		{Key: "write_approval_mode", Type: setting.FieldSelect, Label: "写入审批", Default: string(d.WriteApprovalMode), Required: true, Options: []setting.FieldOption{{Value: "always", Label: "始终审批"}, {Value: "sensitive_only", Label: "仅敏感内容"}, {Value: "never", Label: "无需审批"}}, VisibleWhen: whenEnabled()},
		{Key: "agent_specific_scope", Type: setting.FieldBoolean, Label: "Agent 专属作用域", Default: false, VisibleWhen: whenEnabled()},
		{Key: "max_candidates_per_turn", Type: setting.FieldNumber, Label: "每轮候选上限", Default: float64(d.MaxCandidatesPerTurn), Min: &minOne, Max: &maxCandidates, VisibleWhen: whenEnabled()},
		{Key: "max_consolidation_items", Type: setting.FieldNumber, Label: "批量整理上限", Default: float64(d.MaxConsolidationItems), Min: &minOne, Max: &maxBatch, VisibleWhen: whenEnabled()},
		{Key: "max_recall_items", Type: setting.FieldNumber, Label: "召回条数上限", Default: float64(d.MaxRecallItems), Min: &minOne, Max: &maxRecall, VisibleWhen: whenEnabled()},
		{Key: "max_recall_tokens", Type: setting.FieldNumber, Label: "召回 Token 上限", Default: float64(d.MaxRecallTokens), Min: &minTokens, Max: &maxTokens, VisibleWhen: whenEnabled()},
		{Key: "policy_version", Type: setting.FieldText, Label: "策略版本", Default: d.PolicyVersion, Required: true, VisibleWhen: whenEnabled()},
		{Key: "prompt_version", Type: setting.FieldText, Label: "Prompt 版本", Default: d.PromptVersion, Required: true, VisibleWhen: whenEnabled()},
		{Key: "schema_version", Type: setting.FieldText, Label: "Schema 版本", Default: d.SchemaVersion, Required: true, VisibleWhen: whenEnabled()},
	}})
}

func registerAttachmentUploadPolicySetting(a *app.App) error {
	if err := a.RegisterSettingCategory(setting.Category{
		Key: "attachment", Title: "附件", TitleKey: "setting.category.attachment.title",
		Description: "附件上传与保留策略", DescriptionKey: "setting.category.attachment.description",
		Icon: "mdi:paperclip", Order: 20, Surface: auth.RealmTenant,
	}); err != nil {
		return err
	}
	minMB := 1.0
	maxMB := 200.0
	stepMB := 1.0
	minHours := 1.0
	maxHours := 168.0
	minTTL := 1.0
	maxTTL := 120.0
	minOnce := 1.0
	maxOnce := 50.0
	def := attachment.DefaultUploadPolicy()
	extOpts := make([]setting.FieldOption, 0, len(def.AllowedExt))
	for _, ext := range def.AllowedExt {
		extOpts = append(extOpts, setting.FieldOption{Value: ext, Label: ext})
	}
	mimeOpts := make([]setting.FieldOption, 0, len(def.AllowedMIME))
	for _, mime := range def.AllowedMIME {
		mimeOpts = append(mimeOpts, setting.FieldOption{Value: mime, Label: mime})
	}
	extDefaults := make([]any, 0, len(def.AllowedExt))
	for _, ext := range def.AllowedExt {
		extDefaults = append(extDefaults, ext)
	}
	mimeDefaults := make([]any, 0, len(def.AllowedMIME))
	for _, mime := range def.AllowedMIME {
		mimeDefaults = append(mimeDefaults, mime)
	}
	return a.RegisterSettingDefinition(setting.Definition{
		Key: SettingKeyAttachmentUploadPolicy, CategoryKey: "attachment",
		Title: "上传策略", TitleKey: "setting.definition.attachment.upload_policy.title",
		Description: "控制上传大小、类型与临时保留；代码默认值可在此覆盖", DescriptionKey: "setting.definition.attachment.upload_policy.description",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant,
		Permission: ActionSettingList, Order: 10, SchemaVersion: 1,
		Fields: []setting.Field{
			{
				Key: "max_size_mb", Type: setting.FieldNumber,
				Label: "最大文件大小 (MB)", LabelKey: "setting.field.attachment.upload_policy.max_size_mb.label",
				Required: true, Default: float64(def.MaxSizeBytes / (1024 * 1024)), Min: &minMB, Max: &maxMB, Step: &stepMB,
			},
			{
				Key: "allowed_ext", Type: setting.FieldMultiSelect,
				Label: "允许扩展名", LabelKey: "setting.field.attachment.upload_policy.allowed_ext.label",
				Required: true, Default: extDefaults, Options: extOpts,
				AllowCustomValues: true, CustomValuePattern: `^\.?[A-Za-z0-9][A-Za-z0-9._+-]{0,31}$`,
			},
			{
				Key: "allowed_mime", Type: setting.FieldMultiSelect,
				Label: "允许 MIME", LabelKey: "setting.field.attachment.upload_policy.allowed_mime.label",
				Required: true, Default: mimeDefaults, Options: mimeOpts,
				AllowCustomValues:  true,
				CustomValuePattern: `^[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,63}/[A-Za-z0-9][A-Za-z0-9!#$&^_.+-]{0,127}$`,
			},
			{
				Key: "temp_retain_hours", Type: setting.FieldNumber,
				Label: "临时保留 (小时)", LabelKey: "setting.field.attachment.upload_policy.temp_retain_hours.label",
				Required: true, Default: def.TempRetain.Hours(), Min: &minHours, Max: &maxHours, Step: &stepMB,
			},
			{
				Key: "transfer_ttl_minutes", Type: setting.FieldNumber,
				Label: "传输令牌有效期 (分钟)", LabelKey: "setting.field.attachment.upload_policy.transfer_ttl_minutes.label",
				Required: true, Default: def.TransferTTL.Minutes(), Min: &minTTL, Max: &maxTTL, Step: &stepMB,
			},
			{
				Key: "max_uploads_once", Type: setting.FieldNumber,
				Label: "单次最多上传数", LabelKey: "setting.field.attachment.upload_policy.max_uploads_once.label",
				Required: true, Default: float64(def.MaxUploadsOnce), Min: &minOnce, Max: &maxOnce, Step: &stepMB,
			},
		},
	})
}

func intPtr(v int) *int { return &v }

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
				Default:     "#18A058",
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
