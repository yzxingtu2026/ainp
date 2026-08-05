package knowledge

import (
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"gorm.io/gorm"
)

type KnowledgeBase struct {
	ID                    string                      `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_knowledge_bases_tenant_id,priority:2" json:"id"`
	TenantID              string                      `gorm:"size:64;not null;uniqueIndex:ux_ai_knowledge_bases_tenant_id,priority:1;uniqueIndex:ux_ai_knowledge_bases_tenant_code,where:deleted_at IS NULL,priority:1" json:"-"`
	AccountID             string                      `gorm:"size:64;not null;index" json:"account_id"`
	Code                  string                      `gorm:"size:64;not null;uniqueIndex:ux_ai_knowledge_bases_tenant_code,where:deleted_at IS NULL,priority:2" json:"code"`
	Name                  string                      `gorm:"size:128;not null" json:"name"`
	Description           string                      `gorm:"size:1024;not null;default:''" json:"description"`
	Visibility            string                      `gorm:"size:16;not null;index;check:ck_ai_knowledge_base_visibility,visibility IN ('private','tenant')" json:"visibility"`
	Status                string                      `gorm:"size:16;not null;default:'draft';index;check:ck_ai_knowledge_base_status,status IN ('draft','enabled','disabled')" json:"status"`
	EmbeddingAlias        string                      `gorm:"size:64;not null;default:'';check:ck_ai_knowledge_base_enabled_embedding,status <> 'enabled' OR embedding_alias <> ''" json:"embedding_alias,omitempty"`
	ChunkingConfig        aiknowledge.ChunkingConfig  `gorm:"column:chunking_config;serializer:json;type:jsonb;not null" json:"chunking_config"`
	RetrievalConfig       aiknowledge.RetrievalConfig `gorm:"column:retrieval_config;serializer:json;type:jsonb;not null" json:"retrieval_config"`
	CurrentIndexVersionID *string                     `gorm:"size:64" json:"current_index_version_id,omitempty"`
	Revision              uint64                      `gorm:"not null;default:1;check:ck_ai_knowledge_base_revision,revision > 0" json:"revision"`
	CreatedAt             time.Time                   `json:"created_at"`
	UpdatedAt             time.Time                   `json:"updated_at"`
	DeletedAt             gorm.DeletedAt              `gorm:"index" json:"-"`
}

func (KnowledgeBase) TableName() string { return "ai_knowledge_bases" }

type KnowledgeDocument struct {
	ID                     string         `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_knowledge_documents_tenant_id,priority:2;uniqueIndex:ux_ai_knowledge_documents_base_id,priority:3" json:"id"`
	TenantID               string         `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_documents_tenant_id,priority:1;uniqueIndex:ux_ai_knowledge_documents_base_id,priority:1;uniqueIndex:ux_ai_knowledge_documents_attachment,where:deleted_at IS NULL,priority:1" json:"-"`
	KnowledgeBaseID        string         `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_documents_base_id,priority:2;uniqueIndex:ux_ai_knowledge_documents_attachment,where:deleted_at IS NULL,priority:2" json:"knowledge_base_id"`
	AttachmentID           string         `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_documents_attachment,where:deleted_at IS NULL,priority:3" json:"attachment_id"`
	SourceRevision         string         `gorm:"size:64;not null;check:ck_ai_knowledge_document_source_revision,source_revision ~ '^[a-f0-9]{64}$'" json:"source_revision"`
	ContentType            string         `gorm:"size:128;not null" json:"content_type"`
	OriginalName           string         `gorm:"size:512;not null" json:"original_name"`
	IngestionStatus        string         `gorm:"size:24;not null;default:'pending';index;check:ck_ai_knowledge_document_status,ingestion_status IN ('pending','processing','ready','failed')" json:"ingestion_status"`
	IngestionErrorCategory string         `gorm:"size:64;not null;default:'';check:ck_ai_knowledge_document_error_category,ingestion_error_category = '' OR ingestion_error_category ~ '^[a-z][a-z0-9_.-]{0,63}$'" json:"ingestion_error_category,omitempty"`
	CurrentIndexVersionID  *string        `gorm:"size:64" json:"current_index_version_id,omitempty"`
	Revision               uint64         `gorm:"not null;default:1;check:ck_ai_knowledge_document_revision,revision > 0" json:"revision"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"-"`
}

func (KnowledgeDocument) TableName() string { return "ai_knowledge_documents" }

type KnowledgeIndexVersion struct {
	ID                    string     `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_knowledge_index_versions_tenant_id,priority:2;uniqueIndex:ux_ai_knowledge_index_versions_base_id,priority:3" json:"id"`
	TenantID              string     `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_index_versions_tenant_id,priority:1;uniqueIndex:ux_ai_knowledge_index_versions_base_id,priority:1;uniqueIndex:ux_ai_knowledge_index_versions_base_version,priority:1;uniqueIndex:ux_ai_knowledge_index_source_revision,priority:1" json:"-"`
	KnowledgeBaseID       string     `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_index_versions_base_id,priority:2;uniqueIndex:ux_ai_knowledge_index_versions_base_version,priority:2;uniqueIndex:ux_ai_knowledge_index_source_revision,priority:2" json:"knowledge_base_id"`
	SourceDocumentID      *string    `gorm:"size:64;index;uniqueIndex:ux_ai_knowledge_index_source_revision,priority:3" json:"source_document_id,omitempty"`
	SourceRevision        *string    `gorm:"size:64;uniqueIndex:ux_ai_knowledge_index_source_revision,priority:4;check:ck_ai_knowledge_index_source_revision,source_revision IS NULL OR source_revision ~ '^[a-f0-9]{64}$'" json:"source_revision,omitempty"`
	Version               int        `gorm:"not null;uniqueIndex:ux_ai_knowledge_index_versions_base_version,priority:3;check:ck_ai_knowledge_index_version,version > 0" json:"version"`
	Status                string     `gorm:"size:24;not null;default:'pending';index;check:ck_ai_knowledge_index_status,status IN ('pending','building','ready','failed','superseded','aborted')" json:"status"`
	EmbeddingAlias        string     `gorm:"size:64;not null" json:"embedding_alias"`
	EmbeddingDeploymentID string     `gorm:"size:64;not null" json:"embedding_deployment_id"`
	EmbeddingDimensions   int        `gorm:"not null;check:ck_ai_knowledge_index_dimensions,embedding_dimensions > 0" json:"embedding_dimensions"`
	PreprocessingVersion  string     `gorm:"size:32;not null" json:"preprocessing_version"`
	ParserVersion         string     `gorm:"size:32;not null" json:"parser_version"`
	ChunkingVersion       string     `gorm:"size:32;not null;check:ck_ai_knowledge_index_versions_metadata,embedding_deployment_id <> '' AND preprocessing_version <> '' AND parser_version <> '' AND chunking_version <> ''" json:"chunking_version"`
	DocumentCount         int        `gorm:"not null;default:0;check:ck_ai_knowledge_index_document_count,document_count >= 0" json:"document_count"`
	ChunkCount            int        `gorm:"not null;default:0;check:ck_ai_knowledge_index_chunk_count,chunk_count >= 0" json:"chunk_count"`
	ConfigHash            string     `gorm:"size:64;not null;check:ck_ai_knowledge_index_config_hash,config_hash ~ '^[a-f0-9]{64}$'" json:"config_hash"`
	ErrorCategory         string     `gorm:"size:64;not null;default:'';check:ck_ai_knowledge_index_error_category,error_category = '' OR error_category ~ '^[a-z][a-z0-9_.-]{0,63}$'" json:"error_category,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
}

func (KnowledgeIndexVersion) TableName() string { return "ai_knowledge_index_versions" }

type KnowledgeChunk struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID        string    `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_chunks_position" json:"-"`
	KnowledgeBaseID string    `gorm:"size:64;not null;index" json:"knowledge_base_id"`
	DocumentID      string    `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_chunks_position" json:"document_id"`
	IndexVersionID  string    `gorm:"size:64;not null;index;uniqueIndex:ux_ai_knowledge_chunks_position" json:"index_version_id"`
	Ordinal         int       `gorm:"not null;uniqueIndex:ux_ai_knowledge_chunks_position;check:ck_ai_knowledge_chunk_ordinal,ordinal >= 0" json:"ordinal"`
	SourceRevision  string    `gorm:"size:64;not null;check:ck_ai_knowledge_chunk_source_revision,source_revision ~ '^[a-f0-9]{64}$'" json:"source_revision"`
	PageNumber      *int      `gorm:"check:ck_ai_knowledge_chunk_page,page_number IS NULL OR page_number > 0" json:"page_number,omitempty"`
	PositionStart   *int      `gorm:"check:ck_ai_knowledge_chunk_positions,(position_start IS NULL AND position_end IS NULL) OR (position_start >= 0 AND position_end > position_start)" json:"position_start,omitempty"`
	PositionEnd     *int      `json:"position_end,omitempty"`
	HeadingPath     string    `gorm:"size:1024;not null;default:''" json:"heading_path,omitempty"`
	TokenCount      int       `gorm:"not null;default:0;check:ck_ai_knowledge_chunk_token_count,token_count >= 0" json:"token_count"`
	ContentHash     string    `gorm:"size:64;not null;check:ck_ai_knowledge_chunk_content_hash,content_hash ~ '^[a-f0-9]{64}$'" json:"content_hash"`
	CreatedAt       time.Time `json:"created_at"`
}

func (KnowledgeChunk) TableName() string { return "ai_knowledge_chunks" }

type KnowledgeChunkPayload struct {
	ChunkID        string    `gorm:"primaryKey;size:64" json:"-"`
	TenantID       string    `gorm:"size:64;not null;index" json:"-"`
	IndexVersionID string    `gorm:"size:64;not null;index" json:"-"`
	Text           string    `gorm:"type:text;not null" json:"-"`
	Vector         PGVector  `gorm:"type:public.vector;not null" json:"-"`
	Dimensions     int       `gorm:"not null" json:"-"`
	CreatedAt      time.Time `json:"-"`
}

func (KnowledgeChunkPayload) TableName() string { return "ai_knowledge_chunk_payloads" }

type PGVector = database.PGVector

type AgentKnowledgeBinding struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID        string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_knowledge_binding" json:"-"`
	AgentID         string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_knowledge_binding;index" json:"agent_id"`
	KnowledgeBaseID string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_knowledge_binding;index" json:"knowledge_base_id"`
	CreatedBy       string    `gorm:"size:64;not null" json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}

func (AgentKnowledgeBinding) TableName() string { return "ai_agent_knowledge_bindings" }

func AutoMigrateKnowledge(db *gorm.DB) error {
	if db.Dialector.Name() == "postgres" {
		if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public").Error; err != nil {
			return fmt.Errorf("enable pgvector extension: %w", err)
		}
	}
	if err := db.AutoMigrate(
		&KnowledgeBase{},
		&KnowledgeDocument{},
		&KnowledgeIndexVersion{},
		&KnowledgeChunk{},
		&KnowledgeChunkPayload{},
		&AgentKnowledgeBinding{},
	); err != nil {
		return err
	}
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	if err := db.Exec("ALTER TABLE ai_knowledge_chunk_payloads ADD COLUMN IF NOT EXISTS search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED").Error; err != nil {
		return err
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunk_payload_fts ON ai_knowledge_chunk_payloads USING gin (search_vector)").Error; err != nil {
		return err
	}
	for _, item := range []struct {
		table, name, definition string
	}{
		{"ai_knowledge_bases", "ck_ai_knowledge_base_chunking_config", "CHECK (jsonb_typeof(chunking_config) = 'object' AND chunking_config ?& ARRAY['strategy', 'chunk_size', 'chunk_overlap'])"},
		{"ai_knowledge_bases", "ck_ai_knowledge_base_retrieval_config", "CHECK (jsonb_typeof(retrieval_config) = 'object' AND retrieval_config ?& ARRAY['top_k', 'min_score', 'max_context_tokens'])"},
	} {
		if err := addKnowledgeConstraint(db, item.table, item.name, item.definition); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("tenants") {
		for _, table := range []string{
			"ai_knowledge_bases",
			"ai_knowledge_documents",
			"ai_knowledge_index_versions",
			"ai_knowledge_chunks",
			"ai_agent_knowledge_bindings",
		} {
			if err := addKnowledgeConstraint(db, table, "fk_"+table+"_tenant",
				"FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT"); err != nil {
				return err
			}
		}
	}
	if err := addKnowledgeConstraint(db, "ai_knowledge_documents", "fk_ai_knowledge_document_base_tenant",
		"FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT"); err != nil {
		return err
	}
	if err := addKnowledgeConstraint(db, "ai_knowledge_index_versions", "fk_ai_knowledge_index_base_tenant",
		"FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT"); err != nil {
		return err
	}
	if err := addKnowledgeConstraint(db, "ai_knowledge_index_versions", "fk_ai_knowledge_index_source_document_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id, source_document_id) REFERENCES ai_knowledge_documents(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT"); err != nil {
		return err
	}
	for _, item := range []struct {
		table, name, definition string
	}{
		{"ai_knowledge_bases", "fk_ai_knowledge_base_current_index_tenant", "FOREIGN KEY (tenant_id, id, current_index_version_id) REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT"},
		{"ai_knowledge_documents", "fk_ai_knowledge_document_current_index_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id, current_index_version_id) REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT"},
		{"ai_knowledge_chunks", "fk_ai_knowledge_chunk_base_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT"},
		{"ai_knowledge_chunks", "fk_ai_knowledge_chunk_document_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id, document_id) REFERENCES ai_knowledge_documents(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT"},
		{"ai_knowledge_chunks", "fk_ai_knowledge_chunk_index_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id, index_version_id) REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT"},
		{"ai_agent_knowledge_bindings", "fk_ai_agent_knowledge_base_tenant", "FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT"},
	} {
		if err := addKnowledgeConstraint(db, item.table, item.name, item.definition); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("attachments") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_attachments_tenant_id ON attachments (tenant_id, id)").Error; err != nil {
			return err
		}
		if err := addKnowledgeConstraint(db, "ai_knowledge_documents", "fk_ai_knowledge_document_attachment_tenant",
			"FOREIGN KEY (tenant_id, attachment_id) REFERENCES attachments(tenant_id, id) ON DELETE RESTRICT"); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("ai_agents") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agents_tenant_id ON ai_agents (tenant_id, id)").Error; err != nil {
			return err
		}
		if err := addKnowledgeConstraint(db, "ai_agent_knowledge_bindings", "fk_ai_agent_knowledge_agent_tenant",
			"FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents(tenant_id, id) ON DELETE RESTRICT"); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("ai_model_deployments") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_model_deployments_tenant_id ON ai_model_deployments (tenant_id, id)").Error; err != nil {
			return err
		}
		if err := addKnowledgeConstraint(db, "ai_knowledge_index_versions", "fk_ai_knowledge_index_embedding_deployment_tenant",
			"FOREIGN KEY (tenant_id, embedding_deployment_id) REFERENCES ai_model_deployments(tenant_id, id) ON DELETE RESTRICT"); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("tenant_accounts") {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS ux_tenant_accounts_tenant_id ON tenant_accounts (tenant_id, id)").Error; err != nil {
			return err
		}
		for _, item := range []struct {
			table, name, definition string
		}{
			{"ai_knowledge_bases", "fk_ai_knowledge_base_owner_tenant", "FOREIGN KEY (tenant_id, account_id) REFERENCES tenant_accounts(tenant_id, id) ON DELETE RESTRICT"},
			{"ai_agent_knowledge_bindings", "fk_ai_agent_knowledge_creator_tenant", "FOREIGN KEY (tenant_id, created_by) REFERENCES tenant_accounts(tenant_id, id) ON DELETE RESTRICT"},
		} {
			if err := addKnowledgeConstraint(db, item.table, item.name, item.definition); err != nil {
				return err
			}
		}
	}
	return nil
}

func addKnowledgeConstraint(db *gorm.DB, table, name, definition string) error {
	statement := fmt.Sprintf(`DO $$ BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = '%s' AND conrelid = '%s'::regclass
		) THEN
			ALTER TABLE %s ADD CONSTRAINT %s %s;
		END IF;
	END $$`, name, table, table, name, definition)
	return db.Exec(statement).Error
}
