package tooling

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
)

// RegisterBuiltins is the single, explicit aggregation point for runtime tool
// slots. Callers list every compiled-in slot here; registration never scans
// source files and never relies on init side effects.
func RegisterBuiltins(registry *aitool.Registry, slots ...aitool.RuntimeSlot) error {
	if registry == nil {
		return fmt.Errorf("tool registry is not initialized")
	}
	for _, slot := range slots {
		if err := registry.RegisterSlot(slot); err != nil {
			return err
		}
	}
	return nil
}
