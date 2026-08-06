// Package i18nassets exposes AINP's versioned built-in backend translations.
package i18nassets

import (
	"embed"
	"io/fs"
)

//go:embed *.yaml
var files embed.FS

// FS returns the immutable translations compiled into the platform module.
func FS() fs.FS { return files }
