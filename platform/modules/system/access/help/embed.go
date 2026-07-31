package help

import (
	"embed"

	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

//go:embed *.md
var documents embed.FS

func Register(registry *menu.HelpRegistry) error {
	for _, pattern := range []string{"system.users.*.md", "system.roles.*.md"} {
		if err := registry.Load(documents, pattern); err != nil {
			return err
		}
	}
	return nil
}
