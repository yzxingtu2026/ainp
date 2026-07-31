package help

import (
	"embed"

	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

//go:embed *.md
var documents embed.FS

func Register(registry *menu.HelpRegistry) error {
	return registry.Load(documents, "system.settings.*.md")
}
