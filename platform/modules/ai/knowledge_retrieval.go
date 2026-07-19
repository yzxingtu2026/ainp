package ai

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const maxKnowledgeRetrievalScope = 16

type KnowledgeRetrievalService struct {
	db         *gorm.DB
	access     aiknowledge.Service
	embeddings aimodelruntime.EmbeddingRuntime
}

func NewKnowledgeRetrievalService(db *gorm.DB, access aiknowledge.Service, embeddings aimodelruntime.EmbeddingRuntime) *KnowledgeRetrievalService {
	if err := registerKnowledgeTenantModels(); err != nil {
		panic(err)
	}
	return &KnowledgeRetrievalService{db: tenantdata.RuntimeDB(db), access: access, embeddings: embeddings}
}

type knowledgeCandidate struct {
	ChunkID, KnowledgeBaseID, DocumentID, IndexVersionID, SourceRevision, HeadingPath, Text, Title, AttachmentID string
	Ordinal, TokenCount                                                                                          int
	PageNumber, PositionStart, PositionEnd                                                                       *int
	FTSScore, VectorScore                                                                                        float64
}

func (s *KnowledgeRetrievalService) Retrieve(ctx context.Context, request aiknowledge.RetrievalRequest) ([]aiknowledge.RetrievedChunk, error) {
	response, err := s.RetrieveWithMetadata(ctx, request)
	return response.Items, err
}

func (s *KnowledgeRetrievalService) RetrieveWithMetadata(ctx context.Context, request aiknowledge.RetrievalRequest) (aiknowledge.RetrievalResponse, error) {
	var response aiknowledge.RetrievalResponse
	if s.db == nil || s.access == nil {
		return response, errors.New("knowledge retrieval dependencies unavailable")
	}
	ac, ok := auth.FromContext(ctx)
	if !ok || !ac.Authenticated || ac.TenantID == "" {
		return response, errors.New("knowledge retrieval requires authenticated tenant context")
	}
	request.Config.Normalize()
	if err := request.Config.Validate(); err != nil {
		return response, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > 4096 {
		return response, errors.New("knowledge query is invalid")
	}
	baseIDs := stableUnique(request.KnowledgeBaseIDs)
	if len(baseIDs) == 0 || len(baseIDs) > maxKnowledgeRetrievalScope {
		return response, errors.New("knowledge base scope is invalid")
	}
	documentIDs := stableUnique(request.DocumentIDs)
	if len(documentIDs) > 100 {
		return response, errors.New("document filter is too large")
	}
	candidates := map[string]knowledgeCandidate{}
	candidateLimit := request.Config.TopK * 4
	if candidateLimit < 20 {
		candidateLimit = 20
	}
	if candidateLimit > 200 {
		candidateLimit = 200
	}
	for _, baseID := range baseIDs {
		if err := s.access.CanReadKnowledgeBase(ctx, baseID); err != nil {
			continue
		}
		var base KnowledgeBase
		if err := s.db.WithContext(ctx).Where("id=? AND status=? AND current_index_version_id IS NOT NULL", baseID, aiknowledge.KnowledgeBaseEnabled).First(&base).Error; err != nil {
			continue
		}
		var version KnowledgeIndexVersion
		if err := s.db.WithContext(ctx).Where("id=? AND knowledge_base_id=? AND status=?", *base.CurrentIndexVersionID, base.ID, aiknowledge.IndexReady).First(&version).Error; err != nil {
			continue
		}
		rows, err := s.lexicalCandidates(ctx, ac.TenantID, base, version, query, documentIDs, candidateLimit)
		if err != nil {
			return response, err
		}
		for _, row := range rows {
			candidates[row.ChunkID] = row
		}
		vectorConfigured := version.EmbeddingDimensions > 0 && version.EmbeddingDeploymentID != "" && version.EmbeddingAlias != ""
		if s.embeddings != nil && vectorConfigured {
			embeddingResponse, embedErr := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: version.EmbeddingAlias, Inputs: []aimodelruntime.EmbeddingItem{{ID: "query", Text: query}}, Dimensions: &version.EmbeddingDimensions, Purpose: aimodelruntime.EmbeddingPurposeKnowledgeQuery})
			if embedErr == nil {
				if embeddingResponse == nil {
					return aiknowledge.RetrievalResponse{}, errors.New("knowledge embedding response unavailable")
				}
				if embeddingResponse.Dimensions != version.EmbeddingDimensions || embeddingResponse.Model.DeploymentID != version.EmbeddingDeploymentID || len(embeddingResponse.Vectors) != 1 || len(embeddingResponse.Vectors[0].Vector) != version.EmbeddingDimensions {
					return aiknowledge.RetrievalResponse{}, errors.New("knowledge embedding provenance mismatch")
				}
				vectors, vectorErr := s.vectorCandidates(ctx, ac.TenantID, base, version, PGVector(embeddingResponse.Vectors[0].Vector), documentIDs, candidateLimit)
				if vectorErr != nil {
					return aiknowledge.RetrievalResponse{}, vectorErr
				}
				for _, row := range vectors {
					existing := candidates[row.ChunkID]
					if existing.ChunkID == "" {
						existing = row
					}
					existing.VectorScore = row.VectorScore
					candidates[row.ChunkID] = existing
				}
			} else {
				response.Metadata.Degraded = true
				response.Metadata.DegradeReason = "embedding_runtime"
			}
		} else if vectorConfigured {
			response.Metadata.Degraded = true
			response.Metadata.DegradeReason = "embedding_runtime"
		}
	}
	items := make([]aiknowledge.RetrievedChunk, 0, len(candidates))
	documentRefs := map[string]aiknowledge.DocumentRef{}
	deniedDocuments := map[string]bool{}
	for _, row := range candidates {
		if deniedDocuments[row.DocumentID] {
			continue
		}
		ref, found := documentRefs[row.DocumentID]
		if !found {
			if err := s.access.CanReadDocument(ctx, row.DocumentID); err != nil {
				deniedDocuments[row.DocumentID] = true
				continue
			}
			var err error
			ref, err = s.access.GetDocumentRef(ctx, row.DocumentID)
			if err != nil {
				deniedDocuments[row.DocumentID] = true
				continue
			}
			documentRefs[row.DocumentID] = ref
		}
		if ref.SourceRevision != row.SourceRevision {
			continue
		}
		score := 0.45*clampScore(row.FTSScore) + 0.55*clampScore(row.VectorScore)
		lowerText, lowerQuery := strings.ToLower(row.Text), strings.ToLower(query)
		if lowerText == lowerQuery {
			score = 1
		} else if strings.Contains(lowerText, lowerQuery) && score < 0.9 {
			score = 0.9
		}
		if score < request.Config.MinScore {
			continue
		}
		citation := aiknowledge.Citation{KnowledgeBaseID: row.KnowledgeBaseID, DocumentID: row.DocumentID, ChunkID: row.ChunkID, IndexVersionID: row.IndexVersionID, Ordinal: row.Ordinal, SourceRevision: row.SourceRevision, PageNumber: row.PageNumber, PositionStart: row.PositionStart, PositionEnd: row.PositionEnd, HeadingPath: row.HeadingPath, Title: row.Title, AttachmentID: row.AttachmentID}
		items = append(items, aiknowledge.RetrievedChunk{Citation: citation, Score: score, Text: row.Text, Untrusted: true})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Citation.ChunkID < items[j].Citation.ChunkID
		}
		return items[i].Score > items[j].Score
	})
	out := make([]aiknowledge.RetrievedChunk, 0, request.Config.TopK)
	tokens := 0
	for _, item := range items {
		count := estimateKnowledgeTokens(item.Text)
		if len(out) >= request.Config.TopK {
			break
		}
		if tokens+count > request.Config.MaxContextTokens {
			continue
		}
		tokens += count
		out = append(out, item)
	}
	response.Items = out
	response.Metadata.CandidateCount = len(candidates)
	return response, nil
}

func (s *KnowledgeRetrievalService) lexicalCandidates(ctx context.Context, tenantID string, base KnowledgeBase, version KnowledgeIndexVersion, query string, documents []string, limit int) ([]knowledgeCandidate, error) {
	filter := ""
	if len(documents) > 0 {
		filter = " AND c.document_id IN ?"
	}
	sql := `WITH trusted_tenant AS (SELECT ?::text tenant_id) SELECT c.id chunk_id,c.knowledge_base_id,c.document_id,c.index_version_id,c.source_revision,c.ordinal,c.page_number,c.position_start,c.position_end,c.heading_path,c.token_count,p.text,d.original_name title,d.attachment_id,ts_rank_cd(p.search_vector,plainto_tsquery('simple',?)) fts_score,0::float8 vector_score FROM ai_knowledge_chunks c CROSS JOIN trusted_tenant tt JOIN ai_knowledge_chunk_payloads p ON p.tenant_id=c.tenant_id AND p.chunk_id=c.id JOIN ai_knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id AND d.deleted_at IS NULL WHERE c.tenant_id=tt.tenant_id AND c.knowledge_base_id=? AND c.index_version_id=? AND d.current_index_version_id=? AND d.source_revision=c.source_revision AND (p.search_vector @@ plainto_tsquery('simple',?) OR lower(p.text) LIKE '%'||lower(?)||'%')` + filter + ` ORDER BY fts_score DESC,c.id ASC LIMIT ?`
	queryArgs := []any{query, base.ID, version.ID, version.ID, query, query}
	if len(documents) > 0 {
		queryArgs = append(queryArgs, documents)
	}
	queryArgs = append(queryArgs, limit)
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	return tenantdata.TenantRaw[knowledgeCandidate](tdb, sql, queryArgs...)
}

func (s *KnowledgeRetrievalService) vectorCandidates(ctx context.Context, tenantID string, base KnowledgeBase, version KnowledgeIndexVersion, vector PGVector, documents []string, limit int) ([]knowledgeCandidate, error) {
	filter := ""
	args := []any{vector, base.ID, version.ID, version.ID}
	if len(documents) > 0 {
		filter = " AND c.document_id IN ?"
		args = append(args, documents)
	}
	args = append(args, limit)
	sql := `WITH trusted_tenant AS (SELECT ?::text tenant_id) SELECT c.id chunk_id,c.knowledge_base_id,c.document_id,c.index_version_id,c.source_revision,c.ordinal,c.page_number,c.position_start,c.position_end,c.heading_path,c.token_count,p.text,d.original_name title,d.attachment_id,0::float8 fts_score,(1-(p.vector OPERATOR(public.<=>) ?::public.vector)) vector_score FROM ai_knowledge_chunks c CROSS JOIN trusted_tenant tt JOIN ai_knowledge_chunk_payloads p ON p.tenant_id=c.tenant_id AND p.chunk_id=c.id JOIN ai_knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id AND d.deleted_at IS NULL WHERE c.tenant_id=tt.tenant_id AND c.knowledge_base_id=? AND c.index_version_id=? AND d.current_index_version_id=? AND d.source_revision=c.source_revision` + filter + ` ORDER BY p.vector OPERATOR(public.<=>) ?::public.vector,c.id ASC LIMIT ?`
	args = append(args[:len(args)-1], vector, limit)
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	return tenantdata.TenantRaw[knowledgeCandidate](tdb, sql, args...)
}

func (s *KnowledgeRetrievalService) ResolveCitation(ctx context.Context, citation aiknowledge.Citation) (aiknowledge.CitationLookup, error) {
	citation.Stale = true
	lookup := aiknowledge.CitationLookup{Citation: citation, Stale: true}
	if err := s.access.CanReadKnowledgeBase(ctx, citation.KnowledgeBaseID); err != nil {
		return lookup, nil
	}
	if err := s.access.CanReadDocument(ctx, citation.DocumentID); err != nil {
		return lookup, nil
	}
	ref, err := s.access.GetDocumentRef(ctx, citation.DocumentID)
	if err != nil {
		return lookup, nil
	}
	tdb, err := tenantdata.TenantDB(ctx, s.db)
	if err != nil {
		return lookup, err
	}
	counts, err := tenantdata.TenantRaw[struct{ Count int64 }](tdb, `SELECT COUNT(*) AS count FROM ai_knowledge_chunks c JOIN ai_knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id AND d.deleted_at IS NULL JOIN ai_knowledge_bases kb ON kb.tenant_id=c.tenant_id AND kb.id=c.knowledge_base_id AND kb.deleted_at IS NULL WHERE c.tenant_id=? AND c.id=? AND c.knowledge_base_id=? AND c.document_id=? AND c.index_version_id=? AND c.source_revision=? AND d.source_revision=c.source_revision AND d.current_index_version_id=c.index_version_id AND kb.current_index_version_id=c.index_version_id`, citation.ChunkID, citation.KnowledgeBaseID, citation.DocumentID, citation.IndexVersionID, ref.SourceRevision)
	if err != nil {
		return lookup, err
	}
	var count int64
	if len(counts) == 1 {
		count = counts[0].Count
	}
	lookup.Available = count == 1 && ref.SourceRevision == citation.SourceRevision
	lookup.Stale = !lookup.Available
	lookup.Citation.Stale = lookup.Stale
	return lookup, nil
}

func stableUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
func estimateKnowledgeTokens(text string) int {
	count := len([]rune(text))
	if count == 0 {
		return 0
	}
	return (count + 3) / 4
}

var _ aiknowledge.KnowledgeRetriever = (*KnowledgeRetrievalService)(nil)
var _ aiknowledge.KnowledgeRetrieverWithMetadata = (*KnowledgeRetrievalService)(nil)
var _ aiknowledge.CitationResolver = (*KnowledgeRetrievalService)(nil)
