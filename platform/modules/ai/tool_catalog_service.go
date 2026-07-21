package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ToolCatalogReconcileResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Disabled  int `json:"disabled"`
	Unchanged int `json:"unchanged"`
}

type ToolCatalogChangedPayload struct {
	ToolID          string `json:"tool_id"`
	Source          string `json:"source"`
	SchemaVersion   int    `json:"schema_version"`
	SchemaHash      string `json:"schema_hash"`
	Status          string `json:"status"`
	CatalogRevision uint64 `json:"catalog_revision"`
}

func (s *ToolService) ReconcileCatalog(ctx context.Context, force ...bool) (ToolCatalogReconcileResult, error) {
	var result ToolCatalogReconcileResult
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || strings.TrimSpace(ac.TenantID) == "" {
		return result, fmt.Errorf("authenticated tenant context is required")
	}
	if err := s.registry.Refresh(ctx); err != nil {
		return result, err
	}
	definitions := s.registry.List()
	embeddingAlias, err := s.embeddingAlias(ctx)
	if err != nil {
		return result, err
	}
	forceProjection := len(force) > 0 && force[0]
	write := func(tx *gorm.DB, writer outbox.Writer) error {
		var existing []ToolCatalog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("tool_id").Find(&existing).Error; err != nil {
			return err
		}
		var documents []ToolSearchDocument
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("tool_id").Find(&documents).Error; err != nil {
			return err
		}
		byID := make(map[string]ToolCatalog, len(existing))
		for _, row := range existing {
			byID[row.ToolID] = row
		}
		documentsByID := make(map[string]ToolSearchDocument, len(documents))
		for _, document := range documents {
			documentsByID[document.ToolID] = document
		}
		desired := make(map[string]bool, len(definitions))
		for _, definition := range definitions {
			desired[definition.ID] = true
			current, exists := byID[definition.ID]
			document, documentExists := documentsByID[definition.ID]
			if !forceProjection && exists && catalogMatches(current, definition) && documentExists && documentMatches(document, current, definition, embeddingAlias) {
				result.Unchanged++
				continue
			}
			revision := uint64(1)
			if exists {
				revision = current.Revision + 1
			}
			row := catalogFromDefinition(ac.TenantID, definition, revision)
			if exists {
				row.ID = current.ID
				row.CreatedAt = current.CreatedAt
				result.Updated++
			} else {
				result.Created++
			}
			if err := upsertCatalogProjection(tx, row, definition); err != nil {
				return err
			}
			if err := s.appendCatalogChanged(ctx, tx, writer, row); err != nil {
				return err
			}
		}
		for _, current := range existing {
			if desired[current.ToolID] || current.Status == "disabled" {
				continue
			}
			current.Status = "disabled"
			current.Revision++
			if err := tx.Model(&ToolCatalog{}).Where("tool_id=?", current.ToolID).Updates(map[string]any{"status": current.Status, "revision": current.Revision}).Error; err != nil {
				return err
			}
			if err := tx.Model(&ToolSearchDocument{}).Where("tool_id=?", current.ToolID).Updates(map[string]any{"index_status": "stale", "catalog_revision": current.Revision}).Error; err != nil {
				return err
			}
			if err := s.appendCatalogChanged(ctx, tx, writer, current); err != nil {
				return err
			}
			result.Disabled++
		}
		return nil
	}
	if s.writer != nil {
		if err := outbox.WithinTx(ctx, s.db.WithContext(ctx), s.writer, write); err != nil {
			return ToolCatalogReconcileResult{}, err
		}
	} else if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return write(tx, nil) }); err != nil {
		return ToolCatalogReconcileResult{}, err
	}
	return result, nil
}

func (s *ToolService) ReconcileCatalogForTenant(ctx context.Context, tenantID, actorID string) (ToolCatalogReconcileResult, error) {
	ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: tenantID, UserID: actorID, Client: "tool-catalog-reconciler"})
	return s.ReconcileCatalog(ctx)
}

func (s *ToolService) MarkCatalogDocumentReady(ctx context.Context, payload ToolCatalogChangedPayload) error {
	if payload.ToolID == "" || payload.CatalogRevision == 0 {
		return fmt.Errorf("invalid tool catalog event payload")
	}
	var catalog ToolCatalog
	if err := s.db.WithContext(ctx).Where("tool_id=?", payload.ToolID).First(&catalog).Error; err != nil {
		return err
	}
	if catalog.Revision != payload.CatalogRevision || catalog.Status != "enabled" {
		return nil
	}
	return s.db.WithContext(ctx).Model(&ToolSearchDocument{}).
		Where("tool_id=? AND catalog_revision=? AND index_status=?", payload.ToolID, payload.CatalogRevision, "pending").
		Update("index_status", "ready").Error
}

func (s *ToolService) appendCatalogChanged(ctx context.Context, tx *gorm.DB, writer outbox.Writer, row ToolCatalog) error {
	if writer == nil {
		if row.Status == "enabled" {
			return tx.Model(&ToolSearchDocument{}).Where("tool_id=? AND catalog_revision=?", row.ToolID, row.Revision).Update("index_status", "ready").Error
		}
		return nil
	}
	payload := ToolCatalogChangedPayload{ToolID: row.ToolID, Source: row.Source, SchemaVersion: row.SchemaVersion, SchemaHash: row.SchemaHash, Status: row.Status, CatalogRevision: row.Revision}
	_, err := writer.Append(ctx, tx, outbox.AppendInput{EventType: toolCatalogChangedEvent, AggregateType: "ai_tool_catalog", AggregateID: row.ToolID, AggregateVersion: int64(row.Revision), Payload: payload})
	if err != nil || !s.catalogProjectionSync || row.Status != "enabled" {
		return err
	}
	return tx.Model(&ToolSearchDocument{}).Where("tool_id=? AND catalog_revision=?", row.ToolID, row.Revision).Update("index_status", "ready").Error
}

func catalogFromDefinition(tenantID string, definition aitool.Definition, revision uint64) ToolCatalog {
	return ToolCatalog{ID: uuid.NewString(), TenantID: tenantID, ToolID: definition.ID, Source: string(definition.Source), Name: definition.Name, Capability: definition.Capability, Summary: definition.DescriptionForLLM, TagsJSON: jsonBytes(definition.Tags), Risk: string(definition.Risk), SideEffect: string(definition.SideEffect), SchemaVersion: definition.SchemaVersion, SchemaHash: definition.SchemaHash, Status: definition.Status, Revision: revision}
}

func catalogMatches(row ToolCatalog, definition aitool.Definition) bool {
	var tags []string
	_ = json.Unmarshal(row.TagsJSON, &tags)
	return row.Source == string(definition.Source) && row.Name == definition.Name && row.Capability == definition.Capability && row.Summary == definition.DescriptionForLLM && slices.Equal(tags, definition.Tags) && row.Risk == string(definition.Risk) && row.SideEffect == string(definition.SideEffect) && row.SchemaVersion == definition.SchemaVersion && row.SchemaHash == definition.SchemaHash && row.Status == definition.Status
}

func documentMatches(document ToolSearchDocument, catalog ToolCatalog, definition aitool.Definition, embeddingAlias string) bool {
	var hints []string
	_ = json.Unmarshal(document.InputHintsJSON, &hints)
	expectedStatus := "stale"
	if definition.Status == "enabled" && (document.IndexStatus == "pending" || document.IndexStatus == "ready") {
		expectedStatus = document.IndexStatus
	}
	expectedText := aitool.SearchDocumentText(definition)
	baseMatches := document.CatalogRevision == catalog.Revision && document.IndexStatus == expectedStatus && document.SearchText == expectedText && slices.Equal(hints, aitool.InputHints(definition.InputSchema))
	if !baseMatches || definition.Status != "enabled" || embeddingAlias == "" || document.IndexStatus == "pending" {
		return baseMatches
	}
	return document.EmbeddingAlias == embeddingAlias && document.EmbeddingDeploymentID != "" && document.EmbeddingDimensions > 0 && document.EmbeddingPreprocessing == aimodelruntime.EmbeddingPreprocessingVersionCosine && document.EmbeddingSchemaVersion == toolEmbeddingSchemaVersion && document.EmbeddingVector != nil
}

func upsertCatalogProjection(tx *gorm.DB, row ToolCatalog, definition aitool.Definition) error {
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "tool_id"}}, DoUpdates: clause.AssignmentColumns([]string{"source", "name", "capability", "summary", "tags_json", "risk", "side_effect", "schema_version", "schema_hash", "status", "revision", "updated_at"})}).Create(&row).Error; err != nil {
		return err
	}
	indexStatus := "pending"
	if definition.Status != "enabled" {
		indexStatus = "stale"
	}
	doc := ToolSearchDocument{ID: uuid.NewString(), TenantID: row.TenantID, ToolID: row.ToolID, SearchText: aitool.SearchDocumentText(definition), InputHintsJSON: jsonBytes(aitool.InputHints(definition.InputSchema)), IndexStatus: indexStatus, CatalogRevision: row.Revision}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "tool_id"}}, DoUpdates: clause.AssignmentColumns([]string{"search_text", "input_hints_json", "index_status", "catalog_revision", "embedding_model", "embedding_alias", "embedding_deployment_id", "embedding_dimensions", "embedding_preprocessing", "embedding_schema_version", "embedding_vector", "vector_updated_at", "updated_at"})}).Create(&doc).Error
}
