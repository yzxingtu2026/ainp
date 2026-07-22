package help

import (
	"embed"

	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

//go:embed *.md
var documents embed.FS

func Register(registry *menu.HelpRegistry, menus *menu.Registry) error {
	for _, pattern := range []string{"system.users.*.md", "system.roles.*.md", "system.settings.*.md", "system.attachments.*.md"} {
		if err := registry.Load(documents, pattern); err != nil {
			return err
		}
	}
	for _, item := range menus.List() {
		if item.ID == "system.notification_templates" {
			return registry.Load(documents, "system.notification_templates.*.md")
		}
	}
	return nil
}
