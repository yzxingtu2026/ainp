package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type knowledgeToolCandidate struct {
	ID             string
	Name           string
	Description    string
	IndexVersionID string
}

type knowledgeToolCacheEntry struct {
	Response aiknowledge.RetrievalResponse
}

type toolScopeProjection struct {
	ID, TenantID, PrincipalID string
	AgentRunID                *string
	AuthVersion               uint64
}
type runAgentProjection struct{ AgentVersionID string }
type versionAgentProjection struct{ AgentID string }

type KnowledgeTool struct {
	db        tenantdata.GuardedDatabase
	access    aiknowledge.Service
	retriever aiknowledge.KnowledgeRetriever
	mu        sync.Mutex
	cache     map[string]knowledgeToolCacheEntry
}

func NewKnowledgeTool(db *gorm.DB, access aiknowledge.Service, retriever aiknowledge.KnowledgeRetriever) *KnowledgeTool {
	return &KnowledgeTool{db: tenantdata.GuardedDB(db), access: access, retriever: retriever, cache: map[string]knowledgeToolCacheEntry{}}
}

func (t *KnowledgeTool) Project(ctx context.Context, runtimeScope aitool.RuntimeScope, definition aitool.Definition) (aitool.Definition, bool, error) {
	if runtimeScope.ID == "" || runtimeScope.AgentRunID == "" {
		return definition, false, nil
	}
	tdb, err := t.db.Tenant(ctx)
	if err != nil {
		return definition, false, err
	}
	rows, err := tenantdata.TenantRaw[toolScopeProjection](tdb, `SELECT id,tenant_id,principal_id,agent_run_id,auth_version FROM ai_tool_scopes WHERE tenant_id=? AND id=? AND principal_id=? LIMIT 1`, runtimeScope.ID, runtimeScope.PrincipalID)
	if err != nil || len(rows) != 1 {
		return definition, false, err
	}
	scope := rows[0]
	candidates, err := t.candidates(ctx, &scope)
	if err != nil || len(candidates) == 0 {
		return definition, false, err
	}
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		label := candidate.Name + " (id: " + candidate.ID + ")"
		if candidate.Description != "" {
			label += ": " + candidate.Description
		}
		parts = append(parts, label)
	}
	definition.DescriptionForLLM += " Available candidates: " + strings.Join(parts, "; ") + ". Omit knowledge_base_ids to search all candidates; supplied IDs can only narrow this list."
	return definition, true, nil
}

func (t *KnowledgeTool) Execute(ctx context.Context, _ aitool.LoadedTool, arguments json.RawMessage) (*aitool.Result, error) {
	started := time.Now()
	scopeID := aitool.ScopeIDFromContext(ctx)
	if scopeID == "" {
		return nil, fmt.Errorf("knowledge search requires an agent run scope")
	}
	tdb, queryErr := t.db.Tenant(ctx)
	if queryErr != nil {
		return nil, queryErr
	}
	rows, queryErr := tenantdata.TenantRaw[toolScopeProjection](tdb, `SELECT id,tenant_id,principal_id,agent_run_id,auth_version FROM ai_tool_scopes WHERE tenant_id=? AND id=? LIMIT 1`, scopeID)
	if queryErr != nil || len(rows) != 1 || rows[0].AgentRunID == nil {
		return nil, fmt.Errorf("knowledge search requires an agent run scope")
	}
	scope := rows[0]
	var input aitool.KnowledgeSearchInput
	if json.Unmarshal(arguments, &input) != nil || strings.TrimSpace(input.Query) == "" || len(input.Query) > 4096 {
		return &aitool.Result{Status: "failed", Summary: "knowledge search input is invalid", ErrorCategory: "invalid_request"}, nil
	}
	candidates, err := t.candidates(ctx, &scope)
	if err != nil {
		return knowledgeFailure("authorization_changed", started), nil
	}
	allowed := make(map[string]knowledgeToolCandidate, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.ID] = candidate
	}
	selected := stableUnique(input.KnowledgeBaseIDs)
	if len(selected) == 0 {
		for id := range allowed {
			selected = append(selected, id)
		}
		sort.Strings(selected)
	} else {
		for _, id := range selected {
			if _, ok := allowed[id]; !ok {
				return knowledgeFailure("knowledge_scope_denied", started), nil
			}
		}
	}
	if len(selected) == 0 {
		return knowledgeEmptyResult(0, time.Since(started), false), nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	request := aiknowledge.RetrievalRequest{KnowledgeBaseIDs: selected, DocumentIDs: stableUnique(input.DocumentIDs), Query: strings.TrimSpace(input.Query), Config: aiknowledge.RetrievalConfig{TopK: limit, MinScore: 0.1, MaxContextTokens: 4000}}
	key := t.cacheKey(&scope, candidates, request)
	t.mu.Lock()
	cached, hit := t.cache[key]
	t.mu.Unlock()
	response := cached.Response
	if !hit {
		if rich, ok := t.retriever.(aiknowledge.KnowledgeRetrieverWithMetadata); ok {
			response, err = rich.RetrieveWithMetadata(ctx, request)
		} else {
			response.Items, err = t.retriever.Retrieve(ctx, request)
			response.Metadata.CandidateCount = len(response.Items)
		}
		if err != nil {
			category := "retriever_failed"
			if strings.Contains(err.Error(), "knowledge embedding") {
				category = "embedding_provenance"
			}
			return knowledgeFailure(category, started), nil
		}
		t.mu.Lock()
		if len(t.cache) >= 512 {
			t.cache = map[string]knowledgeToolCacheEntry{}
		}
		t.cache[key] = knowledgeToolCacheEntry{Response: response}
		t.mu.Unlock()
	}
	if len(response.Items) == 0 {
		return knowledgeEmptyResult(response.Metadata.CandidateCount, time.Since(started), hit), nil
	}
	citations := make([]aiknowledge.Citation, 0, len(response.Items))
	for _, item := range response.Items {
		citations = append(citations, item.Citation)
	}
	data := map[string]any{
		"references": aiknowledge.FormatUntrustedReferences(response.Items), "citations": citations,
		"candidate_count": response.Metadata.CandidateCount, "returned_count": len(response.Items),
		"duration_ms": time.Since(started).Milliseconds(), "degraded": response.Metadata.Degraded,
		"degrade_reason": response.Metadata.DegradeReason, "cache_hit": hit,
	}
	return &aitool.Result{Status: "completed", Summary: fmt.Sprintf("knowledge search returned %d references", len(response.Items)), Data: data, ResourceRefs: citationRefs(citations)}, nil
}

func (t *KnowledgeTool) candidates(ctx context.Context, scope *toolScopeProjection) ([]knowledgeToolCandidate, error) {
	if scope == nil || scope.AgentRunID == nil {
		return nil, nil
	}
	tdb, err := t.db.Tenant(ctx)
	if err != nil {
		return nil, err
	}
	runs, err := tenantdata.TenantRaw[runAgentProjection](tdb, `SELECT agent_version_id FROM ai_agent_runs WHERE tenant_id=? AND user_id=? AND id=? LIMIT 1`, scope.PrincipalID, *scope.AgentRunID)
	if err != nil || len(runs) != 1 {
		if err != nil {
			return nil, err
		}
		return nil, gorm.ErrRecordNotFound
	}
	versions, err := tenantdata.TenantRaw[versionAgentProjection](tdb, `SELECT agent_id FROM ai_agent_versions WHERE tenant_id=? AND id=? LIMIT 1`, runs[0].AgentVersionID)
	if err != nil || len(versions) != 1 {
		if err != nil {
			return nil, err
		}
		return nil, gorm.ErrRecordNotFound
	}
	rows, err := tenantdata.TenantRaw[knowledgeToolCandidate](tdb, `SELECT kb.id,kb.name,kb.description,iv.id AS index_version_id
FROM ai_agent_knowledge_bindings b
JOIN ai_knowledge_bases kb ON kb.tenant_id=b.tenant_id AND kb.id=b.knowledge_base_id AND kb.deleted_at IS NULL
JOIN ai_knowledge_index_versions iv ON iv.tenant_id=kb.tenant_id AND iv.id=kb.current_index_version_id AND iv.knowledge_base_id=kb.id
WHERE b.tenant_id=? AND b.agent_id=? AND kb.status=? AND iv.status=? ORDER BY kb.id`,
		versions[0].AgentID, aiknowledge.KnowledgeBaseEnabled, aiknowledge.IndexReady)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, row := range rows {
		if t.access.CanReadKnowledgeBase(ctx, row.ID) == nil {
			out = append(out, row)
		}
	}
	return out, nil
}

func (t *KnowledgeTool) cacheKey(scope *toolScopeProjection, candidates []knowledgeToolCandidate, request aiknowledge.RetrievalRequest) string {
	versions := make([]string, 0, len(candidates))
	selected := make(map[string]bool, len(request.KnowledgeBaseIDs))
	for _, id := range request.KnowledgeBaseIDs {
		selected[id] = true
	}
	for _, candidate := range candidates {
		if selected[candidate.ID] {
			versions = append(versions, candidate.ID+":"+candidate.IndexVersionID)
		}
	}
	payload, _ := json.Marshal([]any{scope.TenantID, scope.PrincipalID, scope.AuthVersion, versions, request.Query, request.DocumentIDs, request.Config})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func knowledgeFailure(category string, started time.Time) *aitool.Result {
	return &aitool.Result{Status: "failed", Summary: "knowledge search unavailable", ErrorCategory: category, Data: map[string]any{"returned_count": 0, "duration_ms": time.Since(started).Milliseconds(), "error_category": category}}
}

func knowledgeEmptyResult(candidates int, duration time.Duration, cacheHit bool) *aitool.Result {
	return &aitool.Result{Status: "completed", Summary: "knowledge search returned no references", Data: map[string]any{"references": "", "citations": []any{}, "candidate_count": candidates, "returned_count": 0, "duration_ms": duration.Milliseconds(), "degraded": false, "cache_hit": cacheHit}}
}

func citationRefs(citations []aiknowledge.Citation) []string {
	refs := make([]string, 0, len(citations))
	for _, citation := range citations {
		refs = append(refs, "knowledge-chunk:"+citation.ChunkID)
	}
	return refs
}

var _ aitool.Executor = (*KnowledgeTool)(nil)
