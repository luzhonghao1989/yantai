package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var built embed.FS

func Assets() fs.FS {
	assets, err := fs.Sub(built, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
