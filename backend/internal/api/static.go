package api

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFS embed.FS

func GetEmbeddedFrontend() (fs.FS, error) {
	return fs.Sub(embeddedFS, "dist")
}
