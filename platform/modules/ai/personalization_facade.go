package ai

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/experience/personalization"
	"gorm.io/gorm"
)

type (
	AssistantPersonalizationSnapshot = personalization.AssistantPersonalizationSnapshot
	PersonalizationSuggestion        = personalization.PersonalizationSuggestion
	PersonalizationResponse          = personalization.PersonalizationResponse
	PersonalizationService           = personalization.PersonalizationService
)

const (
	AssistantPersonalizationSettingKey = personalization.AssistantPersonalizationSettingKey
	AssistantPersonalizationEvent      = personalization.AssistantPersonalizationEvent
	AssistantPersonalizationJobType    = personalization.AssistantPersonalizationJobType
	ActionPersonalizationGet           = personalization.ActionPersonalizationGet
	ActionPersonalizationRebuild       = personalization.ActionPersonalizationRebuild
)

type personalizationGenerator interface {
	Generate(context.Context, aimodelruntime.GenerateRequest) (any, error)
}

func NewPersonalizationService(db *gorm.DB, settings *setting.Provider, writer outbox.Writer, generator personalizationGenerator) *PersonalizationService {
	return personalization.NewPersonalizationService(db, settings, writer, generator)
}

func RegisterPersonalization(a *app.App, service *PersonalizationService) error {
	return personalization.RegisterPersonalization(a, service)
}
