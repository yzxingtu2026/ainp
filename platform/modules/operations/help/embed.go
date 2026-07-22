package operationshelp

import (
	"embed"

	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

//go:embed *.md
var documents embed.FS

func Register(registry *menu.HelpRegistry, includeDeliveries bool) error {
	for _, pattern := range []string{"operations.jobs.*.md", "operations.outbox.*.md", "operations.audit_logs.*.md"} {
		if err := registry.Load(documents, pattern); err != nil {
			return err
		}
	}
	if includeDeliveries {
		return registry.Load(documents, "operations.notification_deliveries.*.md")
	}
	return nil
}
