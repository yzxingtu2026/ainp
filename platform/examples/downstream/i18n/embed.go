package i18n

import "embed"

//go:embed *.yaml
var files embed.FS

func FS() embed.FS { return files }
