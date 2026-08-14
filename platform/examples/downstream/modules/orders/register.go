package orders

import (
	downstreami18n "example.com/ainp-downstream/i18n"
	downstreammigrations "example.com/ainp-downstream/migrations"
	"example.com/ainp-downstream/modules/orders/catalog"
	downstreamprompts "example.com/ainp-downstream/prompts"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/host"
	platformmigrations "github.com/yzxingtu2026/ainp/platform/migrations"
)

type Module struct{}

func (Module) ID() string { return ModuleID }

func (Module) Prepare(application *app.App) error {
	return catalog.Prepare(application)
}

func (Module) Register(application *app.App) error {
	return catalog.Register(application, catalog.Config{
		ModuleID:       ModuleID,
		CapabilityID:   CapabilityID,
		AgentPromptKey: AgentPromptKey,
	})
}

func (Module) MigrationSource() platformmigrations.Source {
	return platformmigrations.Source{Owner: ModuleID, FS: downstreammigrations.FS()}
}

func (Module) RuntimeAssets() host.RuntimeAssets {
	return host.RuntimeAssets{Prompts: downstreamprompts.FS(), I18n: downstreami18n.FS()}
}
