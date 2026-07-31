package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	frameworkruntime "github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/modelruntime"
	"gorm.io/gorm"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func statusOK(v string) bool        { return v == "enabled" || v == "disabled" }
func marshal(v any) json.RawMessage { b, _ := json.Marshal(v); return json.RawMessage(b) }
func dbError(err error, what string) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "duplicate") || strings.Contains(s, "unique") {
		return errs.New(errs.CodeConflict, what+" already exists")
	}
	return errs.Wrap(errs.CodeInternal, what+" failed", err)
}

// Deprecated: new code should import modules/ai/modelruntime directly.
type (
	Service                 = modelruntime.Service
	ListRequest             = modelruntime.ListRequest
	IDRequest               = modelruntime.IDRequest
	ProviderCreateRequest   = modelruntime.ProviderCreateRequest
	ProviderUpdateRequest   = modelruntime.ProviderUpdateRequest
	StatusRequest           = modelruntime.StatusRequest
	DiscoverRequest         = modelruntime.DiscoverRequest
	DeploymentCreateRequest = modelruntime.DeploymentCreateRequest
	DeploymentUpdateRequest = modelruntime.DeploymentUpdateRequest
	ImportRequest           = modelruntime.ImportRequest
	AliasUpsertRequest      = modelruntime.AliasUpsertRequest
	AliasDeleteRequest      = modelruntime.AliasDeleteRequest
	AliasListRequest        = modelruntime.AliasListRequest
	InvocationService       = modelruntime.InvocationService
	ToolProjector           = modelruntime.ToolProjector
	RuntimeBudgetProvider   = modelruntime.RuntimeBudgetProvider
)

const (
	SettingKeyAgentRuntimeBudget       = modelruntime.SettingKeyAgentRuntimeBudget
	RuntimeBudgetSourceTenant          = modelruntime.RuntimeBudgetSourceTenant
	RuntimeBudgetSourceBuiltinFallback = modelruntime.RuntimeBudgetSourceBuiltinFallback
	invocationFinishedEvent            = modelruntime.InvocationFinishedEvent
)

type runtimeBudgetSettingProvider interface {
	Get(context.Context, string, string) (*setting.Effective, error)
}

// Deprecated: use modelruntime.NewService.
func NewService(db *gorm.DB, types *aimodel.ProviderTypeRegistry, credentials aimodel.CredentialCipher) *Service {
	return modelruntime.NewService(db, types, credentials)
}

// Deprecated: use modelruntime.NewInvocationService.
func NewInvocationService(db *gorm.DB, resolver aimodel.Resolver, adapters *frameworkruntime.AdapterRegistry, writer outbox.Writer) *InvocationService {
	return modelruntime.NewInvocationService(db, resolver, adapters, writer)
}

// Deprecated: use modelruntime.NewRuntimeBudgetProvider.
func NewRuntimeBudgetProvider(settings runtimeBudgetSettingProvider) *RuntimeBudgetProvider {
	return modelruntime.NewRuntimeBudgetProvider(settings)
}
