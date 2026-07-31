package demo

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	demoruntime "github.com/yzxingtu2026/ainp/platform/modules/demo/showcase"
)

// Register preserves the module's stable composition entrypoint.
func Register(a *app.App) error {
	return demoruntime.Register(a)
}
