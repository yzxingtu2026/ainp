// Package aiknowledge defines stable, provider-neutral knowledge contracts.
package aiknowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const (
	VisibilityPrivate = "private"
	VisibilityTenant  = "tenant"

	KnowledgeBaseDraft    = "draft"
	KnowledgeBaseEnabled  = "enabled"
	KnowledgeBaseDisabled = "disabled"

	DocumentPending    = "pending"
	DocumentReady      = "ready"
	DocumentProcessing = "processing"
	DocumentFailed     = "failed"

	IndexPending    = "pending"
	IndexBuilding   = "building"
	IndexReady      = "ready"
	IndexFailed     = "failed"
	IndexSuperseded = "superseded"
	IndexAborted    = "aborted"

	ChunkingFixedTokens = "fixed_tokens"
)

// ChunkingConfig is deliberately closed and versionable; arbitrary JSON is not accepted.
type ChunkingConfig struct {
	Strategy     string `json:"strategy"`
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
}

func (c *ChunkingConfig) Normalize() {
	if c.Strategy == "" {
		c.Strategy = ChunkingFixedTokens
	}
	if c.ChunkSize == 0 {
		c.ChunkSize = 800
	}
	if c.ChunkOverlap == 0 {
		c.ChunkOverlap = 100
	}
}

func (c ChunkingConfig) Validate() error {
	if c.Strategy != ChunkingFixedTokens {
		return fmt.Errorf("unsupported chunking strategy")
	}
	if c.ChunkSize < 100 || c.ChunkSize > 4000 {
		return fmt.Errorf("chunk_size must be between 100 and 4000")
	}
	if c.ChunkOverlap < 0 || c.ChunkOverlap >= c.ChunkSize {
		return fmt.Errorf("chunk_overlap must be non-negative and smaller than chunk_size")
	}
	return nil
}

// RetrievalConfig is the bounded retrieval policy persisted with a knowledge base.
type RetrievalConfig struct {
	TopK             int     `json:"top_k"`
	MinScore         float64 `json:"min_score"`
	MaxContextTokens int     `json:"max_context_tokens"`
}

func (c *RetrievalConfig) Normalize() {
	if c.TopK == 0 {
		c.TopK = 5
	}
	if c.MaxContextTokens == 0 {
		c.MaxContextTokens = 4000
	}
}

func (c RetrievalConfig) Validate() error {
	if c.TopK < 1 || c.TopK > 100 {
		return fmt.Errorf("top_k must be between 1 and 100")
	}
	if c.MinScore < 0 || c.MinScore > 1 {
		return fmt.Errorf("min_score must be between 0 and 1")
	}
	if c.MaxContextTokens < 1 || c.MaxContextTokens > 1_000_000 {
		return fmt.Errorf("max_context_tokens must be between 1 and 1000000")
	}
	return nil
}

type KnowledgeBaseRef struct {
	ID             string `json:"id"`
	TenantID       string `json:"-"`
	OwnerUserID    string `json:"owner_user_id"`
	Visibility     string `json:"visibility"`
	Status         string `json:"status"`
	EmbeddingAlias string `json:"embedding_alias,omitempty"`
	Revision       uint64 `json:"revision"`
}

type DocumentRef struct {
	ID              string `json:"id"`
	TenantID        string `json:"-"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	AttachmentID    string `json:"attachment_id"`
	SourceRevision  string `json:"source_revision"`
	ContentType     string `json:"content_type"`
	OriginalName    string `json:"original_name"`
	Revision        uint64 `json:"revision"`
}

// Citation contains stable identifiers only. It intentionally excludes storage locations.
type Citation struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	DocumentID      string `json:"document_id"`
	ChunkID         string `json:"chunk_id"`
	IndexVersionID  string `json:"index_version_id"`
	Ordinal         int    `json:"ordinal"`
	SourceRevision  string `json:"source_revision"`
	PageNumber      *int   `json:"page_number,omitempty"`
	PositionStart   *int   `json:"position_start,omitempty"`
	PositionEnd     *int   `json:"position_end,omitempty"`
	HeadingPath     string `json:"heading_path,omitempty"`
	Title           string `json:"title"`
	AttachmentID    string `json:"attachment_id"`
	Stale           bool   `json:"stale"`
}

type AccessPolicy interface {
	CanReadKnowledgeBase(context.Context, string) error
	CanManageKnowledgeBase(context.Context, string) error
	CanReadDocument(context.Context, string) error
}

type Service interface {
	AccessPolicy
	GetKnowledgeBaseRef(context.Context, string) (KnowledgeBaseRef, error)
	GetDocumentRef(context.Context, string) (DocumentRef, error)
}

type ParsedSection struct {
	Ordinal        int
	Text           string
	SourceRevision string
	PageNumber     *int
	PositionStart  *int
	PositionEnd    *int
	HeadingPath    string
	ParserVersion  string
}

type SourceOpenRequest struct {
	Document DocumentRef
	MaxBytes int64
}

type DocumentSource struct {
	Reader io.ReadCloser
	Size   int64
}

// DocumentSourceResolver reauthorizes and opens a bounded attachment source.
// Callers own the returned Reader and must close it.
type DocumentSourceResolver interface {
	OpenSource(context.Context, SourceOpenRequest) (DocumentSource, error)
}

type DocumentParser interface {
	Parse(context.Context, DocumentRef, io.Reader) ([]ParsedSection, error)
}

type ChunkInput struct {
	Document DocumentRef
	Sections []ParsedSection
	Config   ChunkingConfig
}

type Chunk struct {
	Ordinal              int
	Text                 string
	SourceRevision       string
	PageNumber           *int
	PositionStart        *int
	PositionEnd          *int
	HeadingPath          string
	TokenCount           int
	ContentHash          string
	ParserVersion        string
	ChunkingVersion      string
	PreprocessingVersion string
}

type Chunker interface {
	Chunk(context.Context, ChunkInput) ([]Chunk, error)
}

type RetrievalRequest struct {
	KnowledgeBaseIDs []string        `json:"knowledge_base_ids"`
	Query            string          `json:"query"`
	Config           RetrievalConfig `json:"config"`
}

type RetrievedChunk struct {
	Citation Citation
	Score    float64
	Text     string
}

type KnowledgeRetriever interface {
	Retrieve(context.Context, RetrievalRequest) ([]RetrievedChunk, error)
}

// Fake is deterministic and configurable for contract and consumer tests.
type Fake struct {
	mu             sync.RWMutex
	KnowledgeBases map[string]KnowledgeBaseRef
	Documents      map[string]DocumentRef
	ReadableKBs    map[string]bool
	ManageableKBs  map[string]bool
	ReadableDocs   map[string]bool
	ParsedSections []ParsedSection
	Chunks         []Chunk
	Retrieval      []RetrievedChunk
	Err            error
}

func NewFake() *Fake {
	return &Fake{
		KnowledgeBases: map[string]KnowledgeBaseRef{},
		Documents:      map[string]DocumentRef{},
		ReadableKBs:    map[string]bool{},
		ManageableKBs:  map[string]bool{},
		ReadableDocs:   map[string]bool{},
	}
}

func (f *Fake) GetKnowledgeBaseRef(_ context.Context, id string) (KnowledgeBaseRef, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return KnowledgeBaseRef{}, f.Err
	}
	v, ok := f.KnowledgeBases[id]
	if !ok {
		return KnowledgeBaseRef{}, fmt.Errorf("knowledge base not found")
	}
	return v, nil
}

func (f *Fake) GetDocumentRef(_ context.Context, id string) (DocumentRef, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return DocumentRef{}, f.Err
	}
	v, ok := f.Documents[id]
	if !ok {
		return DocumentRef{}, fmt.Errorf("document not found")
	}
	return v, nil
}

func (f *Fake) CanReadKnowledgeBase(_ context.Context, id string) error {
	return f.allowed(f.ReadableKBs, id)
}

func (f *Fake) CanManageKnowledgeBase(_ context.Context, id string) error {
	return f.allowed(f.ManageableKBs, id)
}

func (f *Fake) CanReadDocument(_ context.Context, id string) error {
	return f.allowed(f.ReadableDocs, id)
}

func (f *Fake) allowed(values map[string]bool, id string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return f.Err
	}
	if !values[id] {
		return fmt.Errorf("knowledge resource not found")
	}
	return nil
}

func (f *Fake) Retrieve(_ context.Context, request RetrievalRequest) ([]RetrievedChunk, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return nil, f.Err
	}
	if len(request.KnowledgeBaseIDs) == 0 {
		return nil, fmt.Errorf("at least one knowledge base is required")
	}
	allowed := make(map[string]bool, len(request.KnowledgeBaseIDs))
	for _, id := range request.KnowledgeBaseIDs {
		allowed[id] = true
	}
	out := make([]RetrievedChunk, 0, len(f.Retrieval))
	for _, item := range f.Retrieval {
		if !allowed[item.Citation.KnowledgeBaseID] {
			continue
		}
		if item.Score < request.Config.MinScore {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return strings.Compare(out[i].Citation.ChunkID, out[j].Citation.ChunkID) < 0
		}
		return out[i].Score > out[j].Score
	})
	limit := request.Config.TopK
	if limit <= 0 || limit > len(out) {
		limit = len(out)
	}
	return append([]RetrievedChunk(nil), out[:limit]...), nil
}

func (f *Fake) Parse(_ context.Context, _ DocumentRef, _ io.Reader) ([]ParsedSection, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]ParsedSection(nil), f.ParsedSections...), nil
}

func (f *Fake) Chunk(_ context.Context, _ ChunkInput) ([]Chunk, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]Chunk(nil), f.Chunks...), nil
}

func MarshalConfig(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

var _ Service = (*Fake)(nil)
var _ DocumentParser = (*Fake)(nil)
var _ Chunker = (*Fake)(nil)
var _ KnowledgeRetriever = (*Fake)(nil)
