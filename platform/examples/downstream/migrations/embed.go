package migrations

import "embed"

//go:embed *.sql
var files embed.FS

func FS() embed.FS { return files }
