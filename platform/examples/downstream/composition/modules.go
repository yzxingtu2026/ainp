package composition

import (
	"example.com/ainp-downstream/modules/orders"

	"github.com/yzxingtu2026/ainp/platform/host"
)

// Modules is the single module declaration shared by API and Worker entrypoints.
var Modules = []host.Module{orders.Module{}}
