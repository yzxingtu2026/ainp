package navigation

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/database"
)

type SearchDocument struct {
	ID                     string             `gorm:"primaryKey;size:64"`
	TenantID               string             `gorm:"size:64;not null;uniqueIndex:ux_navigation_document"`
	MenuID                 string             `gorm:"size:128;not null;uniqueIndex:ux_navigation_document"`
	DocumentKind           string             `gorm:"size:16;not null;uniqueIndex:ux_navigation_document"`
	Locale                 string             `gorm:"size:16;not null;uniqueIndex:ux_navigation_document"`
	UseCase                string             `gorm:"size:160;not null;default:'';uniqueIndex:ux_navigation_document"`
	Title                  string             `gorm:"size:255;not null"`
	Content                string             `gorm:"type:text;not null"`
	HelpVersion            string             `gorm:"size:32;not null;default:''"`
	ContentHash            string             `gorm:"size:64;not null"`
	IndexStatus            string             `gorm:"size:16;not null"`
	CatalogRevision        uint64             `gorm:"not null;default:1"`
	EmbeddingModel         string             `gorm:"size:128"`
	EmbeddingAlias         string             `gorm:"size:64"`
	EmbeddingDeploymentID  string             `gorm:"size:64"`
	EmbeddingDimensions    int                `gorm:"not null;default:0"`
	EmbeddingPreprocessing string             `gorm:"size:32"`
	EmbeddingSchemaVersion int                `gorm:"not null;default:0"`
	EmbeddingVector        *database.PGVector `gorm:"type:public.vector" json:"-"`
	VectorUpdatedAt        *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type NavigationSearchDocument = SearchDocument

func (SearchDocument) TableName() string { return "navigation_search_documents" }
