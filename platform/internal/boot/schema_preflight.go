package boot

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type legacyIdentityColumn struct {
	table  string
	column string
}

var legacyIdentityColumns = []legacyIdentityColumn{
	{table: "audit_logs", column: "user_id"},
	{table: "idempotency_records", column: "user_id"},
	{table: "ai_tool_scopes", column: "user_id"},
	{table: "ai_tool_confirmations", column: "user_id"},
	{table: "ai_tool_invocations", column: "user_id"},
	{table: "mcp_oauth_grants", column: "user_id"},
	{table: "mcp_action_confirmations", column: "user_id"},
	{table: "ai_knowledge_bases", column: "owner_user_id"},
	{table: "user_setting_values", column: "user_id"},
	{table: "ai_assistant_personalization_snapshots", column: "user_id"},
	{table: "user_notifications", column: "user_id"},
}

// ensureIdentityContractSchema prevents GORM from adding replacement NOT NULL
// columns before the versioned migration has preserved existing identity data.
func ensureIdentityContractSchema(db *gorm.DB) error {
	legacyColumns := make([]string, 0)
	for _, legacy := range legacyIdentityColumns {
		if db.Migrator().HasTable(legacy.table) && db.Migrator().HasColumn(legacy.table, legacy.column) {
			legacyColumns = append(legacyColumns, legacy.table+"."+legacy.column)
		}
	}
	if len(legacyColumns) == 0 {
		return nil
	}
	return fmt.Errorf(
		"legacy identity contract schema detected (%s); run versioned migrations before AutoMigrate",
		strings.Join(legacyColumns, ", "),
	)
}
