package ui

import (
	"embed"
	"io/fs"
)

//go:embed embedded
var bundled embed.FS

func Files() fs.FS {
	files, _ := fs.Sub(bundled, "embedded")
	return files
}
