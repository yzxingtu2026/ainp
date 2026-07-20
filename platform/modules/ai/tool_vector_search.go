package ai

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

const toolEmbeddingSchemaVersion = 1

var ErrToolEmbeddingSuperseded = errors.New("tool embedding request is superseded")

func (s *ToolService) embeddingAlias(ctx context.Context) (string, error) {
	if s.settings == nil {
		return "", nil
	}
	ac := auth.MustFromContext(ctx)
	effective, err := s.settings.Get(ctx, ac.TenantID, aitool.SettingKeySearch)
	if err != nil {
		return "", err
	}
	alias, _ := effective.Values[aitool.SettingFieldModelAlias].(string)
	return strings.TrimSpace(alias), nil
}

func (s *ToolService) IndexCatalogDocument(ctx context.Context, toolID string, revision uint64, modelAlias string) error {
	if s.embeddings == nil {
		return errors.New("tool embedding runtime is unavailable")
	}
	currentAlias, err := s.embeddingAlias(ctx)
	if err != nil {
		return err
	}
	if currentAlias != modelAlias {
		_ = s.markCatalogDocumentFailed(ctx, toolID, revision)
		return ErrToolEmbeddingSuperseded
	}
	var catalog ToolCatalog
	if err := s.db.WithContext(ctx).Where("tool_id=?", toolID).First(&catalog).Error; err != nil {
		return err
	}
	if catalog.Revision != revision || catalog.Status != "enabled" {
		return ErrToolEmbeddingSuperseded
	}
	var document ToolSearchDocument
	if err := s.db.WithContext(ctx).Where("tool_id=? AND catalog_revision=?", toolID, revision).First(&document).Error; err != nil {
		return err
	}
	response, err := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: modelAlias, Inputs: []aimodelruntime.EmbeddingItem{{ID: toolID, Text: document.SearchText}}, Purpose: aimodelruntime.EmbeddingPurposeToolIndex, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm})
	if err != nil {
		_ = s.markCatalogDocumentFailed(ctx, toolID, revision)
		return err
	}
	if response == nil || response.Dimensions <= 0 || response.Model.DeploymentID == "" || len(response.Vectors) != 1 || response.Vectors[0].ID != toolID || len(response.Vectors[0].Vector) != response.Dimensions || response.PreprocessingVersion != aimodelruntime.EmbeddingPreprocessingVersionCosine {
		_ = s.markCatalogDocumentFailed(ctx, toolID, revision)
		return errors.New("tool embedding provenance is invalid")
	}
	now := time.Now().UTC()
	vector := PGVector(response.Vectors[0].Vector)
	updates := map[string]any{"embedding_alias": modelAlias, "embedding_model": response.Model.RemoteModelID, "embedding_deployment_id": response.Model.DeploymentID, "embedding_dimensions": response.Dimensions, "embedding_preprocessing": response.PreprocessingVersion, "embedding_schema_version": toolEmbeddingSchemaVersion, "embedding_vector": &vector, "vector_updated_at": now, "index_status": "ready"}
	result := s.db.WithContext(ctx).Model(&ToolSearchDocument{}).Where("tool_id=? AND catalog_revision=? AND index_status IN ?", toolID, revision, []string{"pending", "failed"}).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrToolEmbeddingSuperseded
	}
	return nil
}

func (s *ToolService) markCatalogDocumentFailed(ctx context.Context, toolID string, revision uint64) error {
	return s.db.WithContext(ctx).Model(&ToolSearchDocument{}).Where("tool_id=? AND catalog_revision=? AND index_status=?", toolID, revision, "pending").Update("index_status", "failed").Error
}

type toolVectorCandidate struct {
	ToolID string  `gorm:"column:tool_id"`
	Score  float64 `gorm:"column:score"`
}

func (s *ToolService) mergeHybrid(ctx context.Context, ac *auth.Context, scope *ToolScope, request ToolSearchRequest, lexical []aitool.Candidate) ([]aitool.Candidate, bool, string) {
	lexical = s.mergeFTS(ctx, ac, scope, request, lexical)
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return lexical, false, ""
	}
	alias, err := s.embeddingAlias(ctx)
	if err != nil {
		return lexical, true, "embedding_setting"
	}
	if alias == "" {
		return lexical, false, ""
	}
	if s.embeddings == nil {
		return lexical, true, "embedding_runtime"
	}
	response, err := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: alias, Inputs: []aimodelruntime.EmbeddingItem{{ID: "query", Text: query}}, Purpose: aimodelruntime.EmbeddingPurposeToolIndex, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm})
	if err != nil || response == nil || response.Dimensions <= 0 || response.Model.DeploymentID == "" || len(response.Vectors) != 1 || len(response.Vectors[0].Vector) != response.Dimensions || response.PreprocessingVersion != aimodelruntime.EmbeddingPreprocessingVersionCosine {
		return lexical, true, "embedding_runtime"
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return lexical, true, "vector_index"
	}
	rows, err := tenantdata.TenantRaw[toolVectorCandidate](tdb, `WITH trusted_tenant AS (SELECT ?::text tenant_id)
SELECT d.tool_id, 1-(d.embedding_vector OPERATOR(public.<=>) ?::public.vector) AS score
FROM ai_tool_search_documents d CROSS JOIN trusted_tenant tt
WHERE d.tenant_id=tt.tenant_id AND d.index_status='ready' AND d.embedding_vector IS NOT NULL
  AND embedding_alias=? AND embedding_deployment_id=? AND embedding_dimensions=?
  AND embedding_preprocessing=? AND embedding_schema_version=?
ORDER BY embedding_vector OPERATOR(public.<=>) ?::public.vector, tool_id ASC LIMIT 20`, PGVector(response.Vectors[0].Vector), alias, response.Model.DeploymentID, response.Dimensions, response.PreprocessingVersion, toolEmbeddingSchemaVersion, PGVector(response.Vectors[0].Vector))
	if err != nil {
		return lexical, true, "vector_index"
	}
	if len(rows) == 0 {
		return lexical, true, "vector_index"
	}
	return s.fuseToolCandidates(ctx, ac, scope, request, lexical, rows), false, ""
}

func (s *ToolService) fuseToolCandidates(ctx context.Context, ac *auth.Context, scope *ToolScope, request ToolSearchRequest, lexical []aitool.Candidate, vectors []toolVectorCandidate) []aitool.Candidate {
	byID := make(map[string]aitool.Candidate, len(lexical)+len(vectors))
	scores := make(map[string]float64, len(lexical)+len(vectors))
	for rank, candidate := range lexical {
		byID[candidate.ID] = candidate
		scores[candidate.ID] += 1 / float64(60+rank+1)
		if candidate.Score >= 1 {
			scores[candidate.ID] += 1
		}
	}
	for rank, vector := range vectors {
		if vector.Score <= 0 {
			continue
		}
		definition, ok := s.registry.Get(vector.ToolID)
		if !ok || definition.Status != "enabled" || !scopeAllows(scope, definition) || !toolVisible(ac, definition) {
			continue
		}
		projected, visible, err := s.project(ctx, scope, definition)
		if err != nil || !visible {
			continue
		}
		candidate := byID[vector.ToolID]
		if candidate.ID == "" {
			items, _ := aitool.Search(ctx, []aitool.Definition{projected}, "", request.Capability, request.Source, 1, func(aitool.Definition) bool { return true })
			if len(items) == 0 {
				continue
			}
			candidate = items[0]
			byID[candidate.ID] = candidate
		}
		scores[candidate.ID] += 1 / float64(60+rank+1)
	}
	items := make([]aitool.Candidate, 0, len(byID))
	for id, candidate := range byID {
		candidate.Score = scores[id]
		items = append(items, candidate)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].ID < items[j].ID
		}
		return items[i].Score > items[j].Score
	})
	limit := request.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
