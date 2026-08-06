package downstream

import (
	"io/fs"
)

func fsSub(source fs.FS, dir string) (fs.FS, error) { return fs.Sub(source, dir) }
