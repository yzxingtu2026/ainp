package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const navigationEmbeddingSchemaVersion = 1
const navigationDocumentChangedEvent = "ai.navigation_document.changed.v1"

var ErrNavigationEmbeddingSuperseded = errors.New("navigation embedding request is superseded")

type NavigationDocumentChangedPayload struct {
	DocumentID      string `json:"document_id"`
	CatalogRevision uint64 `json:"catalog_revision"`
}

type NavigationSearchService struct {
	db         tenantdata.GuardedDatabase
	menus      *menu.Registry
	help       *menu.HelpRegistry
	settings   *setting.Provider
	embeddings aimodelruntime.EmbeddingRuntime
	writer     outbox.Writer
}

func NewNavigationSearchService(db *gorm.DB, menus *menu.Registry, help *menu.HelpRegistry, settings *setting.Provider, embeddings aimodelruntime.EmbeddingRuntime, writer outbox.Writer) *NavigationSearchService {
	if err := registerNavigationTenantModels(); err != nil {
		panic(err)
	}
	return &NavigationSearchService{db: tenantdata.GuardedDB(db), menus: menus, help: help, settings: settings, embeddings: embeddings, writer: writer}
}

func (s *NavigationSearchService) embeddingAlias(ctx context.Context, tenantID string) (string, error) {
	if s.settings == nil {
		return "", nil
	}
	effective, err := s.settings.Get(ctx, tenantID, menu.SettingKeySemanticSearch)
	if err != nil {
		return "", err
	}
	alias, _ := effective.Values[menu.SettingFieldModelAlias].(string)
	return strings.TrimSpace(alias), nil
}

func navigationDocumentHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *NavigationSearchService) ReconcileForTenant(ctx context.Context, tenantID string, force ...bool) error {
	if strings.TrimSpace(tenantID) == "" {
		return errors.New("tenant id is required")
	}
	ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: tenantID, Realm: auth.RealmTenant, UserID: "system:navigation-index", Client: "navigation-reconciler"})
	alias, err := s.embeddingAlias(ctx, tenantID)
	if err != nil {
		return err
	}
	desired := make([]NavigationSearchDocument, 0)
	forceRebuild := len(force) > 0 && force[0]
	for _, item := range menu.NewSearchService(s.menus).SearchableMenus(auth.RealmTenant) {
		description := ""
		terms := []string{}
		if item.Navigation != nil {
			description = item.Navigation.Description
			terms = append(terms, item.Navigation.Keywords...)
			terms = append(terms, item.Navigation.UseCases...)
		}
		content := strings.Join([]string{item.Title, description, strings.Join(terms, " ")}, "\n")
		desired = append(desired, NavigationSearchDocument{ID: uuid.NewString(), TenantID: tenantID, MenuID: item.ID, DocumentKind: "page", Locale: "zh-CN", Title: item.Title, Content: content, ContentHash: navigationDocumentHash(item.ID, "page", "zh-CN", content), IndexStatus: "pending", CatalogRevision: 1})
	}
	for _, help := range s.help.List() {
		for _, useCase := range help.UseCases {
			content := help.Content + "\n适用任务: " + useCase
			desired = append(desired, NavigationSearchDocument{ID: uuid.NewString(), TenantID: tenantID, MenuID: help.MenuID, DocumentKind: "help", Locale: help.Locale, UseCase: useCase, Title: useCase, Content: content, HelpVersion: help.Version, ContentHash: navigationDocumentHash(help.MenuID, help.Locale, help.Version, useCase, content), IndexStatus: "pending", CatalogRevision: 1})
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []NavigationSearchDocument
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		byKey := map[string]NavigationSearchDocument{}
		for _, row := range existing {
			byKey[navigationDocumentKey(row)] = row
		}
		seen := map[string]bool{}
		for _, row := range desired {
			key := navigationDocumentKey(row)
			seen[key] = true
			if current, ok := byKey[key]; ok {
				row.ID = current.ID
				row.CreatedAt = current.CreatedAt
				row.CatalogRevision = current.CatalogRevision
				provenanceReady := alias != "" && current.EmbeddingAlias == alias && current.EmbeddingDeploymentID != "" && current.EmbeddingDimensions > 0 && current.EmbeddingSchemaVersion == navigationEmbeddingSchemaVersion && current.EmbeddingPreprocessing == aimodelruntime.EmbeddingPreprocessingVersionCosine && current.EmbeddingVector != nil
				lexicalReady := alias == "" && current.EmbeddingAlias == "" && current.EmbeddingVector == nil && current.IndexStatus == "ready"
				if !forceRebuild && current.ContentHash == row.ContentHash && (lexicalReady || provenanceReady) {
					continue
				}
				row.CatalogRevision++
			}
			if alias == "" {
				row.IndexStatus = "ready"
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "menu_id"}, {Name: "document_kind"}, {Name: "locale"}, {Name: "use_case"}}, DoUpdates: clause.AssignmentColumns([]string{"title", "content", "help_version", "content_hash", "index_status", "catalog_revision", "embedding_model", "embedding_alias", "embedding_deployment_id", "embedding_dimensions", "embedding_preprocessing", "embedding_schema_version", "embedding_vector", "vector_updated_at", "updated_at"})}).Create(&row).Error; err != nil {
				return err
			}
			if alias != "" && s.writer != nil {
				if _, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: navigationDocumentChangedEvent, AggregateType: "navigation_search_document", AggregateID: row.ID, AggregateVersion: int64(row.CatalogRevision), Payload: NavigationDocumentChangedPayload{DocumentID: row.ID, CatalogRevision: row.CatalogRevision}}); err != nil {
					return err
				}
			}
		}
		for key, row := range byKey {
			if !seen[key] && row.IndexStatus != "stale" {
				if err := tx.Model(&NavigationSearchDocument{}).Where("id = ?", row.ID).Update("index_status", "stale").Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

type NavigationIndexStatus struct {
	State         string     `json:"state"`
	ModelAlias    string     `json:"model_alias,omitempty"`
	Total         int        `json:"total"`
	Pending       int        `json:"pending"`
	Ready         int        `json:"ready"`
	Failed        int        `json:"failed"`
	Stale         int        `json:"stale"`
	LastUpdatedAt *time.Time `json:"last_updated_at,omitempty"`
}

func (s *NavigationSearchService) IndexStatus(ctx context.Context, tenantID string) (NavigationIndexStatus, error) {
	status := NavigationIndexStatus{}
	alias, err := s.embeddingAlias(ctx, tenantID)
	if err != nil {
		return status, err
	}
	status.ModelAlias = alias
	var documents []NavigationSearchDocument
	if err = s.db.WithContext(ctx).Order("updated_at DESC").Find(&documents).Error; err != nil {
		return status, err
	}
	status.Total = len(documents)
	for i, document := range documents {
		if i == 0 {
			updated := document.UpdatedAt
			status.LastUpdatedAt = &updated
		}
		switch document.IndexStatus {
		case "pending":
			status.Pending++
		case "ready":
			status.Ready++
		case "failed":
			status.Failed++
		case "stale":
			status.Stale++
		}
	}
	switch {
	case alias == "":
		status.State = "disabled"
	case status.Total == 0:
		status.State = "not_indexed"
	case status.Pending > 0:
		status.State = "pending"
	case status.Failed > 0 && status.Ready == 0:
		status.State = "failed"
	case status.Failed > 0 || status.Stale > 0:
		status.State = "degraded"
	default:
		status.State = "ready"
	}
	return status, nil
}

func navigationDocumentKey(row NavigationSearchDocument) string {
	return strings.Join([]string{row.MenuID, row.DocumentKind, row.Locale, row.UseCase}, "\x00")
}

func (s *NavigationSearchService) IndexPending(ctx context.Context, tenantID string) error {
	alias, err := s.embeddingAlias(ctx, tenantID)
	if err != nil || alias == "" || s.embeddings == nil {
		return err
	}
	var documents []NavigationSearchDocument
	if err := s.db.WithContext(ctx).Where("index_status IN ?", []string{"pending", "failed"}).Find(&documents).Error; err != nil {
		return err
	}
	for _, document := range documents {
		if err := s.IndexDocument(ctx, document.ID, document.CatalogRevision, alias); err != nil && !errors.Is(err, ErrNavigationEmbeddingSuperseded) {
			return err
		}
	}
	return nil
}

func (s *NavigationSearchService) IndexDocument(ctx context.Context, documentID string, revision uint64, alias string) error {
	currentAlias, err := s.embeddingAlias(ctx, auth.MustFromContext(ctx).TenantID)
	if err != nil {
		return err
	}
	if alias == "" || currentAlias != alias {
		return ErrNavigationEmbeddingSuperseded
	}
	if s.embeddings == nil {
		return errors.New("navigation embedding runtime is unavailable")
	}
	var document NavigationSearchDocument
	if err = s.db.WithContext(ctx).Where("id = ? AND catalog_revision = ? AND index_status IN ?", documentID, revision, []string{"pending", "failed"}).First(&document).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNavigationEmbeddingSuperseded
		}
		return err
	}
	response, embedErr := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: alias, Inputs: []aimodelruntime.EmbeddingItem{{ID: document.ID, Text: document.Content}}, Purpose: aimodelruntime.EmbeddingPurposeNavigationIndex, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm})
	if embedErr != nil || response == nil || response.Dimensions <= 0 || response.Model.DeploymentID == "" || len(response.Vectors) != 1 || len(response.Vectors[0].Vector) != response.Dimensions || response.PreprocessingVersion != aimodelruntime.EmbeddingPreprocessingVersionCosine {
		_ = s.db.WithContext(ctx).Model(&NavigationSearchDocument{}).Where("id = ? AND catalog_revision = ?", document.ID, document.CatalogRevision).Update("index_status", "failed").Error
		if embedErr != nil {
			return embedErr
		}
		return errors.New("navigation embedding provenance is invalid")
	}
	now := time.Now().UTC()
	vector := PGVector(response.Vectors[0].Vector)
	updates := map[string]any{"embedding_alias": alias, "embedding_model": response.Model.RemoteModelID, "embedding_deployment_id": response.Model.DeploymentID, "embedding_dimensions": response.Dimensions, "embedding_preprocessing": response.PreprocessingVersion, "embedding_schema_version": navigationEmbeddingSchemaVersion, "embedding_vector": &vector, "vector_updated_at": now, "index_status": "ready"}
	if err := s.db.WithContext(ctx).Model(&NavigationSearchDocument{}).Where("id = ? AND catalog_revision = ?", document.ID, document.CatalogRevision).Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

type navigationVectorCandidate struct {
	MenuID string  `gorm:"column:menu_id"`
	Score  float64 `gorm:"column:score"`
}

func (s *NavigationSearchService) SearchNavigation(ctx context.Context, request menu.SemanticSearchRequest) menu.SemanticSearchResult {
	if len(request.AllowedMenuIDs) == 0 {
		return menu.SemanticSearchResult{}
	}
	alias, err := s.embeddingAlias(ctx, request.TenantID)
	if err != nil {
		return menu.SemanticSearchResult{Degraded: true, Reason: "embedding_setting"}
	}
	if alias == "" {
		return menu.SemanticSearchResult{}
	}
	if s.embeddings == nil {
		return menu.SemanticSearchResult{Degraded: true, Reason: "embedding_runtime"}
	}
	response, err := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: alias, Inputs: []aimodelruntime.EmbeddingItem{{ID: "query", Text: request.Query}}, Purpose: aimodelruntime.EmbeddingPurposeNavigationQuery, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm})
	if err != nil || response == nil || response.Dimensions <= 0 || response.Model.DeploymentID == "" || len(response.Vectors) != 1 || len(response.Vectors[0].Vector) != response.Dimensions || response.PreprocessingVersion != aimodelruntime.EmbeddingPreprocessingVersionCosine {
		return menu.SemanticSearchResult{Degraded: true, Reason: "embedding_runtime"}
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return menu.SemanticSearchResult{Degraded: true, Reason: "vector_index"}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(request.AllowedMenuIDs)), ",")
	args := []any{PGVector(response.Vectors[0].Vector), request.Language, alias, response.Model.DeploymentID, response.Dimensions, response.PreprocessingVersion, navigationEmbeddingSchemaVersion}
	for _, id := range request.AllowedMenuIDs {
		args = append(args, id)
	}
	args = append(args, PGVector(response.Vectors[0].Vector))
	query := fmt.Sprintf(`WITH trusted_tenant AS (SELECT ?::text tenant_id), query_vector AS (SELECT ?::public.vector value) SELECT d.menu_id, MAX(1-(d.embedding_vector OPERATOR(public.<=>) q.value)) AS score FROM navigation_search_documents d CROSS JOIN trusted_tenant tt CROSS JOIN query_vector q WHERE d.tenant_id=tt.tenant_id AND d.index_status='ready' AND d.embedding_vector IS NOT NULL AND d.locale IN (?, 'zh-CN') AND d.embedding_alias=? AND d.embedding_deployment_id=? AND d.embedding_dimensions=? AND d.embedding_preprocessing=? AND d.embedding_schema_version=? AND d.menu_id IN (%s) GROUP BY d.menu_id ORDER BY score DESC, d.menu_id ASC LIMIT 20`, placeholders)
	rows, err := tenantdata.TenantRaw[navigationVectorCandidate](tdb, query, args...)
	if err != nil {
		return menu.SemanticSearchResult{Degraded: true, Reason: "vector_index"}
	}
	candidates := make([]menu.SemanticCandidate, len(rows))
	for i, row := range rows {
		candidates[i] = menu.SemanticCandidate{MenuID: row.MenuID, Score: row.Score}
	}
	return menu.SemanticSearchResult{Candidates: candidates}
}
