package operations

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	operationsruntime "github.com/yzxingtu2026/ainp/platform/modules/operations/center"
)

// Register preserves the module's stable composition entrypoint.
func Register(a *app.App, deliveries NotificationDeliveryService) error {
	return operationsruntime.Register(a, deliveries)
}
