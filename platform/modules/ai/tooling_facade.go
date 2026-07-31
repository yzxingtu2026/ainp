package ai

import (
	"encoding/json"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"gorm.io/gorm"
	"time"
)

// Deprecated: new code should import modules/ai/tooling directly.
type (
	ToolActivationResult        = tooling.ToolActivationResult
	ToolService                 = tooling.ToolService
	ToolSearchRequest           = tooling.ToolSearchRequest
	ToolSearchResponse          = tooling.ToolSearchResponse
	ToolLoadRequest             = tooling.ToolLoadRequest
	ToolLoadResponse            = tooling.ToolLoadResponse
	ToolExecuteRequest          = tooling.ToolExecuteRequest
	ToolConfirmationRequest     = tooling.ToolConfirmationRequest
	ToolConfirmationListRequest = tooling.ToolConfirmationListRequest
	ToolCatalogReconcileResult  = tooling.ToolCatalogReconcileResult
	ToolIndexStatus             = tooling.ToolIndexStatus
	ToolCatalogChangedPayload   = tooling.ToolCatalogChangedPayload
)

const ToolCatalogEmbeddingJobType = tooling.ToolCatalogEmbeddingJobType

const (
	toolCatalogChangedEvent     = tooling.ToolCatalogChangedEvent
	toolInvocationFinishedEvent = tooling.ToolInvocationFinishedEvent
)

var ErrToolEmbeddingSuperseded = tooling.ErrToolEmbeddingSuperseded

type referenceEvidence struct {
	ResolverID      string    `json:"resolver_id"`
	DiscoveryToolID string    `json:"discovery_tool_id"`
	Version         string    `json:"version"`
	InvocationID    string    `json:"invocation_id"`
	ReadAt          time.Time `json:"read_at"`
}

func jsonBytes(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func modelToolName(id string) string  { return tooling.ModelToolName(id) }
func registerToolTenantModels() error { return tooling.RegisterTenantModels() }

// Deprecated: use tooling.NewToolService.
func NewToolService(db *gorm.DB, registry *aitool.Registry, signer *aitool.HandleSigner, writer outbox.Writer) *ToolService {
	return tooling.NewToolService(db, registry, signer, writer)
}

// Deprecated: use tooling.RegisterToolCatalogEmbedding.
func RegisterToolCatalogEmbedding(a *app.App, tools *ToolService) error {
	return tooling.RegisterToolCatalogEmbedding(a, tools)
}
