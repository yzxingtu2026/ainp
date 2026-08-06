// Package prompts exposes AINP's versioned built-in model prompts.
package prompts

import (
	"embed"
	"io/fs"
)

//go:embed agent/*.md ai/*.md
var files embed.FS

// FS returns the immutable prompt resources compiled into the platform module.
func FS() fs.FS { return files }
