package prompts

import (
	"embed"
	"io/fs"
)

//go:embed example.orders/*.md
var files embed.FS

func FS() fs.FS {
	source, err := fs.Sub(files, ".")
	if err != nil {
		panic(err)
	}
	return source
}
