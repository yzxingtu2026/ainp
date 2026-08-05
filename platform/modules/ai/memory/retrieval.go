package memory

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

type MemoryRecallRequest struct {
	Query            string
	AgentID          string
	Types            []aimemory.ItemType
	DateFrom, DateTo *time.Time
	Limit, MaxTokens int
	NormalOnly       bool
}
type MemoryProvenanceRef struct {
	ItemID       string   `json:"item_id"`
	ItemRevision uint64   `json:"item_revision"`
	EvidenceIDs  []string `json:"evidence_ids"`
}
type MemoryRecallItem struct {
	ID               string                 `json:"id"`
	Type             aimemory.ItemType      `json:"type"`
	Summary          string                 `json:"summary"`
	Content          string                 `json:"content"`
	Score            float64                `json:"score"`
	OccurredAt       *time.Time             `json:"occurred_at,omitempty"`
	OccurredEndAt    *time.Time             `json:"occurred_end_at,omitempty"`
	OccurredTimezone string                 `json:"occurred_timezone"`
	LocalDate        *time.Time             `json:"local_date,omitempty"`
	DatePrecision    aimemory.DatePrecision `json:"date_precision"`
	ObservedAt       time.Time              `json:"observed_at"`
	ValidFrom        *time.Time             `json:"valid_from,omitempty"`
	ValidTo          *time.Time             `json:"valid_to,omitempty"`
	Current          bool                   `json:"is_current"`
	Provenance       MemoryProvenanceRef    `json:"provenance"`
}
type MemoryRecallResponse struct {
	Items          []MemoryRecallItem `json:"items"`
	CandidateCount int                `json:"candidate_count"`
	ReturnedCount  int                `json:"returned_count"`
	TokenEstimate  int                `json:"token_estimate"`
	Degraded       bool               `json:"degraded"`
	DegradeReason  string             `json:"degrade_reason,omitempty"`
}

type memoryRecallCandidate struct {
	MemoryItem
	ExactText   string
	FTSScore    float64
	VectorScore float64
}
type MemoryRetrievalService struct {
	db         tenantdata.GuardedDatabase
	embeddings aimodelruntime.EmbeddingRuntime
}

func NewMemoryRetrievalService(db *gorm.DB, e aimodelruntime.EmbeddingRuntime) *MemoryRetrievalService {
	return &MemoryRetrievalService{db: tenantdata.GuardedDB(db), embeddings: e}
}

func (s *MemoryRetrievalService) Retrieve(ctx context.Context, req MemoryRecallRequest) (MemoryRecallResponse, error) {
	var out MemoryRecallResponse
	trusted, ok := runtimecontext.FromContext(ctx)
	if !ok || trusted.TenantID == "" || trusted.PrincipalID == "" {
		return out, errors.New("memory recall requires trusted runtime context")
	}
	now := trusted.Time.EvaluatedAtUTC
	if err := trusted.Time.Validate(); err != nil {
		return out, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2000
	}
	if req.MaxTokens > 10000 {
		req.MaxTokens = 10000
	}
	query := strings.TrimSpace(req.Query)
	if len(query) > 4096 {
		return out, errors.New("memory query too long")
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return out, err
	}
	var localFrom, localTo any
	loc, _ := time.LoadLocation(trusted.Time.Timezone)
	if req.DateFrom != nil {
		localFrom = req.DateFrom.In(loc).Format("2006-01-02")
	}
	if req.DateTo != nil {
		localTo = req.DateTo.In(loc).Format("2006-01-02")
	}
	sql := `WITH trusted_tenant AS (SELECT ?::text tenant_id) SELECT i.*,COALESCE(ep.search_text,i.summary||' '||i.content) exact_text,CASE WHEN ?='' THEN 0 ELSE ts_rank_cd(to_tsvector('simple',COALESCE(fp.search_text,i.summary||' '||i.content)),plainto_tsquery('simple',?)) END fts_score,0::float8 vector_score FROM ai_memory_items i CROSS JOIN trusted_tenant tt LEFT JOIN ai_memory_projections ep ON ep.tenant_id=i.tenant_id AND ep.item_id=i.id AND ep.kind='exact' AND ep.status='ready' AND ep.item_revision=i.revision LEFT JOIN ai_memory_projections fp ON fp.tenant_id=i.tenant_id AND fp.item_id=i.id AND fp.kind='fts' AND fp.status='ready' AND fp.item_revision=i.revision WHERE i.tenant_id=tt.tenant_id AND i.user_id=? AND i.status='active' AND (i.agent_id IS NULL OR i.agent_id=?) AND (i.expires_at IS NULL OR i.expires_at>?) AND (i.valid_from IS NULL OR i.valid_from<=?) AND (i.valid_to IS NULL OR i.valid_to>?) AND (?::date IS NULL OR (i.local_date IS NOT NULL AND i.local_date>=?::date) OR (i.local_date IS NULL AND COALESCE(i.occurred_end_at,i.occurred_at,i.observed_at)>=?)) AND (?::date IS NULL OR (i.local_date IS NOT NULL AND i.local_date<?::date) OR (i.local_date IS NULL AND COALESCE(i.occurred_at,i.observed_at)<?)) ORDER BY i.importance DESC,i.confidence DESC,i.recorded_at DESC LIMIT 500`
	rows, err := tenantdata.TenantRaw[memoryRecallCandidate](tdb, sql, query, query, trusted.PrincipalID, req.AgentID, now, now, now, localFrom, localFrom, req.DateFrom, localTo, localTo, req.DateTo)
	if err != nil {
		return out, err
	}
	allowed := map[aimemory.ItemType]bool{}
	for _, typ := range req.Types {
		allowed[typ] = true
	}
	filtered := rows[:0]
	for _, row := range rows {
		if len(allowed) > 0 && !allowed[row.Type] {
			continue
		}
		if req.NormalOnly && row.Sensitivity != aimemory.SensitivityNormal {
			continue
		}
		filtered = append(filtered, row)
	}
	rows = filtered
	out.CandidateCount = len(rows)
	if query != "" && len(rows) > 0 {
		s.applyVector(ctx, tdb, query, rows, &out)
	}
	lower := strings.ToLower(query)
	for i := range rows {
		exact := 0.0
		text := strings.ToLower(rows[i].ExactText)
		if lower != "" {
			if text == lower {
				exact = 1
			} else if strings.Contains(text, lower) {
				exact = .9
			}
		}
		recency := 1 / (1 + now.Sub(rows[i].ObservedAt).Hours()/24/365)
		if recency < 0 {
			recency = 0
		}
		rows[i].VectorScore = clampScore(rows[i].VectorScore)
		rows[i].FTSScore = clampScore(rows[i].FTSScore)
		rows[i].Confidence = clampScore(rows[i].Confidence)
		rows[i].FTSScore = .3*exact + .25*rows[i].FTSScore + .25*rows[i].VectorScore + .08*recency + .07*rows[i].Confidence + .05*float64(rows[i].Importance)/100
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].FTSScore == rows[j].FTSScore {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].FTSScore > rows[j].FTSScore
	})
	tokens := 0
	for _, row := range rows {
		if len(out.Items) >= limit {
			break
		}
		estimate := (len(row.Summary) + len(row.Content) + 3) / 4
		if tokens+estimate > req.MaxTokens {
			continue
		}
		if query != "" && row.FTSScore < .08 {
			continue
		}
		tokens += estimate
		out.Items = append(out.Items, MemoryRecallItem{ID: row.ID, Type: row.Type, Summary: row.Summary, Content: row.Content, Score: row.FTSScore, OccurredAt: row.OccurredAt, OccurredEndAt: row.OccurredEndAt, OccurredTimezone: row.OccurredTimezone, LocalDate: row.LocalDate, DatePrecision: row.DatePrecision, ObservedAt: row.ObservedAt, ValidFrom: row.ValidFrom, ValidTo: row.ValidTo, Current: true, Provenance: MemoryProvenanceRef{ItemID: row.ID, ItemRevision: row.Revision}})
	}
	out.TokenEstimate = tokens
	out.ReturnedCount = len(out.Items)
	if len(out.Items) > 0 {
		s.loadEvidence(ctx, tdb, out.Items)
	}
	return out, nil
}

func (s *MemoryRetrievalService) applyVector(ctx context.Context, tdb tenantdata.TenantDatabase, query string, rows []memoryRecallCandidate, out *MemoryRecallResponse) {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	projections, err := tenantdata.TenantRaw[MemoryProjection](tdb, `SELECT * FROM ai_memory_projections WHERE tenant_id=? AND item_id IN ? AND kind='vector' AND status='ready'`, ids)
	if err != nil || len(projections) == 0 {
		out.Degraded = true
		out.DegradeReason = "vector_index_unavailable"
		return
	}
	p := projections[0]
	if s.embeddings == nil {
		out.Degraded = true
		out.DegradeReason = "embedding_runtime"
		return
	}
	response, err := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: p.ModelAlias, Inputs: []aimodelruntime.EmbeddingItem{{ID: "query", Text: query}}, Dimensions: &p.Dimensions, Purpose: aimodelruntime.EmbeddingPurposeMemory, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm})
	if err != nil || response == nil || response.Dimensions != p.Dimensions || response.Model.DeploymentID != p.DeploymentID || len(response.Vectors) != 1 {
		out.Degraded = true
		out.DegradeReason = "embedding_provenance"
		return
	}
	byID := map[string]MemoryProjection{}
	for _, projection := range projections {
		byID[projection.ItemID] = projection
	}
	for i := range rows {
		projection, ok := byID[rows[i].ID]
		if !ok || projection.ItemRevision != rows[i].Revision {
			continue
		}
		var vector []float32
		if json.Unmarshal(projection.VectorJSON, &vector) != nil || len(vector) != p.Dimensions {
			continue
		}
		rows[i].VectorScore = cosine(response.Vectors[0].Vector, vector)
	}
}
func (s *MemoryRetrievalService) loadEvidence(ctx context.Context, tdb tenantdata.TenantDatabase, items []MemoryRecallItem) {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	rows, err := tenantdata.TenantRaw[MemoryEvidence](tdb, `SELECT * FROM ai_memory_evidence WHERE tenant_id=? AND item_id IN ? AND revoked_at IS NULL ORDER BY created_at,id`, ids)
	if err != nil {
		return
	}
	by := map[string][]string{}
	for _, row := range rows {
		by[row.ItemID] = append(by[row.ItemID], row.ID)
	}
	for i := range items {
		items[i].Provenance.EvidenceIDs = by[items[i].ID]
	}
}
func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, aa, bb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
